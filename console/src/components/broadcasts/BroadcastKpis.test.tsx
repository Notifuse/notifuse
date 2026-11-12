import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { BroadcastKpis } from './BroadcastKpis'
import { runBroadcastQuery } from './broadcastQueryClient'
import { broadcastApi } from '../../services/api/broadcast'
import type { Workspace } from '../../services/api/types'
import type { UserPermissions } from '../../services/api/workspace'
import type { AnalyticsQuery } from '../../services/api/analytics'

vi.mock('./broadcastQueryClient', () => ({ runBroadcastQuery: vi.fn() }))
vi.mock('../../services/api/broadcast', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/broadcast')>()
  return { ...actual, broadcastApi: { ...actual.broadcastApi, list: vi.fn() } }
})

const workspace = (web = true) =>
  ({ id: 'ws', settings: { timezone: 'UTC', web_analytics: { enabled: web } } }) as unknown as Workspace
const permissions = (over: Partial<UserPermissions> = {}) =>
  ({
    message_history: { read: true, write: false },
    web_analytics: { read: true, write: false },
    ...over
  }) as UserPermissions

let current: Record<string, number> = {}
let previous: Record<string, number> = {}

const renderKpis = (ws = workspace(), perms: UserPermissions | null = permissions()) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <App>
        <BroadcastKpis workspace={ws} permissions={perms} />
      </App>
    </QueryClientProvider>
  )

describe('BroadcastKpis', () => {
  beforeEach(() => {
    current = {
      count_sent: 1000,
      count_opened: 300,
      count_opens_tracked: 1000,
      count_clicked: 50,
      count_clicks_tracked: 1000,
      count_unsubscribed: 5,
      count_bounced: 20
    }
    previous = { ...current, count_opened: 280, count_clicked: 60 }
    let call = 0
    vi.mocked(runBroadcastQuery).mockReset().mockImplementation(async (query: AnalyticsQuery) => {
      if (query.schema === 'web_sessions') {
        return { data: [{ utm_campaign: 'news', year: 2000, month: 1, day: 1, sessions: 9 }], meta: { total: 1, query: '', params: [] } }
      }
      // L4 is asked before L5.
      return { data: [call++ === 0 ? current : previous], meta: { total: 1, query: '', params: [] } }
    })
    vi.mocked(broadcastApi.list).mockReset().mockResolvedValue({ broadcasts: [], total_count: 0 })
  })

  it('shows rates and point deltas with their tone', async () => {
    renderKpis()
    expect(await screen.findByText('30.0%')).toBeInTheDocument()
    expect(screen.getByText('5.0%')).toBeInTheDocument()
    expect(screen.getByText('+2.0 pts')).toBeInTheDocument()
    expect(screen.getByText('+2.0 pts').className).toContain('text-success-fg')
    expect(screen.getByText('−1.0 pt').className).toContain('text-danger-fg')
    expect(screen.getByText('5 contacts')).toBeInTheDocument()
    expect(screen.getByText('20 addresses')).toBeInTheDocument()
  })

  it('shows "—" for a rate over nothing', async () => {
    current = { count_sent: 0 }
    renderKpis()
    await waitFor(() => expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(4))
  })

  it('has no traffic cell without web analytics, nor without the right to read it', async () => {
    const { unmount } = renderKpis(workspace(false))
    await screen.findByText('30.0%')
    expect(screen.queryByText('Website traffic')).not.toBeInTheDocument()
    unmount()
    renderKpis(workspace(true), permissions({ web_analytics: { read: false, write: false } }))
    await screen.findByText('Open rate')
    await waitFor(() => expect(screen.getAllByText(/%$/).length).toBeGreaterThan(0))
    expect(screen.queryByText('Website traffic')).not.toBeInTheDocument()
  })

  it('counts traffic as 0 without asking web sessions when no broadcast has a campaign', async () => {
    renderKpis()
    expect(await screen.findByText('Website traffic')).toBeInTheDocument()
    await waitFor(() => expect(broadcastApi.list).toHaveBeenCalled())
    expect(vi.mocked(broadcastApi.list).mock.calls[0][0]).toMatchObject({ limit: 100, offset: 0 })
    expect(vi.mocked(broadcastApi.list).mock.calls[0][0].started_after).toMatch(/Z$/)
    expect(vi.mocked(runBroadcastQuery).mock.calls.some(([q]) => q.schema === 'web_sessions')).toBe(false)
  })

  it('is hidden without message_history read', () => {
    const { container } = renderKpis(workspace(), permissions({ message_history: { read: false, write: false } }))
    expect(container.querySelector('[data-kpi-strip]')).toBeNull()
    expect(runBroadcastQuery).not.toHaveBeenCalled()
  })

  it('says it could not load when the totals fail', async () => {
    vi.mocked(runBroadcastQuery).mockRejectedValue(new Error('boom'))
    renderKpis(workspace(false))
    await waitFor(() => expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(5))
  })
})
