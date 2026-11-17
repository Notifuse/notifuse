import { useMemo, useState } from 'react'
import { Alert } from 'antd'
import { Calendar } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { FullDrawer, Panel, type DrawerNav } from '../../layout'
import { useWebAnalytics } from '../useWebAnalytics'
import { ColumnConfig, DimensionTabConfig, DimensionTableWidget } from '../DimensionTableWidget'
import { MetricChart, useAnnotations } from '../MetricChart'
import { MetricSummary } from '../MetricSummary'
import { TrafficHeatmapWidget } from '../TrafficHeatmapWidget'
import { GranularitySelector } from '../toolbar'
import { CountryFlag, getDeviceIcon } from '../lib/icons'
import { formatDateRangeLabel, timeBucketColumn } from '../lib/dates'
import { namedCustomDimensions } from '../lib/dimensions'
import { changePercent, formatRangeSeriesName, toNumber } from '../lib/format'
import { buildWebQuery, readTotals, useWebComparisonQuery } from '../lib/query'
import { ChartDataPoint, MetricConfig, MetricTotals, PRIMARY_COLOR } from '../lib/types'
import { GoalFiltersLine } from './GoalFiltersLine'
import { conversionRate, GOAL_BREAKDOWN_MEASURES, GOAL_MEASURES, goalFilters, goalNameFilter } from './queries'

/** A goal's figures; labels are translated at render time. Its rate is a figure, not a series. */
const GOAL_METRICS: MetricConfig[] = [
  { key: 'goals', label: 'Count', format: 'number', color: PRIMARY_COLOR },
  { key: 'conversion_rate', label: 'Conv. Rate', format: 'percentage', color: PRIMARY_COLOR, selectable: false },
  { key: 'sum_goal_value', label: 'Total Value', format: 'currency', color: PRIMARY_COLOR },
  { key: 'median_goal_value', label: 'Median Value', format: 'currency', color: PRIMARY_COLOR }
]

export interface GoalDashboardDrawerProps {
  /** The goal shown; null renders nothing. */
  goalName: string | null
  /** The goals of the grid, in its order, for previous and next. */
  goals: string[]
  onNavigate: (goalName: string) => void
  onClose: () => void
  /**
   * The period's sessions (and the comparison period's), as the goals grid already read them:
   * the denominator of the conversion rate, passed rather than queried again.
   */
  sessionTotals: { current: number; previous: number }
}

/**
 * A full dashboard for one goal: the same KPI strip, chart and breakdowns as the main tab, every
 * one of them filtered down to conversions of that goal.
 */
export function GoalDashboardDrawer(props: GoalDashboardDrawerProps) {
  if (!props.goalName) return null
  return <GoalDashboard {...props} goalName={props.goalName} />
}

