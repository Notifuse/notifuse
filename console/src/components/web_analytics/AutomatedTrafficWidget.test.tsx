import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import dayjs from '../../lib/dayjs'
import type { AnalyticsQuery } from '../../services/api/analytics'

interface CapturedOption {
  series: { name: string; stack: string; data: number[] }[]
  tooltip: { formatter: (params: { axisValue: string; dataIndex: number }[]) => string }
}

const COMPARE = {
  startDay: '2026-10-29',
  endDay: '2026-10-31',
  startUtc: '2026-10-29T00:00:00Z',
  endUtc: '2026-10-31T23:59:59Z'
}

const { query, captured, context } = vi.hoisted(() => ({
  query: vi.fn(),
  captured: { option: null as CapturedOption | null },
  context: {
    workspaceId: 'ws1',
    timezone: 'UTC',
    granularity: 'day' as const,
    range: { start: null as unknown, end: null as unknown },
    compareRange: null as null | { start: unknown; end: unknown },
    resolved: {
      startDay: '2026-11-01',
      endDay: '2026-11-03',
      startUtc: '2026-11-01T00:00:00Z',
      endUtc: '2026-11-03T23:59:59Z'
    },
    resolvedCompare: null as null | typeof COMPARE,
    showComparison: false,
    filters: [] as { dimension: string; operator: string; values: string[] }[],
    toggleFilter: vi.fn()
  }
}))

vi.mock('../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

vi.mock('../../services/api/analytics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/analytics')>()),
  AnalyticsService: { create: () => ({ query }) }
}))

vi.mock('./useWebAnalytics', () => ({ useWebAnalytics: () => context }))

vi.mock('echarts-for-react', () => ({
  default: (props: { option: CapturedOption }) => {
    captured.option = props.option
    return <div data-testid="echart" />
  }
}))

import { AutomatedTrafficWidget } from './AutomatedTrafficWidget'

const SERIES = [
  { created_at_day: '2026-11-01T00:00:00Z', sessions: 100, automated_sessions: 10, automated_share: 10 },
  { created_at_day: '2026-11-02T00:00:00Z', sessions: 200, automated_sessions: 50, automated_share: 25 },
  { created_at_day: '2026-11-03T00:00:00Z', sessions: 400, automated_sessions: 260, automated_share: 65 }
]
const TOTALS = { sessions: 700, automated_sessions: 320, automated_share: 45.71 }
const PREVIOUS_TOTALS = { sessions: 500, automated_sessions: 70, automated_share: 14 }
const PAGES = [
  { landing_path: '/', automated_sessions: 300, automated_share: 60 },
  { landing_path: '/pricing', automated_sessions: 20, automated_share: 10 }
]

const isCompare = (q: AnalyticsQuery) => JSON.stringify(q.filters).includes(COMPARE.startUtc)
const queries = () => query.mock.calls.map(([q]) => q as AnalyticsQuery)

function renderWidget() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <AutomatedTrafficWidget />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  captured.option = null
  context.filters = []
  context.showComparison = false
  context.resolvedCompare = null
  context.compareRange = null
  context.range = { start: dayjs.utc('2026-11-01'), end: dayjs.utc('2026-11-03') }
  query.mockReset().mockImplementation(async (q: AnalyticsQuery) => ({
    data: q.dimensions?.includes('landing_path')
      ? PAGES
      : q.timeDimensions?.length
        ? SERIES
        : [isCompare(q) ? PREVIOUS_TOTALS : TOTALS],
    meta: { total: 0, query: '', params: [] }
  }))
})

