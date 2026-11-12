import type React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent, act } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { DeleteObjectCommand, ListObjectsV2Command } from '@aws-sdk/client-s3'
import { FileManager } from './fileManager'
import type { FileManagerProps } from './interfaces'

// Every S3Client the component builds answers through this send.
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

// The shared macro mock returns a new `t` on every render, and the listing effect depends on
// `t`: it would list the bucket forever. A `t` that keeps its identity, as the real one does.
const stableLingui = vi.hoisted(() => {
  const t = (strings: TemplateStringsArray, ...values: unknown[]) =>
    strings.reduce(
      (result, str, idx) => result + str + (idx < values.length ? String(values[idx]) : ''),
      ''
    )
  return { t, i18n: { locale: 'en' } }
})
vi.mock('@lingui/react/macro', () => ({
  useLingui: () => stableLingui,
  Trans: ({ children }: { children: React.ReactNode }) => children
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

// The settings object keeps its identity across renders, as the page's would once memoised:
// a new one recreates the client and lists again.
const settings = { endpoint: 'https://s3.example.com', access_key: 'key', bucket: 'media' }

const renderManager = (props: Partial<FileManagerProps> = {}) => {
  const onPathChange = vi.fn()
  const tree = (path: string) => (
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <FileManager
            settings={settings}
            onUpdateSettings={vi.fn()}
            onError={vi.fn()}
            onSelect={vi.fn()}
            height={600}
            acceptFileType="*"
            acceptItem={() => true}
            controlledPath={path}
            onPathChange={onPathChange}
            {...props}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
  const utils = render(tree('docs/'))
  return { ...utils, onPathChange, rerenderAt: (path: string) => utils.rerender(tree(path)) }
}

// The folder's trash button comes first; the confirmation's button follows once it opens.
const trashButton = () => screen.getAllByRole('button', { name: 'Delete folder' })[0]

const confirmFolderDelete = async () => {
  fireEvent.click(trashButton())
  await waitFor(() =>
    expect(screen.getAllByRole('button', { name: 'Delete folder' })).toHaveLength(2)
  )
  fireEvent.click(screen.getAllByRole('button', { name: 'Delete folder' })[1])
}

// A folder of one file whose first delete stays pending until release() is called.
const pendingFolderDelete = () => {
  let release: (() => void) | undefined
  const deleteSent = new Promise<void>((resolve) => {
    s3.send.mockImplementation(async (command: unknown) => {
      if (command instanceof ListObjectsV2Command) {
        return {
          Contents: [
            { Key: 'docs/', LastModified: new Date() },
            { Key: 'docs/a.png', LastModified: new Date() }
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

// The listings a folder delete makes, under the folder's prefix.
const folderListings = () =>
  s3.send.mock.calls.filter(
    ([command]) => command instanceof ListObjectsV2Command && command.input.Prefix === 'docs/'
  ).length

const deleteCalls = () =>
  s3.send.mock.calls.filter(([command]) => command instanceof DeleteObjectCommand).length

describe('FileManager folder delete', () => {
  beforeEach(() => {
    s3.send.mockReset()
  })

  it('shows the delete in progress and ignores a second confirmation', async () => {
    const pending = pendingFolderDelete()
    renderManager()
    await screen.findByText('a.png')

    await confirmFolderDelete()
    await pending.deleteSent

    await waitFor(() => expect(trashButton().className).toContain('ant-btn-loading'))
    // Confirm again: the popover's button stays in the document once it has opened.
    fireEvent.click(trashButton())
    fireEvent.click(screen.getAllByRole('button', { name: 'Delete folder' })[1])
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(folderListings()).toBe(1)
    expect(deleteCalls()).toBe(1)

    await act(async () => pending.release())
  })

  it('goes to the parent folder when the delete ends in the deleted folder', async () => {
    const pending = pendingFolderDelete()
    const { onPathChange } = renderManager()
    await screen.findByText('a.png')

    await confirmFolderDelete()
    await pending.deleteSent
    await act(async () => pending.release())

    await waitFor(() => expect(onPathChange).toHaveBeenCalledWith(''))
  })

  it('stays where the user went while the delete ran', async () => {
    const pending = pendingFolderDelete()
    const { onPathChange, rerenderAt } = renderManager()
    await screen.findByText('a.png')

    await confirmFolderDelete()
    await pending.deleteSent
    rerenderAt('other/')
    await act(async () => pending.release())

    // Let the delete's callbacks run before checking that nothing navigated.
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(onPathChange).not.toHaveBeenCalled()
  })

  it('does not navigate once the file manager is gone', async () => {
    const pending = pendingFolderDelete()
    const { onPathChange, unmount } = renderManager()
    await screen.findByText('a.png')

    await confirmFolderDelete()
    await pending.deleteSent
    unmount()
    await act(async () => pending.release())

    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(onPathChange).not.toHaveBeenCalled()
  })
})
