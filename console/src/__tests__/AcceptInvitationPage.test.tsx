import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { AcceptInvitationPage } from '../pages/AcceptInvitationPage'
import { workspaceService } from '../services/api/workspace'

// Signing out ends the account's sessions on every device, so the page may do it only for a
// valid link addressed to someone else — never for a dead link, such as one whose invitation
// was already joined from the workspace list after signing in with SSO.

const mockSignout = vi.fn()
const mockAuth: { user: { email: string } | null } = { user: null }
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ signin: vi.fn(), signout: mockSignout, user: mockAuth.user })
}))

vi.mock('../services/api/workspace', () => ({
  workspaceService: {
    verifyInvitationToken: vi.fn(),
    acceptInvitation: vi.fn()
  }
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useSearch: () => ({ token: 'invitation-token' })
}))

vi.mock('../layouts/MainLayout', () => ({
  MainLayout: ({ children }: { children: React.ReactNode }) => <div>{children}</div>
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const validFor = (email: string) =>
  ({
    status: 'success',
    valid: true,
    invitation: { id: 'inv-1', workspace_id: 'wsacme', email },
    workspace: { id: 'wsacme', name: 'Acme' }
  }) as unknown as Awaited<ReturnType<typeof workspaceService.verifyInvitationToken>>

const renderPage = () =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <AcceptInvitationPage />
      </App>
    </I18nProvider>
  )

describe('AcceptInvitationPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuth.user = null
  })

  it('signs nobody out for a link that no longer works', async () => {
    mockAuth.user = { email: 'jane@example.com' }
    vi.mocked(workspaceService.verifyInvitationToken).mockRejectedValue(
      new Error('Invitation not found')
    )
    renderPage()

    expect(await screen.findByText('Invalid Invitation')).toBeInTheDocument()
    expect(mockSignout).not.toHaveBeenCalled()
  })

  it('signs out another account before a valid invitation is accepted', async () => {
    mockAuth.user = { email: 'bob@example.com' }
    vi.mocked(workspaceService.verifyInvitationToken).mockResolvedValue(validFor('Jane@Example.com'))
    renderPage()

    await waitFor(() => expect(mockSignout).toHaveBeenCalledTimes(1))
  })

  it('keeps the session of the account the invitation is addressed to', async () => {
    mockAuth.user = { email: 'jane@example.com' }
    vi.mocked(workspaceService.verifyInvitationToken).mockResolvedValue(validFor('Jane@Example.com'))
    renderPage()

    await waitFor(() => expect(workspaceService.verifyInvitationToken).toHaveBeenCalled())
    expect(await screen.findByRole('button', { name: 'Accept Invitation' })).toBeInTheDocument()
    expect(mockSignout).not.toHaveBeenCalled()
  })
})
