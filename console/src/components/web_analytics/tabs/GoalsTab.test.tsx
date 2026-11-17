import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { AnalyticsQuery } from '../../../services/api/analytics'

const { query, context } = vi.hoisted(() => ({
  query: vi.fn(),
  context: {
    workspaceId: 'ws1',
    timezone: 'UTC',
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

// The card and the drawer have their own suites; here they only stand for what the grid renders.
vi.mock('../goals/GoalCard', () => ({
  GoalCard: ({ goalName, onOpenDashboard }: { goalName: string; onOpenDashboard: () => void }) => (
    <button type="button" data-testid="goal-card" onClick={onOpenDashboard}>
      {goalName}
    </button>
  )
}))
const { drawerProps } = vi.hoisted(() => ({ drawerProps: { current: null as Record<string, unknown> | null } }))
vi.mock('../goals/GoalDashboardDrawer', () => ({
  GoalDashboardDrawer: (props: { goalName: string | null; onNavigate: (goal: string) => void }) => {
    drawerProps.current = props
    return props.goalName ? (
      <div data-testid="drawer">
        {props.goalName}
        <button type="button" onClick={() => props.onNavigate('purchase')}>
          next
        </button>
      </div>
    ) : null
  }
}))

import { GoalsTab } from './GoalsTab'

function renderTab() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <GoalsTab />
    </QueryClientProvider>
  )
}

function answer(goals: Record<string, unknown>[]) {
  query.mockReset().mockImplementation(async (q: AnalyticsQuery) => ({
    data: q.schema === 'web_goals' ? goals : [{ sessions: 100 }],
    meta: { total: 0, query: '', params: [] }
  }))
}

beforeEach(() => answer([]))

describe('GoalsTab', () => {
  it('renders one card per named goal', async () => {
    answer([
      { goal_name: 'signup', goals: 5, sum_goal_value: 0, median_goal_value: 0 },
      { goal_name: 'purchase', goals: 2, sum_goal_value: 80, median_goal_value: 40 },
      { goal_name: '', goals: 1, sum_goal_value: 0, median_goal_value: 0 }
    ])
    renderTab()
    const cards = await screen.findAllByTestId('goal-card')
    expect(cards.map((card) => card.textContent)).toEqual(['signup', 'purchase'])
  })

  it('opens the dashboard of the goal whose card asks for it', async () => {
    answer([{ goal_name: 'signup', goals: 5, sum_goal_value: 0, median_goal_value: 0 }])
    renderTab()
    fireEvent.click(await screen.findByTestId('goal-card'))
    expect(screen.getByTestId('drawer')).toHaveTextContent('signup')
  })

  it('hands the drawer the goals in grid order and the sessions it divides by, then lets it walk them', async () => {
    answer([
      { goal_name: 'signup', goals: 5, sum_goal_value: 0, median_goal_value: 0 },
      { goal_name: 'purchase', goals: 2, sum_goal_value: 80, median_goal_value: 40 }
    ])
    renderTab()
    expect(screen.queryByTestId('drawer')).toBeNull()
    fireEvent.click((await screen.findAllByTestId('goal-card'))[0])

    expect(drawerProps.current).toMatchObject({
      goalName: 'signup',
      goals: ['signup', 'purchase'],
      sessionTotals: { current: 100, previous: 0 }
    })
    fireEvent.click(screen.getByRole('button', { name: 'next' }))
    expect(screen.getByTestId('drawer')).toHaveTextContent('purchase')
    // The grid asked for the sessions once, for the cards; the drawer reuses them.
    expect(query.mock.calls.filter(([q]) => (q as AnalyticsQuery).schema === 'web_sessions')).toHaveLength(1)
  })

  it('shows the empty state inside a panel when no goal was tracked', async () => {
    renderTab()
    const empty = await screen.findByText('No goals tracked yet')
    expect(empty.closest('section')).not.toBeNull()
  })
})