describe('AutomatedTrafficWidget', () => {
  it('opens on the trend: the share of the period and both kinds of sessions', async () => {
    const { container } = renderWidget()
    await screen.findByTestId('echart')
    const trend = container.querySelector('[data-automated-trend]') as HTMLElement
    expect(within(trend).getByText('45.7%')).toBeInTheDocument()
    expect(within(trend).getByText('320')).toBeInTheDocument()
    expect(within(trend).getByText('380')).toBeInTheDocument()

    // The totals are one query without buckets, the chart another with them.
    const totals = queries().find((q) => !q.timeDimensions?.length && !q.dimensions?.length)
    expect(totals?.measures).toEqual(['sessions', 'automated_sessions', 'automated_share'])
    expect(queries().some((q) => q.dimensions?.includes('traffic_type'))).toBe(false)
  })

  it('stacks automated sessions under the human ones, bucket by bucket', async () => {
    renderWidget()
    await waitFor(() => expect(captured.option?.series).toHaveLength(2))
    const [automated, human] = captured.option!.series
    expect(automated.name).toBe('Automated')
    expect(automated.data).toEqual([10, 50, 260])
    expect(human.name).toBe('Human')
    expect(human.data).toEqual([90, 150, 140])
    expect(automated.stack).toBe(human.stack)

    const tooltip = captured.option!.tooltip.formatter([{ axisValue: 'Nov 3', dataIndex: 2 }])
    expect(tooltip).toContain('Nov 3')
    expect(tooltip).toContain('<b>260</b>')
    expect(tooltip).toContain('65.0%')
    expect(tooltip).toContain('<b>140</b>')
  })

  it('gives the change against the comparison period in points', async () => {
    context.showComparison = true
    context.resolvedCompare = COMPARE
    context.compareRange = { start: dayjs.utc('2026-10-29'), end: dayjs.utc('2026-10-31') }
    const { container } = renderWidget()
    const trend = await waitFor(() => {
      const node = container.querySelector('[data-automated-trend]') as HTMLElement
      expect(within(node).getByText('+31.7 pts')).toBeInTheDocument()
      return node
    })
    expect(within(trend).getByText(/^vs /)).toBeInTheDocument()
  })

  it('says what its figures leave out', async () => {
    const { container } = renderWidget()
    await screen.findByTestId('echart')
    const footer = container.querySelector('[data-panel-footer]') as HTMLElement
    expect(footer.textContent).toBe('Counted in Sessions, left out of TimeScore and Bounce Rate')
  })

  it('breaks automated sessions down by landing page, with their share of the page', async () => {
    renderWidget()
    await screen.findByTestId('echart')
    fireEvent.click(screen.getByRole('tab', { name: 'Landing pages' }))
    const row = (await screen.findByText('/pricing')).closest('tr') as HTMLElement
    expect(within(row).getByText('20')).toBeInTheDocument()
    expect(within(row).getByText('10.0%')).toBeInTheDocument()

    const breakdown = queries().find((q) => q.dimensions?.includes('landing_path'))
    expect(breakdown?.measures).toEqual(['automated_sessions', 'automated_share'])
    expect(breakdown?.order).toEqual({ automated_sessions: 'desc' })
    // A page no robot reached is left out by the engine rather than listed at 0.
    expect(breakdown?.having).toEqual([{ member: 'automated_sessions', operator: 'gt', values: ['0'] }])
  })

  it('shows no share and no change for a period without sessions', async () => {
    const empty = { sessions: 0, automated_sessions: 0, automated_share: null }
    query.mockImplementation(async (q: AnalyticsQuery) => ({
      data: q.timeDimensions?.length ? [] : [empty],
      meta: { total: 0, query: '', params: [] }
    }))
    context.showComparison = true
    context.resolvedCompare = COMPARE
    context.compareRange = { start: dayjs.utc('2026-10-29'), end: dayjs.utc('2026-10-31') }
    const { container } = renderWidget()
    await screen.findByText('No data for this period')
    const share = container.querySelector('[data-automated-share]') as HTMLElement
    expect(share.textContent).toBe('–')
    expect(screen.queryByText(/pts$/)).toBeNull()
  })

  it('gives no change against a comparison period without sessions', async () => {
    query.mockImplementation(async (q: AnalyticsQuery) => ({
      data: q.timeDimensions?.length
        ? SERIES
        : [isCompare(q) ? { sessions: 0, automated_sessions: 0, automated_share: null } : TOTALS],
      meta: { total: 0, query: '', params: [] }
    }))
    context.showComparison = true
    context.resolvedCompare = COMPARE
    context.compareRange = { start: dayjs.utc('2026-10-29'), end: dayjs.utc('2026-10-31') }
    renderWidget()
    await screen.findByText('45.7%')
    await waitFor(() => expect(queries().some(isCompare)).toBe(true))
    expect(screen.queryByText(/pts$/)).toBeNull()
  })

  it('asks the series for the two counts only', async () => {
    renderWidget()
    await screen.findByTestId('echart')
    const series = queries().find((q) => q.timeDimensions?.length)
    expect(series?.measures).toEqual(['sessions', 'automated_sessions'])
  })
})
