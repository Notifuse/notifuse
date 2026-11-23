import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { FileManagerProps, FileManagerSettings } from '../components/file_manager/interfaces'
import type { StoredPermissions } from '../services/api/permissions'
import type { WorkspaceRole } from '../components/file_manager/fileRights'
import { workspaceService } from '../services/api/workspace'
import { fakeFiles } from '../components/file_manager/fakeFilesApi'

const state = vi.hoisted(() => ({
  search: {} as Record<string, string | undefined>,
  navigate: vi.fn(),
  permissions: {} as Record<string, { read: boolean; write: boolean }>,
  role: 'member' as string | undefined,
  fileManager: undefined as Record<string, unknown> | undefined,
  received: undefined as FileManagerProps | undefined,
  // Render the real explorer rather than the stand-in.
  real: false
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => state.search,
  useNavigate: () => state.navigate
}))
vi.mock('../router', () => ({
  workspaceFileManagerRoute: { id: 'file-manager', to: '/console/workspace/$workspaceId/file-manager' }
}))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [{ id: 'ws1', name: 'Acme', settings: { file_manager: state.fileManager } }],
    refreshWorkspaces: vi.fn()
  }),
  useWorkspacePermissions: () => ({
    permissions: state.permissions as StoredPermissions,
    role: state.role as WorkspaceRole,
    loading: false
  })
}))
vi.mock('../services/api/workspace', () => ({ workspaceService: { update: vi.fn() } }))
vi.mock('../services/api/files', async () => ({
  filesApi: (await import('../components/file_manager/fakeFilesApi')).fakeFiles.api
}))
// The explorer is tested on its own; here it mostly shows what the page gave it.
vi.mock('../components/file_manager/fileManager', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../components/file_manager/fileManager')>()
  return {
    FileManager: (props: FileManagerProps) => {
      state.received = props
      return state.real ? <actual.FileManager {...props} /> : <div data-testid="file-manager" />
    }
  }
})

import { FileManagerPage } from './FileManagerPage'

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ConfigProvider>
        <App>
          <FileManagerPage />
        </App>
      </ConfigProvider>
    </QueryClientProvider>
  )

const templates = (read: boolean, write: boolean) => ({ templates: { read, write } })
const blog = (read: boolean, write: boolean) => ({ blog: { read, write } })

