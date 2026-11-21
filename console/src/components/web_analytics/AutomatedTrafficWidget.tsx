import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import { Empty } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Pill } from '../layout'
import { DimensionTableWidget, ColumnConfig } from './DimensionTableWidget'
import { useWebAnalytics } from './useWebAnalytics'
import { CountryFlag, getDeviceIcon } from './lib/icons'
import { timeBucketColumn } from './lib/dates'
import {
  formatNumber,
  formatPercent,
  formatPoints,
  formatRangeSeriesName,
  formatXAxisLabel,
  toNumber
} from './lib/format'
import { buildWebQuery, readMeasure, useWebComparisonQuery } from './lib/query'
import { filtersForSchema } from './lib/dimensions'
import { colors, typography } from '../../theme/tokens'

const TOTAL_MEASURES = ['sessions', 'automated_sessions', 'automated_share']
// The tooltip works the share out per bucket from the two counts, so the series does not ask for it.
const SERIES_MEASURES = ['sessions', 'automated_sessions']
// A page, browser or country that no robot reached has nothing to show here.
const AUTOMATED_ONLY = [{ metric: 'automated_sessions', operator: 'gt' as const, values: [0] }]
const CHART_HEIGHT = 180
const AUTOMATED_COLOR = colors.chart.primary
const HUMAN_COLOR = colors.primary[300]

/**
 * Automated traffic: sessions that loaded a page and never sent a second beat (crawlers, AI
 * agents, headless browsers). The rule lives on the server, in the `traffic_type` dimension and
 * the `automated_*` measures, so this card, the filter and TimeScore cannot disagree about it.
 */
export function AutomatedTrafficWidget() {
  const { t } = useLingui()

  const columns: ColumnConfig[] = [
    // More robots is neither good nor bad news, so their changes are drawn in grey.
    { key: 'automated_sessions', label: t`Automated`, format: 'number', neutralTrend: true },
    { key: 'automated_share', label: t`Share of sessions`, format: 'percentage', neutralTrend: true }
  ]

  return (
    <DimensionTableWidget
      title={t`Automated traffic`}
      infoTooltip={t`Sessions that loaded a page and sent no further signal: crawlers, AI agents and headless browsers`}
      columns={columns}
      footer={<span>{t`Counted in Sessions, left out of TimeScore and Bounce Rate`}</span>}
      iconPrefix={(value, tabKey) =>
        tabKey === 'countries' ? (
          <CountryFlag iso2={value} />
        ) : tabKey === 'browsers' ? (
          getDeviceIcon(value, tabKey)
        ) : null
      }
      tabs={[
        {
          key: 'trend',
          label: t`Trend`,
          dimensionLabel: '',
          dimension: 'traffic_type',
          content: <AutomatedTrafficTrend />
        },
        {
          key: 'landing',
          label: t`Landing pages`,
          dimensionLabel: t`Page`,
          dimension: 'landing_path',
          metricFilters: AUTOMATED_ONLY
        },
        {
          key: 'browsers',
          label: t`Browsers`,
          dimensionLabel: t`Browser`,
          dimension: 'browser',
          metricFilters: AUTOMATED_ONLY
        },
        {
          key: 'countries',
          label: t`Countries`,
          dimensionLabel: t`Country`,
          dimension: 'country',
          metricFilters: AUTOMATED_ONLY
        }
      ]}
    />
  )
}

