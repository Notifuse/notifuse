import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent, act, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { FileExplorer, type FileExplorerProps } from './FileExplorer'
import { clearImageSizes } from './imageDimensions'
import type { StorageObject } from './interfaces'
import { fakeFiles, type FakeObject } from './fakeFilesApi'
import type { DeleteFolderResult, PresignedUpload } from '../../services/api/files'

// The bucket lives behind the API: an in-memory one here, and the presigned PUTs go to fetch.
vi.mock('../../services/api/files', async () => ({
  filesApi: (await import('./fakeFilesApi')).fakeFiles.api
}))

const settings = { endpoint: 'https://s3.example.com', access_key: 'key', bucket: 'media' }
const at = (day: number) => new Date(Date.UTC(2026, 0, day))

// docs/ holds two files and a sub-folder with one; photos/ exists only through its file.
const BUCKET: FakeObject[] = [
  { key: 'docs/', lastModified: at(1) },
  { key: 'docs/a.png', lastModified: at(3), size: 2048 },
  { key: 'docs/report.pdf', lastModified: at(2), size: 1024 },
  { key: 'docs/2026/', lastModified: at(1) },
  { key: 'docs/2026/b.png', lastModified: at(4), size: 100 },
  { key: 'photos/beach.jpg', lastModified: at(5), size: 300 },
  { key: 'notes.txt', lastModified: at(6), size: 10 }
]

// Answers the listings from a copy of the objects, which an upload or a delete changes.
const listing = (initial: FakeObject[] = BUCKET) => fakeFiles.reset(initial)

const isImage = (item: StorageObject) =>
  !item.is_folder && item.file_info?.content_type?.startsWith('image/')

const renderExplorer = (props: Partial<FileExplorerProps> = {}, path = 'docs/') => {
  const onPathChange = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const tree = (currentPath: string, extra: Partial<FileExplorerProps> = {}) => (
    <QueryClientProvider client={queryClient}>
      <ConfigProvider>
        <App>
          <FileExplorer
            mode="page"
            workspaceId="ws"
            settings={settings}
            onUpdateSettings={vi.fn()}
            onError={vi.fn()}
            onSelect={vi.fn()}
            height={600}
            acceptFileType="*"
            acceptItem={() => true}
            controlledPath={currentPath}
            onPathChange={onPathChange}
            {...props}
            {...extra}
          />
        </App>
      </ConfigProvider>
    </QueryClientProvider>
  )
  const utils = render(tree(path))
  return {
    ...utils,
    onPathChange,
    rerenderAt: (next: string, extra: Partial<FileExplorerProps> = {}) =>
      utils.rerender(tree(next, extra))
  }
}

const listCalls = () => fakeFiles.api.list.mock.calls

// The preview's delete confirmation, found by its question.
const openPopconfirm = async () => {
  const question = await screen.findByText(
    'Do you want to permanently delete this file from your storage?'
  )
  return question.closest('.ant-popover') as HTMLElement
}

// The folders' rail: a card on the page, a column in the picker.
const rail = () => screen.getByRole('navigation', { name: 'Folders' })

// The open dropdown's menu: a closed one stays in the document, hidden. A folder shown in the rail
// has a menu there too: `inRail` picks that one, the list's otherwise.
const openMenu = async (name: string, inRail = false) => {
  const buttons = screen.getAllByRole('button', { name: `Actions for ${name}` })
  const button = buttons.find((candidate) => rail().contains(candidate) === inRail)
  if (!button) throw new Error(`no menu for ${name}${inRail ? ' in the rail' : ''}`)
  fireEvent.click(button)
  return waitFor(() => {
    const menu = document.querySelector<HTMLElement>(
      '.ant-dropdown:not(.ant-dropdown-hidden) [role="menu"]'
    )
    expect(menu).not.toBeNull()
    return menu as HTMLElement
  })
}

