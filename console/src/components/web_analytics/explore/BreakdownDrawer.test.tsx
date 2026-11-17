import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import dayjs from '../../../lib/dayjs'
import type { AnalyticsQuery } from '../../../services/api/analytics'
import type { ExploreRow } from '../lib/exploreRows'

interface CapturedOption {
  series: { data: number[] }[]
}

const { query, captured, context } = vi.hoisted(() => ({
  query: vi.fn(),
  captured: { option: null as CapturedOption | null },
  context: {
    workspaceId: 'ws1',
    timezone: 'Europe/Paris',
    granularity: 'day' as const,
    availableGranularities: ['day' as const, 'week' as const],
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
    filters: [] as { dimension: string; operator: string; values: string[] }[],
    metricFilters: [],
    minSessions: 5,
    customDimensionLabels: {},
    dimensions: ['channel_group', 'channel']
  }
}))

vi.mock('../../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
vi.mock('../../../services/api/analytics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../services/api/analytics')>()),
  AnalyticsService: { create: () => ({ query }) }
}))
vi.mock('../useWebAnalytics', () => ({ useWebAnalytics: () => context }))
vi.mock('echarts-for-react', () => ({
  default: (props: { option: CapturedOption }) => {
    captured.option = props.option
    return <div data-testid="echart" />
  }
}))

import { BreakdownDrawer } from './BreakdownDrawer'
import { buildBreakdownQuery } from './breakdownQuery'

const LANDING_PATHS = [
  { landing_path: '/us/shop/goto/buy_iphone', sessions: 412, median_duration: 72, bounce_rate: 36.2, median_scroll: 64 },
  ...[286, 198, 174, 121, 98, 71, 52, 44, 38, 30, 25].map((sessions, index) => ({
    landing_path: `/page-${index}/`,
    sessions,
    median_duration: 60,
    bounce_rate: 40,
    median_scroll: 50 + index
  }))
]
const DEVICES = [
  { device: 'mobile', sessions: 934, median_duration: 62, bounce_rate: 40.1, median_scroll: 55 },
  { device: 'desktop', sessions: 571, median_duration: 84, bounce_rate: 35.6, median_scroll: 63 },
  { device: 'tablet', sessions: 107, median_duration: 55, bounce_rate: 42.9, median_scroll: 51 }
]
const SERIES = [
  { created_at_day: '2026-11-10T00:00:00Z', sessions: 220, median_duration: 60, bounce_rate: 30, median_scroll: 50 },
  { created_at_day: '2026-11-11T00:00:00Z', sessions: 245, median_duration: 70, bounce_rate: 45, median_scroll: 55 }
]

const ROW: ExploreRow = {
  key: 'search-paid:google-ads',
  dimensionIndex: 1,
  childrenLoaded: false,
  channel_group: 'search-paid',
  channel: 'google-ads',
  sessions: 1612,
  median_duration: 68,
  bounce_rate: 38.9,
  median_scroll: 59
}
const PARENT_FILTERS = [
  { dimension: 'channel_group', operator: 'equals' as const, values: ['search-paid'] },
  { dimension: 'channel', operator: 'equals' as const, values: ['google-ads'] }
]

let failDevice = false

const sent = () => query.mock.calls.map(([sentQuery]) => sentQuery as AnalyticsQuery)
const tableQueries = () => sent().filter((q) => q.dimensions.length > 0)

function renderDrawer(dimensions = ['landing_path', 'device'], parentFilters: typeof PARENT_FILTERS = PARENT_FILTERS) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <App>
        <BreakdownDrawer
          onClose={() => {}}
          row={ROW}
          dimensions={['channel_group', 'channel']}
          breakdownDimensions={dimensions}
          parentFilters={parentFilters}
          shareBase={7533}
          shareLabel="search-paid"
        />
      </App>
    </QueryClientProvider>
  )
}

const card = (title: string) =>
  screen.getByRole('heading', { name: new RegExp(`^${title}`) }).closest('section') as HTMLElement

