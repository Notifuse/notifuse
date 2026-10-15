import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { PendingInvitations } from './PendingInvitations'
import { workspaceService } from '../../services/api/workspace'

vi.mock('../../services/api/workspace', () => ({
  workspaceService: {
    pendingInvitations: vi.fn(),
    joinInvitation: vi.fn()
  }
}))

const mockRefreshWorkspaces = vi.fn()
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { id: 'user-1' }, refreshWorkspaces: mockRefreshWorkspaces })
}))

const mockNavigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => mockNavigate
}))

const mockMessage = { success: vi.fn(), error: vi.fn() }

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const acme = {
  id: 'inv-1',
  workspace_id: 'wsacme',
  workspace_name: 'Acme',
  inviter_name: 'Jane Owner',
  expires_at: '2099-01-01T00:00:00Z'
}

let queryClient: QueryClient

const renderComponent = () =>
  render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <PendingInvitations />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )

describe('PendingInvitations', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    mockRefreshWorkspaces.mockResolvedValue(undefined)
    vi.spyOn(App, 'useApp').mockReturnValue({
      message: mockMessage,
      notification: {},
      modal: {}
    } as unknown as ReturnType<typeof App.useApp>)
  })

  it('renders nothing when there is nothing to join', async () => {
    vi.mocked(workspaceService.pendingInvitations).mockResolvedValue({ invitations: [] })
    renderComponent()

    // Wait for the answer, so "nothing" is not just "not loaded yet".
    await waitFor(() =>
      expect(queryClient.getQueryState(['pendingInvitations', 'user-1'])?.status).toBe('success')
    )
    expect(screen.queryByText('Pending invitations')).not.toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('lists each invitation with its workspace and who sent it', async () => {
    vi.mocked(workspaceService.pendingInvitations).mockResolvedValue({
      invitations: [acme, { ...acme, id: 'inv-2', workspace_id: 'wsbeta', workspace_name: 'Beta', inviter_name: undefined }]
    })
    renderComponent()

    expect(await screen.findByText('Acme')).toBeInTheDocument()
    expect(screen.getByText('Invited by Jane Owner')).toBeInTheDocument()
    expect(screen.getByText('Beta')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Join' })).toHaveLength(2)
  })

  it('joins, refreshes the workspace list and opens the workspace', async () => {
    vi.mocked(workspaceService.pendingInvitations).mockResolvedValue({ invitations: [acme] })
    vi.mocked(workspaceService.joinInvitation).mockResolvedValue({
      status: 'success',
      workspace_id: 'wsacme'
    })
    renderComponent()

    fireEvent.click(await screen.findByRole('button', { name: 'Join' }))

    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith({
        to: '/console/workspace/$workspaceId',
        params: { workspaceId: 'wsacme' }
      })
    )
    expect(workspaceService.joinInvitation).toHaveBeenCalledWith('inv-1')
    expect(mockRefreshWorkspaces).toHaveBeenCalled()
    expect(mockMessage.success).toHaveBeenCalledWith('You joined Acme')
  })

  // The join already happened server-side: a failed refresh of the workspace list must not
  // read as a failed join, nor open a workspace the console does not know yet.
  it('reports a join that succeeded even when refreshing the workspace list fails', async () => {
    vi.mocked(workspaceService.pendingInvitations).mockResolvedValue({ invitations: [acme] })
    vi.mocked(workspaceService.joinInvitation).mockResolvedValue({
      status: 'success',
      workspace_id: 'wsacme'
    })
    mockRefreshWorkspaces.mockRejectedValue(new Error('network down'))
    renderComponent()

    fireEvent.click(await screen.findByRole('button', { name: 'Join' }))

    await waitFor(() => expect(mockMessage.success).toHaveBeenCalledWith('You joined Acme'))
    await waitFor(() => expect(mockRefreshWorkspaces).toHaveBeenCalled())
    expect(mockMessage.error).not.toHaveBeenCalled()
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  // A refusal usually means the card is stale (joined elsewhere, revoked, expired): the list
  // is fetched again instead of leaving a Join button that can only fail.
  it('refreshes the list after a refused join', async () => {
    vi.mocked(workspaceService.pendingInvitations).mockResolvedValue({ invitations: [acme] })
    vi.mocked(workspaceService.joinInvitation).mockRejectedValue(
      new Error('Invitation not found or no longer valid')
    )
    renderComponent()

    fireEvent.click(await screen.findByRole('button', { name: 'Join' }))

    await waitFor(() => expect(workspaceService.pendingInvitations).toHaveBeenCalledTimes(2))
  })

  it("shows the server's reason when the join is refused, and stays put", async () => {
    vi.mocked(workspaceService.pendingInvitations).mockResolvedValue({ invitations: [acme] })
    vi.mocked(workspaceService.joinInvitation).mockRejectedValue(
      new Error('Invitation not found or no longer valid')
    )
    renderComponent()

    fireEvent.click(await screen.findByRole('button', { name: 'Join' }))

    await waitFor(() =>
      expect(mockMessage.error).toHaveBeenCalledWith('Invitation not found or no longer valid')
    )
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(mockRefreshWorkspaces).not.toHaveBeenCalled()
  })
})
