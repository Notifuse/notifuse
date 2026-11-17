import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import dayjs from '../../../lib/dayjs'
import type { AnalyticsQuery } from '../../../services/api/analytics'

interface CapturedOption {
  series: { data: number[] }[]
}

const { query, captured, context } = vi.hoisted(() => ({
  query: vi.fn(),
  captured: { option: null as CapturedOption | null },
  context: {
    workspaceId: 'ws1',
    timezone: 'UTC',
    granularity: 'day' as const,
    availableGranularities: ['day', 'week'],
    setGranularity: vi.fn(),
    range: { start: null as unknown, end: null as unknown },
    compareRange: null,
    resolved: {
      startDay: '2026-11-01',
      endDay: '2026-11-03',
      startUtc: '2026-11-01T00:00:00Z',
      endUtc: '2026-11-03T23:59:59Z'
    },
    resolvedCompare: null,
    showComparison: false,
    filters: [] as { dimension: string; operator: string; values: string[] }[],
    toggleFilter: vi.fn()
  }
}))

vi.mock('../../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

vi.mock('../../../services/api/analytics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../services/api/analytics')>()),
  AnalyticsService: { create: () => ({ query }) }
}))

vi.mock('../../../services/api/annotation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../services/api/annotation')>()),
  annotationService: { list: vi.fn(async () => []) }
}))

vi.mock('../useWebAnalytics', () => ({ useWebAnalytics: () => context }))

vi.mock('echarts-for-react', () => ({
  default: (props: { option: CapturedOption }) => {
    captured.option = props.option
    return <div data-testid="echart" />
  }
}))

// The widgets under the chart have their own suites; here they only take their place in the grid.
vi.mock('../DimensionTableWidget', () => ({
  DimensionTableWidget: ({ title }: { title: string }) => <div data-testid="widget">{title}</div>
}))
vi.mock('../TrafficHeatmapWidget', () => ({
  TrafficHeatmapWidget: ({ title }: { title: string }) => <div data-testid="widget">{title}</div>
}))
vi.mock('../SdkVersionWarning', () => ({ SdkVersionWarning: () => null }))

import { DashboardTab } from './DashboardTab'

const SERIES = [
  { created_at_day: '2026-11-01T00:00:00Z', sessions: 10, bounce_rate: 41, median_duration: 0, median_scroll: 0 },
  { created_at_day: '2026-11-02T00:00:00Z', sessions: 20, bounce_rate: 42, median_duration: 0, median_scroll: 0 },
  { created_at_day: '2026-11-03T00:00:00Z', sessions: 30, bounce_rate: 43, median_duration: 0, median_scroll: 0 }
]

function renderTab() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <DashboardTab />
    </QueryClientProvider>
  )
}

const chartPanel = () => screen.getByTestId('echart').closest('section') as HTMLElement
const chartTitle = () => chartPanel().querySelector('[data-panel-head] h3')?.textContent

beforeEach(() => {
  captured.option = null
  context.filters = []
  context.range = { start: dayjs.utc('2026-11-01'), end: dayjs.utc('2026-11-03') }
  query.mockReset().mockImplementation(async (q: AnalyticsQuery) => ({
    data: q.timeDimensions?.length
      ? SERIES
      : [{ sessions: 60, median_duration: 75, bounce_rate: 42, median_scroll: 50 }],
    meta: { total: 0, query: '', params: [] }
  }))
})

describe('DashboardTab', () => {
  it('shows the four session metrics as a standalone KPI strip', async () => {
    const { container } = renderTab()
    await screen.findByTestId('echart')
    const strip = container.querySelector('[data-kpi-strip]') as HTMLElement
    expect(strip.closest('section')).toBeNull()
    expect(strip.querySelectorAll('[data-kpi-cell]')).toHaveLength(4)
  })

  it('retitles the chart panel and plots the metric picked in the strip', async () => {
    renderTab()
    await waitFor(() => expect(captured.option?.series[0].data).toEqual([10, 20, 30]))
    expect(chartTitle()).toBe('Sessions')

    fireEvent.click(screen.getByText('Bounce Rate').closest('[data-kpi-cell]') as HTMLElement)

    expect(chartTitle()).toBe('Bounce Rate')
    expect(captured.option?.series[0].data).toEqual([41, 42, 43])
  })

  it('puts the granularity selector in the head of the chart panel', async () => {
    renderTab()
    await screen.findByTestId('echart')
    const head = chartPanel().querySelector('[data-panel-head]') as HTMLElement
    expect(within(head).getByRole('combobox')).toBeInTheDocument()
  })

  it('keeps every widget below the chart', async () => {
    renderTab()
    await screen.findByTestId('echart')
    expect(screen.getAllByTestId('widget').map((widget) => widget.textContent)).toEqual([
      'Top Pages',
      'Top Sources',
      'Top Campaigns',
      'Countries',
      'Traffic by day and hour',
      'Devices',
      'Goals'
    ])
  })

  it('shows an alert when the series cannot load', async () => {
    query.mockImplementation(async (q: AnalyticsQuery) => {
      if (q.timeDimensions?.length) throw new Error('boom')
      return { data: [{ sessions: 1 }], meta: { total: 0, query: '', params: [] } }
    })
    renderTab()
    expect(await screen.findByText('Could not load the dashboard')).toBeInTheDocument()
  })

  it('leaves a goal filter out of the session metrics, which web_sessions cannot group by', async () => {
    // Clicking a goal in the Goals widget filters the page on goal_name: sent to web_sessions,
    // the engine rejected the strip's and the chart's queries and the dashboard blanked.
    context.filters = [
      { dimension: 'goal_name', operator: 'equals', values: ['add_to_cart'] },
      { dimension: 'device', operator: 'equals', values: ['mobile'] }
    ]
    renderTab()
    await screen.findByTestId('echart')
    const sent = query.mock.calls.map(([q]) => q as AnalyticsQuery).filter((q) => q.schema === 'web_sessions')
    expect(sent.length).toBeGreaterThan(0)
    for (const q of sent) {
      const members = (q.filters ?? []).map((f) => f.member)
      expect(members).not.toContain('goal_name')
      expect(members).toContain('device')
    }
  })

  it('says a goal filter only narrows the Goals table, and nothing without one', async () => {
    context.filters = [{ dimension: 'goal_name', operator: 'equals', values: ['add_to_cart'] }]
    const { container, unmount } = renderTab()
    await screen.findByTestId('echart')
    // Top Pages, Campaigns, Devices and the heatmap read sessions too: the note covers them all.
    expect(container.querySelector('[data-filters-not-applied]')).toHaveTextContent(
      'Goal Name filter only applies to the Goals table'
    )
    unmount()
    context.filters = [{ dimension: 'page_path', operator: 'equals', values: ['/pricing'] }]
    const pages = renderTab()
    await screen.findByTestId('echart')
    expect(pages.container.querySelector('[data-filters-not-applied]')).toHaveTextContent(
      'Page Path filter not applied on this dashboard'
    )
    pages.unmount()
    unmount()
    context.filters = [{ dimension: 'device', operator: 'equals', values: ['mobile'] }]
    const second = renderTab()
    await screen.findByTestId('echart')
    expect(second.container.querySelector('[data-filters-not-applied]')).toBeNull()
  })
})
