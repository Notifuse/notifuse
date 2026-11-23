import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { FileManagerProvider, useFileManager } from './context'
import { fakeFiles } from './fakeFilesApi'

vi.mock('../../services/api/files', async () => ({
  filesApi: (await import('./fakeFilesApi')).fakeFiles.api
}))

const settings = { endpoint: 'https://s3.example.com', access_key: 'key', bucket: 'media' }

function Consumer(props: {
  onSelect: (url: string) => void
  acceptFileType?: string
  targetLabel?: string
}) {
  const { SelectFileButton } = useFileManager()
  return <SelectFileButton buttonText="Browse" {...props} />
}

const renderPicker = (props: Parameters<typeof Consumer>[0]) =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <ConfigProvider>
        <App>
          <FileManagerProvider workspaceId="ws" settings={settings} onUpdateSettings={vi.fn()}>
            <Consumer {...props} />
          </FileManagerProvider>
        </App>
      </ConfigProvider>
    </QueryClientProvider>
  )

const card = (key: string) => document.querySelector(`[data-file-card="${key}"]`) as HTMLElement
const footer = () => document.querySelector('[data-picker-footer]') as HTMLElement

describe('FileManagerProvider picker', () => {
  beforeEach(() => {
    localStorage.clear()
    fakeFiles.reset([
      { key: 'docs/', lastModified: new Date() },
      { key: 'small.png', lastModified: new Date(), size: 1000 },
      { key: 'big.png', lastModified: new Date(), size: 500_000 }
    ])
  })

  it('is titled after the files it accepts', async () => {
    const { unmount } = renderPicker({ onSelect: vi.fn() })
    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
    expect(await screen.findByText('Choose an image')).toBeTruthy()
    unmount()

    renderPicker({ onSelect: vi.fn(), acceptFileType: '*' })
    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
    expect(await screen.findByText('Choose a file')).toBeTruthy()
    expect(await screen.findByRole('button', { name: 'Select file' })).toBeTruthy()
  })

  it('hands the URL over on Insert image only, then closes', async () => {
    const onSelect = vi.fn()
    renderPicker({ onSelect, targetLabel: 'the Image block' })
    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
    await waitFor(() => expect(card('small.png')).not.toBeNull())

    fireEvent.click(card('small.png'))
    expect(onSelect).not.toHaveBeenCalled()
    expect(footer().textContent).toContain('will be inserted into the Image block')

    fireEvent.click(within(footer()).getByRole('button', { name: 'Insert image' }))
    expect(onSelect).toHaveBeenCalledWith('https://s3.example.com/media/small.png')
    expect(await screen.findByText('Selected: small.png')).toBeTruthy()
    await waitFor(() => expect(footer()).toBeNull())
  })

  it('warns before inserting a large file', async () => {
    const onSelect = vi.fn()
    renderPicker({ onSelect })
    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
    await waitFor(() => expect(card('big.png')).not.toBeNull())

    fireEvent.doubleClick(card('big.png'))
    expect(await screen.findByText('Large File Warning')).toBeTruthy()
    expect(onSelect).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Use Anyway' }))
    expect(onSelect).toHaveBeenCalledWith('https://s3.example.com/media/big.png')
  })

  it('reopens at the last folder visited', async () => {
    renderPicker({ onSelect: vi.fn() })
    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
    await waitFor(() => expect(card('docs/')).not.toBeNull())

    fireEvent.click(card('docs/'))
    await screen.findByText('Folder is empty')
    fireEvent.click(within(footer()).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(footer()).toBeNull())

    fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
    const breadcrumb = await screen.findByRole('navigation', { name: 'Breadcrumb' })
    expect(within(breadcrumb).getByText('docs').getAttribute('aria-current')).toBe('page')
  })
})
