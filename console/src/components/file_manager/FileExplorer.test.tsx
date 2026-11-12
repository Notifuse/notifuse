import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent, act, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  DeleteObjectCommand,
  ListObjectsV2Command,
  PutObjectCommand
} from '@aws-sdk/client-s3'
import { FileExplorer, type FileExplorerProps } from './FileExplorer'
import { clearImageSizes } from './imageDimensions'
import type { StorageObject } from './interfaces'

// Every S3Client the explorer builds answers through this send.
const s3 = vi.hoisted(() => ({ send: vi.fn() }))
vi.mock('@aws-sdk/client-s3', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@aws-sdk/client-s3')>()
  return {
    ...actual,
    S3Client: class {
      send = s3.send
    }
  }
})

const settings = { endpoint: 'https://s3.example.com', access_key: 'key', bucket: 'media' }
const at = (day: number) => new Date(Date.UTC(2026, 0, day))

// docs/ holds two files and a sub-folder with one; photos/ exists only through its file.
const BUCKET = [
  { Key: 'docs/', LastModified: at(1) },
  { Key: 'docs/a.png', LastModified: at(3), Size: 2048 },
  { Key: 'docs/report.pdf', LastModified: at(2), Size: 1024 },
  { Key: 'docs/2026/', LastModified: at(1) },
  { Key: 'docs/2026/b.png', LastModified: at(4), Size: 100 },
  { Key: 'photos/beach.jpg', LastModified: at(5), Size: 300 },
  { Key: 'notes.txt', LastModified: at(6), Size: 10 }
]

// Answers the listings from a copy of the objects, which an upload adds to.
const listing = (initial: typeof BUCKET = BUCKET) => {
  const contents = [...initial]
  s3.send.mockImplementation(async (command: unknown) => {
    if (command instanceof ListObjectsV2Command) {
      const prefix = command.input.Prefix ?? ''
      return {
        Contents: contents.filter((object) => object.Key.startsWith(prefix)),
        IsTruncated: false
      }
    }
    if (command instanceof PutObjectCommand) {
      contents.push({ Key: command.input.Key as string, LastModified: at(9), Size: 1 })
    }
    return {}
  })
}

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

const listCalls = () =>
  s3.send.mock.calls.filter(([command]) => command instanceof ListObjectsV2Command)