describe('FileManagerPage', () => {
  beforeEach(() => {
    state.search = {}
    state.navigate.mockReset()
    state.permissions = templates(true, true)
    state.role = 'member'
    state.fileManager = { endpoint: 'https://s3.example.com', bucket: 'media', access_key: 'k' }
    state.received = undefined
    state.real = false
    fakeFiles.reset([{ key: 'a.png', size: 10, lastModified: new Date(Date.UTC(2026, 0, 1)) }])
    vi.mocked(workspaceService.update).mockReset()
  })

  it('gives the explorer the workspace, its storage and the folder and file of the URL', () => {
    state.search = { path: 'docs/', file: 'docs/a.png' }
    renderPage()

    expect(screen.getByTestId('file-manager')).toBeTruthy()
    expect(state.received).toMatchObject({
      workspaceId: 'ws1',
      controlledPath: 'docs/',
      controlledFile: 'docs/a.png',
      readOnly: false,
      settings: { endpoint: 'https://s3.example.com', bucket: 'media' }
    })
  })

  it('is writable with write access to templates or to the blog', () => {
    state.permissions = { ...templates(false, false), ...blog(true, true) }
    renderPage()
    expect(state.received?.readOnly).toBe(false)
  })

  it('is read-only with read access only', () => {
    state.permissions = { ...templates(true, false), ...blog(true, false) }
    renderPage()
    expect(state.received?.readOnly).toBe(true)
  })

  it('refuses a member without any right on templates or the blog, and lists nothing', async () => {
    state.permissions = { ...templates(false, false), ...blog(false, false), workspace: { read: true, write: true } }
    state.real = true
    renderPage()

    expect(screen.getByText("You don't have access to the files")).toBeTruthy()
    expect(state.received).toBeUndefined()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(fakeFiles.api.list).not.toHaveBeenCalled()
  })

  it('disables the upload for a member who can only read', async () => {
    state.permissions = templates(true, false)
    state.real = true
    renderPage()

    expect(await screen.findByText('a.png')).toBeTruthy()
    expect(fakeFiles.api.list).toHaveBeenCalledWith('ws1')
    expect((screen.getByRole('button', { name: 'Upload' }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('locks the storage settings for a member, even one who writes files', async () => {
    state.permissions = { ...templates(true, true), ...blog(true, true) }
    state.real = true
    renderPage()

    expect(await screen.findByText('a.png')).toBeTruthy()
    expect(state.received?.canEditSettings).toBe(false)
    expect((screen.getByRole('button', { name: 'Upload' }) as HTMLButtonElement).disabled).toBe(false)
    expect(
      (screen.getByRole('button', { name: 'Storage settings' }) as HTMLButtonElement).disabled
    ).toBe(true)
  })

  it.each(['owner', 'root'])('lets an %s change the storage settings', (role) => {
    state.role = role
    state.permissions = {}
    renderPage()
    expect(state.received).toMatchObject({ readOnly: false, canEditSettings: true })
  })

  it('passes on whether a secret is stored, never a secret', () => {
    state.role = 'owner'
    state.fileManager = {
      endpoint: 'https://s3.example.com',
      bucket: 'media',
      access_key: 'k',
      has_secret_key: true,
      secret_key_hint: 'WXYZ'
    }
    renderPage()
    expect(state.received?.settings).toMatchObject({ has_secret_key: true, secret_key_hint: 'WXYZ' })
    expect(state.received?.settings).not.toHaveProperty('secret_key')
  })

  it('saves the settings without an empty secret nor the fields only the server writes', async () => {
    state.role = 'owner'
    renderPage()
    const next: FileManagerSettings = {
      endpoint: 'https://s3.example.com',
      access_key: 'k',
      bucket: 'media',
      secret_key: '',
      has_secret_key: true,
      secret_key_hint: 'WXYZ'
    }

    await state.received?.onUpdateSettings(next)

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalledTimes(1))
    const saved = vi.mocked(workspaceService.update).mock.calls[0][0].settings?.file_manager
    expect(saved).toMatchObject({ endpoint: 'https://s3.example.com', access_key: 'k', bucket: 'media' })
    expect(saved).not.toHaveProperty('secret_key')
    expect(saved).not.toHaveProperty('has_secret_key')
    expect(saved).not.toHaveProperty('secret_key_hint')
  })

  it('saves a typed secret', async () => {
    state.role = 'owner'
    renderPage()

    await state.received?.onUpdateSettings({
      endpoint: 'https://s3.example.com',
      access_key: 'k',
      bucket: 'media',
      secret_key: 'new-secret'
    })

    expect(vi.mocked(workspaceService.update).mock.calls[0][0].settings?.file_manager).toMatchObject({
      secret_key: 'new-secret'
    })
  })

  it('opens a folder in the URL, leaving the previewed file behind', () => {
    renderPage()
    state.received?.onPathChange?.('docs/')
    expect(state.navigate).toHaveBeenCalledWith(
      expect.objectContaining({ params: { workspaceId: 'ws1' }, search: { path: 'docs/' } })
    )
    state.received?.onPathChange?.('')
    expect(state.navigate).toHaveBeenLastCalledWith(
      expect.objectContaining({ search: { path: undefined } })
    )
  })

  it('replaces the previewed file in the URL, keeping the folder', () => {
    renderPage()
    state.received?.onFileChange?.('docs/b.png')

    const call = state.navigate.mock.calls[0][0]
    expect(call.replace).toBe(true)
    expect(call.search({ path: 'docs/' })).toEqual({ path: 'docs/', file: 'docs/b.png' })
  })
})
