import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { WorkspaceMember } from '../services/api/workspace'

const state = vi.hoisted(() => ({
  navigate: vi.fn(),
  section: 'general',
  workspaces: [{ id: 'ws1', name: 'Acme', settings: { timezone: 'UTC' } }] as unknown[],
  getMembers: vi.fn()
}))

// services/api/client imports the router, which imports every page.
vi.mock('../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn(), download: vi.fn() } }))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => state.navigate,
  useParams: () => ({ workspaceId: 'ws1', section: state.section }),
  useSearch: () => ({}),
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>
}))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'u1', email: 'ann@example.com' },
    workspaces: state.workspaces,
    refreshWorkspaces: vi.fn()
  })
}))
vi.mock('../services/api/workspace', () => ({ workspaceService: { getMembers: state.getMembers } }))
vi.mock('../components/settings/GeneralSettings', () => ({
  GeneralSettings: ({ isOwner }: { isOwner: boolean }) => <div>general owner={String(isOwner)}</div>
}))
vi.mock('../components/settings/WorkspaceMembers', () => ({
  WorkspaceMembers: ({
    loading,
    error,
    onMembersChange
  }: {
    loading: boolean
    error: boolean
    onMembersChange: () => void
  }) => (
    <div>
      team loading={String(loading)} error={String(error)}
      <button onClick={onMembersChange}>refetch</button>
    </div>
  )
}))
vi.mock('../components/settings/WebhooksSettings', () => ({
  WebhooksSettings: ({ canManage }: { canManage: boolean }) => <div>webhooks canManage={String(canManage)}</div>
}))

import { WorkspaceSettingsPage } from './WorkspaceSettingsPage'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const me = (overrides: Partial<WorkspaceMember>): WorkspaceMember => ({
  user_id: 'u1',
  workspace_id: 'ws1',
  role: 'member',
  email: 'ann@example.com',
  type: 'user',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  permissions: undefined,
  ...overrides
})

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <WorkspaceSettingsPage />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )

describe('WorkspaceSettingsPage', () => {
  beforeEach(() => {
    state.section = 'general'
    state.workspaces = [{ id: 'ws1', name: 'Acme', settings: { timezone: 'UTC' } }]
    state.navigate.mockClear()
    state.getMembers.mockReset()
  })

  it('sends an unknown section to General', async () => {
    state.section = 'nope'
    state.getMembers.mockResolvedValue({ members: [] })
    renderPage()
    await waitFor(() =>
      expect(state.navigate).toHaveBeenCalledWith(
        expect.objectContaining({ params: { workspaceId: 'ws1', section: 'general' }, replace: true })
      )
    )
  })

  it('shows a skeleton until the workspace is known', () => {
    state.workspaces = []
    state.getMembers.mockReturnValue(new Promise(() => {}))
    renderPage()
    expect(screen.getByTestId('settings-skeleton')).toBeInTheDocument()
    expect(screen.queryByText(/general owner/)).not.toBeInTheDocument()
  })

  it('shows General as a skeleton while the members load, never the read-only view first', async () => {
    let answer: (value: { members: WorkspaceMember[] }) => void = () => {}
    state.getMembers.mockReturnValue(new Promise((resolve) => (answer = resolve)))
    renderPage()
    expect(screen.getByTestId('settings-skeleton')).toBeInTheDocument()
    expect(screen.queryByText(/general owner/)).not.toBeInTheDocument()

    answer({ members: [me({ role: 'owner' })] })
    expect(await screen.findByText('general owner=true')).toBeInTheDocument()
  })

  it('lets the owner manage webhooks', async () => {
    state.section = 'webhooks'
    state.getMembers.mockResolvedValue({ members: [me({ role: 'owner' })] })
    renderPage()
    expect(await screen.findByText('webhooks canManage=true')).toBeInTheDocument()
  })

  it('lets a member with webhook subscriptions write manage webhooks', async () => {
    state.section = 'webhooks'
    state.getMembers.mockResolvedValue({
      members: [me({ permissions: { webhook_subscriptions: { read: true, write: true } } })]
    })
    renderPage()
    expect(await screen.findByText('webhooks canManage=true')).toBeInTheDocument()
  })

  it('keeps webhooks read-only for a member without that write', async () => {
    state.section = 'webhooks'
    state.getMembers.mockResolvedValue({
      members: [me({ permissions: { webhook_subscriptions: { read: true, write: false } } })]
    })
    renderPage()
    await waitFor(() => expect(state.getMembers).toHaveBeenCalled())
    expect(await screen.findByText('webhooks canManage=false')).toBeInTheDocument()
  })

  it('tells the team section when the members could not load', async () => {
    state.section = 'team'
    state.getMembers.mockRejectedValue(new Error('boom'))
    renderPage()
    expect(await screen.findByText('team loading=false error=true')).toBeInTheDocument()
  })

  it('treats a member as without write rights when the members could not load', async () => {
    state.getMembers.mockRejectedValue(new Error('boom'))
    renderPage()
    expect(await screen.findByText('general owner=false')).toBeInTheDocument()
  })

  it('keeps the section mounted while the members refetch', async () => {
    state.section = 'team'
    state.getMembers.mockResolvedValueOnce({ members: [me({ role: 'owner' })] })
    renderPage()
    expect(await screen.findByText('team loading=false error=false')).toBeInTheDocument()

    state.getMembers.mockReturnValueOnce(new Promise(() => {}))
    screen.getByRole('button', { name: 'refetch' }).click()

    await waitFor(() => expect(state.getMembers).toHaveBeenCalledTimes(2))
    expect(screen.getByText('team loading=false error=false')).toBeInTheDocument()
    expect(screen.queryByTestId('settings-skeleton')).not.toBeInTheDocument()
  })
})
