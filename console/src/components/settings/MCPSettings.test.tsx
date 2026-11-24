import type React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import enUS from 'antd/locale/en_US'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MCPSettings } from './MCPSettings'
import type { Workspace, WorkspaceMember } from '../../services/api/types'
import type { OAuthGrant } from '../../services/api/mcp'
import { api } from '../../services/api/client'
import { ApiError } from '../../services/api/errors'
import { copyToClipboard } from '../../lib/copyToClipboard'

// The MCP page seen from the API client: the client is the only service mocked, so mcpApi runs
// for real and the tests read the exact paths and bodies.
vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))
vi.mock('../../lib/copyToClipboard', () => ({ copyToClipboard: vi.fn() }))
const { licence } = vi.hoisted(() => ({ licence: { rbac: true } }))
vi.mock('../../hooks/useLicense', () => ({
  useLicense: () => ({ has: (feature: string) => feature !== 'rbac' || licence.rbac })
}))
// The page renders outside a router: a link shows the path it would open.
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    params,
    className
  }: {
    children: React.ReactNode
    to: string
    params: { workspaceId: string; section: string }
    className?: string
  }) => (
    <a href={to.replace('$workspaceId', params.workspaceId).replace('$section', params.section)} className={className}>
      {children}
    </a>
  )
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const LIST_GRANTS = '/api/oauth.listWorkspaceGrants?workspace_id=ws1'

const workspaceWith = (mcp?: { disabled: boolean }): Workspace => ({
  id: 'ws1',
  name: 'Acme',
  settings: {
    timezone: 'UTC',
    email_tracking_enabled: true,
    default_language: 'en',
    languages: ['en'],
    ...(mcp ? { mcp } : {})
  },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
})

const apiKey = (userId: string, name: string, permissions: WorkspaceMember['permissions']): WorkspaceMember => ({
  user_id: userId,
  workspace_id: 'ws1',
  role: 'member',
  email: `${name}@api.acme.io`,
  type: 'api_key',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  permissions
})

const grant = (overrides: Partial<OAuthGrant>): OAuthGrant => ({
  id: 'g1',
  api_key_user_id: 'key-claude',
  api_key_email: 'claude-marketing@api.acme.io',
  workspace_id: 'ws1',
  client_id: 'https://claude.ai/oauth/mcp-oauth-client-metadata',
  client_name: 'Claude',
  resource: 'https://notifuse.acme.io/mcp',
  created_via: 'key',
  created_at: '2026-11-14T10:00:00Z',
  // Read relative to the runner's clock.
  last_used_at: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
  status: 'active',
  ...overrides
})

const members: WorkspaceMember[] = [
  apiKey('key-claude', 'claude-marketing', { contacts: { read: true, write: false } }),
  apiKey('key-n8n', 'n8n-local', null as unknown as WorkspaceMember['permissions'])
]

let grants: OAuthGrant[]

beforeEach(() => {
  vi.clearAllMocks()
  window.MCP_ENABLED = true
  window.MCP_DISABLED_REASON = ''
  window.MCP_URL = 'https://notifuse.acme.io/mcp'
  window.MCP_OAUTH_UNAVAILABLE_REASON = ''
  window.API_ENDPOINT = 'https://notifuse.acme.io'
  grants = []
  licence.rbac = true
  vi.mocked(copyToClipboard).mockResolvedValue(true)
  vi.mocked(api.get).mockImplementation((async (path: string) =>
    path === LIST_GRANTS ? { grants } : {}) as typeof api.get)
  vi.mocked(api.post).mockImplementation((async (path: string, body: { disabled?: boolean }) => {
    if (path === '/api/workspaces.setMCPSettings') return { mcp: { disabled: body.disabled } }
    return { success: true }
  }) as typeof api.post)
})