function GoalDashboard(props: GoalDashboardDrawerProps & { goalName: string }) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [selected, setSelected] = useState('goals')
  const { goalName, goals, onNavigate } = props

  const labels: Record<string, string> = {
    goals: t`Count`,
    conversion_rate: t`Conv. Rate`,
    sum_goal_value: t`Total Value`,
    median_goal_value: t`Median Value`
  }
  const tooltips: Record<string, string> = {
    conversion_rate: t`Not all visitors come with the same intent. This rate should be interpreted with caution as it includes all traffic regardless of purpose.`
  }

  const filters = useMemo(() => goalFilters(context.filters, goalName), [context.filters, goalName])

  const base = {
    schema: 'web_goals' as const,
    measures: GOAL_MEASURES,
    dimensions: [],
    filters,
    granularity: context.granularity,
    timezone: context.timezone
  }

  const series = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({ ...base, range: context.resolved }),
    context.showComparison && context.resolvedCompare ? buildWebQuery({ ...base, range: context.resolvedCompare }) : null
  )

  // The KPI strip reads its own period aggregate, the query the goal cards run, rather than adding
  // up the chart's buckets: a median cannot be rebuilt from per-day medians, so the strip and the
  // card would show two different medians for the same goal and period.
  const totalsBase = { ...base, granularity: undefined }
  const periodTotals = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({ ...totalsBase, range: context.resolved }),
    context.showComparison && context.resolvedCompare
      ? buildWebQuery({ ...totalsBase, range: context.resolvedCompare })
      : null
  )

  const currentRows = series.current?.data
  const previousRows = series.previous?.data
  const currentTotals = periodTotals.current
  const previousTotals = periodTotals.previous
  const { current: currentSessions, previous: previousSessions } = props.sessionTotals

  const totals: Record<string, MetricTotals> = useMemo(() => {
    const current = readTotals(currentTotals, GOAL_MEASURES)
    const previous = readTotals(previousTotals, GOAL_MEASURES)
    const measured = Object.fromEntries(
      GOAL_MEASURES.map((measure) => [
        measure,
        {
          current: current[measure],
          previous: previous[measure],
          changePercent: changePercent(current[measure], previous[measure])
        }
      ])
    )
    // The card's rate: this goal's conversions over every session of the period.
    const rate = conversionRate(current.goals, currentSessions)
    const previousRate = conversionRate(previous.goals, previousSessions)
    return {
      ...measured,
      conversion_rate: { current: rate, previous: previousRate, changePercent: changePercent(rate, previousRate) }
    }
  }, [currentTotals, previousTotals, currentSessions, previousSessions])

  const bucketColumn = timeBucketColumn('goal_at', context.granularity)
  const toPoints = (rows: Record<string, unknown>[] | undefined): ChartDataPoint[] =>
    (rows ?? []).map((row) => ({
      timestamp: String(row[bucketColumn] ?? ''),
      value: toNumber(row[selected])
    }))

  const metric = GOAL_METRICS.find((candidate) => candidate.key === selected) ?? GOAL_METRICS[0]
  const chartMetric: MetricConfig = { ...metric, label: labels[metric.key] ?? metric.label }

  const annotations = useAnnotations(context.workspaceId, context.resolved)

  const columns: ColumnConfig[] = [
    { key: 'goals', label: t`Count`, format: 'number' },
    { key: 'sum_goal_value', label: t`Value`, format: 'currency' }
  ]

  // Every breakdown reads the goal table rather than the session one, so a row counts conversions
  // of this goal instead of the sessions behind them.
  const breakdownTab = (
    tab: Pick<DimensionTabConfig, 'key' | 'label' | 'dimensionLabel' | 'dimension'> & Partial<DimensionTabConfig>
  ): DimensionTabConfig => ({
    schema: 'web_goals',
    measures: GOAL_BREAKDOWN_MEASURES,
    order: { goals: 'desc' },
    ...tab,
    filters: [goalNameFilter(goalName), ...(tab.filters ?? [])]
  })

  // Previous and next walk the grid. A filter set from the drawer can drop this goal from the
  // grid: the drawer stays on it, the buttons disabled and no position shown.
  const position = goals.indexOf(goalName)
  const nav: DrawerNav =
    position === -1
      ? {}
      : {
          position: position + 1,
          total: goals.length,
          onPrev: position > 0 ? () => onNavigate(goals[position - 1]) : undefined,
          onNext: position < goals.length - 1 ? () => onNavigate(goals[position + 1]) : undefined
        }

  const custom = namedCustomDimensions(context.customDimensionLabels)
  const period = formatDateRangeLabel(context.range)
  const comparedTo = context.showComparison && context.compareRange ? formatDateRangeLabel(context.compareRange) : null

  return (
    <FullDrawer
      open
      onClose={props.onClose}
      backLabel={t`Goals`}
      // The goal opened is not in the address, so a copied link would not reopen it.
      copyLink={false}
      // Expand opens a drawer of its own inside this one, which must not slide aside.
      push={false}
      ariaLabel={t`Goal dashboard: ${goalName}`}
      nav={nav}
      barTitle={
        <>
          <span className="truncate text-sm font-semibold text-ink">{goalName}</span>
          <span className="hidden shrink-0 text-xs text-ink-3 sm:inline">{t`Goal dashboard`}</span>
        </>
      }
      topActions={
        <span data-goal-period className="hidden items-center gap-1.5 text-sm text-ink-2 md:inline-flex">
          <Calendar size={14} aria-hidden="true" className="text-ink-3" />
          {period}
          {comparedTo ? <span className="text-ink-3">{t`vs ${comparedTo}`}</span> : null}
        </span>
      }
    >
      <div className={`flex flex-col gap-3 ${series.isFetching ? 'opacity-75 transition-opacity' : ''}`}>
        <GoalFiltersLine />

        {series.error || periodTotals.error ? (
          <Alert type="error" showIcon title={t`Could not load this goal`} />
        ) : null}

        <MetricSummary
          metrics={GOAL_METRICS}
          totals={periodTotals.isLoading ? {} : totals}
          selected={selected}
          onSelect={setSelected}
          showComparison={context.showComparison}
          loading={periodTotals.isLoading}
          labels={labels}
          tooltips={tooltips}
        />

        <div className="grid grid-cols-1 gap-3 xl:grid-cols-3">
          <div className="min-w-0 xl:col-span-2">
            {/* The strip above is the chart's metric selector, so the panel is titled with its choice. */}
            <Panel title={chartMetric.label} extra={<GranularitySelector />}>
              <MetricChart
                metric={chartMetric}
                current={toPoints(currentRows)}
                previous={context.showComparison ? toPoints(previousRows) : []}
                granularity={context.granularity}
                currentLabel={formatRangeSeriesName(context.range)}
                previousLabel={context.compareRange ? formatRangeSeriesName(context.compareRange) : undefined}
                loading={series.isLoading}
                height={240}
                annotations={annotations}
                timezone={context.timezone}
              />
            </Panel>
          </div>
          <TrafficHeatmapWidget
            title={t`Goals by day and hour`}
            schema="web_goals"
            emptyText={t`No goal data`}
            extraFilters={[goalNameFilter(goalName)]}
            tabs={[{ key: 'goals', label: t`Goals`, measure: 'goals', format: 'number' }]}
          />
        </div>

        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          <DimensionTableWidget
            title={t`Top Sources`}
            columns={columns}
            emptyText={t`No source data`}
            tabs={[
              breakdownTab({
                key: 'referrers',
                label: t`Referrers`,
                dimensionLabel: t`Referrer domain`,
                dimension: 'referrer_domain'
              }),
              breakdownTab({ key: 'channels', label: t`Channels`, dimensionLabel: t`Channel`, dimension: 'channel' }),
              breakdownTab({
                key: 'channel_groups',
                label: t`Channel groups`,
                dimensionLabel: t`Channel group`,
                dimension: 'channel_group'
              })
            ]}
          />
          <DimensionTableWidget
            title={t`Top Campaigns`}
            columns={columns}
            emptyText={t`No campaign data`}
            tabs={(
              [
                ['campaign', t`Campaigns`, 'utm_campaign'],
                ['source', t`Sources`, 'utm_source'],
                ['medium', t`Mediums`, 'utm_medium']
              ] as const
            ).map(([key, label, dimension]) =>
              breakdownTab({
                key,
                label,
                dimensionLabel: label,
                dimension,
                // Most conversions carry no campaign at all; without this the table would be one
                // giant "(empty)" row on every tab.
                filters: [{ dimension, operator: 'isNotEmpty', values: [] }]
              })
            )}
          />
          <DimensionTableWidget
            title={t`Devices`}
            columns={columns}
            emptyText={t`No device data`}
            iconPrefix={(value, tabKey) => getDeviceIcon(value, tabKey)}
            tabs={[
              breakdownTab({ key: 'devices', label: t`Devices`, dimensionLabel: t`Device`, dimension: 'device' }),
              breakdownTab({ key: 'browsers', label: t`Browsers`, dimensionLabel: t`Browser`, dimension: 'browser' }),
              breakdownTab({ key: 'os', label: t`OS`, dimensionLabel: t`OS`, dimension: 'os' })
            ]}
          />
          <DimensionTableWidget
            title={t`Countries`}
            columns={columns}
            emptyText={t`No country data`}
            iconPrefix={(value, tabKey) => (tabKey === 'list' ? <CountryFlag iso2={value} /> : null)}
            tabs={[
              breakdownTab({
                key: 'map',
                label: t`Map`,
                dimensionLabel: t`Country`,
                dimension: 'country',
                type: 'country_map',
                limit: 100
              }),
              breakdownTab({ key: 'list', label: t`List`, dimensionLabel: t`Country`, dimension: 'country' })
            ]}
          />
          <DimensionTableWidget
            title={t`Landing pages`}
            columns={columns}
            emptyText={t`No page data`}
            tabs={[
              breakdownTab({
                key: 'landing',
                label: t`Landing pages`,
                dimensionLabel: t`Landing path`,
                dimension: 'landing_path'
              }),
              breakdownTab({ key: 'goal_pages', label: t`Goal pages`, dimensionLabel: t`Goal path`, dimension: 'goal_path' })
            ]}
          />
          {custom.length > 0 ? (
            <DimensionTableWidget
              title={custom.length === 1 ? custom[0].label : t`Custom dimensions`}
              columns={columns}
              emptyText={t`No data`}
              tabs={custom.map((slot) =>
                breakdownTab({ key: slot.key, label: slot.label, dimensionLabel: slot.label, dimension: slot.key })
              )}
            />
          ) : null}
        </div>
      </div>
    </FullDrawer>
  )
}
