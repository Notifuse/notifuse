import { Skeleton, Tooltip } from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { Delta } from '../Delta'
import { ExploreTotals } from '../lib/exploreRows'
import { formatValue } from '../lib/format'
import { getHeatMapStyle } from '../lib/heatmap'
import { SESSION_METRICS, TIMESCORE_REFERENCE_SECONDS } from '../lib/types'

interface ExploreSummaryProps {
  totals?: ExploreTotals
  showComparison: boolean
  loading?: boolean
  /** Highest TimeScore loaded so far, which scales the heat dot. */
  bestValue: number
}

/**
 * Period totals for the whole report, above the drill-down.
 *
 * These come from an ungrouped query rather than a sum of the visible rows: a
 * median or a rate cannot be recovered by aggregating the medians and rates of
 * its parts.
 */
export function ExploreSummary(props: ExploreSummaryProps) {
  const { t } = useLingui()

  const labels: Record<string, string> = {
    sessions: t`Sessions`,
    median_duration: t`Median TimeScore`,
    bounce_rate: t`Bounce Rate`,
    median_scroll: t`Median Scroll Depth`
  }
  const tooltips: Record<string, string> = {
    median_duration: t`TimeScore is the median engaged time across all sessions`
  }

  return (
    <div className="mb-4 grid grid-cols-2 overflow-hidden rounded-md bg-white md:grid-cols-4">
      {SESSION_METRICS.map((metric, index) => {
        const value = props.totals?.[metric.key as keyof ExploreTotals] as number | undefined
        const change = props.totals?.[
          `${metric.key}_change` as keyof ExploreTotals
        ] as number | undefined
        const tooltip = tooltips[metric.key]

        return (
          <div
            key={metric.key}
            className={index < SESSION_METRICS.length - 1 ? 'border-r border-gray-200 p-4' : 'p-4'}
          >
            <div className="mb-1 flex items-center gap-1 text-xs text-gray-500">
              {labels[metric.key] ?? metric.label}
              {tooltip ? (
                <Tooltip title={tooltip}>
                  <InfoCircleOutlined className="text-[10px]" />
                </Tooltip>
              ) : null}
            </div>
            {props.loading && !props.totals ? (
              <Skeleton active paragraph={false} title={{ width: '60%' }} />
            ) : (
              <div className="flex items-baseline gap-2">
                {metric.key === 'median_duration' ? (
                  <span
                    style={getHeatMapStyle(
                      value ?? 0,
                      props.bestValue,
                      TIMESCORE_REFERENCE_SECONDS,
                      10
                    )}
                  />
                ) : null}
                <span className="text-xl font-semibold text-gray-800">
                  {formatValue(value ?? 0, metric.format)}
                </span>
                {props.showComparison ? (
                  <Delta change={change} invertTrend={metric.invertTrend} decimals={1} />
                ) : null}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
