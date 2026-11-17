import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import dayjs from '../../../lib/dayjs'

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
    range: { start: null as unknown, end: null as unknown },
    compareRange: null,
    resolved: {
      startDay: '2026-11-01',
      endDay: '2026-11-02',
      startUtc: '2026-11-01T00:00:00Z',
      endUtc: '2026-11-02T23:59:59Z'
    },
    resolvedCompare: null,
    showComparison: false,
    filters: []
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
vi.mock('../DimensionTableWidget', () => ({
  DimensionTableWidget: ({ title }: { title: string }) => <div data-testid="widget">{title}</div>
}))
vi.mock('../TrafficHeatmapWidget', () => ({
  TrafficHeatmapWidget: ({ title }: { title: string }) => <div data-testid="widget">{title}</div>
}))

import { GoalDashboardDrawer } from './GoalDashboardDrawer'

function renderDrawer() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <GoalDashboardDrawer open goalName="signup" onClose={() => {}} />
    </QueryClientProvider>
  )
}

const chartPanel = () => screen.getByTestId('echart').closest('section') as HTMLElement
const chartTitle = () => chartPanel().querySelector('[data-panel-head] h3')?.textContent

beforeEach(() => {
  captured.option = null
  context.range = { start: dayjs.utc('2026-11-01'), end: dayjs.utc('2026-11-02') }
  query.mockReset().mockImplementation(async (sent: { timeDimensions?: unknown[] }) =>
    sent.timeDimensions
      ? {
          data: [
            { goal_at_day: '2026-11-01T00:00:00Z', goals: 5, sum_goal_value: 100, median_goal_value: 50 },
            { goal_at_day: '2026-11-02T00:00:00Z', goals: 7, sum_goal_value: 150, median_goal_value: 75 }
          ],
          meta: { total: 0, query: '', params: [] }
        }
      : {
          // The period aggregate: the engine's median over every conversion,
          // which no combination of the daily medians above reproduces.
          data: [{ goals: 12, sum_goal_value: 250, median_goal_value: 80 }],
          meta: { total: 0, query: '', params: [] }
        }
  )
})

describe('GoalDashboardDrawer', () => {
  it('shows the goal strip above a chart panel titled with the selected figure', async () => {
    renderDrawer()
    await waitFor(() => expect(captured.option?.series[0].data).toEqual([5, 7]))
    const strip = document.querySelector('[data-kpi-strip]') as HTMLElement
    expect(strip.querySelectorAll('[data-kpi-cell]')).toHaveLength(3)
    expect(chartTitle()).toBe('Count')
    // The period total of a count is the sum of its buckets.
    expect(screen.getByText('12')).toBeInTheDocument()
  })

  it('shows the period median the goal card shows, not an average of daily medians', async () => {
    renderDrawer()
    await screen.findByTestId('echart')
    const median = screen.getByText('Median Value').closest('[data-kpi-cell]') as HTMLElement
    await waitFor(() => expect(median.textContent).toContain('80'))
    expect(median.textContent).not.toContain('62.5')
    // The strip asks for the whole period in one row, the same query a goal card runs.
    expect(query).toHaveBeenCalledWith(
      expect.objectContaining({
        schema: 'web_goals',
        dimensions: [],
        filters: expect.arrayContaining([
          expect.objectContaining({ member: 'goal_at', operator: 'inDateRange' }),
          expect.objectContaining({ member: 'goal_name', operator: 'equals', values: ['signup'] })
        ])
      }),
      'ws1'
    )
  })

  it('retitles the chart panel and plots the figure picked in the strip', async () => {
    renderDrawer()
    await screen.findByTestId('echart')
    fireEvent.click(screen.getByText('Total Value').closest('[data-kpi-cell]') as HTMLElement)
    expect(chartTitle()).toBe('Total Value')
    expect(captured.option?.series[0].data).toEqual([100, 150])
  })

  it('keeps the breakdowns of the goal under the chart', async () => {
    renderDrawer()
    await screen.findByTestId('echart')
    expect(screen.getAllByTestId('widget').map((widget) => widget.textContent)).toEqual([
      'Top Sources',
      'Top Campaigns',
      'Countries',
      'Devices',
      'Goals by day and hour'
    ])
  })
})