beforeEach(() => {
  captured.option = null
  failDevice = false
  context.filters = []
  context.range = { start: dayjs.utc('2026-11-10'), end: dayjs.utc('2026-11-16') }
  query.mockReset().mockImplementation(async (q: AnalyticsQuery) => {
    if (q.timeDimensions?.length) return { data: SERIES, meta: { total: 0, query: '', params: [] } }
    if (q.dimensions.includes('device') && failDevice) throw new Error('boom')
    const data = q.dimensions.includes('landing_path')
      ? LANDING_PATHS
      : q.dimensions.includes('device')
        ? DEVICES
        : q.dimensions.includes('browser')
          ? [{ browser: 'Safari', sessions: 900, median_duration: 60, bounce_rate: 30, median_scroll: 50 }]
          : []
    return { data, meta: { total: 0, query: '', params: [] } }
  })
})

describe('BreakdownDrawer', () => {
  it('queries one table per dimension within the row, plus the row over the period', async () => {
    renderDrawer()
    await screen.findByText('/us/shop/goto/buy_iphone')
    await waitFor(() => expect(sent()).toHaveLength(3))

    expect(tableQueries()).toEqual([
      buildBreakdownQuery({
        dimension: 'landing_path',
        filters: PARENT_FILTERS,
        metricFilters: [],
        minSessions: 5,
        range: context.resolved,
        timezone: 'Europe/Paris'
      }),
      buildBreakdownQuery({
        dimension: 'device',
        filters: PARENT_FILTERS,
        metricFilters: [],
        minSessions: 5,
        range: context.resolved,
        timezone: 'Europe/Paris'
      })
    ])
    const [landing] = tableQueries()
    expect(landing.having).toEqual([{ member: 'sessions', operator: 'gte', values: ['5'] }])
    expect(landing.limit).toBe(100)
    expect(landing.filters).toEqual(
      expect.arrayContaining([
        { member: 'channel_group', operator: 'equals', values: ['search-paid'] },
        { member: 'channel', operator: 'equals', values: ['google-ads'] }
      ])
    )

    const series = sent().find((q) => q.timeDimensions?.length) as AnalyticsQuery
    expect(series.timeDimensions).toEqual([
      { dimension: 'created_at', granularity: 'day', dateRange: ['2026-11-10', '2026-11-16'] }
    ])
    expect(series.having).toBeUndefined()
    expect(series.dimensions).toEqual([])
  })

  it('writes long values whole, with their share of the row', async () => {
    renderDrawer()
    const value = await screen.findByText('/us/shop/goto/buy_iphone')
    expect(value.closest('.ant-table-cell-ellipsis')).toBeNull()
    const row = value.closest('tr') as HTMLElement
    expect(within(row).getByText('412')).toBeInTheDocument()
    // 412 of the row's 1,612 sessions.
    expect(within(row).getByText('25.6%')).toBeInTheDocument()
  })

  it('shows the scroll depth of each value', async () => {
    renderDrawer()
    const row = (await screen.findByText('/us/shop/goto/buy_iphone')).closest('tr') as HTMLElement
    expect(within(row).getByText('64.0%')).toBeInTheDocument()
    expect(within(card('Landing Path')).getByRole('columnheader', { name: /Scroll/ })).toBeInTheDocument()
  })

  it('adds up the page in the footer, and follows a sort', async () => {
    renderDrawer()
    await screen.findByText('/us/shop/goto/buy_iphone')
    const footer = () => card('Landing Path').querySelector('[data-breakdown-footer]')?.textContent
    // 412 + 286 + 198 + 174 + 121 + 98 + 71 + 52 + 44 + 38: the first ten of twelve.
    expect(footer()).toBe('1,494 of 1,612 sessions on this page')
    expect(card('Device').querySelector('[data-breakdown-footer]')?.textContent).toBe('1,612 of 1,612 sessions')

    // Sorted by scroll depth, highest first, the page holds other rows.
    const scroll = within(card('Landing Path')).getByRole('columnheader', { name: /Scroll/ })
    fireEvent.click(scroll)
    fireEvent.click(scroll)
    await waitFor(() => expect(footer()).toBe('1,065 of 1,612 sessions on this page'))
  })

  it('removes a dimension from its chip, with no new query', async () => {
    renderDrawer()
    await screen.findByText('mobile')
    const before = sent().length
    fireEvent.click(screen.getByLabelText('Remove Device'))
    await waitFor(() => expect(screen.queryByRole('heading', { name: /^Device/ })).toBeNull())
    expect(screen.getByRole('heading', { name: /^Landing Path/ })).toBeInTheDocument()
    expect(sent()).toHaveLength(before)
  })

  it('adds a dimension from the picker, over the drawer', async () => {
    renderDrawer()
    await screen.findByText('mobile')
    fireEvent.click(screen.getByRole('button', { name: /Add dimension/ }))
    const picker = (await screen.findByText('Breakdown by')).closest('.ant-modal') as HTMLElement
    fireEvent.click(within(picker).getByText('Browser'))
    fireEvent.click(within(picker).getByRole('button', { name: 'Apply' }))

    expect(await screen.findByText('Safari')).toBeInTheDocument()
    expect(tableQueries().map((q) => q.dimensions)).toContainEqual(['browser'])
    expect(screen.getByRole('dialog', { name: 'Breakdown of google-ads' })).toBeInTheDocument()
  })

  it('says a table could not load, retries it, and keeps the others', async () => {
    failDevice = true
    renderDrawer()
    expect(await screen.findByText('Could not load this breakdown')).toBeInTheDocument()
    expect(screen.getByText('/us/shop/goto/buy_iphone')).toBeInTheDocument()

    const deviceCalls = () => tableQueries().filter((q) => q.dimensions.includes('device')).length
    const before = deviceCalls()
    failDevice = false
    fireEvent.click(within(card('Device')).getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('mobile')).toBeInTheDocument()
    expect(deviceCalls()).toBe(before + 1)
  })

  it('plots the figure picked in the strip, and titles the chart with it', async () => {
    renderDrawer()
    await waitFor(() => expect(captured.option?.series[0].data).toEqual([220, 245]))
    const strip = document.querySelector('[data-kpi-strip]') as HTMLElement
    expect(within(strip).getByText('21.4% of search-paid')).toBeInTheDocument()

    fireEvent.click(within(strip).getByText('Bounce Rate').closest('[data-kpi-cell]') as HTMLElement)
    const chart = screen.getByTestId('echart').closest('section') as HTMLElement
    expect(chart.querySelector('[data-panel-head] h3')?.textContent).toBe('Bounce Rate')
    expect(captured.option?.series[0].data).toEqual([30, 45])
  })

  it('opens without a dimension on an empty state that offers to add one', async () => {
    renderDrawer([])
    expect(await screen.findByText(/Pick the dimensions to break this row down by/)).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /Add dimension/ })).toHaveLength(2)
    expect(tableQueries()).toHaveLength(0)
  })

  it('says where the row sits and what filters the page applies', async () => {
    renderDrawer()
    const context = document.querySelector('[data-breakdown-context]') as HTMLElement
    expect([...context.querySelectorAll('[data-path-chip]')].map((chip) => chip.textContent)).toEqual([
      'Channel Group search-paid',
      'Channel google-ads'
    ])
    expect(context).toHaveTextContent('Page filters: none')
    expect(context).toHaveTextContent('Min. sessions: 5')
  })

  it('keeps the page filters in every query, apart from the path of the row', async () => {
    const pageFilter = { dimension: 'device', operator: 'equals' as const, values: ['mobile'] }
    context.filters = [pageFilter]
    renderDrawer(['landing_path'], [pageFilter, ...PARENT_FILTERS])
    await screen.findByText('/us/shop/goto/buy_iphone')

    const box = document.querySelector('[data-breakdown-context]') as HTMLElement
    expect([...box.querySelectorAll('[data-path-chip]')].map((chip) => chip.textContent)).toEqual([
      'Channel Group search-paid',
      'Channel google-ads'
    ])
    expect(box).toHaveTextContent('Page filters: Device equals mobile')
    for (const q of sent()) {
      expect(q.filters).toContainEqual({ member: 'device', operator: 'equals', values: ['mobile'] })
    }
  })
})