describe('FileExplorer page', () => {
  beforeEach(() => {
    fakeFiles.reset()
    localStorage.clear()
    clearImageSizes()
  })

  it('shows the folder tree with the files under each folder, the root holding them all', async () => {
    listing()
    renderExplorer()
    const tree = await screen.findByRole('navigation', { name: 'Folders' })

    await waitFor(() => expect(within(tree).getByRole('button', { name: 'Root 5' })).toBeTruthy())
    expect(within(tree).getByRole('button', { name: 'docs 3' }).getAttribute('aria-current')).toBe(
      'page'
    )
    expect(within(tree).getByRole('button', { name: '2026 1' })).toBeTruthy()
    // An implicit folder is a folder like any other.
    expect(within(tree).getByRole('button', { name: 'photos 1' })).toBeTruthy()
    expect(within(tree).getByText(/media · /)).toBeTruthy()
  })

  it('shows no count nor size in the rail before the listing answers', async () => {
    fakeFiles.api.list.mockReturnValue(new Promise(() => {}))
    renderExplorer()
    const tree = await screen.findByRole('navigation', { name: 'Folders' })
    expect(within(tree).getByRole('button', { name: 'Root' })).toBeTruthy()
    expect(within(tree).getByText('media')).toBeTruthy()
    expect(within(tree).queryByText(/ · /)).toBeNull()
  })

  it('lays the folders out as a card beside the list, the Blog rail’s way', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    expect(rail().hasAttribute('data-rail-card')).toBe(true)
    // The root on its own, then a "Folders" section with its "+".
    expect(within(rail()).getByText('Folders')).toBeTruthy()
    expect(within(rail()).getByRole('button', { name: 'New folder' })).toBeTruthy()
    // The page header spans the width, above the card.
    const heading = screen.getByRole('heading', { level: 1, name: 'docs' })
    expect(rail().contains(heading)).toBe(false)
    expect(heading.compareDocumentPosition(rail()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('nests sub-folders under their parent, collapsed off the open path', async () => {
    listing()
    renderExplorer({}, '')
    await waitFor(() => expect(within(rail()).getByRole('button', { name: 'docs 3' })).toBeTruthy())

    expect(within(rail()).queryByRole('button', { name: '2026 1' })).toBeNull()
    fireEvent.click(within(rail()).getByRole('button', { name: 'Expand docs' }))
    const child = within(rail()).getByRole('button', { name: '2026 1' })
    // One level of indentation under docs, which has none.
    const indents = (button: HTMLElement) =>
      button.closest('[data-rail-item]')?.querySelectorAll('[data-folder-indent]').length
    expect(indents(child)).toBe(1)
    expect(indents(within(rail()).getByRole('button', { name: 'docs 3' }))).toBe(0)
    // A folder without sub-folders has nothing to expand.
    expect(within(rail()).queryByRole('button', { name: 'Expand photos' })).toBeNull()

    fireEvent.click(within(rail()).getByRole('button', { name: 'Collapse docs' }))
    expect(within(rail()).queryByRole('button', { name: '2026 1' })).toBeNull()
  })

  it('opens a folder from the rail and lists its files', async () => {
    listing()
    const { onPathChange, rerenderAt } = renderExplorer({}, '')
    await waitFor(() =>
      expect(within(rail()).getByRole('button', { name: 'photos 1' })).toBeTruthy()
    )
    expect(within(rail()).getByRole('button', { name: 'Root 5' }).getAttribute('aria-current')).toBe(
      'page'
    )

    fireEvent.click(within(rail()).getByRole('button', { name: 'photos 1' }))
    expect(onPathChange).toHaveBeenCalledWith('photos/')

    rerenderAt('photos/')
    expect(await screen.findByText('beach.jpg')).toBeTruthy()
    expect(screen.queryByText('notes.txt')).toBeNull()
    expect(
      within(rail()).getByRole('button', { name: 'photos 1' }).getAttribute('aria-current')
    ).toBe('page')
    expect(screen.getByRole('heading', { level: 1, name: 'photos' })).toBeTruthy()

    fireEvent.click(within(rail()).getByRole('button', { name: 'Root 5' }))
    expect(onPathChange).toHaveBeenLastCalledWith('')
  })

  it('creates a folder in the open one from the rail’s +', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    fireEvent.click(within(rail()).getByRole('button', { name: 'New folder' }))
    const dialog = await screen.findByRole('dialog')
    expect(dialog.textContent).toContain('Create new folder')
    // The name has the focus, the folder it goes in written before it, its label above it.
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'Folder name' })).toHaveFocus())
    expect(within(dialog).getByText('docs/')).toBeInTheDocument()
    expect(dialog.querySelector('.ant-form-vertical')).not.toBeNull()
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Folder name' }), {
      target: { value: 'drafts' }
    })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create' }))

    expect(await screen.findByText('Folder created successfully.')).toBeTruthy()
    expect(fakeFiles.api.createFolder).toHaveBeenCalledWith('ws', 'docs/drafts/')
  })

  it('deletes a folder from its menu in the rail', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    const menu = await openMenu('2026', true)
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Delete folder' }))
    expect(await screen.findByText('Delete the folder 2026 and its 1 file?')).toBeTruthy()
    fireEvent.click(confirmButton())

    expect(await screen.findByText('Folder deleted successfully.')).toBeTruthy()
    expect(fakeFiles.api.deleteFolder.mock.calls).toEqual([['ws', 'docs/2026/']])
    expect(fakeFiles.keys().filter((key) => key.startsWith('docs/2026/'))).toEqual([])
  })

  it('offers to create the first folder of a bucket without one', async () => {
    listing([{ key: 'notes.txt', lastModified: at(6), size: 10 }])
    renderExplorer({}, '')
    fireEvent.click(await within(rail()).findByRole('button', { name: 'Create your first folder' }))
    expect((await screen.findByRole('dialog')).textContent).toContain('Create new folder')
  })

  it('says there are no folders yet when it cannot create one', async () => {
    listing([{ key: 'notes.txt', lastModified: at(6), size: 10 }])
    renderExplorer({ readOnly: true }, '')
    expect(await within(rail()).findByText('No folders yet')).toBeTruthy()
    expect(within(rail()).queryByRole('button', { name: 'Create your first folder' })).toBeNull()
  })

  it('titles the page after the folder, with its file count', async () => {
    listing()
    renderExplorer()
    const heading = await screen.findByRole('heading', { level: 1, name: 'docs' })
    await waitFor(() => expect(heading.parentElement?.textContent).toBe('docs3'))
  })

  it('lists sub-folders first, then files from the newest, with a folder sized in files', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    const rows = screen.getAllByRole('row').slice(1)
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining('2026'),
      expect.stringContaining('a.png'),
      expect.stringContaining('report.pdf')
    ])
    expect(rows[0].textContent).toContain('1 file')
    expect(rows[1].textContent).toContain('2 kB')
  })

  it('opens a folder on click and the breadcrumb goes back up', async () => {
    listing()
    const { onPathChange } = renderExplorer({}, 'docs/2026/')
    await screen.findByText('b.png')

    const breadcrumb = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(within(breadcrumb).getByText('2026').getAttribute('aria-current')).toBe('page')
    fireEvent.click(within(breadcrumb).getByRole('button', { name: 'docs' }))
    expect(onPathChange).toHaveBeenCalledWith('docs/')
  })

  it('lists the bucket again on Refresh', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')
    expect(listCalls()).toHaveLength(1)

    fireEvent.click(screen.getByRole('button', { name: 'Refresh the list' }))
    await waitFor(() => expect(listCalls()).toHaveLength(2))
  })

  it('keeps the chosen view for the next visit', async () => {
    listing()
    const { unmount } = renderExplorer()
    await screen.findByText('a.png')

    fireEvent.click(screen.getByText('Grid'))
    expect(localStorage.setItem).toHaveBeenCalledWith('notifuse.files.view.page', 'grid')
    unmount()

    renderExplorer()
    await waitFor(() =>
      expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull()
    )
  })

  it('searches file names in the whole bucket and shows each file’s folder', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    fireEvent.change(screen.getByRole('textbox', { name: 'File name' }), {
      target: { value: 'PNG' }
    })

    expect(await screen.findByRole('heading', { level: 1, name: 'Search results' })).toBeTruthy()
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining('b.png'),
      expect.stringContaining('a.png')
    ])
    expect(rows[0].textContent).toContain('docs/2026/')

    fireEvent.change(screen.getByRole('textbox', { name: 'File name' }), {
      target: { value: 'zzz' }
    })
    expect(await screen.findByText('No files match your search')).toBeTruthy()
  })

  it('shows each file’s folder in the grid’s search results too', async () => {
    listing()
    localStorage.setItem('notifuse.files.view.page', 'grid')
    renderExplorer()
    await waitFor(() => expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull())

    fireEvent.change(screen.getByRole('textbox', { name: 'File name' }), { target: { value: 'PNG' } })
    await waitFor(() => expect(document.querySelector('[data-file-card="docs/2026/b.png"]')).not.toBeNull())
    const card = document.querySelector('[data-file-card="docs/2026/b.png"]') as HTMLElement
    expect(card.querySelector('[data-file-folder]')).toHaveTextContent('docs/2026/')
  })

  it('lists the folders in the rail’s order, by name, whatever their dates', async () => {
    listing([
      { key: 'zebra/', lastModified: at(9) },
      { key: 'alpha/', lastModified: at(1) },
      { key: 'notes.txt', lastModified: at(6), size: 10 }
    ])
    renderExplorer({}, '')
    await screen.findByText('notes.txt')
    const listed = screen.getAllByRole('row').slice(1).map((row) => row.textContent ?? '')
    expect(listed[0]).toContain('alpha')
    expect(listed[1]).toContain('zebra')
    const inRail = within(rail()).getAllByRole('button').map((button) => button.textContent ?? '')
    expect(inRail.findIndex((name) => name.startsWith('alpha'))).toBeLessThan(
      inRail.findIndex((name) => name.startsWith('zebra'))
    )
  })

  it('says when a folder is empty', async () => {
    listing([{ key: 'empty/', lastModified: at(1) }])
    renderExplorer({}, 'empty/')
    expect(await screen.findByText('Folder is empty')).toBeTruthy()
  })

  it('replaces the panel header with the selection bar once a file is checked', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    fireEvent.click(screen.getByRole('checkbox', { name: 'a.png' }))
    const bar = document.querySelector('[data-selection-bar]') as HTMLElement
    expect(bar.textContent).toContain('1 selected')
    expect(within(bar).getByRole('button', { name: 'Download as zip' })).toBeTruthy()
    // A folder cannot be checked.
    expect((screen.getByRole('checkbox', { name: '2026' }) as HTMLInputElement).disabled).toBe(true)

    fireEvent.click(within(bar).getByRole('button', { name: 'Delete' }))
    expect((await screen.findAllByText('Delete 1 selected file?')).length).toBeGreaterThan(0)
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    fireEvent.click(within(bar).getByRole('button', { name: 'Clear' }))
    expect(document.querySelector('[data-selection-bar]')).toBeNull()
  })

  it('deletes the checked files in order and says how many went', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    fireEvent.click(screen.getByRole('checkbox', { name: 'a.png' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'report.pdf' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await screen.findAllByText('Delete 2 selected files?')
    fireEvent.click(screen.getAllByRole('button', { name: 'Delete' }).at(-1) as HTMLElement)

    expect(await screen.findByText('Deleted 2 files')).toBeTruthy()
    expect(fakeFiles.api.delete.mock.calls).toEqual([['ws', ['docs/a.png', 'docs/report.pdf']]])
  })

  it('says why a bulk delete failed', async () => {
    listing()
    fakeFiles.api.delete.mockRejectedValueOnce(new Error('too many keys'))
    renderExplorer()
    await screen.findByText('a.png')

    fireEvent.click(screen.getByRole('checkbox', { name: 'a.png' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await screen.findAllByText('Delete 1 selected file?')
    fireEvent.click(screen.getAllByRole('button', { name: 'Delete' }).at(-1) as HTMLElement)

    expect(await screen.findByText('Failed to delete the files: too many keys')).toBeTruthy()
  })

  it('leaves the file in the URL alone when a delete ends after the page is gone', async () => {
    listing()
    let release: () => void = () => {}
    fakeFiles.api.delete.mockImplementation(
      () =>
        new Promise((resolve) => {
          release = () => resolve({ deleted: 1, failed: 0 })
        })
    )
    const onFileChange = vi.fn()
    const { unmount } = renderExplorer({ controlledFile: 'docs/a.png', onFileChange })
    await screen.findByText('report.pdf')

    fireEvent.click(screen.getByRole('checkbox', { name: 'a.png' }))
    const bar = document.querySelector('[data-selection-bar]') as HTMLElement
    fireEvent.click(within(bar).getByRole('button', { name: 'Delete' }))
    await screen.findAllByText('Delete 1 selected file?')
    fireEvent.click(screen.getAllByRole('button', { name: 'Delete' }).at(-1) as HTMLElement)
    await waitFor(() => expect(fakeFiles.api.delete).toHaveBeenCalled())
    unmount()
    await act(async () => release())

    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(onFileChange).not.toHaveBeenCalled()
  })

  it('disables the writes in read-only mode', async () => {
    listing()
    renderExplorer({ readOnly: true })
    await screen.findByText('a.png')

    expect((screen.getByRole('button', { name: 'Upload' }) as HTMLButtonElement).disabled).toBe(
      true
    )
    // New folder is the rail's "+".
    const newFolder = within(rail()).getByRole('button', { name: 'New folder' })
    expect((newFolder as HTMLButtonElement).disabled).toBe(true)
    fireEvent.mouseEnter(newFolder.parentElement as HTMLElement)
    expect(await screen.findByText('New folder (Read-only mode)')).toBeTruthy()
    const menu = await openMenu('a.png')
    expect(
      within(menu).getByRole('menuitem', { name: 'Delete' }).getAttribute('aria-disabled')
    ).toBe('true')

    // A folder's menu in the rail: Delete folder exists only there.
    await openMenu('docs', true)
    expect(
      (await screen.findByRole('menuitem', { name: 'Delete folder' })).getAttribute('aria-disabled')
    ).toBe('true')
  })

  it('offers copy, open, download and delete on a file, open and delete on a folder', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    let menu = await openMenu('a.png')
    expect(
      within(menu)
        .getAllByRole('menuitem')
        .map((item) => item.textContent)
    ).toEqual(['Copy URL', 'Open in a new tab', 'Download', 'Delete'])
    fireEvent.keyDown(menu, { key: 'Escape' })

    menu = await openMenu('2026')
    await waitFor(() =>
      expect(
        within(menu)
          .getAllByRole('menuitem')
          .map((item) => item.textContent)
      ).toEqual(['Open', 'Delete folder'])
    )
  })

  it('previews the file named in the URL', async () => {
    listing()
    renderExplorer({ controlledFile: 'docs/report.pdf', onFileChange: vi.fn() })

    const preview = await waitFor(() => {
      const element = document.querySelector('[data-file-preview]')
      expect(element).not.toBeNull()
      return element as HTMLElement
    })
    expect(preview.textContent).toContain('report.pdf')
    expect(preview.textContent).toContain('application/pdf')
    expect(within(preview).getByRole('button', { name: 'Delete' })).toBeTruthy()
  })

  it('opens the file in a new tab and deletes it from icon buttons in the preview', async () => {
    listing()
    const onFileChange = vi.fn()
    renderExplorer({ controlledFile: 'docs/report.pdf', onFileChange })
    const preview = await waitFor(() => {
      const element = document.querySelector<HTMLElement>('[data-file-preview]')
      expect(element).not.toBeNull()
      return element as HTMLElement
    })

    // Icons only: their names are their aria-labels.
    const open = within(preview).getByRole('link', { name: 'Open in a new tab' })
    expect(open.getAttribute('href')).toMatch(/\/docs\/report\.pdf$/)
    expect(open.getAttribute('target')).toBe('_blank')
    expect(open.textContent).toBe('')
    const trash = within(preview).getByRole('button', { name: 'Delete' })
    expect(trash.textContent).toBe('')
    // Copy URL and Download keep their text, on the same row.
    expect(within(preview).getByRole('button', { name: 'Copy URL' }).parentElement).toBe(
      trash.parentElement
    )

    fireEvent.click(trash)
    const popconfirm = await openPopconfirm()
    // Nothing goes before the confirmation.
    expect(fakeFiles.api.delete).not.toHaveBeenCalled()
    fireEvent.click(within(popconfirm).getByRole('button', { name: 'Delete' }))

    expect(await screen.findByText('File deleted successfully.')).toBeTruthy()
    expect(fakeFiles.api.delete.mock.calls).toEqual([['ws', ['docs/report.pdf']]])
    expect(onFileChange).toHaveBeenCalledWith(undefined)
  })

  it('keeps the file when the preview’s Popconfirm is cancelled', async () => {
    listing()
    renderExplorer({ controlledFile: 'docs/report.pdf', onFileChange: vi.fn() })
    const preview = await waitFor(() => {
      const element = document.querySelector<HTMLElement>('[data-file-preview]')
      expect(element).not.toBeNull()
      return element as HTMLElement
    })

    fireEvent.click(within(preview).getByRole('button', { name: 'Delete' }))
    const popconfirm = await openPopconfirm()
    fireEvent.click(within(popconfirm).getByRole('button', { name: 'Cancel' }))
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(fakeFiles.api.delete).not.toHaveBeenCalled()
  })

  it('disables the preview’s delete in read-only mode', async () => {
    listing()
    renderExplorer({ readOnly: true, controlledFile: 'docs/report.pdf', onFileChange: vi.fn() })
    const preview = await waitFor(() => {
      const element = document.querySelector<HTMLElement>('[data-file-preview]')
      expect(element).not.toBeNull()
      return element as HTMLElement
    })

    const trash = within(preview).getByRole('button', { name: 'Delete' }) as HTMLButtonElement
    expect(trash.disabled).toBe(true)
    fireEvent.mouseEnter(trash.parentElement as HTMLElement)
    expect(await screen.findByText('Delete file (Read-only mode)')).toBeTruthy()
    fireEvent.click(trash)
    expect(
      screen.queryByText('Do you want to permanently delete this file from your storage?')
    ).toBeNull()
    // Opening in a new tab reads only.
    expect(within(preview).getByRole('link', { name: 'Open in a new tab' })).toBeTruthy()
  })

  it('closes the preview of a file the bucket no longer holds', async () => {
    listing()
    const onFileChange = vi.fn()
    renderExplorer({ controlledFile: 'docs/gone.png', onFileChange })

    await waitFor(() => expect(onFileChange).toHaveBeenCalledWith(undefined))
  })

  it('previews a file on click', async () => {
    listing()
    const onFileChange = vi.fn()
    renderExplorer({ onFileChange })
    await screen.findByText('a.png')

    fireEvent.click(screen.getByText('report.pdf'))
    expect(onFileChange).toHaveBeenCalledWith('docs/report.pdf')
  })

  describe('with room for the preview column', () => {
    const original = window.matchMedia
    beforeEach(() => {
      window.matchMedia = vi.fn().mockImplementation((media: string) => ({
        media,
        matches: true,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn()
      }))
    })
    afterEach(() => {
      window.matchMedia = original
    })

    it('walks through the files with J and K, and Escape closes the preview', async () => {
      listing()
      const onFileChange = vi.fn()
      const { rerenderAt } = renderExplorer({ controlledFile: 'docs/a.png', onFileChange })
      await screen.findByRole('complementary', { name: 'Preview' })

      fireEvent.keyDown(document, { key: 'j' })
      expect(onFileChange).toHaveBeenLastCalledWith('docs/report.pdf')

      rerenderAt('docs/', { controlledFile: 'docs/report.pdf', onFileChange })
      fireEvent.keyDown(document, { key: 'k' })
      expect(onFileChange).toHaveBeenLastCalledWith('docs/a.png')

      fireEvent.keyDown(document, { key: 'Escape' })
      expect(onFileChange).toHaveBeenLastCalledWith(undefined)
    })
  })

  it('warns that only the first files are shown when the listing is capped', async () => {
    // The server stops at its cap and says so.
    listing()
    fakeFiles.setListing({ capped: true })
    renderExplorer({}, '')
    expect(
      await screen.findByText(
        'Showing the first 10,000 files of this bucket. Folder counts, total size and search only cover these files.'
      )
    ).toBeTruthy()
  })

  it('warns when the storage cuts the listing short', async () => {
    listing([{ key: 'a.png', lastModified: at(1) }])
    fakeFiles.setListing({ cut_short: true })
    renderExplorer({}, '')
    expect(
      await screen.findByText(
        'The storage did not return the whole bucket. Folder counts, total size and search only cover the files shown.'
      )
    ).toBeTruthy()
  })

  it('shows the storage error with a Retry', async () => {
    listing()
    fakeFiles.api.list.mockRejectedValueOnce(new Error('Access Denied'))
    renderExplorer()

    expect(await screen.findByText('Access Denied')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('a.png')).toBeTruthy()
  })

  it('offers the storage settings beside Retry to whoever can change them', async () => {
    listing()
    fakeFiles.api.list.mockRejectedValueOnce(new Error('Access Denied'))
    renderExplorer({ canEditSettings: true })

    const subtitle = await screen.findByText('Access Denied')
    const result = subtitle.closest('.ant-result') as HTMLElement
    expect(within(result).getByRole('button', { name: 'Retry' })).toBeTruthy()
    expect(
      (within(result).getByRole('button', { name: 'Storage settings' }) as HTMLButtonElement).disabled
    ).toBe(false)
  })

  it('leaves the storage error without settings for whoever cannot change them', async () => {
    listing()
    fakeFiles.api.list.mockRejectedValueOnce(new Error('Access Denied'))
    renderExplorer()

    const subtitle = await screen.findByText('Access Denied')
    const result = subtitle.closest('.ant-result') as HTMLElement
    expect(within(result).queryByRole('button', { name: 'Storage settings' })).toBeNull()
  })

  it('asks to configure a storage that has no endpoint, an owner only', () => {
    const { unmount } = renderExplorer({ settings: { ...settings, endpoint: '' } })
    expect(screen.getByText('File storage is not configured.')).toBeTruthy()
    expect(
      (screen.getByRole('button', { name: 'Configure now' }) as HTMLButtonElement).disabled
    ).toBe(true)
    expect(fakeFiles.api.list).not.toHaveBeenCalled()
    unmount()

    renderExplorer({ settings: { ...settings, endpoint: '' }, canEditSettings: true })
    expect(
      (screen.getByRole('button', { name: 'Configure now' }) as HTMLButtonElement).disabled
    ).toBe(false)
  })

  it('enables the storage settings only for whoever can change them', async () => {
    listing()
    const { unmount } = renderExplorer()
    await screen.findByText('a.png')
    const locked = screen.getByRole('button', { name: 'Storage settings' }) as HTMLButtonElement
    expect(locked.disabled).toBe(true)
    unmount()

    // Read-only files do not lock the settings of an owner.
    renderExplorer({ canEditSettings: true, readOnly: true })
    await screen.findByText('a.png')
    expect(
      (screen.getByRole('button', { name: 'Storage settings' }) as HTMLButtonElement).disabled
    ).toBe(false)
  })

  it('tests and saves the storage settings, then lists the bucket again', async () => {
    listing()
    const onUpdateSettings = vi.fn<(next: FileExplorerProps['settings']) => Promise<void>>(
      async () => {}
    )
    renderExplorer({
      canEditSettings: true,
      settings: { ...settings, has_secret_key: true, secret_key_hint: 'WXYZ' },
      onUpdateSettings
    })
    await screen.findByText('a.png')
    const before = listCalls().length

    fireEvent.click(screen.getByRole('button', { name: 'Storage settings' }))
    const dialog = await screen.findByRole('dialog')
    await waitFor(() =>
      expect(within(dialog).getByLabelText('S3 bucket')).toHaveValue('media')
    )
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onUpdateSettings).toHaveBeenCalledTimes(1))
    expect(fakeFiles.api.testSettings).toHaveBeenCalledWith(
      'ws',
      expect.objectContaining({ endpoint: settings.endpoint, bucket: 'media' })
    )
    await waitFor(() => expect(listCalls().length).toBe(before + 1))
  })
})