/** The share of automated sessions, its change, and automated and human sessions per bucket. */
function AutomatedTrafficTrend() {
  const { t } = useLingui()
  const context = useWebAnalytics()

  const base = {
    schema: 'web_sessions' as const,
    measures: TOTAL_MEASURES,
    dimensions: [],
    filters: filtersForSchema(context.filters, 'web_sessions'),
    timezone: context.timezone
  }

  // Totals without a granularity: a share averaged across buckets is not the share of the period.
  const totals = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({ ...base, range: context.resolved }),
    context.showComparison && context.resolvedCompare
      ? buildWebQuery({ ...base, range: context.resolvedCompare })
      : null
  )
  const series = useWebComparisonQuery(
    context.workspaceId,
    buildWebQuery({
      ...base,
      measures: SERIES_MEASURES,
      range: context.resolved,
      granularity: context.granularity
    }),
    null
  )

  const current = totals.current?.data?.[0]
  const previous = totals.previous?.data?.[0]
  const share = readMeasure(current, 'automated_share')
  const automated = readMeasure(current, 'automated_sessions')
  const human = readMeasure(current, 'sessions') - automated
  // A period with no session has no share (the engine answers NULL), and 0% would be made up:
  // neither the figure nor a change against such a period is shown.
  const hasShare = readMeasure(current, 'sessions') > 0
  const pointsChange =
    hasShare && previous && readMeasure(previous, 'sessions') > 0
      ? share - readMeasure(previous, 'automated_share')
      : null

  const bucketColumn = timeBucketColumn('created_at', context.granularity)
  const rows = series.current?.data
  const automatedLabel = t`Automated`
  const humanLabel = t`Human`

  const option = useMemo(() => {
    // Named here rather than reused from the legend: `t` then belongs to the memo, which is what
    // rebuilds the axis dates and the series names when the console changes language.
    const automatedName = t`Automated`
    const humanName = t`Human`
    const buckets = (rows ?? []).map((row) => {
      const total = toNumber(row.sessions)
      const auto = toNumber(row.automated_sessions)
      return { timestamp: String(row[bucketColumn] ?? ''), automated: auto, human: total - auto }
    })
    const firstDay = buckets[0]?.timestamp.slice(0, 10)
    const withDate =
      context.granularity === 'hour' && buckets.some((bucket) => bucket.timestamp.slice(0, 10) !== firstDay)
    const labels = buckets.map((bucket) =>
      formatXAxisLabel(bucket.timestamp, context.granularity, { withDate })
    )
    const labelInterval = labels.length > 14 ? Math.floor(labels.length / 7) - 1 : 0
    const axisLabel = { color: colors.ink3, fontSize: typography.size.overline }

    return {
      animation: false,
      grid: { left: '1%', right: '1%', bottom: '5%', top: '5%', containLabel: true },
      legend: { show: false },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow', shadowStyle: { color: colors.gray[100] } },
        confine: true,
        backgroundColor: 'rgba(255, 255, 255, 0.95)',
        borderColor: colors.line,
        textStyle: { color: colors.ink, fontSize: typography.size.xs },
        formatter: (params: { axisValue: string; dataIndex: number }[]) => {
          if (!Array.isArray(params) || params.length === 0) return ''
          const bucket = buckets[params[0].dataIndex]
          if (!bucket) return ''
          const total = bucket.automated + bucket.human
          const bullet = (color: string) =>
            `<span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:${color};margin-right:6px"></span>`
          return [
            `<div style="font-weight:600;margin-bottom:4px">${params[0].axisValue}</div>`,
            `<div>${bullet(AUTOMATED_COLOR)}${automatedName}: <b>${formatNumber(bucket.automated)}</b>` +
              (total > 0
                ? ` <span style="color:${colors.ink3}">${formatPercent((bucket.automated / total) * 100)}</span>`
                : '') +
              `</div>`,
            `<div>${bullet(HUMAN_COLOR)}${humanName}: <b>${formatNumber(bucket.human)}</b></div>`
          ].join('')
        }
      },
      xAxis: {
        type: 'category',
        data: labels,
        axisTick: { show: false },
        axisLine: { lineStyle: { color: colors.line } },
        axisLabel: { ...axisLabel, interval: labelInterval }
      },
      yAxis: {
        type: 'value',
        minInterval: 1,
        axisLine: { show: false },
        axisTick: { show: false },
        splitLine: { lineStyle: { color: colors.gray[100] } },
        axisLabel
      },
      series: [
        {
          name: automatedName,
          type: 'bar',
          stack: 'traffic',
          barMaxWidth: 24,
          itemStyle: { color: AUTOMATED_COLOR },
          data: buckets.map((bucket) => bucket.automated)
        },
        {
          name: humanName,
          type: 'bar',
          stack: 'traffic',
          barMaxWidth: 24,
          itemStyle: { color: HUMAN_COLOR, borderRadius: [2, 2, 0, 0] },
          data: buckets.map((bucket) => bucket.human)
        }
      ]
    }
  }, [rows, bucketColumn, context.granularity, t])

  const loading = totals.isLoading || series.isLoading
  const empty = !loading && !hasShare
  const pointsText = pointsChange === null ? '' : formatPoints(pointsChange)
  const previousLabel = context.compareRange ? formatRangeSeriesName(context.compareRange) : ''

  return (
    <div data-automated-trend className="flex flex-col gap-2.5 px-3.5 pt-3 pb-2">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-col gap-0.5">
          <span className="text-xs text-ink-3">{t`Share of sessions`}</span>
          <div className="flex items-baseline gap-2">
            <span data-automated-share className="text-kpi font-semibold text-ink tabular-nums">
              {loading || !hasShare ? '–' : formatPercent(share)}
            </span>
            {pointsChange !== null && !loading ? (
              <>
                <Pill tone="neutral" dot={false}>
                  {t`${pointsText} pts`}
                </Pill>
                {previousLabel ? <span className="text-xs text-ink-3">{t`vs ${previousLabel}`}</span> : null}
              </>
            ) : null}
          </div>
        </div>
        <div className="flex gap-3.5 text-xs text-ink-2">
          <span className="inline-flex items-center gap-1.5">
            <span aria-hidden="true" className="size-2 rounded-xs bg-primary-500" />
            {automatedLabel}
            <b className="font-semibold text-ink tabular-nums">{loading ? '–' : formatNumber(automated)}</b>
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span aria-hidden="true" className="size-2 rounded-xs bg-primary-300" />
            {humanLabel}
            <b className="font-semibold text-ink tabular-nums">{loading ? '–' : formatNumber(human)}</b>
          </span>
        </div>
      </div>

      {loading ? (
        <div className="h-45 animate-pulse rounded bg-fill" />
      ) : empty ? (
        <div className="flex h-45 items-center justify-center">
          <Empty description={t`No data for this period`} image={Empty.PRESENTED_IMAGE_SIMPLE} />
        </div>
      ) : (
        <ReactECharts option={option} style={{ height: CHART_HEIGHT }} notMerge />
      )}
    </div>
  )
}
