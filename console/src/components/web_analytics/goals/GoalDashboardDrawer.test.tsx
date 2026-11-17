import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
    availableGranularities: ['day' as const, 'week' as const],
    setGranularity: vi.fn(),
    setFilters: vi.fn(),
    customDimensionLabels: {} as Record<string, string>,
    range: { start: null as unknown, end: null as unknown },
    compareRange: null as unknown,
    resolved: {
      startDay: '2026-11-01',
      endDay: '2026-11-02',
      startUtc: '2026-11-01T00:00:00Z',
      endUtc: '2026-11-02T23:59:59Z'
    },
    resolvedCompare: null,
    showComparison: false,
    filters: [] as { dimension: string; operator: string; values: string[] }[]
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

import { GoalDashboardDrawer, type GoalDashboardDrawerProps } from './GoalDashboardDrawer'

function renderDrawer(props: Partial<GoalDashboardDrawerProps> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onNavigate = vi.fn()
  const utils = render(
    <QueryClientProvider client={client}>
      <GoalDashboardDrawer
        goalName="signup"
        goals={['signup']}
        onNavigate={onNavigate}
        onClose={() => {}}
        sessionTotals={{ current: 200, previous: 0 }}
        {...props}
      />
    </QueryClientProvider>
  )
  return { ...utils, onNavigate }
}

const kpi = (label: string) => screen.getByText(label).closest('[data-kpi-cell]') as HTMLElement
const sentSchemas = () => query.mock.calls.map(([sent]) => (sent as { schema: string }).schema)

const chartPanel = () => screen.getByTestId('echart').closest('section') as HTMLElement
const chartTitle = () => chartPanel().querySelector('[data-panel-head] h3')?.textContent

beforeEach(() => {
  captured.option = null
  context.range = { start: dayjs.utc('2026-11-01'), end: dayjs.utc('2026-11-02') }
  context.compareRange = null
  context.showComparison = false
  context.filters = []
  context.customDimensionLabels = {}
  context.setFilters.mockReset()
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
    expect(strip.querySelectorAll('[data-kpi-cell]')).toHaveLength(4)
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
      'Goals by day and hour',
      'Top Sources',
      'Top Campaigns',
      'Devices',
      'Countries',
      'Landing pages'
    ])
  })

  it('divides the goal count by the sessions it is given, with no query of its own for them', async () => {
    renderDrawer()
    const rate = kpi('Conv. Rate')
    // 12 goals over 200 sessions.
    await waitFor(() => expect(rate).toHaveTextContent('6.0%'))
    expect(rate.tagName).not.toBe('BUTTON')
    fireEvent.click(rate)
    expect(chartTitle()).toBe('Count')
    expect(sentSchemas()).not.toContain('web_sessions')
  })

  it('adds a card for a custom dimension the workspace named, and none otherwise', async () => {
    const { unmount } = renderDrawer()
    await screen.findByTestId('echart')
    expect(screen.queryByText('Product line')).toBeNull()
    unmount()

    context.customDimensionLabels = { custom_1: 'Product line' }
    renderDrawer()
    await screen.findByTestId('echart')
    expect(screen.getAllByTestId('widget').at(-1)).toHaveTextContent('Product line')
  })

  it('walks the goals of the grid with the bar buttons and J / K', async () => {
    const { onNavigate } = renderDrawer({ goals: ['a', 'signup', 'b'] })
    await screen.findByTestId('echart')
    expect(screen.getByText('2 of 3')).toBeInTheDocument()

    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNavigate).toHaveBeenLastCalledWith('b')
    fireEvent.keyDown(document.body, { key: 'k' })
    expect(onNavigate).toHaveBeenLastCalledWith('a')
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(onNavigate).toHaveBeenLastCalledWith('b')
  })

  it('stays on a goal a filter dropped from the grid, the buttons disabled', async () => {
    const { onNavigate } = renderDrawer({ goals: ['a', 'b'] })
    await screen.findByTestId('echart')
    expect(screen.queryByText(/of 2/)).toBeNull()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onNavigate).not.toHaveBeenCalled()
  })

  it('names the period in the bar, and the comparison only while comparing', async () => {
    renderDrawer()
    await screen.findByTestId('echart')
    const period = document.querySelector('[data-goal-period]') as HTMLElement
    expect(period).toHaveTextContent('Nov 1 – 2')
    expect(period).not.toHaveTextContent('vs')
  })

  it('adds the comparison period to the bar while comparing', async () => {
    context.showComparison = true
    context.compareRange = { start: dayjs.utc('2026-10-30'), end: dayjs.utc('2026-10-31') }
    renderDrawer()
    await screen.findByTestId('echart')
    expect(document.querySelector('[data-goal-period]')).toHaveTextContent('vs Oct 30 – 31')
  })

  it('lists the page filters it applies, each removable, but not the goal name', async () => {
    const device = { dimension: 'device', operator: 'equals', values: ['mobile'] }
    context.filters = [device, { dimension: 'goal_name', operator: 'equals', values: ['purchase'] }]
    renderDrawer()
    const line = document.querySelector('[data-goal-filters]') as HTMLElement
    expect(within(line).getByText('Device equals mobile')).toBeInTheDocument()
    expect(within(line).queryByText(/Goal Name/)).toBeNull()

    fireEvent.click(within(line).getByLabelText('Remove Device equals mobile'))
    expect(context.setFilters).toHaveBeenCalledWith([context.filters[1]])
  })

  it('says how to filter when no filter applies', async () => {
    renderDrawer()
    expect(
      screen.getByText('Click a row or a country to filter the whole Web Analytics section.')
    ).toBeInTheDocument()
  })

  it('renders nothing without a goal', () => {
    const { container } = renderDrawer({ goalName: null })
    expect(container).toBeEmptyDOMElement()
    expect(query).not.toHaveBeenCalled()
  })
})
