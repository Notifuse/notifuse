import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { FileManagerProps } from '../components/file_manager/interfaces'

const state = vi.hoisted(() => ({
  search: {} as Record<string, string | undefined>,
  navigate: vi.fn(),
  write: true,
  fileManager: undefined as Record<string, unknown> | undefined,
  received: undefined as FileManagerProps | undefined
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
    permissions: { templates: { read: true, write: state.write } },
    loading: false
  })
}))
vi.mock('../services/api/workspace', () => ({ workspaceService: { update: vi.fn() } }))
// The explorer is tested on its own; here it only shows what the page gave it.
vi.mock('../components/file_manager/fileManager', () => ({
  FileManager: (props: FileManagerProps) => {
    state.received = props
    return <div data-testid="file-manager" />
  }
}))

import { FileManagerPage } from './FileManagerPage'

describe('FileManagerPage', () => {
  beforeEach(() => {
    state.search = {}
    state.navigate.mockReset()
    state.write = true
    state.fileManager = { endpoint: 'https://s3.example.com', bucket: 'media', access_key: 'k' }
    state.received = undefined
  })

  it('gives the explorer the workspace, its storage and the folder and file of the URL', () => {
    state.search = { path: 'docs/', file: 'docs/a.png' }
    render(<FileManagerPage />)

    expect(screen.getByTestId('file-manager')).toBeTruthy()
    expect(state.received).toMatchObject({
      workspaceId: 'ws1',
      controlledPath: 'docs/',
      controlledFile: 'docs/a.png',
      readOnly: false,
      settings: { endpoint: 'https://s3.example.com', bucket: 'media' }
    })
  })

  it('is read-only without the right to write templates', () => {
    state.write = false
    render(<FileManagerPage />)
    expect(state.received?.readOnly).toBe(true)
  })

  it('opens a folder in the URL, leaving the previewed file behind', () => {
    render(<FileManagerPage />)
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
    render(<FileManagerPage />)
    state.received?.onFileChange?.('docs/b.png')

    const call = state.navigate.mock.calls[0][0]
    expect(call.replace).toBe(true)
    expect(call.search({ path: 'docs/' })).toEqual({ path: 'docs/', file: 'docs/b.png' })
  })
})