// The open dropdown's menu: a closed one stays in the document, hidden.
const openMenu = async (name: string) => {
  fireEvent.click(screen.getByRole('button', { name: `Actions for ${name}` }))
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
    s3.send.mockReset()
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
    await waitFor(() => expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull())
  })

  it('searches file names in the whole bucket and shows each file’s folder', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    fireEvent.change(screen.getByRole('textbox', { name: 'File name' }), { target: { value: 'PNG' } })

    expect(await screen.findByRole('heading', { level: 1, name: 'Search results' })).toBeTruthy()
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining('b.png'),
      expect.stringContaining('a.png')
    ])
    expect(rows[0].textContent).toContain('docs/2026/')

    fireEvent.change(screen.getByRole('textbox', { name: 'File name' }), { target: { value: 'zzz' } })
    expect(await screen.findByText('No files match your search')).toBeTruthy()
  })

  it('says when a folder is empty', async () => {
    listing([{ Key: 'empty/', LastModified: at(1) }])
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

  it('deletes the checked files one by one and says how many went', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    fireEvent.click(screen.getByRole('checkbox', { name: 'a.png' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'report.pdf' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await screen.findAllByText('Delete 2 selected files?')
    fireEvent.click(screen.getAllByRole('button', { name: 'Delete' }).at(-1) as HTMLElement)

    expect(await screen.findByText('Deleted 2 files')).toBeTruthy()
    const deleted = s3.send.mock.calls
      .filter(([command]) => command instanceof DeleteObjectCommand)
      .map(([command]) => command.input.Key)
    expect(deleted).toEqual(['docs/a.png', 'docs/report.pdf'])
  })

  it('disables the writes in read-only mode', async () => {
    listing()
    renderExplorer({ readOnly: true })
    await screen.findByText('a.png')

    expect((screen.getByRole('button', { name: 'Upload' }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: 'New folder' }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: 'Storage settings' }) as HTMLButtonElement).disabled).toBe(
      true
    )
    const menu = await openMenu('a.png')
    expect(within(menu).getByRole('menuitem', { name: 'Delete' }).getAttribute('aria-disabled')).toBe(
      'true'
    )
  })

  it('offers copy, open, download and delete on a file, open and delete on a folder', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    let menu = await openMenu('a.png')
    expect(within(menu).getAllByRole('menuitem').map((item) => item.textContent)).toEqual([
      'Copy URL',
      'Open in a new tab',
      'Download',
      'Delete'
    ])
    fireEvent.keyDown(menu, { key: 'Escape' })

    menu = await openMenu('2026')
    await waitFor(() =>
      expect(within(menu).getAllByRole('menuitem').map((item) => item.textContent)).toEqual([
        'Open',
        'Delete folder'
      ])
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
    // One more than the cap, in a folder so the root lists a single row.
    const many = Array.from({ length: 10_001 }, (_, index) => ({
      Key: `many/f${index}.png`,
      LastModified: at(1),
      Size: 1
    }))
    listing(many)
    renderExplorer({}, '')
    expect(
      await screen.findByText(
        'Showing the first 10,000 files of this bucket. Folder counts, total size and search only cover these files.'
      )
    ).toBeTruthy()
  })

  it('warns when the storage cuts the listing short', async () => {
    s3.send.mockResolvedValue({
      Contents: [{ Key: 'a.png', LastModified: at(1) }],
      IsTruncated: true
    })
    renderExplorer({}, '')
    expect(
      await screen.findByText(
        'The storage did not return the whole bucket. Folder counts, total size and search only cover the files shown.'
      )
    ).toBeTruthy()
  })

  it('shows the storage error with a Retry', async () => {
    listing()
    s3.send.mockRejectedValueOnce(new Error('Access Denied'))
    renderExplorer()

    expect(await screen.findByText('Access Denied')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('a.png')).toBeTruthy()
  })

  it('asks to configure a storage that has no endpoint', () => {
    renderExplorer({ settings: { ...settings, endpoint: '' } })
    expect(screen.getByText('File storage is not configured.')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Configure now' })).toBeTruthy()
    expect(s3.send).not.toHaveBeenCalled()
  })
})

// A folder of one file whose first delete stays pending until release() is called.
const pendingFolderDelete = () => {
  let release: (() => void) | undefined
  const deleteSent = new Promise<void>((resolve) => {
    s3.send.mockImplementation(async (command: unknown) => {
      if (command instanceof ListObjectsV2Command) {
        return {
          Contents: [
            { Key: 'docs/', LastModified: at(1) },
            { Key: 'docs/a.png', LastModified: at(1) }
          ],
          IsTruncated: false
        }
      }
      if (command instanceof DeleteObjectCommand) {
        if (release) return {}
        resolve()
        return new Promise((done) => {
          release = () => done({})
        })
      }
      throw new Error('unexpected command')
    })
  })
  return { deleteSent, release: () => release?.() }
}

// The listings under the folder's prefix: one for the count, one for the delete.
const folderListings = () =>
  s3.send.mock.calls.filter(
    ([command]) => command instanceof ListObjectsV2Command && command.input.Prefix === 'docs/'
  ).length

const deleteCalls = () =>
  s3.send.mock.calls.filter(([command]) => command instanceof DeleteObjectCommand).length

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
    s3.send.mockReset()
  })

  it('asks with the number of files read from the folder’s own listing', async () => {
    listing()
    renderExplorer()
    await screen.findByText('a.png')

    await openFolderDelete()
    expect(await screen.findByText('Delete the folder docs and its 3 files?')).toBeTruthy()
    expect(screen.getByText('Images used in emails or blog posts will stop displaying.')).toBeTruthy()
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
    // One listing for the count, one for the delete; one delete started.
    expect(folderListings()).toBe(2)
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

  it('keeps the folder and says how many files failed', async () => {
    s3.send.mockImplementation(async (command: unknown) => {
      if (command instanceof ListObjectsV2Command) return { Contents: BUCKET, IsTruncated: false }
      if (command instanceof DeleteObjectCommand && command.input.Key === 'docs/a.png') {
        throw new Error('denied')
      }
      return {}
    })
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
    s3.send.mockReset()
    localStorage.clear()
    clearImageSizes()
  })

  it('shows a grid, dims the files it cannot use and asks to select a file', async () => {
    listing()
    renderPicker()
    await waitFor(() => expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull())

    const pdf = document.querySelector('[data-file-card="docs/report.pdf"]') as HTMLElement
    expect(pdf.getAttribute('aria-disabled')).toBe('true')
    expect(pdf.className).toContain('opacity-50')
    expect(footer().textContent).toContain('Select a file')
    expect((within(footer()).getByRole('button', { name: 'Insert image' }) as HTMLButtonElement).disabled).toBe(
      true
    )
    // No checkboxes in the picker.
    expect(screen.queryByRole('checkbox')).toBeNull()
  })

  it('selects on click, inserts on Insert image, and says where the image goes', async () => {
    listing()
    const onSelect = vi.fn()
    renderPicker({ onSelect, targetLabel: 'the Image block' })
    await waitFor(() => expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull())

    fireEvent.click(document.querySelector('[data-file-card="docs/a.png"]') as HTMLElement)
    expect(onSelect).not.toHaveBeenCalled()
    expect(footer().textContent).toContain('a.png · 2 kB · will be inserted into the Image block')

    fireEvent.click(within(footer()).getByRole('button', { name: 'Insert image' }))
    expect(onSelect).toHaveBeenCalledWith([expect.objectContaining({ key: 'docs/a.png' })])
  })

  it('leaves the target out of the footer when the caller names none', async () => {
    listing()
    renderPicker()
    await waitFor(() => expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull())

    fireEvent.click(document.querySelector('[data-file-card="docs/a.png"]') as HTMLElement)
    expect(footer().textContent).toContain('a.png · 2 kB')
    expect(footer().textContent).not.toContain('will be inserted')
  })

  it('inserts on a double click and on Enter, never a file it cannot use', async () => {
    listing()
    const onSelect = vi.fn()
    renderPicker({ onSelect })
    await waitFor(() => expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull())
    const card = document.querySelector('[data-file-card="docs/a.png"]') as HTMLElement

    fireEvent.doubleClick(card)
    expect(onSelect).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(card, { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledTimes(2)

    fireEvent.doubleClick(document.querySelector('[data-file-card="docs/report.pdf"]') as HTMLElement)
    expect(onSelect).toHaveBeenCalledTimes(2)
  })

  it('keeps Delete folder in a folder’s menu', async () => {
    listing()
    renderPicker()
    await waitFor(() => expect(document.querySelector('[data-file-card="docs/2026/"]')).not.toBeNull())

    const menu = await openMenu('2026')
    await waitFor(() =>
      expect(within(menu).getByRole('menuitem', { name: 'Delete folder' })).toBeTruthy()
    )
  })

  it('chooses the image it has just uploaded', async () => {
    listing()
    renderPicker()
    await waitFor(() => expect(document.querySelector('[data-file-card="docs/a.png"]')).not.toBeNull())

    const file = new File(['x'], 'New Logo.PNG', { type: 'image/png' })
    Object.defineProperty(file, 'arrayBuffer', { value: async () => new ArrayBuffer(1) })
    fireEvent.change(screen.getByTestId('file-manager-upload-input'), { target: { files: [file] } })

    await waitFor(() => expect(footer().textContent).toContain('New-Logo.png'))
    const put = s3.send.mock.calls.find(([command]) => command instanceof PutObjectCommand)
    expect(put?.[0].input.Key).toBe('docs/New-Logo.png')
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