const renderPage = ({ isOwner = true, mcp }: { isOwner?: boolean; mcp?: { disabled: boolean } } = {}) => {
  let rerender: (ui: React.ReactElement) => void = () => {}
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const tree = (workspace: Workspace) => (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider locale={enUS}>
          <App>
            <MCPSettings
              workspace={workspace}
              members={members}
              isOwner={isOwner}
              onWorkspaceUpdate={onWorkspaceUpdate}
            />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
  // Mirrors WorkspaceSettingsPage: the update swaps the workspace prop.
  const onWorkspaceUpdate = vi.fn(async (workspace: Workspace) => rerender(tree(workspace)))
  const utils = render(tree(workspaceWith(mcp)))
  rerender = utils.rerender
  return { ...utils, onWorkspaceUpdate }
}

const block = (name: string) => document.querySelector(`[data-mcp-block="${name}"]`) as HTMLElement
const mcpSwitch = () => screen.getByRole('switch', { name: 'Allow MCP connections' })
const rowOf = (text: string) => within(block('connections')).getByText(text).closest('tr') as HTMLElement

describe('MCP settings: access', () => {
  it('lets an owner turn MCP off, then shows the workspace it got back', async () => {
    const { onWorkspaceUpdate } = renderPage()
    expect(mcpSwitch()).toBeChecked()
    expect(mcpSwitch()).not.toBeDisabled()
    expect(screen.getByText('When off, no assistant reaches this workspace. Nothing is revoked.')).toBeInTheDocument()

    fireEvent.click(mcpSwitch())

    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/workspaces.setMCPSettings', { workspace_id: 'ws1', disabled: true })
    )
    await waitFor(() => expect(onWorkspaceUpdate).toHaveBeenCalledTimes(1))
    expect(onWorkspaceUpdate.mock.calls[0][0].settings.mcp).toEqual({ disabled: true })
    await waitFor(() => expect(mcpSwitch()).not.toBeChecked())
    expect(
      screen.getByText('Turning it back on restores existing connections: nothing is revoked.')
    ).toBeInTheDocument()
    expect(screen.getByText('Calls to this workspace answer "not found" while the switch is off.')).toBeInTheDocument()
  })

  it('turns MCP back on for an owner', async () => {
    renderPage({ mcp: { disabled: true } })
    expect(mcpSwitch()).not.toBeChecked()
    expect(screen.getByText('When off, no assistant reaches this workspace, whatever key it uses.')).toBeInTheDocument()

    fireEvent.click(mcpSwitch())
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/workspaces.setMCPSettings', { workspace_id: 'ws1', disabled: false })
    )
    await waitFor(() => expect(mcpSwitch()).toBeChecked())
  })

  it('keeps the switch where it was when the server refuses the change', async () => {
    vi.mocked(api.post).mockRejectedValue(new ApiError('Only owners can change MCP access', 403))
    const { onWorkspaceUpdate } = renderPage()

    fireEvent.click(mcpSwitch())
    expect(await screen.findByText('Only owners can change MCP access')).toBeInTheDocument()
    expect(onWorkspaceUpdate).not.toHaveBeenCalled()
    expect(mcpSwitch()).toBeChecked()
  })

  it('shows a member the switch read-only and no connections', () => {
    renderPage({ isOwner: false })
    expect(mcpSwitch()).toBeDisabled()
    expect(screen.getByText('Only an owner can change this.')).toBeInTheDocument()
    expect(block('connections')).toBeNull()
    expect(api.get).not.toHaveBeenCalled()
    // The rest of the page is theirs too.
    expect(within(block('access')).getByText('https://notifuse.acme.io/mcp')).toBeInTheDocument()
    expect(block('connect')).not.toBeNull()
  })

  it('tells a member an owner turned MCP off', () => {
    renderPage({ isOwner: false, mcp: { disabled: true } })
    expect(mcpSwitch()).not.toBeChecked()
    expect(
      screen.getByText(
        'An owner turned off MCP for this workspace. Connected assistants get no answer until it is turned back on.'
      )
    ).toBeInTheDocument()
  })

  it('copies the MCP server URL', async () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Copy the MCP server URL' }))
    await waitFor(() => expect(copyToClipboard).toHaveBeenCalledWith('https://notifuse.acme.io/mcp'))
    expect(await screen.findByText('Copied')).toBeInTheDocument()
  })

  it('says the connection page is available on an https endpoint without a path', () => {
    renderPage()
    expect(within(block('access')).getByText('Available')).toBeInTheDocument()
    expect(within(block('access')).queryByText('Unavailable')).toBeNull()
  })

  it('says why the connection page is unavailable', () => {
    window.MCP_OAUTH_UNAVAILABLE_REASON = 'API_ENDPOINT is not https'
    window.API_ENDPOINT = 'http://notifuse.acme.io'
    renderPage()
    const access = within(block('access'))
    expect(access.getByText('Unavailable')).toBeInTheDocument()
    expect(access.getByText('claude.ai and ChatGPT need it.')).toBeInTheDocument()
    expect(access.getByText('http://notifuse.acme.io')).toBeInTheDocument()
    expect(access.getByText('Claude Code and Cursor can still connect with the key in a header.')).toBeInTheDocument()
  })

  it('shows only a note when MCP is turned off on the instance', () => {
    window.MCP_ENABLED = false
    window.MCP_DISABLED_REASON = 'instance'
    renderPage()
    expect(screen.getByText('MCP_ENABLED=true')).toBeInTheDocument()
    expect(screen.queryByText('MCP is not available on the demo.')).toBeNull()
    expect(screen.queryByRole('switch')).toBeNull()
    expect(block('connections')).toBeNull()
    expect(api.get).not.toHaveBeenCalled()
  })

  it('does not point at MCP_ENABLED on a demo deployment, where it changes nothing', () => {
    window.MCP_ENABLED = false
    window.MCP_DISABLED_REASON = 'demo'
    renderPage()
    expect(screen.getByText('MCP is not available on the demo.')).toBeInTheDocument()
    expect(screen.queryByText('MCP_ENABLED=true')).toBeNull()
    expect(screen.queryByRole('switch')).toBeNull()
    expect(api.get).not.toHaveBeenCalled()
  })
})