// A folder of one file whose first delete stays pending until release() is called.
const pendingFolderDelete = () => {
  fakeFiles.reset([
    { key: 'docs/', lastModified: at(1) },
    { key: 'docs/a.png', lastModified: at(1) }
  ])
  let release: (() => void) | undefined
  const deleteSent = new Promise<void>((resolve) => {
    fakeFiles.api.deleteFolder.mockImplementation(async (workspaceId, prefix) => {
      if (release) return fakeFiles.impl.deleteFolder(workspaceId, prefix)
      resolve()
      return new Promise<DeleteFolderResult>((done) => {
        release = () => void fakeFiles.impl.deleteFolder(workspaceId, prefix).then(done)
      })
    })
  })
  return { deleteSent, release: () => release?.() }
}

// The listings under the folder's prefix: the count's; the server lists for the delete.
const folderListings = () =>
  fakeFiles.api.list.mock.calls.filter(([, prefix]) => prefix === 'docs/').length

const deleteCalls = () => fakeFiles.api.deleteFolder.mock.calls.length

const confirmButton = () => {
  const dialog = screen.getByRole('dialog')
  // A loading button's name starts with its spinner's.
  return within(dialog).getByRole('button', { name: /Delete folder/ })
}

const openFolderDelete = async () => {
  fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete folder' }))
  await screen.findByRole('dialog')
}

