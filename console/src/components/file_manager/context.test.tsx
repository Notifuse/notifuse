import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ListObjectsV2Command } from '@aws-sdk/client-s3'
import { FileManagerProvider, useFileManager } from './context'

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
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
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
    s3.send.mockReset()
    localStorage.clear()
    s3.send.mockImplementation(async (command: unknown) => {
      if (command instanceof ListObjectsV2Command) {
        return {
          Contents: [
            { Key: 'docs/', LastModified: new Date() },
            { Key: 'small.png', LastModified: new Date(), Size: 1000 },
            { Key: 'big.png', LastModified: new Date(), Size: 500_000 }
          ],
          IsTruncated: false
        }
      }
      return {}
    })
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
