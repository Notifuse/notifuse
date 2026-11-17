import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import dayjs from '../../../lib/dayjs'
import type { AnalyticsQuery } from '../../../services/api/analytics'

const { query, context } = vi.hoisted(() => ({
  query: vi.fn(),
  context: {
    workspaceId: 'ws1',
    timezone: 'UTC',
    granularity: 'day' as const,
    availableGranularities: ['day' as const],
    setGranularity: vi.fn(),
    range: { start: null as unknown, end: null as unknown },
    compareRange: null,
    resolved: {
      startDay: '2026-11-10',
      endDay: '2026-11-16',
      startUtc: '2026-11-10T00:00:00Z',
      endUtc: '2026-11-16T23:59:59Z'
    },
    resolvedCompare: null,
    showComparison: false,
    filters: [],
    metricFilters: [],
    minSessions: 1,
    customDimensionLabels: {},
    dimensions: ['channel_group', 'channel', 'device'],
    setDimensions: vi.fn(),
    setFilters: vi.fn(),
    setMinSessions: vi.fn()
  }
}))

vi.mock('../../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
vi.mock('../../../services/api/analytics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../services/api/analytics')>()),
  AnalyticsService: { create: () => ({ query }) }
}))
vi.mock('../useWebAnalytics', () => ({ useWebAnalytics: () => context }))
vi.mock('echarts-for-react', () => ({ default: () => <div data-testid="echart" /> }))

import { ExploreTab } from './ExploreTab'

const metrics = { median_duration: 60, bounce_rate: 40, median_scroll: 50 }
const ANSWERS: Record<string, Record<string, unknown>[]> = {
  channel_group: [
    { channel_group: 'search-paid', sessions: 800, ...metrics },
    { channel_group: 'social', sessions: 200, ...metrics }
  ],
  'channel_group,channel': [{ channel_group: 'search-paid', channel: 'google-ads', sessions: 400, ...metrics }],
  '': [{ sessions: 1000, ...metrics }],
  channel: [{ channel: 'google-ads', sessions: 400, ...metrics }],
  device: [{ device: 'tablet', sessions: 300, ...metrics }]
}

const sent = () => query.mock.calls.map(([sentQuery]) => sentQuery as AnalyticsQuery)
const breakdownCalls = () => sent().filter((q) => q.limit === 100 && q.dimensions.length === 1 && q.dimensions[0] !== 'channel_group')

function renderTab() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <App>
        <ExploreTab />
      </App>
    </QueryClientProvider>
  )
}

const eyeOf = (value: string) =>
  within(screen.getByText(value).closest('tr') as HTMLElement).getByRole('button', { name: 'eye' })

const strip = () => document.querySelector('[role="dialog"] [data-kpi-strip]') as HTMLElement

beforeEach(() => {
  context.range = { start: dayjs.utc('2026-11-10'), end: dayjs.utc('2026-11-16') }
  query.mockReset().mockImplementation(async (q: AnalyticsQuery) => ({
    data: q.timeDimensions?.length ? [] : (ANSWERS[q.dimensions.join(',')] ?? []),
    meta: { total: 0, query: '', params: [] }
  }))
})

describe('ExploreTab breakdown', () => {
  it('opens the breakdown at once on the first two free dimensions, no picker first', async () => {
    renderTab()
    await screen.findByText('search-paid')
    fireEvent.click(eyeOf('search-paid'))

    const drawer = await screen.findByRole('dialog', { name: 'Breakdown of search-paid' })
    expect(screen.queryByText('Breakdown by')).toBeNull()
    expect(within(drawer).getByRole('heading', { name: /^Channel/ })).toBeInTheDocument()
    expect(within(drawer).getByRole('heading', { name: /^Device/ })).toBeInTheDocument()
    // A top-level row weighs against the period.
    expect(within(strip()).getByText('80.0% of all sessions')).toBeInTheDocument()
  })

  it('weighs a child row against its parent', async () => {
    renderTab()
    fireEvent.click(await screen.findByText('search-paid'))
    await screen.findByText('google-ads')
    fireEvent.click(eyeOf('google-ads'))

    const drawer = await screen.findByRole('dialog', { name: 'Breakdown of google-ads' })
    expect(within(strip()).getByText('50.0% of search-paid')).toBeInTheDocument()
    // Only the dimension left below the row is broken down.
    expect(within(drawer).getAllByRole('heading', { name: /^Device/ })).toHaveLength(1)
    expect(within(drawer).queryByRole('heading', { name: /^Channel\b/ })).toBeNull()
  })

  it('opens on the tables the hover prefetched, under the same query', async () => {
    renderTab()
    await screen.findByText('search-paid')
    fireEvent.mouseEnter(eyeOf('search-paid'))
    await waitFor(() => expect(breakdownCalls().map((q) => q.dimensions[0])).toEqual(['channel', 'device']))
    const prefetched = breakdownCalls()

    fireEvent.click(eyeOf('search-paid'))
    const drawer = await screen.findByRole('dialog', { name: 'Breakdown of search-paid' })
    // On screen at once: read from the prefetch's cache entry.
    expect(within(drawer).getByText('tablet')).toBeInTheDocument()
    // Any request the drawer sends for a table is the prefetch's query.
    for (const q of breakdownCalls().slice(prefetched.length)) {
      expect(prefetched).toContainEqual(q)
    }
  })
})
