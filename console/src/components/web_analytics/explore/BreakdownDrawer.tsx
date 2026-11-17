import { useState } from 'react'
import { Button, Tag } from 'antd'
import { useQueries } from '@tanstack/react-query'
import { ChevronRight, Plus, X } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { FullDrawer, Panel } from '../../layout'
import { useWebAnalytics } from '../useWebAnalytics'
import { MetricChart } from '../MetricChart'
import { MetricSummary } from '../MetricSummary'
import { GranularitySelector } from '../toolbar'
import { formatDateRangeLabel, timeBucketColumn } from '../lib/dates'
import { formatDimensionValue, getDimensionLabel } from '../lib/dimensions'
import { ExploreRow } from '../lib/exploreRows'
import { describeDimensionFilter, describeMetricFilter, useOperatorLabels } from '../lib/filterLabels'
import { formatPercent, formatRangeSeriesName, toNumber } from '../lib/format'
import { useWebQuery, webAnalyticsClient } from '../lib/query'
import { ChartDataPoint, MetricTotals, SESSION_METRICS, WebDimensionFilter } from '../lib/types'
import { buildBreakdownQuery, buildBreakdownSeriesQuery } from './breakdownQuery'
import { BreakdownModal } from './BreakdownModal'
import { shareOf } from './breakdownRows'
import { BreakdownTableCard } from './BreakdownTableCard'

export interface BreakdownDrawerProps {
  onClose: () => void
  row: ExploreRow
  /** The report's dimensions, the row's path among them. */
  dimensions: string[]
  /** Dimensions to break the row down by first, one table each. */
  breakdownDimensions: string[]
  /**
   * The page filters, then the row's own path as filters: the array the hover prefetch queried
   * with, so the tables open on its cache.
   */
  parentFilters: WebDimensionFilter[]
  /** Sessions of the row's parent (the period's for a top-level row): the base of its share. */
  shareBase?: number
  /** The parent's value, or "all sessions". */
  shareLabel: string
}

/**
 * What one row of the explore report weighs, and what it is made of, dimension by dimension.
 * The drill-down answers "what is inside this row" in a fixed order; the breakdown answers "what
 * characterises it" across several dimensions at once, each queried on its own.
 */
