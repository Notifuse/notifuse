import '../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'

const state = vi.hoisted(() => ({
  permissions: {} as Record<string, { read: boolean; write: boolean }>
}))

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => ({}),
  useNavigate: () => vi.fn()
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace', integrations: [], settings: {} }] }),
  useWorkspacePermissions: () => ({ permissions: state.permissions })
}))

vi.mock('@fortawesome/react-fontawesome', () => ({ FontAwesomeIcon: () => <span /> }))

// The drawers and the modal expose only what this file checks: the permission they get.
vi.mock('../components/broadcasts/UpsertBroadcastDrawer', () => ({
  UpsertBroadcastDrawer: ({ canReadTemplates }: { canReadTemplates: boolean }) => (
    <span data-testid="broadcast-drawer" data-can-read-templates={String(canReadTemplates)} />
  )
}))
vi.mock('../components/broadcasts/ChangeVariationSenderModal', () => ({
  ChangeVariationSenderModal: ({ canReadTemplates }: { canReadTemplates: boolean }) => (
    <span data-testid="sender-modal" data-can-read-templates={String(canReadTemplates)} />
  )
}))
vi.mock('../components/broadcasts/SendOrScheduleModal', () => ({ SendOrScheduleModal: () => null }))
vi.mock('../components/broadcasts/BroadcastStats', () => ({ BroadcastStats: () => null }))
vi.mock('../components/broadcasts/BroadcastLinkStats', () => ({ BroadcastLinkStats: () => null }))
vi.mock('../components/broadcasts/SendingProgress', () => ({ SendingProgress: () => null }))
vi.mock('../components/broadcasts/BroadcastAnalyticsLinks', () => ({
  BroadcastAnalyticsLinks: () => null
}))
vi.mock('../components/templates/TemplatePreviewDrawer', () => ({ default: () => null }))
vi.mock('../components/templates/SendTemplateModal', () => ({ default: () => null }))
vi.mock('../components/templates/SendTestEmailButton', () => ({ default: () => null }))

vi.mock('../services/api/broadcast', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api/broadcast')>()
  return {
    ...actual,
    broadcastApi: {
      // Without Templates read the server lists the broadcast without its templates.
      list: vi.fn(() =>
        Promise.resolve({
          broadcasts: [
            {
              id: 'bc1',
              workspace_id: 'ws1',
              name: 'Spring sale',
              channel_type: 'email',
              status: 'paused',
              audience: { list: 'list1', segments: [], exclude_unsubscribed: true },
              schedule: { is_scheduled: false, use_recipient_timezone: false },
              test_settings: {
                enabled: false,
                sample_percentage: 0,
                auto_send_winner: false,
                variations: [{ variation_name: 'default', template_id: 'tpl-spring' }]
              },
              test_phase_recipient_count: 0,
              winner_phase_recipient_count: 0,
              created_at: '2026-01-01T00:00:00Z',
              updated_at: '2026-01-01T00:00:00Z'
            }
          ],
          total_count: 1
        })
      ),
      getTestResults: vi.fn()
    }
  }
})
vi.mock('../services/api/list', () => ({
  listsApi: { list: vi.fn(() => Promise.resolve({ lists: [] })) }
}))
vi.mock('../services/api/segment', () => ({
  listSegments: vi.fn(() => Promise.resolve({ segments: [] }))
}))
vi.mock('../services/api/task', () => ({
  taskApi: { findByBroadcastId: vi.fn(() => Promise.resolve(null)) }
}))
vi.mock('../services/api/messages_history', () => ({
  getBroadcastVariationStats: vi.fn(() => Promise.resolve(null))
}))

import { BroadcastsPage } from './BroadcastsPage'

const renderPage = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <App>
          <BroadcastsPage />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

const flags = (testId: string) =>
  screen.getAllByTestId(testId).map((el) => el.getAttribute('data-can-read-templates'))

// Without Templates read, templates.list and templates.get answer 403, and the broadcasts come
// without their templates: a variation shows its template's ID rather than "Untitled", and the
// drawers and the sender modal lock what they cannot read.
describe('BroadcastsPage without Templates read', () => {
  beforeEach(() => {
    state.permissions = { broadcasts: { read: true, write: true } }
  })

  it("names each variation's template by its ID and passes the permission on", async () => {
    state.permissions.templates = { read: false, write: false }
    renderPage()

    expect(await screen.findByText('tpl-spring')).toBeInTheDocument()
    expect(screen.queryByText('Untitled')).toBeNull()
    expect(new Set(flags('broadcast-drawer'))).toEqual(new Set(['false']))
    expect(flags('sender-modal')).toEqual(['false'])
  })

  it('keeps "Untitled" for a missing template with Templates read', async () => {
    state.permissions.templates = { read: true, write: false }
    renderPage()

    expect(await screen.findByText('Untitled')).toBeInTheDocument()
    expect(screen.queryByText('tpl-spring')).toBeNull()
    expect(new Set(flags('broadcast-drawer'))).toEqual(new Set(['true']))
    expect(flags('sender-modal')).toEqual(['true'])
  })
})
