import { Key } from 'react'
import { Button, Empty, Table, Tooltip } from 'antd'
import { EyeOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { Loader2, SquareMinus, SquarePlus, TriangleAlert } from 'lucide-react'
import { i18n } from '@lingui/core'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from '../context'
import { Delta } from '../Delta'
import { formatDimensionValue, getDimensionLabel } from '../lib/dimensions'
import { canExpandRow, ExploreRow, ExploreTotals, isBestPathRow } from '../lib/exploreRows'
import { formatDuration, formatNumber } from '../lib/format'
import { getHeatMapStyle } from '../lib/heatmap'
import { TIMESCORE_REFERENCE_SECONDS } from '../lib/types'

/**
 * One box shared by every expand-icon state, including the blank one standing
 * in for rows that cannot expand.
 *
 * It has to be inline-flex rather than a plain span: `width` does not apply to
 * a non-replaced inline element, so the blank spacer collapsed to nothing and
 * leaf rows started a whole icon further left than expandable ones. The fixed
 * 16px box holds the 14px glyphs, and `align-middle` centres it on the line
 * rather than letting an inline SVG hang off the text baseline.
 */
const EXPAND_ICON_BOX = 'mr-2 inline-flex h-4 w-4 items-center justify-center align-middle'

interface ExploreTableProps {
  data: ExploreRow[]
  dimensions: string[]
  expandedRowKeys: Key[]
  onExpand: (expanded: boolean, record: ExploreRow) => void
  onExpandedRowsChange: (keys: Key[]) => void
  loadingRows: Set<string>
  /** Highest TimeScore across every combination; scales the heat map. */
  bestValue: number
  /** The winning row, whose path down the tree is marked. */
  bestCombination?: Record<string, unknown>
  loading?: boolean
  totals?: ExploreTotals
  onBreakdownClick: (row: ExploreRow) => void
  onBreakdownHover: (row: ExploreRow) => void
}

/**
 * The drill-down itself: one dimension per level, children fetched on expand.
 *
 * Every level is capped server-side, so there is no pagination — going deeper
 * is how you narrow the report, not paging through a long tail. Sorting is
 * client-side over what is loaded, which keeps a re-sort from re-querying
 * every open branch.
 */
export function ExploreTable(props: ExploreTableProps) {
  const { t } = useLingui()
  const context = useWebAnalytics()

  const renderDimension = (_: unknown, record: ExploreRow) => {
    const dimension = props.dimensions[record.dimensionIndex] ?? props.dimensions[0]
    const value = record[dimension]
    const isEmpty = value === null || value === '' || value === undefined
    const display = formatDimensionValue(dimension, value, {
      emptyLabel: t`(empty)`,
      locale: i18n.locale
    })

    return (
      <span className="whitespace-nowrap">
        <span className="text-xs text-gray-400">
          {getDimensionLabel(dimension, context.customDimensionLabels)}:
        </span>
        <span className={isEmpty ? 'ml-1 italic text-gray-400' : 'ml-1 font-medium'}>
          {display}
        </span>
      </span>
    )
  }

  // Not memoized: the column titles are translated, and `t` changes identity on
  // every render, so a memo would either be useless or freeze the headers in
  // the locale that was active when the table first mounted.
  const columns: ColumnsType<ExploreRow> = [
    {
      title: t`Dimension`,
      key: 'dimension',
      width: 300,
      render: renderDimension
    },
    {
      title: t`Sessions`,
      dataIndex: 'sessions',
      key: 'sessions',
      align: 'right',
      width: 160,
      sorter: (a, b) => a.sessions - b.sessions,
      defaultSortOrder: 'descend',
      render: (value: number, record) => {
        // Only the top level adds up to the period total; a child's share of
        // it would silently mean something different on every row.
        const share =
          record.dimensionIndex === 0 && props.totals && props.totals.sessions > 0
            ? (value / props.totals.sessions) * 100
            : null

        return (
          <div className="flex items-center justify-end gap-1">
            <span>{formatNumber(value)}</span>
            {share !== null ? (
              <span className="text-xs text-gray-400">({share.toFixed(1)}%)</span>
            ) : null}
            {context.showComparison ? (
              <Delta change={record.sessions_change as number | undefined} />
            ) : null}
          </div>
        )
      }
    },
    {
      title: (
        <Tooltip
          title={t`Median session duration. Green meets the reference, cyan exceeds it.`}
        >
          <span className="cursor-help border-b border-dotted border-gray-400">{t`TimeScore`}</span>
        </Tooltip>
      ),
      dataIndex: 'median_duration',
      key: 'median_duration',
      align: 'right',
      width: 150,
      sorter: (a, b) => a.median_duration - b.median_duration,
      render: (value: number, record) => (
        <div className="flex items-center justify-end gap-1.5">
          <span style={getHeatMapStyle(value, props.bestValue, TIMESCORE_REFERENCE_SECONDS)} />
          <span className="font-medium">{formatDuration(value)}</span>
          {context.showComparison ? (
            <Delta change={record.median_duration_change as number | undefined} />
          ) : null}
        </div>
      )
    },
    {
      title: t`Bounce Rate`,
      dataIndex: 'bounce_rate',
      key: 'bounce_rate',
      align: 'right',
      width: 130,
      sorter: (a, b) => a.bounce_rate - b.bounce_rate,
      render: (value: number, record) => (
        <div className="flex items-center justify-end gap-1">
          <span>{value.toFixed(1)}%</span>
          {context.showComparison ? (
            <Delta change={record.bounce_rate_change as number | undefined} invertTrend />
          ) : null}
        </div>
      )
    },
    {
      title: t`Median Scroll Depth`,
      dataIndex: 'median_scroll',
      key: 'median_scroll',
      align: 'right',
      width: 130,
      sorter: (a, b) => a.median_scroll - b.median_scroll,
      render: (value: number, record) => (
        <div className="flex items-center justify-end gap-1">
          <span>{value.toFixed(1)}%</span>
          {context.showComparison ? (
            <Delta change={record.median_scroll_change as number | undefined} />
          ) : null}
        </div>
      )
    },
    {
      title: '',
      key: 'actions',
      align: 'right',
      width: 48,
      render: (_: unknown, record: ExploreRow) => (
        <Tooltip title={t`View breakdown`}>
          <Button
            type="text"
            size="small"
            icon={<EyeOutlined />}
            onClick={(event) => {
              event.stopPropagation()
              props.onBreakdownClick(record)
            }}
            onMouseEnter={() => props.onBreakdownHover(record)}
          />
        </Tooltip>
      )
    }
  ]

  return (
    <div className="overflow-hidden rounded-md">
      <Table<ExploreRow>
        className="border border-gray-200 rounded-md"
        rowClassName={(record) =>
          isBestPathRow(record, props.dimensions, props.bestCombination) ? 'best-timescore-row' : ''
        }
        columns={columns}
        dataSource={props.data}
        rowKey="key"
        loading={props.loading}
        pagination={false}
        scroll={{ x: 'max-content' }}
        locale={{
          emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No data found`} />
        }}
        expandable={{
          childrenColumnName: 'children',
          expandedRowKeys: props.expandedRowKeys,
          expandRowByClick: true,
          onExpand: props.onExpand,
          onExpandedRowsChange: (keys) => props.onExpandedRowsChange([...keys]),
          rowExpandable: (record) => canExpandRow(record, props.dimensions),
          expandIcon: ({ expanded, record }) => {
            if (!canExpandRow(record, props.dimensions)) {
              return <span className={EXPAND_ICON_BOX} />
            }
            if (props.loadingRows.has(record.key)) {
              return (
                <span className={`${EXPAND_ICON_BOX} text-gray-400`}>
                  <Loader2 size={14} className="animate-spin" />
                </span>
              )
            }
            // An expanded row with nothing under it is almost always the
            // threshold at work, not missing data; saying so beats an empty
            // branch the operator cannot explain.
            if (expanded && record.childrenFilteredByMinSessions) {
              return (
                <Tooltip
                  title={t`Every sub-item has fewer than ${context.minSessions} sessions. Lower the threshold to see them.`}
                >
                  <span className={`${EXPAND_ICON_BOX} cursor-help text-amber-500`}>
                    <TriangleAlert size={14} />
                  </span>
                </Tooltip>
              )
            }
            return (
              <span className={`${EXPAND_ICON_BOX} text-gray-500`}>
                {expanded ? <SquareMinus size={14} /> : <SquarePlus size={14} />}
              </span>
            )
          }
        }}
      />
    </div>
  )
}
