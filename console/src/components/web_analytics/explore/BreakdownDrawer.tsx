import { Divider, Drawer, Space, Table, Tag } from 'antd'
import { useQueries } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../context'
import { formatDimensionValue, getDimensionLabel } from '../lib/dimensions'
import { ExploreRow } from '../lib/exploreRows'
import { formatDuration, formatNumber, toNumber } from '../lib/format'
import { getHeatMapStyle } from '../lib/heatmap'
import { buildWebQuery, webAnalyticsClient } from '../lib/query'
import {
  ResolvedRange,
  SESSION_METRIC_KEYS,
  TIMESCORE_REFERENCE_SECONDS,
  WebDimensionFilter,
  WebMetricFilter
} from '../lib/types'
import type { AnalyticsQuery, AnalyticsResponse } from '../../../services/api/analytics'

export interface BreakdownQueryParams {
  dimension: string
  filters: WebDimensionFilter[]
  metricFilters: WebMetricFilter[]
  minSessions: number
  range: ResolvedRange
  timezone: string
}

/**
 * One breakdown column's query. Shared with the hover prefetch on the table so
 * both produce the same cache key and the drawer opens on warm data.
 */
export function buildBreakdownQuery(params: BreakdownQueryParams): AnalyticsQuery {
  return buildWebQuery({
    schema: 'web_sessions',
    measures: SESSION_METRIC_KEYS,
    dimensions: [params.dimension],
    range: params.range,
    filters: params.filters,
    metricFilters: params.metricFilters,
    minSessions: params.minSessions,
    order: { sessions: 'desc' },
    limit: 100,
    timezone: params.timezone
  })
}

interface BreakdownDrawerProps {
  open: boolean
  onClose: () => void
  row: ExploreRow
  /** Dimensions to break the row down by, one small table each. */
  breakdownDimensions: string[]
  /** The row's own path, already turned into filters. */
  parentFilters: WebDimensionFilter[]
  dimensions: string[]
}

/**
 * Side-by-side profile of one row.
 *
 * The drill-down answers "what is inside this row" one dimension at a time and
 * in a fixed order; the breakdown answers "what characterises it" across
 * several dimensions at once, each queried independently.
 */
export function BreakdownDrawer(props: BreakdownDrawerProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()

  const dimension = props.dimensions[props.row.dimensionIndex]
  const rawValue = props.row[dimension]
  const title =
    rawValue === null || rawValue === '' || rawValue === undefined
      ? t`(empty)`
      : String(rawValue)

  const results = useQueries({
    queries: props.breakdownDimensions.map((breakdownDimension) => {
      const query = buildBreakdownQuery({
        dimension: breakdownDimension,
        filters: props.parentFilters,
        metricFilters: context.metricFilters,
        minSessions: context.minSessions,
        range: context.resolved,
        timezone: context.timezone
      })
      return {
        queryKey: ['web-analytics', context.workspaceId, query],
        queryFn: () => webAnalyticsClient.query(query, context.workspaceId),
        enabled: props.open
      }
    })
  })

  return (
    <Drawer
      open={props.open}
      onClose={props.onClose}
      placement="right"
      size="100%"
      title={
        <div className="flex items-center gap-2">
          <span className="text-gray-400">
            {getDimensionLabel(dimension, context.customDimensionLabels)}:
          </span>
          <span className="font-medium">{title}</span>
        </div>
      }
    >
      <div className="rounded-lg bg-gray-50 p-3 text-sm text-gray-600">
        <div className="mb-2">
          <span className="font-medium">{t`Filters:`}</span>{' '}
          {props.parentFilters.length === 0 ? (
            <span className="text-gray-400">{t`None`}</span>
          ) : (
            <Space size={[4, 4]} wrap>
              {props.parentFilters.map((filter, index) => (
                <Tag key={`${filter.dimension}-${index}`} className="!m-0">
                  {getDimensionLabel(filter.dimension, context.customDimensionLabels)}{' '}
                  {filter.values.length > 0 ? filter.values.join(', ') : t`(empty)`}
                </Tag>
              ))}
            </Space>
          )}
        </div>
        <div>
          <span className="font-medium">{t`Breaking down by:`}</span>{' '}
          <Space size={[4, 4]} wrap>
            {props.breakdownDimensions.map((entry) => (
              <Tag key={entry} color="blue" className="!m-0">
                {getDimensionLabel(entry, context.customDimensionLabels)}
              </Tag>
            ))}
          </Space>
        </div>
      </div>

      <Divider className="!my-4" />

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {props.breakdownDimensions.map((entry, index) => (
          <BreakdownTable
            key={entry}
            dimension={entry}
            data={results[index]?.data}
            loading={results[index]?.isLoading ?? false}
            error={results[index]?.error ?? null}
          />
        ))}
      </div>
    </Drawer>
  )
}

interface BreakdownTableProps {
  dimension: string
  data?: AnalyticsResponse
  loading: boolean
  error: Error | null
}

function BreakdownTable(props: BreakdownTableProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()

  const rows = props.data?.data ?? []
  const best = rows.reduce((max, row) => Math.max(max, toNumber(row.median_duration)), 0)

  if (props.error) {
    return (
      <div className="rounded bg-red-50 p-4 text-sm text-red-600">{t`Could not load this breakdown`}</div>
    )
  }

  return (
    <Table<Record<string, unknown>>
      size="small"
      loading={props.loading}
      dataSource={rows}
      rowKey={(row, index) => `${index ?? 0}:${String(row[props.dimension] ?? '')}`}
      pagination={{ pageSize: 10, size: 'small', showSizeChanger: false, hideOnSinglePage: true }}
      columns={[
        {
          title: getDimensionLabel(props.dimension, context.customDimensionLabels),
          dataIndex: props.dimension,
          ellipsis: true,
          render: (value: unknown) => {
            if (value === null || value === '' || value === undefined) {
              return <span className="text-gray-400">{t`(empty)`}</span>
            }
            return formatDimensionValue(props.dimension, value, {
              emptyLabel: t`(empty)`,
              locale: i18n.locale
            })
          }
        },
        {
          title: t`Sessions`,
          dataIndex: 'sessions',
          width: 80,
          align: 'right',
          render: (value: unknown) => formatNumber(toNumber(value))
        },
        {
          title: t`TimeScore`,
          dataIndex: 'median_duration',
          width: 100,
          align: 'right',
          render: (value: unknown) => (
            <div className="flex items-center justify-end gap-1.5">
              <span
                style={getHeatMapStyle(toNumber(value), best, TIMESCORE_REFERENCE_SECONDS)}
              />
              <span>{formatDuration(toNumber(value))}</span>
            </div>
          )
        },
        {
          title: t`Bounce`,
          dataIndex: 'bounce_rate',
          width: 70,
          align: 'right',
          render: (value: unknown) => `${toNumber(value).toFixed(1)}%`
        }
      ]}
    />
  )
}