describe('MCP settings: connecting an assistant', () => {
  it('warns that the assistant gets every right of its key, and links to the keys', () => {
    renderPage()
    const connect = within(block('connect'))
    expect(connect.getByText('The assistant gets every right its key has.')).toBeInTheDocument()
    const links = connect.getAllByRole('link', { name: 'Team & API keys' })
    expect(links[0]).toHaveAttribute('href', '/console/workspace/ws1/settings/team')
  })

  it('with custom permissions licensed, warns only about keys with full access', () => {
    renderPage()
    const connect = within(block('connect'))
    expect(
      connect.getByText('A key with full access lets the assistant send emails, change templates and delete contacts.')
    ).toBeInTheDocument()
    expect(connect.queryByText(/Without a licence/)).toBeNull()
  })

  it('without custom permissions licensed, says every key has full access', () => {
    licence.rbac = false
    renderPage()
    const connect = within(block('connect'))
    expect(
      connect.getByText(
        'Without a licence that includes custom permissions, every key has full access: the assistant can send emails, change templates and delete contacts.'
      )
    ).toBeInTheDocument()
    expect(connect.queryByText(/^A key with full access/)).toBeNull()
  })

  it('writes the steps as an ordered list, one item per step', () => {
    renderPage()
    const steps = document.querySelector('[data-mcp-steps]') as HTMLElement
    expect(steps.tagName).toBe('OL')
    expect(steps).toHaveClass('list-decimal')
    expect(within(steps).getAllByRole('listitem')).toHaveLength(3)
  })

  it('writes the steps of each client with the MCP URL', () => {
    renderPage()
    const steps = () => (document.querySelector('[data-mcp-steps]') as HTMLElement).textContent
    expect(steps()).toContain('In claude.ai, open the connector settings and add a custom connector.')
    expect(steps()).toContain('Paste https://notifuse.acme.io/mcp as its URL')
    // claude.ai and ChatGPT connect through the connection page: no header command under them.
    expect(within(block('connect')).queryByText(/claude mcp add/)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: 'ChatGPT' }))
    expect(steps()).toContain('Paste the key you created for ChatGPT.')
    expect(within(block('connect')).queryByText(/claude mcp add/)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: 'Claude Code' }))
    expect(
      within(block('connect')).getByText(/claude mcp add --transport http notifuse https:\/\/notifuse\.acme\.io\/mcp/)
    ).toBeInTheDocument()
    expect(within(block('connect')).queryByText(/instead/)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: 'Cursor' }))
    expect(steps()).toContain('.cursor/mcp.json')
    expect(within(block('connect')).getByText(/"mcpServers"/)).toBeInTheDocument()
  })

  it('copies the command with the URL', async () => {
    renderPage()
    fireEvent.click(screen.getByRole('tab', { name: 'Claude Code' }))
    fireEvent.click(screen.getByRole('button', { name: 'Copy the command' }))
    await waitFor(() =>
      expect(copyToClipboard).toHaveBeenCalledWith(
        'claude mcp add --transport http notifuse https://notifuse.acme.io/mcp \\\n  --header "Authorization: Bearer YOUR_API_KEY"'
      )
    )
  })
})