describe('FileExplorer folder delete', () => {
  beforeEach(() => {
    fakeFiles.reset()
  })

  it('asks with the number of files read from the folder’s own listing', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    expect(await screen.findByText('Delete the folder docs and its 3 files?')).toBeTruthy()
    expect(
      screen.getByText('Images used in emails or blog posts will stop displaying.')
    ).toBeTruthy()
  })

  it('says an empty folder is empty rather than counting its 0 files', async () => {
    listing([{ key: 'empty/', lastModified: at(1) }])
    renderExplorer({}, 'empty/')
    await screen.findByText('Folder is empty')

    await openFolderDelete()
    expect(await screen.findByText('Delete the empty folder empty?')).toBeTruthy()
    expect(screen.queryByText(/0 files/)).toBeNull()
  })

  it('shows the delete in progress and ignores a second confirmation', async () => {
    const pending = pendingFolderDelete()
    renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    await screen.findByText('Delete the folder docs and its 1 file?')
    fireEvent.click(confirmButton())
    await pending.deleteSent

    await waitFor(() => expect(confirmButton().className).toContain('ant-btn-loading'))
    fireEvent.click(confirmButton())
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    // One listing for the count; one delete started.
    expect(folderListings()).toBe(1)
    expect(deleteCalls()).toBe(1)

    await act(async () => pending.release())
  })

  it('goes to the parent folder when the delete ends in the deleted folder', async () => {
    const pending = pendingFolderDelete()
    const { onPathChange } = renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    fireEvent.click(confirmButton())
    await pending.deleteSent
    await act(async () => pending.release())

    await waitFor(() => expect(onPathChange).toHaveBeenCalledWith(''))
  })

  it('stays where the user went while the delete ran', async () => {
    const pending = pendingFolderDelete()
    const { onPathChange, rerenderAt } = renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    fireEvent.click(confirmButton())
    await pending.deleteSent
    rerenderAt('other/')
    await act(async () => pending.release())

    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(onPathChange).not.toHaveBeenCalled()
  })

  it('does not navigate once the file manager is gone', async () => {
    const pending = pendingFolderDelete()
    const { onPathChange, unmount } = renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    fireEvent.click(confirmButton())
    await pending.deleteSent
    unmount()
    await act(async () => pending.release())

    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(onPathChange).not.toHaveBeenCalled()
  })

  it('runs a second folder delete once the first has ended', async () => {
    const pending = pendingFolderDelete()
    renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    fireEvent.click(confirmButton())
    await pending.deleteSent
    await act(async () => pending.release())
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    const first = deleteCalls()

    await openFolderDelete()
    fireEvent.click(confirmButton())
    await waitFor(() => expect(deleteCalls()).toBeGreaterThan(first))
  })

  it('refuses to cancel while the folder is being deleted', async () => {
    const pending = pendingFolderDelete()
    renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    fireEvent.click(confirmButton())
    await pending.deleteSent
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }))
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(screen.getByRole('dialog')).toBeTruthy()

    await act(async () => pending.release())
  })

  it('says when the folder held nothing to delete', async () => {
    listing()
    // The bucket's listing still shows the folder; the server finds nothing in it now.
    fakeFiles.api.deleteFolder.mockResolvedValue({
      deleted: 0,
      failed: 0,
      deleted_files: 0,
      failed_files: 0
    })
    renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    fireEvent.click(confirmButton())

    expect(await screen.findByText('The folder held nothing to delete.')).toBeTruthy()
    expect(screen.queryByText('Folder deleted successfully.')).toBeNull()
    expect(fakeFiles.api.deleteFolder.mock.calls).toEqual([['ws', 'docs/']])
  })

  it('refuses to delete a folder of more files than the server lists', async () => {
    listing()
    // The folder's own listing stops at the server's cap.
    fakeFiles.api.list.mockImplementation(async (workspaceId, prefix) => ({
      ...(await fakeFiles.impl.list(workspaceId, prefix)),
      capped: prefix === 'docs/'
    }))
    renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    expect(
      await screen.findByText(
        'The folder docs holds more than 10,000 files. Delete its subfolders first.'
      )
    ).toBeTruthy()
    expect((confirmButton() as HTMLButtonElement).disabled).toBe(true)
    fireEvent.click(confirmButton())
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(deleteCalls()).toBe(0)
  })

  it('keeps the folder and says how many files failed', async () => {
    listing()
    fakeFiles.refuseDelete('docs/a.png')
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const { onPathChange } = renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    fireEvent.click(confirmButton())

    expect(await screen.findByText('Deleted 2 files, 1 failed')).toBeTruthy()
    expect(onPathChange).not.toHaveBeenCalled()
  })
})

