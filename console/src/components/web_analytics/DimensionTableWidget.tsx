import { ReactNode, useMemo, useState } from 'react'
import { Drawer, Empty, Pagination, Tooltip } from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { ArrowDown, ArrowUp, Maximize2 } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from './context'
import { CountryMapView } from './CountryMapView'
import { Delta } from './Delta'
import { formatValue } from './lib/format'
import { WidgetTabs } from './WidgetTabs'
import { mergeWidgetFilters } from './lib/dimensions'
import { getHeatMapStyle } from './lib/heatmap'
import {
  buildWebQuery,
  mergeComparisonRows,
  useWebComparisonQuery,
  webAnalyticsLiveClient
} from './lib/query'
import {
  DimensionRow,
  ResolvedRange,
  TIMESCORE_REFERENCE_SECONDS,
  ValueFormat,
  WebDimensionFilter,
  WebSchema
} from './lib/types'

export interface ColumnConfig {
  key: string
  label: string
  format: ValueFormat
  currency?: string
  /** Renders a colour dot scaled against the best value in the table. */
  heatMap?: boolean
}

export interface DimensionTabConfig {
  key: string
  label: string
  /** Header of the first column, e.g. "Page" or "Referrer". */
  dimensionLabel: string
  dimension: string
  schema?: WebSchema
  measures?: string[]
  /** Widget-specific filters, merged on top of the global ones. */
  filters?: WebDimensionFilter[]
  order?: Record<string, 'asc' | 'desc'>
  limit?: number
  type?: 'table' | 'country_map'
}

/**
 * Turns a widget into a live one: it reads a rolling window measured on last
 * activity instead of the page's period, and polls. Set only by the live view.
 */
export interface LiveOverride {
  range: ResolvedRange
  /** Dimension the window applies to, e.g. `updated_at` for "still here". */
  timeDimension: string
  refetchInterval: number
}

interface DimensionTableWidgetProps {
  title: string
  infoTooltip?: string
  tabs: DimensionTabConfig[]
  columns?: ColumnConfig[]
  iconPrefix?: (value: string, tabKey: string) => ReactNode
  emptyText?: string
  live?: LiveOverride
}

const VISIBLE_ROWS = 7
const EXPANDED_ROWS = 200
const DRAWER_PAGE_SIZE = 20

/**
 * A card showing the top values of one dimension, with tabs to swap which
 * dimension. Clicking a row filters the whole page by that value, which is how
 * the dashboard drills down without a dedicated report builder.
 */