describe('MCP settings: connections', () => {
  it('lists the live connections with their key, application, access and dates', async () => {
    grants = [
      grant({}),
      grant({
        id: 'g2',
        api_key_user_id: 'key-n8n',
        api_key_email: 'n8n-local@api.acme.io',
        client_id: 'https://tools.example.dev/client.json',
        client_name: 'n8n local',
        created_at: '2026-10-28T10:00:00Z',
        last_used_at: undefined
      }),
      grant({ id: 'g3', client_name: 'Old ChatGPT', status: 'revoked', revoked_at: '2026-11-15T10:00:00Z' })
    ]
    renderPage()

    expect(await within(block('connections')).findByText('n8n local')).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith(LIST_GRANTS)
    expect(within(block('connections')).queryByText('Old ChatGPT')).toBeNull()

    const claude = within(rowOf('Claude'))
    expect(claude.getByText('claude-marketing')).toBeInTheDocument()
    expect(claude.getByText('claude.ai')).toBeInTheDocument()
    expect(claude.getByText('Nov 14, 2026')).toBeInTheDocument()
    expect(claude.getByText('2 hours ago')).toBeInTheDocument()
    expect(claude.queryByText('Unknown')).toBeNull()
    expect(claude.getByText(/Contacts/)).toBeInTheDocument()

    const n8n = within(rowOf('n8n local'))
    expect(n8n.getByText('n8n-local')).toBeInTheDocument()
    expect(n8n.getByText('Unknown')).toBeInTheDocument()
    expect(n8n.getByText('tools.example.dev')).toBeInTheDocument()
    expect(n8n.getByText('Never')).toBeInTheDocument()
  })

  it('revokes a connection once confirmed, then lists them again', async () => {
    grants = [grant({})]
    renderPage()
    fireEvent.click(await within(block('connections')).findByRole('button', { name: 'Revoke' }))

    const dialog = await screen.findByRole('dialog')
    expect(
      within(dialog).getByText(
        'Claude loses the access it has through the key claude-marketing. The key keeps working.'
      )
    ).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()

    grants = []
    fireEvent.click(within(dialog).getByRole('button', { name: 'Revoke' }))
    await waitFor(() => expect(api.post).toHaveBeenCalledWith('/api/oauth.revokeGrant', { grant_id: 'g1' }))
    expect(await within(block('connections')).findByText('No connections yet')).toBeInTheDocument()
    expect(vi.mocked(api.get).mock.calls.filter(([path]) => path === LIST_GRANTS)).toHaveLength(2)
  })

  it('does not revoke when the confirmation is cancelled', async () => {
    grants = [grant({})]
    renderPage()
    fireEvent.click(await within(block('connections')).findByRole('button', { name: 'Revoke' }))
    fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.post).not.toHaveBeenCalled()
  })

  it('says so when the connection was already gone', async () => {
    grants = [grant({})]
    vi.mocked(api.post).mockRejectedValue(new ApiError('oauth_grant_not_found', 404))
    renderPage()
    fireEvent.click(await within(block('connections')).findByRole('button', { name: 'Revoke' }))
    fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Revoke' }))
    expect(await screen.findByText('This connection no longer exists')).toBeInTheDocument()
  })

  it('handles the server refusing the list', async () => {
    vi.mocked(api.get).mockRejectedValue(new ApiError('user is not an owner of the workspace', 403))
    renderPage()
    expect(await within(block('connections')).findByText('Only owners can see the connections.')).toBeInTheDocument()
    expect(within(block('connections')).queryByRole('table')).toBeNull()
  })
})
