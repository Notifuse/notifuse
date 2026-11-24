import type React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import enUS from 'antd/locale/en_US'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { OAuthAuthorizePage } from './OAuthAuthorizePage'
import { api } from '../services/api/client'
import { ApiError } from '../services/api/errors'
import type { OAuthAuthorizationRequest } from '../services/api/mcp'
import {
  createEmptyPermissions,
  createFullPermissions,
  grantUnenforcedPermissions
} from '../services/api/permissions'

// The page seen from the API client: only the client is mocked, so oauthApi runs for real and the
// tests read the exact paths and bodies.
vi.mock('../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))

const { search, navigate, auth, licence } = vi.hoisted(() => ({
  search: { request: 'req1' as string | undefined },
  navigate: vi.fn(),
  auth: { user: undefined as { id: string; email: string } | undefined },
  licence: { rbac: true }
}))

// Outside a router: a link shows where it would go, its search included.
vi.mock('@tanstack/react-router', () => ({
  useSearch: () => search,
  useNavigate: () => navigate,
  Link: ({
    children,
    to,
    search: linkSearch,
    className
  }: {
    children: React.ReactNode
    to: string
    search?: Record<string, string>
    className?: string
  }) => (
    <a href={linkSearch ? `${to}?${new URLSearchParams(linkSearch).toString()}` : to} className={className}>
      {children}
    </a>
  )
}))
vi.mock('../contexts/AuthContext', () => ({ useAuth: () => auth }))
vi.mock('../hooks/useLicense', () => ({
  useLicense: () => ({ has: (feature: string) => feature !== 'rbac' || licence.rbac })
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const GET_REQUEST = '/api/oauth.getAuthorizationRequest?request_id=req1'
const KEY = 'eyJhbGciOiJIUzI1NiIs.pasted-key.signature'

const claude = (overrides: Partial<OAuthAuthorizationRequest> = {}): OAuthAuthorizationRequest => ({
  request_id: 'req1',
  client_id: 'https://claude.ai/oauth/mcp-oauth-client-metadata',
  client_name: 'Claude',
  client_host: 'claude.ai',
  known_client: true,
  redirect_host: 'claude.ai',
  loopback_redirect: false,
  expires_at: '2026-11-23T12:10:00Z',
  ...overrides
})

const owner = (overrides: Partial<OAuthAuthorizationRequest> = {}) =>
  claude({
    owned_workspaces: [
      { id: 'ws1', name: 'Acme', mcp_disabled: false },
      { id: 'ws2', name: 'Acme Staging', mcp_disabled: true },
      { id: 'ws3', name: 'Beta', mcp_disabled: false }
    ],
    ...overrides
  })

const refusal = (status: number, code?: string) =>
  new ApiError(code ?? 'refused', status, code ? { error: code, message: 'server prose' } : { error: 'refused' })

const assign = vi.fn()
const originalLocation = window.location

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider locale={enUS}>
          <App>
            <OAuthAuthorizePage />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )

const loadWith = (view: OAuthAuthorizationRequest) => vi.mocked(api.get).mockResolvedValueOnce(view)

const pasteKey = (value = KEY) => fireEvent.change(screen.getByLabelText('API key'), { target: { value } })

// Down then up: the field ignores a held key's repeats until the key comes back up.
const pressEnter = () => {
  const field = screen.getByLabelText('API key')
  fireEvent.keyDown(field, { key: 'Enter', code: 'Enter', keyCode: 13 })
  fireEvent.keyUp(field, { key: 'Enter', code: 'Enter', keyCode: 13 })
}

const clickConnect = () => fireEvent.click(screen.getByRole('button', { name: 'Connect' }))

beforeEach(() => {
  vi.mocked(api.get).mockReset()
  vi.mocked(api.post).mockReset()
  navigate.mockReset()
  assign.mockReset()
  search.request = 'req1'
  auth.user = undefined
  licence.rbac = true
  window.API_ENDPOINT = 'https://api.acme.io'
  Object.defineProperty(window, 'location', { value: { ...originalLocation, assign }, writable: true })
})

afterEach(() => {
  Object.defineProperty(window, 'location', { value: originalLocation, writable: true })
})

describe('OAuthAuthorizePage: the request', () => {
  it('shows an anonymous visitor the application, where it returns, and the key field', async () => {
    loadWith(claude())
    renderPage()

    expect(await screen.findByRole('heading', { name: 'Connect Claude to Notifuse' })).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith(GET_REQUEST)
    const field = screen.getByLabelText('API key')
    expect(field).toHaveAttribute('type', 'password')
    expect(field).toHaveAttribute('autocomplete', 'off')
    expect(screen.queryByText('Create a key for this assistant')).not.toBeInTheDocument()
    // A known client: no caution, no "Published at".
    expect(screen.queryByText('Unknown')).not.toBeInTheDocument()
    expect(screen.queryByText('Published at')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Owner? Sign in to create a key here' })).toHaveAttribute(
      'href',
      `/console/signin?${new URLSearchParams({ return_to: '/console/oauth/authorize?request=req1' }).toString()}`
    )
  })

  it('keeps Cancel and Connect together, apart from the sign-in link, so they wrap as one group', async () => {
    loadWith(claude())
    renderPage()
    const link = await screen.findByRole('link', { name: 'Owner? Sign in to create a key here' })

    const actions = screen.getByRole('button', { name: 'Cancel' }).closest('[data-oauth-footer-actions]')
    expect(actions).not.toBeNull()
    expect(actions).toContainElement(screen.getByRole('button', { name: 'Connect' }))
    expect(actions).not.toContainElement(link)
    expect(link.closest('[data-oauth-footer-link]')?.parentElement).toBe(actions?.parentElement)
  })

  it('keeps Cancel and Create key and connect together in the owner shortcut', async () => {
    auth.user = { id: 'u1', email: 'owner@acme.io' }
    loadWith(owner())
    renderPage()
    const paste = await screen.findByRole('button', { name: 'Paste an existing key' })

    const actions = screen.getByRole('button', { name: 'Cancel' }).closest('[data-oauth-footer-actions]')
    expect(actions).toContainElement(screen.getByRole('button', { name: 'Create key and connect' }))
    expect(actions).not.toContainElement(paste)
  })

  it('offers an owner the shortcut, in the first workspace with MCP on', async () => {
    auth.user = { id: 'u1', email: 'owner@acme.io' }
    loadWith(owner({ owned_workspaces: [{ id: 'ws2', name: 'Acme Staging', mcp_disabled: true }, { id: 'ws1', name: 'Acme', mcp_disabled: false }] }))
    renderPage()

    expect(await screen.findByText('Create a key for this assistant')).toBeInTheDocument()
    expect(screen.getByText('owner@acme.io')).toBeInTheDocument()
    expect(screen.queryByText('Owner? Sign in to create a key here')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Key name')).toHaveValue('claude')
    expect(screen.getByText('@api.acme.io')).toBeInTheDocument()
    expect(screen.getByRole('combobox').closest('.ant-select')).toHaveTextContent(/^Acme$/)
  })

  it('warns about an application Notifuse does not know', async () => {
    loadWith(claude({ client_name: 'n8n local', client_host: 'tools.example.dev', known_client: false, client_id: 'https://tools.example.dev/client.json' }))
    renderPage()

    expect(await screen.findByRole('heading', { name: 'Connect "n8n local" to Notifuse' })).toBeInTheDocument()
    expect(screen.getByText(/Notifuse does not know this application/)).toBeInTheDocument()
    expect(screen.getByText('Unknown')).toBeInTheDocument()
    expect(screen.getByText('Published at')).toBeInTheDocument()
    expect(screen.getByText('tools.example.dev')).toBeInTheDocument()
    expect(screen.queryByText(/returns to a program on your own computer/)).not.toBeInTheDocument()
  })

  it('warns when the application returns to a program on this computer', async () => {
    loadWith(claude({ redirect_host: '127.0.0.1:33418', loopback_redirect: true }))
    renderPage()

    expect(await screen.findByText(/returns to a program on your own computer/)).toBeInTheDocument()
    expect(screen.getByText('127.0.0.1:33418')).toBeInTheDocument()
  })

  it('says a request that is gone cannot continue', async () => {
    vi.mocked(api.get).mockRejectedValueOnce(refusal(410, 'oauth_request_unavailable'))
    renderPage()

    expect(await screen.findByRole('heading', { name: "This connection request can't continue" })).toBeInTheDocument()
    expect(screen.getByText('Go back to your application and connect again.')).toBeInTheDocument()
  })

  it('asks nothing of the API without a request ID', () => {
    search.request = undefined
    renderPage()

    expect(screen.getByRole('heading', { name: "This connection request can't continue" })).toBeInTheDocument()
    expect(api.get).not.toHaveBeenCalled()
  })

  it('says MCP is off on the server', async () => {
    vi.mocked(api.get).mockRejectedValueOnce(refusal(404))
    renderPage()

    expect(await screen.findByRole('heading', { name: 'MCP is not available on this server' })).toBeInTheDocument()
  })
})

describe('OAuthAuthorizePage: pasting a key', () => {
  it('approves with the key, follows the redirect and forgets the key', async () => {
    loadWith(claude())
    vi.mocked(api.post).mockResolvedValueOnce({ redirect_url: 'https://claude.ai/api/mcp/auth_callback?code=c1&state=s1' })
    renderPage()
    await screen.findByLabelText('API key')

    expect(screen.getByRole('button', { name: 'Connect' })).toBeDisabled()
    pasteKey(`  ${KEY}  `)
    clickConnect()

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/oauth.approveWithKey', { request_id: 'req1', api_key: KEY })
    )
    await waitFor(() => expect(assign).toHaveBeenCalledWith('https://claude.ai/api/mcp/auth_callback?code=c1&state=s1'))
    expect(await screen.findByText('Connected. Returning you to Claude…')).toBeInTheDocument()
    expect(document.body.innerHTML).not.toContain(KEY)
  })

  it.each([
    ['text that is not a key, or a deleted key', 400, 'oauth_not_an_api_key', /This is not a Notifuse API key, or the key was deleted/],
    ["a key whose workspace has MCP off", 409, 'oauth_workspace_mcp_disabled', /MCP is switched off in this key's workspace/],
    ['too many attempts', 429, 'rate_limited', /Too many attempts from this address/]
  ])('explains %s under the field and keeps the request', async (_label, status, code, text) => {
    loadWith(claude())
    vi.mocked(api.post).mockRejectedValueOnce(refusal(status, code))
    renderPage()
    await screen.findByLabelText('API key')

    pasteKey()
    clickConnect()

    expect(await screen.findByRole('alert')).toHaveTextContent(text)
    expect(screen.getByLabelText('API key')).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByRole('button', { name: 'Connect' })).toBeInTheDocument()
    expect(assign).not.toHaveBeenCalled()
  })

  it('ends the page when the request is gone', async () => {
    loadWith(claude())
    vi.mocked(api.post).mockRejectedValueOnce(refusal(410, 'oauth_request_unavailable'))
    renderPage()
    await screen.findByLabelText('API key')

    pasteKey()
    clickConnect()

    expect(await screen.findByRole('heading', { name: "This connection request can't continue" })).toBeInTheDocument()
    expect(screen.getByText('Go back to Claude and connect again.')).toBeInTheDocument()
  })

  it('ends the page when the request was started in another browser', async () => {
    loadWith(claude())
    vi.mocked(api.post).mockRejectedValueOnce(refusal(403, 'oauth_browser_mismatch'))
    renderPage()
    await screen.findByLabelText('API key')

    pasteKey()
    clickConnect()

    expect(await screen.findByRole('heading', { name: 'Finish connecting in the browser you started in' })).toBeInTheDocument()
    expect(screen.getByText('Go back to Claude and connect again from this browser.')).toBeInTheDocument()
  })

  it('sends one call when Enter is pressed twice in the key field', async () => {
    loadWith(claude())
    vi.mocked(api.post).mockReturnValueOnce(new Promise(() => {}))
    renderPage()
    await screen.findByLabelText('API key')

    pasteKey()
    pressEnter()
    pressEnter()

    await waitFor(() => expect(api.post).toHaveBeenCalledWith('/api/oauth.approveWithKey', { request_id: 'req1', api_key: KEY }))
    expect(api.post).toHaveBeenCalledTimes(1)
  })

  it('sends nothing on Enter while the cancel is pending', async () => {
    loadWith(claude())
    vi.mocked(api.post).mockReturnValueOnce(new Promise(() => {}))
    renderPage()
    await screen.findByLabelText('API key')

    pasteKey()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(api.post).toHaveBeenCalledWith('/api/oauth.deny', { request_id: 'req1' }))
    pressEnter()

    expect(api.post).toHaveBeenCalledTimes(1)
    expect(api.post).not.toHaveBeenCalledWith('/api/oauth.approveWithKey', expect.anything())
  })

  it('never opens a redirect that is not http or https', async () => {
    loadWith(claude())
    vi.mocked(api.post).mockResolvedValueOnce({ redirect_url: 'javascript:alert(document.cookie)' })
    renderPage()
    await screen.findByLabelText('API key')

    pasteKey()
    clickConnect()

    expect(await screen.findByRole('heading', { name: "Notifuse did not open the application's address" })).toBeInTheDocument()
    expect(assign).not.toHaveBeenCalled()
  })
})

describe('OAuthAuthorizePage: cancelling', () => {
  it('denies the request and returns to the application', async () => {
    loadWith(claude())
    vi.mocked(api.post).mockResolvedValueOnce({ redirect_url: 'https://claude.ai/api/mcp/auth_callback?error=access_denied&state=s1' })
    renderPage()
    await screen.findByLabelText('API key')

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => expect(api.post).toHaveBeenCalledWith('/api/oauth.deny', { request_id: 'req1' }))
    await waitFor(() => expect(assign).toHaveBeenCalledWith('https://claude.ai/api/mcp/auth_callback?error=access_denied&state=s1'))
    expect(await screen.findByText('Cancelled. Returning you to Claude…')).toBeInTheDocument()
  })

  it('refuses a javascript: redirect on cancel too', async () => {
    loadWith(claude())
    vi.mocked(api.post).mockResolvedValueOnce({ redirect_url: 'javascript:alert(1)' })
    renderPage()
    await screen.findByLabelText('API key')

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(await screen.findByRole('heading', { name: "Notifuse did not open the application's address" })).toBeInTheDocument()
    expect(assign).not.toHaveBeenCalled()
  })
})

describe('OAuthAuthorizePage: the owner shortcut', () => {
  beforeEach(() => {
    auth.user = { id: 'u1', email: 'owner@acme.io' }
  })

  const openWorkspaces = () => fireEvent.mouseDown(screen.getByRole('combobox'))
  const confirmDialog = () => document.querySelector('.ant-modal-confirm') as HTMLElement

  it('lists a workspace with MCP off without letting it be picked, and creates the key in the one picked', async () => {
    loadWith(owner())
    vi.mocked(api.post).mockResolvedValueOnce({ redirect_url: 'https://claude.ai/api/mcp/auth_callback?code=c1' })
    renderPage()
    await screen.findByText('Create a key for this assistant')

    expect(screen.getByText(/A workspace with MCP off can't be picked/)).toBeInTheDocument()
    openWorkspaces()
    const staging = await screen.findByText('Acme Staging')
    expect(staging.closest('.ant-select-item-option')).toHaveClass('ant-select-item-option-disabled')
    fireEvent.click(staging)
    fireEvent.click(screen.getByText('Beta'))

    fireEvent.click(screen.getByRole('button', { name: 'Create key and connect' }))
    // Full access: the person confirms, with the application's address in view.
    const dialog = await waitFor(() => {
      const element = confirmDialog()
      expect(element).toBeTruthy()
      return element
    })
    expect(dialog).toHaveTextContent('Give Claude full access?')
    expect(dialog).toHaveTextContent('Beta')
    expect(dialog).toHaveTextContent('claude.ai')
    expect(api.post).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create key and connect' }))

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/oauth.createKeyAndApprove', {
        request_id: 'req1',
        workspace_id: 'ws3',
        name: 'claude',
        permissions: createFullPermissions()
      })
    )
    await waitFor(() => expect(assign).toHaveBeenCalledWith('https://claude.ai/api/mcp/auth_callback?code=c1'))
  })

  it('sends the permissions set with the drawer editor, without the full-access confirmation', async () => {
    loadWith(owner())
    vi.mocked(api.post).mockResolvedValueOnce({ redirect_url: 'https://claude.ai/cb?code=c1' })
    renderPage()
    await screen.findByText('Create a key for this assistant')

    fireEvent.change(screen.getByLabelText('Key name'), { target: { value: 'Claude Marketing!' } })
    fireEvent.click(screen.getByRole('button', { name: 'Revoke all' }))
    fireEvent.click(screen.getByRole('button', { name: 'Create key and connect' }))

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/oauth.createKeyAndApprove', {
        request_id: 'req1',
        workspace_id: 'ws1',
        name: 'claude_marketing',
        permissions: grantUnenforcedPermissions(createEmptyPermissions())
      })
    )
    expect(confirmDialog()).toBeNull()
  })

  it('without the rbac licence, allows full access only and says what it means', async () => {
    licence.rbac = false
    loadWith(owner())
    renderPage()
    await screen.findByText('Create a key for this assistant')

    expect(screen.getByText('The assistant gets every right its key has.')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Without a licence that includes custom permissions, every key has full access: the assistant can send emails, change templates and delete contacts.'
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Revoke all' })).toBeDisabled()
  })

  it('shows a taken name on the name field', async () => {
    loadWith(owner())
    vi.mocked(api.post).mockRejectedValueOnce(refusal(409, 'oauth_key_name_taken'))
    renderPage()
    await screen.findByText('Create a key for this assistant')

    fireEvent.click(screen.getByRole('button', { name: 'Revoke all' }))
    fireEvent.click(screen.getByRole('button', { name: 'Create key and connect' }))

    const error = await screen.findByText(/An API key already uses this name/)
    expect(error.closest('.ant-form-item')).toContainElement(screen.getByLabelText('Key name'))
    expect(assign).not.toHaveBeenCalled()
  })

  it('cannot create a key when every workspace has MCP off', async () => {
    loadWith(owner({ owned_workspaces: [{ id: 'ws2', name: 'Acme Staging', mcp_disabled: true }] }))
    renderPage()
    await screen.findByText('Create a key for this assistant')

    expect(screen.getByRole('button', { name: 'Create key and connect' })).toBeDisabled()
  })

  it('blocks key creation, and only that, when the server requires two-factor authentication', async () => {
    loadWith(owner())
    vi.mocked(api.post).mockRejectedValueOnce(refusal(403, 'two_factor_required'))
    renderPage()
    await screen.findByText('Create a key for this assistant')

    fireEvent.click(screen.getByRole('button', { name: 'Revoke all' }))
    fireEvent.click(screen.getByRole('button', { name: 'Create key and connect' }))

    expect(await screen.findByText(/requires two-factor authentication before you can create a key/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Set up two-factor authentication' })).toHaveAttribute('href', '/console/account/security')
    expect(screen.getByRole('button', { name: 'Create key and connect' })).toBeDisabled()

    fireEvent.click(screen.getAllByRole('button', { name: 'Paste an existing key' })[0])
    expect(screen.getByLabelText('API key')).toBeInTheDocument()
  })

  it('offers to sign in again when the session is dead, back to this request', async () => {
    loadWith(owner())
    vi.mocked(api.post).mockRejectedValueOnce(refusal(401))
    renderPage()
    await screen.findByText('Create a key for this assistant')

    fireEvent.click(screen.getByRole('button', { name: 'Revoke all' }))
    fireEvent.click(screen.getByRole('button', { name: 'Create key and connect' }))

    expect(await screen.findByText(/Your session has expired/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Sign in again' }))
    expect(navigate).toHaveBeenCalledWith({
      to: '/console/signin',
      search: { return_to: '/console/oauth/authorize?request=req1' }
    })
  })

  it('lets an owner paste a key instead', async () => {
    loadWith(owner())
    vi.mocked(api.post).mockResolvedValueOnce({ redirect_url: 'https://claude.ai/cb?code=c1' })
    renderPage()
    await screen.findByText('Create a key for this assistant')

    fireEvent.click(screen.getByRole('button', { name: 'Paste an existing key' }))
    pasteKey()
    clickConnect()

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/oauth.approveWithKey', { request_id: 'req1', api_key: KEY })
    )
  })
})