export function DimensionTableWidget(props: DimensionTableWidgetProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()
  const [activeTabKey, setActiveTabKey] = useState(props.tabs[0].key)
  const [expanded, setExpanded] = useState(false)

  const tab = props.tabs.find((candidate) => candidate.key === activeTabKey) ?? props.tabs[0]
  const columns: ColumnConfig[] = props.columns ?? [
    { key: 'sessions', label: t`Sessions`, format: 'number' },
    { key: 'median_duration', label: t`TimeScore`, format: 'duration', heatMap: true }
  ]

  const [sortBy, setSortBy] = useState(columns[0].key)
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('desc')

  const isMap = tab.type === 'country_map'
  const limit = isMap ? (tab.limit ?? 100) : expanded ? EXPANDED_ROWS : (tab.limit ?? VISIBLE_ROWS)

  const rows = useDimensionRows(tab, columns, {
    order: isMap ? tab.order : { [sortBy]: sortDirection },
    limit,
    live: props.live
  })

  const toggleSort = (key: string) => {
    if (key === sortBy) {
      setSortDirection((current) => (current === 'desc' ? 'asc' : 'desc'))
      return
    }
    setSortBy(key)
    setSortDirection('desc')
  }

  const header = (
    <>
      <div className="flex items-start justify-between gap-2 px-4 pb-4 pt-4">
        <h3 className="flex items-center gap-1 text-base font-semibold text-gray-900">
          {props.title}
          {props.infoTooltip ? (
            <Tooltip title={props.infoTooltip}>
              <InfoCircleOutlined className="text-xs text-gray-400" />
            </Tooltip>
          ) : null}
        </h3>
        {!isMap ? (
          <Tooltip title={t`Expand`}>
            <button
              type="button"
              onClick={() => setExpanded(true)}
              className="rounded p-1 text-gray-400 hover:bg-gray-50 hover:text-gray-700"
            >
              <Maximize2 size={14} />
            </button>
          </Tooltip>
        ) : null}
      </div>
      <WidgetTabs tabs={props.tabs} activeKey={activeTabKey} onChange={setActiveTabKey} />
    </>
  )

  return (
    <div className="overflow-hidden rounded-md border border-gray-200">
      {header}
      {isMap ? (
        <div className="px-4 pb-4">
          <CountryMapView
            data={rows.data}
            metric={columns[0].key}
            loading={rows.isLoading}
            onSelect={(iso2) =>
              context.toggleFilter({ dimension: tab.dimension, operator: 'equals', values: [iso2] })
            }
          />
        </div>
      ) : (
        <DimensionRowsTable
          tab={tab}
          columns={columns}
          rows={rows.data.slice(0, VISIBLE_ROWS)}
          loading={rows.isLoading}
          sortBy={sortBy}
          sortDirection={sortDirection}
          onSort={toggleSort}
          iconPrefix={props.iconPrefix}
          emptyText={props.emptyText}
        />
      )}

      <Drawer
        open={expanded}
        onClose={() => setExpanded(false)}
        size={640}
        title={
          <div className="flex items-center gap-2">
            {props.title}
            <Tooltip title={t`Limited to the top ${EXPANDED_ROWS} values for performance`}>
              <InfoCircleOutlined className="text-xs text-gray-400" />
            </Tooltip>
          </div>
        }
      >
        <WidgetTabs
          className="-mx-6 mb-4"
          tabs={props.tabs.filter((candidate) => candidate.type !== 'country_map')}
          activeKey={activeTabKey}
          onChange={setActiveTabKey}
        />
        <ExpandedRows
          tab={tab}
          columns={columns}
          rows={rows.data}
          loading={rows.isLoading}
          sortBy={sortBy}
          sortDirection={sortDirection}
          onSort={toggleSort}
          iconPrefix={props.iconPrefix}
          emptyText={props.emptyText}
        />
      </Drawer>
    </div>
  )
}

/** Runs the tab's breakdown over the current range and, if set, the comparison. */
function useDimensionRows(
  tab: DimensionTabConfig,
  columns: ColumnConfig[],
  options: { order?: Record<string, 'asc' | 'desc'>; limit: number; live?: LiveOverride }
) {
  const context = useWebAnalytics()
  const schema = tab.schema ?? 'web_sessions'
  const measures = tab.measures ?? columns.map((column) => column.key)

  const filters = useMemo(
    () => mergeWidgetFilters(context.filters, tab.filters, schema),
    [context.filters, tab.filters, schema]
  )

  const base = {
    schema,
    measures,
    dimensions: [tab.dimension],
    filters,
    order: options.order,
    limit: options.limit,
    timezone: context.timezone,
    timeDimension: options.live?.timeDimension
  }

  const current = buildWebQuery({ ...base, range: options.live?.range ?? context.resolved })
  // "Now" has no period to be compared against, so a live widget never asks
  // for one however the page's comparison toggle is set.
  const previous =
    !options.live && context.showComparison && context.resolvedCompare
      ? buildWebQuery({ ...base, range: context.resolvedCompare })
      : null

  const result = useWebComparisonQuery(context.workspaceId, current, previous, {
    refetchInterval: options.live?.refetchInterval,
    client: options.live ? webAnalyticsLiveClient : undefined
  })
  const currentRows = result.current?.data
  const previousRows = result.previous?.data

  const data = useMemo(
    () => mergeComparisonRows(currentRows ?? [], previousRows, tab.dimension, measures),
    [currentRows, previousRows, tab.dimension, measures]
  )

  return { data, isLoading: result.isLoading, isFetching: result.isFetching }
}

interface RowsTableProps {
  tab: DimensionTabConfig
  columns: ColumnConfig[]
  rows: DimensionRow[]
  loading?: boolean
  sortBy: string
  sortDirection: 'asc' | 'desc'
  onSort: (key: string) => void
  iconPrefix?: (value: string, tabKey: string) => ReactNode
  emptyText?: string
}

