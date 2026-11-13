import { useMemo, useState } from 'react'
import { Alert } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../useWebAnalytics'
import { DimensionTableWidget, ColumnConfig } from '../DimensionTableWidget'
import { MetricChart, useAnnotations } from '../MetricChart'
import { MetricSummary } from '../MetricSummary'
import { SdkVersionWarning } from '../SdkVersionWarning'
import { TrafficHeatmapWidget } from '../TrafficHeatmapWidget'
import { GranularitySelector } from '../toolbar'
import { CountryFlag, getDeviceIcon } from '../lib/icons'
import { timeBucketColumn } from '../lib/dates'
import { changePercent, formatRangeSeriesName, toNumber } from '../lib/format'
import { buildWebQuery, readMeasure, useWebComparisonQuery } from '../lib/query'
import {
  ChartDataPoint,
  MetricTotals,
  SESSION_METRIC_KEYS,
  SESSION_METRICS
} from '../lib/types'

export function DashboardTab() {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [selectedMetric, setSelectedMetric] = useState('sessions')

  const metricLabels: Record<string, string> = {
    sessions: t`Sessions`,
    median_duration: t`Median TimeScore`,
    bounce_rate: t`Bounce Rate`,
    median_scroll: t`Median Scroll Depth`
  }
  const metricTooltips: Record<string, string> = {
    median_duration: t`TimeScore is the median engaged time across all sessions`
  }

  const base = {
    schema: 'web_sessions' as const,
    measures: SESSION_METRIC_KEYS,
    dimensions: [],
    filters: context.filters,
    timezone: context.timezone
  }

  // Totals are queried without a granularity on purpose: a median or a rate
  // averaged across buckets is not the median or the rate of the period.
  const totalsResult = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({ ...base, range: context.resolved }),
    context.showComparison && context.resolvedCompare
      ? buildWebQuery({ ...base, range: context.resolvedCompare })
      : null
  )

  const seriesResult = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({ ...base, range: context.resolved, granularity: context.granularity }),
    context.showComparison && context.resolvedCompare
      ? buildWebQuery({
          ...base,
          range: context.resolvedCompare,
          granularity: context.granularity
        })
      : null
  )

  const currentTotalsRow = totalsResult.current?.data?.[0]
  const previousTotalsRow = totalsResult.previous?.data?.[0]

  const totals: Record<string, MetricTotals> = useMemo(() => {
    const currentRow = currentTotalsRow
    const previousRow = previousTotalsRow
    return Object.fromEntries(
      SESSION_METRIC_KEYS.map((measure) => {
        const current = readMeasure(currentRow, measure)
        const previous = readMeasure(previousRow, measure)
        return [measure, { current, previous, changePercent: changePercent(current, previous) }]
      })
    )
  }, [currentTotalsRow, previousTotalsRow])

  const bucketColumn = timeBucketColumn('created_at', context.granularity)
  const toSeries = (rows: Record<string, unknown>[] | undefined): ChartDataPoint[] =>
    (rows ?? []).map((row) => ({
      timestamp: String(row[bucketColumn] ?? ''),
      value: toNumber(row[selectedMetric])
    }))

  const metric = SESSION_METRICS.find((candidate) => candidate.key === selectedMetric) ?? SESSION_METRICS[0]

  const annotations = useAnnotations(context.workspaceId, context.resolved)

  const sessionColumns: ColumnConfig[] = [
    { key: 'sessions', label: t`Sessions`, format: 'number' },
    { key: 'median_duration', label: t`TimeScore`, format: 'duration', heatMap: true }
  ]

  return (
    <div className={seriesResult.isFetching ? 'opacity-75 transition-opacity' : ''}>
      <SdkVersionWarning />

      {seriesResult.error ? (
        <Alert type="error" showIcon title={t`Could not load the dashboard`} className="mb-4" />
      ) : null}

      <div className="overflow-hidden rounded-md border border-gray-200">
        <MetricSummary
          metrics={SESSION_METRICS}
          totals={totals}
          selected={selectedMetric}
          onSelect={setSelectedMetric}
          showComparison={context.showComparison}
          loading={totalsResult.isLoading}
          labels={metricLabels}
          tooltips={metricTooltips}
        />
        <div className="relative pb-3 pl-2 pr-4 pt-4">
          <div className="absolute right-4 top-4 z-10">
            <GranularitySelector />
          </div>
          <MetricChart
            metric={metric}
            current={toSeries(seriesResult.current?.data)}
            previous={context.showComparison ? toSeries(seriesResult.previous?.data) : []}
            granularity={context.granularity}
            currentLabel={formatRangeSeriesName(context.range)}
            previousLabel={
              context.compareRange ? formatRangeSeriesName(context.compareRange) : undefined
            }
            loading={seriesResult.isLoading}
            height={220}
            annotations={annotations}
            timezone={context.timezone}
          />
        </div>
      </div>

      <div className="mt-6 grid grid-cols-1 gap-x-4 gap-y-6 md:grid-cols-2">
        <DimensionTableWidget
          title={t`Top Pages`}
          columns={sessionColumns}
          tabs={[
            {
              key: 'landing',
              label: t`Landing pages`,
              dimensionLabel: t`Page`,
              dimension: 'landing_path'
            },
            {
              key: 'exits',
              label: t`Exits`,
              dimensionLabel: t`Exit page`,
              dimension: 'exit_path'
            }
          ]}
        />
        <DimensionTableWidget
          title={t`Top Sources`}
          columns={sessionColumns}
          tabs={[
            {
              key: 'referrers',
              label: t`Referrers`,
              dimensionLabel: t`Referrer domain`,
              dimension: 'referrer_domain'
            },
            {
              key: 'channels',
              label: t`Channels`,
              dimensionLabel: t`Channel`,
              dimension: 'channel'
            },
            {
              key: 'channel_groups',
              label: t`Channel groups`,
              dimensionLabel: t`Channel group`,
              dimension: 'channel_group'
            }
          ]}
        />
      </div>

      <div className="mt-6 grid grid-cols-1 gap-x-4 gap-y-6 md:grid-cols-2">
        <DimensionTableWidget
          title={t`Top Campaigns`}
          columns={sessionColumns}
          emptyText={t`No campaign data`}
          tabs={(
            [
              ['campaign', t`Campaigns`, 'utm_campaign'],
              ['source', t`Sources`, 'utm_source'],
              ['medium', t`Mediums`, 'utm_medium'],
              ['content', t`Contents`, 'utm_content'],
              ['term', t`Terms`, 'utm_term']
            ] as const
          ).map(([key, label, dimension]) => ({
            key,
            label,
            dimensionLabel: label,
            dimension,
            // Most sessions carry no campaign at all; without this the table
            // would be one giant "(empty)" row on every tab.
            filters: [{ dimension, operator: 'isNotEmpty' as const, values: [] }]
          }))}
        />
        <DimensionTableWidget
          title={t`Countries`}
          columns={sessionColumns}
          emptyText={t`No country data`}
          iconPrefix={(value, tabKey) => (tabKey === 'list' ? <CountryFlag iso2={value} /> : null)}
          tabs={[
            {
              key: 'map',
              label: t`Map`,
              dimensionLabel: t`Country`,
              dimension: 'country',
              type: 'country_map',
              limit: 100
            },
            { key: 'list', label: t`List`, dimensionLabel: t`Country`, dimension: 'country' }
          ]}
        />
      </div>

      <div className="mt-6 grid grid-cols-1 gap-x-4 gap-y-6 md:grid-cols-2">
        <TrafficHeatmapWidget title={t`Traffic by day and hour`} />
        <DimensionTableWidget
          title={t`Devices`}
          columns={sessionColumns}
          iconPrefix={(value, tabKey) => getDeviceIcon(value, tabKey)}
          tabs={[
            { key: 'devices', label: t`Devices`, dimensionLabel: t`Device`, dimension: 'device' },
            {
              key: 'browsers',
              label: t`Browsers`,
              dimensionLabel: t`Browser`,
              dimension: 'browser'
            },
            { key: 'os', label: t`OS`, dimensionLabel: t`OS`, dimension: 'os' }
          ]}
        />
      </div>

      <div className="mt-6 grid grid-cols-1 gap-x-4 gap-y-6 md:grid-cols-2">
        <DimensionTableWidget
          title={t`Goals`}
          infoTooltip={t`Goal conversions and their total value`}
          emptyText={t`No goals data`}
          columns={[
            { key: 'goals', label: t`Count`, format: 'number' },
            { key: 'sum_goal_value', label: t`Value`, format: 'currency' }
          ]}
          tabs={[
            {
              key: 'goals',
              label: t`Goals`,
              dimensionLabel: t`Goal`,
              dimension: 'goal_name',
              schema: 'web_goals',
              measures: ['goals', 'sum_goal_value'],
              order: { goals: 'desc' }
            }
          ]}
        />
      </div>
    </div>
  )
}