const renderPicker = (props: Partial<FileExplorerProps> = {}) =>
  renderExplorer(
    {
      mode: 'picker',
      acceptFileType: 'image/*',
      acceptItem: isImage,
      onCancel: vi.fn(),
      ...props
    },
    'docs/'
  )

const footer = () => document.querySelector('[data-picker-footer]') as HTMLElement

describe('FileExplorer picker', () => {
  beforeEach(() => {
    fakeFiles.reset()
    vi.stubGlobal('fetch', fakeFiles.fetch)
    localStorage.clear()
    clearImageSizes()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows a grid, dims the files it cannot use and asks to select a file', async () => {
    listing()
    renderPicker()
    await waitFor(() =>
      expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull()
    )

    const pdf = document.querySelector('[data-file-card="docs/report.pdf"]') as HTMLElement
    expect(pdf.getAttribute('aria-disabled')).toBe('true')
    expect(pdf.className).toContain('opacity-50')
    expect(footer().textContent).toContain('Select a file')
    expect(
      (within(footer()).getByRole('button', { name: 'Insert image' }) as HTMLButtonElement).disabled
    ).toBe(true)
    // No checkboxes in the picker.
    expect(screen.queryByRole('checkbox')).toBeNull()
  })

  it('selects on click, inserts on Insert image, and says where the image goes', async () => {
    listing()
    const onSelect = vi.fn()
    renderPicker({ onSelect, targetLabel: 'the Image block' })
    await waitFor(() =>
      expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull()
    )

    fireEvent.click(document.querySelector('[data-file-card="docs/a.png"]') as HTMLElement)
    expect(onSelect).not.toHaveBeenCalled()
    expect(footer().textContent).toContain('a.png · 2 kB · will be inserted into the Image block')

    fireEvent.click(within(footer()).getByRole('button', { name: 'Insert image' }))
    expect(onSelect).toHaveBeenCalledWith([expect.objectContaining({ key: 'docs/a.png' })])
  })

  it('scrolls the selected file back into view once the preview column narrows the grid', async () => {
    listing()
    const scrolled: Element[] = []
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = function (this: Element) {
      scrolled.push(this)
    }
    try {
      renderPicker()
      await waitFor(() => expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull())
      fireEvent.click(document.querySelector('[data-file-card="docs/a.png"]') as HTMLElement)
      await waitFor(() =>
        expect(scrolled).toContain(document.querySelector('[data-file-card="docs/a.png"]'))
      )
    } finally {
      Element.prototype.scrollIntoView = original
    }
  })

  it('leaves the target out of the footer when the caller names none', async () => {
    listing()
    renderPicker()
    await waitFor(() =>
      expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull()
    )

    fireEvent.click(document.querySelector('[data-file-card="docs/a.png"]') as HTMLElement)
    expect(footer().textContent).toContain('a.png · 2 kB')
    expect(footer().textContent).not.toContain('will be inserted')
  })

  it('inserts on a double click and on Enter, never a file it cannot use', async () => {
    listing()
    const onSelect = vi.fn()
    renderPicker({ onSelect })
    await waitFor(() =>
      expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull()
    )
    const card = document.querySelector('[data-file-card="docs/a.png"]') as HTMLElement

    fireEvent.doubleClick(card)
    expect(onSelect).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(card, { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledTimes(2)

    fireEvent.doubleClick(
      document.querySelector('[data-file-card="docs/report.pdf"]') as HTMLElement
    )
    expect(onSelect).toHaveBeenCalledTimes(2)
  })

  it('walks with J and K over the files it can use only', async () => {
    listing([...BUCKET, { key: 'docs/c.png', lastModified: at(1), size: 5 }])
    renderPicker()
    await waitFor(() =>
      expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull()
    )

    fireEvent.click(document.querySelector('[data-file-card="docs/a.png"]') as HTMLElement)
    // report.pdf comes between a.png and c.png, and is skipped.
    fireEvent.keyDown(document, { key: 'j' })
    expect(footer().textContent).toContain('c.png')
    expect(footer().textContent).not.toContain('report.pdf')
    fireEvent.keyDown(document, { key: 'k' })
    expect(footer().textContent).toContain('a.png')
  })

  it('does not insert on a double click of a row’s menu button in the list', async () => {
    localStorage.setItem('notifuse.files.view.picker', 'list')
    listing()
    const onSelect = vi.fn()
    renderPicker({ onSelect })
    const button = await screen.findByRole('button', { name: 'Actions for a.png' })

    fireEvent.doubleClick(button)
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('keeps Delete folder in a folder’s menu', async () => {
    listing()
    renderPicker()
    await waitFor(() =>
      expect(document.querySelector('[data-file-card="docs/2026/"]')).not.toBeNull()
    )

    const menu = await openMenu('2026')
    await waitFor(() =>
      expect(within(menu).getByRole('menuitem', { name: 'Delete folder' })).toBeTruthy()
    )
  })

  it('chooses the image it has just uploaded', async () => {
    listing()
    renderPicker()
    await waitFor(() =>
      expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull()
    )

    const file = new File(['x'], 'New Logo.PNG', { type: 'image/png' })
    fireEvent.change(screen.getByTestId('file-manager-upload-input'), { target: { files: [file] } })

    await waitFor(() => expect(footer().textContent).toContain('New-Logo.png'))
    expect(fakeFiles.api.presignUpload).toHaveBeenCalledWith(
      expect.objectContaining({ workspace_id: 'ws', key: 'docs/New-Logo.png' })
    )
    const signed = (await fakeFiles.api.presignUpload.mock.results[0].value) as PresignedUpload
    expect(fakeFiles.fetch).toHaveBeenCalledWith(
      signed.url,
      expect.objectContaining({ method: 'PUT', headers: signed.headers, body: file })
    )
  })

  it('navigates and creates folders from the same rail, as a column', async () => {
    listing()
    const { onPathChange } = renderPicker()
    await waitFor(() => expect(within(rail()).getByRole('button', { name: '2026 1' })).toBeTruthy())
    expect(rail().hasAttribute('data-rail-card')).toBe(false)

    fireEvent.click(within(rail()).getByRole('button', { name: '2026 1' }))
    expect(onPathChange).toHaveBeenCalledWith('docs/2026/')
    fireEvent.click(within(rail()).getByRole('button', { name: 'Root 5' }))
    expect(onPathChange).toHaveBeenLastCalledWith('')

    // One New folder: the rail's "+".
    expect(screen.getAllByRole('button', { name: 'New folder' })).toHaveLength(1)
    fireEvent.click(within(rail()).getByRole('button', { name: 'New folder' }))
    expect((await screen.findByRole('dialog')).textContent).toContain('Create new folder')
  })

  it('cancels from the footer', async () => {
    listing()
    const onCancel = vi.fn()
    renderPicker({ onCancel })
    await waitFor(() => expect(footer()).not.toBeNull())

    fireEvent.click(within(footer()).getByRole('button', { name: 'Cancel' }))
    expect(onCancel).toHaveBeenCalled()
  })
})
