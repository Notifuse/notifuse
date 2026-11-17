import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { AnalyticsQuery } from '../../../services/api/analytics'
import type { MetricTotals } from '../lib/types'
import type { GoalMetricKey } from './GoalCard'

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
vi.mock('../useWebAnalytics', () => ({ useWebAnalytics: () => context }))
vi.mock('echarts-for-react', () => ({
  default: (props: { option: CapturedOption }) => {
    captured.option = props.option
    return <div data-testid="echart" />
  }
}))

import { GoalCard } from './GoalCard'

const totals = (current: number, changePercent = 0): MetricTotals => ({
  current,
  previous: 0,
  changePercent
})

const TOTALS: Record<GoalMetricKey, MetricTotals> = {
  goals: totals(12, 20),
  conversion_rate: totals(3.5),
  sum_goal_value: totals(250),
  median_goal_value: totals(0)
}

function renderCard(onOpenDashboard = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const utils = render(
    <QueryClientProvider client={client}>
      <GoalCard goalName="signup" totals={TOTALS} onOpenDashboard={onOpenDashboard} />
    </QueryClientProvider>
  )
  const cell = (label: string) => screen.getByText(label).closest<HTMLElement>('[data-kpi-cell]')!
  return { ...utils, onOpenDashboard, cell }
}

beforeEach(() => {
  captured.option = null
  context.showComparison = false
  query.mockReset().mockImplementation(async (q: AnalyticsQuery) => ({
    data: q.timeDimensions?.length
      ? [
          { goal_at_day: '2026-11-01T00:00:00Z', goals: 5, sum_goal_value: 100, median_goal_value: 50 },
          { goal_at_day: '2026-11-02T00:00:00Z', goals: 7, sum_goal_value: 150, median_goal_value: 75 }
        ]
      : [],
    meta: { total: 0, query: '', params: [] }
  }))
})

describe('GoalCard', () => {
  it('titles its panel with the goal and shows the four figures as a strip', () => {
    const { container } = renderCard()
    const head = container.querySelector('section [data-panel-head]') as HTMLElement
    expect(within(head).getByTitle('signup')).toHaveTextContent('signup')
    expect(container.querySelectorAll('[data-kpi-cell]')).toHaveLength(4)
  })

  it('plots the selected figure in the mini chart', async () => {
    const { cell } = renderCard()
    await waitFor(() => expect(captured.option?.series[0].data).toEqual([5, 7]))
    expect(cell('Count')).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(cell('Value'))
    expect(cell('Value')).toHaveAttribute('aria-pressed', 'true')
    expect(captured.option?.series[0].data).toEqual([100, 150])
  })

  it('selects the conversion rate, which has no chart of its own', async () => {
    const { cell } = renderCard()
    await screen.findByTestId('echart')
    fireEvent.click(cell('Conv. Rate'))
    expect(cell('Conv. Rate')).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText('Select another metric to view chart')).toBeInTheDocument()
    expect(cell('Conv. Rate').querySelector('.anticon-info-circle')).not.toBeNull()
  })

  it('opens the goal dashboard from the eye button', () => {
    const { onOpenDashboard } = renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'View full dashboard' }))
    expect(onOpenDashboard).toHaveBeenCalled()
  })

  it('writes "-" for a currency figure of zero', () => {
    const { cell } = renderCard()
    expect(within(cell('Median')).getByText('-')).toBeInTheDocument()
  })

  it('shows the change of a figure when comparing', () => {
    context.showComparison = true
    const { cell } = renderCard()
    expect(within(cell('Count')).getByText('20%')).toHaveClass('text-success-fg')
  })
})
