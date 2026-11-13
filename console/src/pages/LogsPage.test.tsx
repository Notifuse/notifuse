import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useEffect } from 'react'
import type { LogsSearch } from '../components/logs/logsSearch'

interface Rights {
  message_history?: boolean
  webhook_events?: boolean
  webhook_subscriptions?: boolean
}

const state = vi.hoisted(() => ({
  navigate: vi.fn(),
  search: {} as Record<string, unknown>,
  rights: {} as Record<string, boolean>,
  permissionsLoading: false,
  access: { canRead: true, isRoot: false, loading: false },
  mounts: { messages: 0 }
}))

// services/api/client imports the router, which imports every page; stub it so the
// real router never loads under the mock below.
vi.mock('../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() }
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => state.navigate,
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => state.search
}))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'u1', email: 'ann@example.com' },
    workspaces: [{ id: 'ws1', settings: { timezone: 'UTC' } }]
  }),
  useWorkspacePermissions: () => ({
    loading: state.permissionsLoading,
    permissions: state.permissionsLoading
      ? null
      : {
          message_history: { read: !!state.rights.message_history, write: false },
          webhook_events: { read: !!state.rights.webhook_events, write: false },
          webhook_subscriptions: { read: !!state.rights.webhook_subscriptions, write: false }
        }
  })
}))
vi.mock('../hooks/useAuditLogAccess', () => ({
  useAuditLogAccess: () => state.access
}))
vi.mock('../components/messages/MessageHistoryTab', () => ({
  MessageHistoryTab: function FakeMessages() {
    useEffect(() => {
      state.mounts.messages += 1
    }, [])
    return <div>messages-tab</div>
  }
}))
vi.mock('../components/webhooks/InboundWebhookEventsTab', () => ({
  InboundWebhookEventsTab: () => <div>inbound-tab</div>
}))
vi.mock('../components/logs/OutgoingWebhooksTab', () => ({
  OutgoingWebhooksTab: () => <div>outgoing-tab</div>
}))
vi.mock('../components/logs/AuditLogTab', () => ({
  AuditLogTab: () => <div>audit-tab</div>
}))

import { LogsPage } from './LogsPage'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const tree = () => (
  <QueryClientProvider client={new QueryClient()}>
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <LogsPage />
        </App>
      </ConfigProvider>
    </I18nProvider>
  </QueryClientProvider>
)

const renderPage = (search: LogsSearch = {}) => {
  state.search = search
  const view = render(tree())
  return {
    ...view,
    rerenderWith: (next: LogsSearch) => ((state.search = next), view.rerender(tree()))
  }
}

const allRights = (): Rights => ({
  message_history: true,
  webhook_events: true,
  webhook_subscriptions: true
})

const tabNames = () => screen.queryAllByRole('tab').map((tab) => tab.textContent)

describe('LogsPage', () => {
  beforeEach(() => {
    state.navigate.mockClear()
    state.rights = allRights() as Record<string, boolean>
    state.permissionsLoading = false
    state.access = { canRead: true, isRoot: false, loading: false }
    state.mounts.messages = 0
  })

  it('names the page and its four tabs', () => {
    renderPage()
    expect(
      screen.getByText('Everything sent, received and changed in this workspace')
    ).toBeInTheDocument()
    expect(tabNames()).toEqual(['Messages', 'Incoming webhooks', 'Outgoing webhooks', 'Audit log'])
  })

  it.each([
    ['message_history', 'Messages'],
    ['webhook_events', 'Incoming webhooks'],
    ['webhook_subscriptions', 'Outgoing webhooks']
  ])('shows only the tab of %s when it is the only right', (right, tab) => {
    state.rights = { [right]: true }
    state.access = { canRead: false, isRoot: false, loading: false }
    renderPage()
    expect(tabNames()).toEqual([tab])
  })

  it('shows only the audit tab with the audit read alone', () => {
    state.rights = {}
    renderPage()
    expect(tabNames()).toEqual(['Audit log'])
    expect(screen.getByText('audit-tab')).toBeInTheDocument()
  })

  it('opens the first permitted tab without touching a link that has no tab', () => {
    renderPage({ automation_id: 'a1' })
    expect(screen.getByText('messages-tab')).toBeInTheDocument()
    expect(state.navigate).not.toHaveBeenCalled()
  })

  it('replaces a tab the member may not open by the first one, alone', () => {
    state.access = { canRead: false, isRoot: false, loading: false }
    renderPage({ tab: 'audit', id: 'entry-1' })
    expect(state.navigate).toHaveBeenCalledTimes(1)
    expect(state.navigate).toHaveBeenCalledWith(
      expect.objectContaining({ search: { tab: 'messages' }, replace: true })
    )
    // Mounted now, the messages tab would read the audit entry's id from the URL as a message id.
    expect(screen.queryByText('messages-tab')).not.toBeInTheDocument()
    expect(state.mounts.messages).toBe(0)
  })

  it('replaces a tab that does not exist by the first one, alone', () => {
    renderPage({ tab: 'settings', id: 'x' } as unknown as LogsSearch)
    expect(state.navigate).toHaveBeenCalledWith(
      expect.objectContaining({ search: { tab: 'messages' }, replace: true })
    )
    expect(state.mounts.messages).toBe(0)
  })

  it('keeps an audit link while the access loads, then opens it', () => {
    state.access = { canRead: false, isRoot: false, loading: true }
    const view = renderPage({ tab: 'audit', id: 'entry-1' })
    expect(state.navigate).not.toHaveBeenCalled()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()

    state.access = { canRead: true, isRoot: false, loading: false }
    view.rerenderWith({ tab: 'audit', id: 'entry-1' })
    expect(screen.getByText('audit-tab')).toBeInTheDocument()
    expect(state.navigate).not.toHaveBeenCalled()
  })

  it('waits for the workspace rights before choosing a tab', () => {
    state.permissionsLoading = true
    renderPage({ tab: 'incoming-webhooks' })
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(state.navigate).not.toHaveBeenCalled()
  })

  it('keeps only the tab when changing tab, as a new history entry', () => {
    renderPage({ tab: 'messages', is_failed: true })
    fireEvent.click(screen.getByRole('tab', { name: 'Outgoing webhooks' }))
    expect(state.navigate).toHaveBeenCalledWith(
      expect.objectContaining({ search: { tab: 'outgoing-webhooks' } })
    )
    expect(state.navigate.mock.calls[0][0]).not.toHaveProperty('replace')
  })

  it('remounts a tab left and opened again, so it reads the URL afresh', () => {
    const view = renderPage({ tab: 'messages', is_failed: true })
    expect(state.mounts.messages).toBe(1)
    view.rerenderWith({ tab: 'audit' })
    expect(screen.queryByText('messages-tab')).not.toBeInTheDocument()
    view.rerenderWith({ tab: 'messages' })
    expect(state.mounts.messages).toBe(2)
  })

  it('says so without any logs right', () => {
    state.rights = {}
    state.access = { canRead: false, isRoot: false, loading: false }
    renderPage()
    expect(screen.getByText("You don't have access to logs")).toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
  })
})