function DimensionRowsTable(props: RowsTableProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()

  const best = useMemo(() => {
    const heatColumn = props.columns.find((column) => column.heatMap)
    if (!heatColumn) return 0
    return props.rows.reduce((max, row) => Math.max(max, Number(row[heatColumn.key] ?? 0)), 0)
  }, [props.rows, props.columns])

  const leadValue = Number(props.rows[0]?.[props.sortBy] ?? 0)

  if (!props.loading && props.rows.length === 0) {
    return (
      <div className="px-4 pb-6">
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={props.emptyText ?? t`No data available`}
        />
      </div>
    )
  }

  return (
    <table className="w-full table-fixed">
      <thead>
        <tr className="h-[46px] border-b border-gray-100 text-xs text-gray-500">
          <th className="px-4 text-left font-normal">{props.tab.dimensionLabel}</th>
          {props.columns.map((column) => (
            <th key={column.key} className="w-24 px-2 text-right font-normal md:w-32">
              <button
                type="button"
                onClick={() => props.onSort(column.key)}
                className="inline-flex items-center gap-1 hover:text-gray-800"
              >
                {column.label}
                {props.sortBy === column.key ? (
                  props.sortDirection === 'desc' ? (
                    <ArrowDown size={12} className="text-[var(--primary)]" />
                  ) : (
                    <ArrowUp size={12} className="text-[var(--primary)]" />
                  )
                ) : (
                  <ArrowDown size={12} className="opacity-0" />
                )}
              </button>
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {props.loading && props.rows.length === 0
          ? Array.from({ length: 5 }, (_, index) => (
              <tr key={index} className="h-9">
                <td className="px-4" colSpan={props.columns.length + 1}>
                  <div className="h-3 animate-pulse rounded bg-gray-100" />
                </td>
              </tr>
            ))
          : props.rows.map((row) => (
              <tr
                key={row.dimension_value}
                onClick={() =>
                  context.toggleFilter({
                    dimension: props.tab.dimension,
                    operator: 'equals',
                    values: [row.dimension_value]
                  })
                }
                className="h-9 cursor-pointer border-b border-gray-50 text-sm hover:bg-gray-50/60"
              >
                <td className="relative px-4">
                  <div
                    aria-hidden
                    className="absolute inset-y-1 left-2 rounded bg-[var(--primary)] opacity-[0.06]"
                    style={{
                      width: leadValue
                        ? `${Math.max((Number(row[props.sortBy] ?? 0) / leadValue) * 100, 2)}%`
                        : '0'
                    }}
                  />
                  <div className="relative flex items-center gap-2 truncate">
                    {props.iconPrefix?.(row.dimension_value, props.tab.key)}
                    <span className="truncate text-xs text-gray-700" title={row.dimension_value}>
                      {row.dimension_value || t`(empty)`}
                    </span>
                  </div>
                </td>
                {props.columns.map((column) => (
                  <td key={column.key} className="px-2 text-right text-xs text-gray-700">
                    <div className="flex items-center justify-end gap-1">
                      {column.heatMap ? (
                        <span
                          style={getHeatMapStyle(
                            Number(row[column.key] ?? 0),
                            best,
                            TIMESCORE_REFERENCE_SECONDS
                          )}
                        />
                      ) : null}
                      <span>
                        {column.format === 'currency' && Number(row[column.key] ?? 0) === 0
                          ? '-'
                          : formatValue(
                              Number(row[column.key] ?? 0),
                              column.format,
                              column.currency
                            )}
                      </span>
                      {context.showComparison ? (
                        <Delta change={row[`${column.key}_change`] as number | undefined} />
                      ) : null}
                    </div>
                  </td>
                ))}
              </tr>
            ))}
      </tbody>
    </table>
  )
}

/** The drawer's paginated view of the same rows. */
function ExpandedRows(props: RowsTableProps) {
  const [page, setPage] = useState(1)
  const start = (page - 1) * DRAWER_PAGE_SIZE
  const visible = props.rows.slice(start, start + DRAWER_PAGE_SIZE)

  return (
    <>
      <DimensionRowsTable {...props} rows={visible} />
      {props.rows.length > DRAWER_PAGE_SIZE ? (
        <div className="mt-4 flex justify-end">
          <Pagination
            size="small"
            current={page}
            pageSize={DRAWER_PAGE_SIZE}
            total={props.rows.length}
            showSizeChanger={false}
            onChange={setPage}
          />
        </div>
      ) : null}
    </>
  )
}
