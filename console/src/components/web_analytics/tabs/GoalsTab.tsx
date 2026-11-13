import { useMemo, useState } from 'react'
import { Alert, Empty, Skeleton } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../useWebAnalytics'
import { GoalCard, GoalMetricKey } from '../goals/GoalCard'
import { GoalDashboardDrawer } from '../goals/GoalDashboardDrawer'
import { GOAL_MEASURES, filtersForSchema } from '../goals/queries'
import { changePercent, toNumber } from '../lib/format'
import { buildWebQuery, mergeComparisonRows, readMeasure, useWebComparisonQuery } from '../lib/query'
import { DimensionRow, MetricTotals } from '../lib/types'

/**
 * Cap on the grid. Every card runs its own sparkline query, so an unbounded
 * list would fan out into hundreds of requests; a wall of more than a few
 * dozen cards is unreadable long before that. Goals are ordered by conversion
 * count, so anything cut off is the least used.
 */
const MAX_GOALS = 30

interface GoalSummary {
  name: string
  totals: Record<GoalMetricKey, MetricTotals>
}

/**
 * The goals grid. Goals are discovered from the data rather than declared in
 * settings: whatever the SDK reported in the period is what gets a card.
 */
export function GoalsTab() {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [selectedGoal, setSelectedGoal] = useState<string | null>(null)
  const [drawerOpen, setDrawerOpen] = useState(false)

  const goalsBase = {
    schema: 'web_goals' as const,
    measures: GOAL_MEASURES,
    dimensions: ['goal_name'],
    filters: filtersForSchema(context.filters, 'web_goals'),
    order: { goals: 'desc' as const },
    limit: MAX_GOALS,
    timezone: context.timezone
  }

  const goalsResult = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({ ...goalsBase, range: context.resolved }),
    context.showComparison && context.resolvedCompare
      ? buildWebQuery({ ...goalsBase, range: context.resolvedCompare })
      : null
  )

  // The conversion rate divides by every session of the period, not by the
  // sessions that converted, which is exactly what its tooltip warns about:
  // traffic that never intended to convert is in the denominator. The page
  // filters still apply, so a drilled-down grid compares like with like.
  const sessionsBase = {
    schema: 'web_sessions' as const,
    measures: ['sessions'],
    dimensions: [],
    filters: filtersForSchema(context.filters, 'web_sessions'),
    timezone: context.timezone
  }

  const sessionsResult = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({ ...sessionsBase, range: context.resolved }),
    context.showComparison && context.resolvedCompare
      ? buildWebQuery({ ...sessionsBase, range: context.resolvedCompare })
      : null
  )

  const currentGoalRows = goalsResult.current?.data
  const previousGoalRows = goalsResult.previous?.data
  const currentSessionRow = sessionsResult.current?.data?.[0]
  const previousSessionRow = sessionsResult.previous?.data?.[0]

  const goals: GoalSummary[] = useMemo(() => {
    const rows = mergeComparisonRows(
      currentGoalRows ?? [],
      previousGoalRows,
      'goal_name',
      GOAL_MEASURES
    )
    const currentSessions = readMeasure(currentSessionRow, 'sessions')
    const previousSessions = readMeasure(previousSessionRow, 'sessions')

    return (
      rows
        // A goal with no name is not a goal anyone can act on, and it would
        // render as a nameless card.
        .filter((row) => row.dimension_value !== '')
        .map((row) => {
          const counts = measureTotals(row, 'goals')
          const currentRate = conversionRate(counts.current, currentSessions)
          const previousRate = conversionRate(counts.previous, previousSessions)

          return {
            name: row.dimension_value,
            totals: {
              goals: counts,
              conversion_rate: {
                current: currentRate,
                previous: previousRate,
                changePercent: changePercent(currentRate, previousRate)
              },
              sum_goal_value: measureTotals(row, 'sum_goal_value'),
              median_goal_value: measureTotals(row, 'median_goal_value')
            }
          }
        })
    )
  }, [currentGoalRows, previousGoalRows, currentSessionRow, previousSessionRow])

  const isLoading = goalsResult.isLoading || sessionsResult.isLoading

  return (
    <div className={goalsResult.isFetching ? 'opacity-75 transition-opacity' : ''}>
      {goalsResult.error ? (
        <Alert type="error" showIcon title={t`Could not load the goals`} className="mb-4" />
      ) : null}

      {isLoading && goals.length === 0 ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2].map((index) => (
            <div key={index} className="rounded-md border border-gray-200 p-4">
              <Skeleton active paragraph={{ rows: 4 }} />
            </div>
          ))}
        </div>
      ) : goals.length === 0 ? (
        <div className="rounded-md border border-gray-200 py-12">
          <Empty description={t`No goals tracked yet`} image={Empty.PRESENTED_IMAGE_SIMPLE} />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {goals.map((goal) => (
            <GoalCard
              key={goal.name}
              goalName={goal.name}
              totals={goal.totals}
              onOpenDashboard={() => {
                setSelectedGoal(goal.name)
                setDrawerOpen(true)
              }}
            />
          ))}
        </div>
      )}

      {selectedGoal ? (
        <GoalDashboardDrawer
          open={drawerOpen}
          goalName={selectedGoal}
          onClose={() => setDrawerOpen(false)}
          // Dropping the drawer only once it has finished closing keeps its
          // animation, and stops its dozen widget queries from living on.
          afterOpenChange={(open) => {
            if (!open) setSelectedGoal(null)
          }}
        />
      ) : null}
    </div>
  )
}

function measureTotals(row: DimensionRow, measure: string): MetricTotals {
  const current = toNumber(row[measure])
  const previous = toNumber(row[`prev_${measure}`])
  return { current, previous, changePercent: changePercent(current, previous) }
}

function conversionRate(conversions: number, sessions: number): number {
  return sessions > 0 ? (conversions / sessions) * 100 : 0
}