export function BreakdownDrawer(props: BreakdownDrawerProps) {
  const { t, i18n } = useLingui()
  const context = useWebAnalytics()
  const operatorLabels = useOperatorLabels()
  const { row, dimensions, parentFilters } = props
  const [selected, setSelected] = useState<string[]>(props.breakdownDimensions)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [chartMetric, setChartMetric] = useState('sessions')

  const dimension = dimensions[row.dimensionIndex]
  const dimensionLabel = getDimensionLabel(dimension, context.customDimensionLabels)
  const valueLabel = formatDimensionValue(dimension, row[dimension], { emptyLabel: t`(empty)`, locale: i18n.locale })
  // The page filters come first in parentFilters; what follows is the row's path.
  const pathFilters = parentFilters.slice(context.filters.length)
  const pathDimensions = dimensions.slice(0, row.dimensionIndex + 1)

  const metricLabels: Record<string, string> = {
    sessions: t`Sessions`,
    median_duration: t`Median TimeScore`,
    bounce_rate: t`Bounce Rate`,
    median_scroll: t`Median Scroll Depth`
  }
  const thresholdLabels: Record<string, string> = {
    median_duration: t`TimeScore`,
    bounce_rate: t`Bounce Rate`,
    median_scroll: t`Median Scroll Depth`
  }

  const tables = useQueries({
    queries: selected.map((breakdownDimension) => {
      const query = buildBreakdownQuery({
        dimension: breakdownDimension,
        filters: parentFilters,
        metricFilters: context.metricFilters,
        minSessions: context.minSessions,
        range: context.resolved,
        timezone: context.timezone
      })
      return {
        queryKey: ['web-analytics', context.workspaceId, query],
        queryFn: () => webAnalyticsClient.query(query, context.workspaceId)
      }
    })
  })

  const series = useWebQuery(
    context.workspaceId,
    buildBreakdownSeriesQuery({
      filters: parentFilters,
      range: context.resolved,
      timezone: context.timezone,
      granularity: context.granularity
    })
  )

  // The row's own figures, already in the explore table: no query for them.
  const totals: Record<string, MetricTotals> = Object.fromEntries(
    SESSION_METRICS.map((metric) => [
      metric.key,
      { current: toNumber(row[metric.key]), previous: 0, changePercent: 0 }
    ])
  )
  const share =
    props.shareBase !== undefined && props.shareBase > 0
      ? formatPercent(shareOf(row.sessions, props.shareBase))
      : null
  const parent = props.shareLabel

  const bucketColumn = timeBucketColumn('created_at', context.granularity)
  const points: ChartDataPoint[] = (series.data?.data ?? []).map((point) => ({
    timestamp: String(point[bucketColumn] ?? ''),
    value: toNumber(point[chartMetric])
  }))
  const metric = SESSION_METRICS.find((candidate) => candidate.key === chartMetric) ?? SESSION_METRICS[0]

  const pageFilters = [
    ...context.filters.map((filter) => describeDimensionFilter(filter, operatorLabels, context.customDimensionLabels)),
    ...context.metricFilters.map((filter) => describeMetricFilter(filter, operatorLabels, thresholdLabels))
  ]
  const pageFilterList = pageFilters.length > 0 ? pageFilters.join(', ') : t`none`
  const minSessions = context.minSessions

  const addDimension = (
    <Button aria-label={t`Add dimension`} icon={<Plus size={14} aria-hidden="true" />} onClick={() => setPickerOpen(true)}>
      {/* An icon alone on a phone, where the bar is short of room. */}
      <span className="hidden sm:inline">{t`Add dimension`}</span>
    </Button>
  )

  return (
    <>
      <FullDrawer
        open
        onClose={props.onClose}
        backLabel={t`Explore`}
        // The breakdown is not in the address, so a copied link would not reopen it.
        copyLink={false}
        ariaLabel={t`Breakdown of ${valueLabel}`}
        barTitle={
          <>
            <span className="hidden shrink-0 text-sm text-ink-3 sm:inline">{dimensionLabel}</span>
            <span className="truncate text-sm font-semibold text-ink">{valueLabel}</span>
            <span className="hidden shrink-0 text-xs text-ink-3 sm:inline">{formatDateRangeLabel(context.range)}</span>
          </>
        }
        topActions={addDimension}
      >
        <div
          data-breakdown-context
          className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm shadow-card"
        >
          <span className="inline-flex flex-wrap items-center gap-1.5">
            <span className="text-ink-3">{t`Within`}</span>
            {pathFilters.map((filter, index) => (
              <span key={`${filter.dimension}-${index}`} className="inline-flex items-center gap-1.5">
                {index > 0 ? <ChevronRight size={12} aria-hidden="true" className="text-ink-4" /> : null}
                <span data-path-chip className="rounded-md bg-fill px-1.5 py-0.5 text-ink-2">
                  {getDimensionLabel(filter.dimension, context.customDimensionLabels)}{' '}
                  <span className="text-ink">
                    {filter.operator === 'isEmpty'
                      ? t`(empty)`
                      : formatDimensionValue(filter.dimension, filter.values[0], {
                          emptyLabel: t`(empty)`,
                          locale: i18n.locale
                        })}
                  </span>
                </span>
              </span>
            ))}
          </span>
          <span className="text-ink-3">· {t`Page filters: ${pageFilterList}`}</span>
          <span className="text-ink-3">· {t`Min. sessions: ${minSessions}`}</span>
          <span className="inline-flex flex-wrap items-center gap-1.5">
            <span className="text-ink-3">{t`Broken down by`}</span>
            {selected.map((entry) => {
              const label = getDimensionLabel(entry, context.customDimensionLabels)
              return (
                <Tag
                  key={entry}
                  color="processing"
                  closable
                  closeIcon={<X size={12} aria-label={t`Remove ${label}`} />}
                  onClose={(event) => {
                    event.preventDefault()
                    setSelected((current) => current.filter((candidate) => candidate !== entry))
                  }}
                >
                  {label}
                </Tag>
              )
            })}
          </span>
        </div>

        <MetricSummary
          metrics={SESSION_METRICS}
          totals={totals}
          selected={chartMetric}
          onSelect={setChartMetric}
          showComparison={false}
          labels={metricLabels}
          subs={share ? { sessions: t`${share} of ${parent}` } : undefined}
        />

        {/* The strip above is the chart's metric selector, so the panel is titled with its choice. */}
        <Panel title={metricLabels[metric.key] ?? metric.label} extra={<GranularitySelector />}>
          <MetricChart
            metric={metric}
            current={points}
            granularity={context.granularity}
            currentLabel={formatRangeSeriesName(context.range)}
            loading={series.isLoading}
            height={220}
          />
        </Panel>

        {selected.length === 0 ? (
          <Panel>
            <div className="flex flex-col items-center gap-3 py-6 text-center">
              <p className="m-0 text-sm text-ink-3">
                {t`Pick the dimensions to break this row down by: each one gets its own table.`}
              </p>
              {addDimension}
            </div>
          </Panel>
        ) : (
          <div className="grid grid-cols-1 gap-3 min-[1100px]:grid-cols-2">
            {selected.map((entry, index) => (
              <BreakdownTableCard
                key={entry}
                dimension={entry}
                label={getDimensionLabel(entry, context.customDimensionLabels)}
                rows={tables[index]?.data?.data ?? undefined}
                loading={tables[index]?.isLoading ?? true}
                error={Boolean(tables[index]?.error)}
                onRetry={() => void tables[index]?.refetch()}
                base={row.sessions}
                minSessions={context.minSessions}
              />
            ))}
          </div>
        )}
      </FullDrawer>

      {/* A sibling: the picker opens over the drawer, which stays as it is. */}
      <BreakdownModal
        open={pickerOpen}
        initialDimensions={selected}
        excludeDimensions={pathDimensions}
        submitText={t`Apply`}
        onCancel={() => setPickerOpen(false)}
        onSubmit={(next) => {
          setSelected(next)
          setPickerOpen(false)
        }}
      />
    </>
  )
}
