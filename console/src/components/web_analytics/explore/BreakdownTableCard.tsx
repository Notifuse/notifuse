import { useState } from 'react'
import { Button, Skeleton, Table } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import { Panel, useNumberFormat } from '../../layout'
import { formatDimensionValue } from '../lib/dimensions'
import { formatDuration, formatNumber, formatPercent, toNumber } from '../lib/format'
import { getHeatMapStyle } from '../lib/heatmap'
import { TIMESCORE_REFERENCE_SECONDS } from '../lib/types'
import { pageFooter, shareOf } from './breakdownRows'

const PAGE_SIZE = 10

type BreakdownRow = Record<string, unknown>

export interface BreakdownTableCardProps {
  dimension: string
  /** The dimension's name, in the workspace's words. */
  label: string
  rows?: BreakdownRow[]
  loading: boolean
  error: boolean
  onRetry: () => void
  /** Sessions of the row broken down: the base of the shares. */
  base: number
  minSessions: number
}

/** One dimension of a breakdown: its values whole, their share of the row, sortable, 10 a page. */
export function BreakdownTableCard(props: BreakdownTableCardProps) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()
  const rows = props.rows ?? []
  // The order and page the table shows, for the footer: antd sorts and pages internally.
  const [view, setView] = useState<{ source: BreakdownRow[]; sorted: BreakdownRow[]; page: number }>({
    source: rows,
    sorted: rows,
    page: 1
  })
  const current = view.source === rows ? view : { source: rows, sorted: rows, page: 1 }

  const best = rows.reduce((max, row) => Math.max(max, toNumber(row.median_duration)), 0)
  // Each row's position, as its key: a null and an empty value would share one.
  const positions = new Map(rows.map((row, index) => [row, index]))

  const state = props.error ? t`error` : props.loading ? t`loading` : rows.length === 0 ? t`empty` : null
  const title = (
    <span className="inline-flex items-baseline gap-1">
      {props.label}
      {state ? <span className="text-sm font-normal text-ink-3">· {state}</span> : null}
    </span>
  )
  const count = rows.length
  const extra =
    !state && count > 0 ? (
      <span className="text-xs text-ink-3">{plural(count, { one: '# value', other: '# values' })}</span>
    ) : null

  if (props.error) {
    return (
      <Panel title={title} tone="danger">
        <div data-breakdown-error className="flex items-center justify-between gap-3 text-sm text-danger-fg">
          <span>{t`Could not load this breakdown`}</span>
          <Button size="small" onClick={props.onRetry}>
            {t`Retry`}
          </Button>
        </div>
      </Panel>
    )
  }

  if (props.loading && rows.length === 0) {
    return (
      <Panel title={title}>
        <Skeleton active title={false} paragraph={{ rows: 3 }} />
      </Panel>
    )
  }

  if (rows.length === 0) {
    return (
      <Panel title={title}>
        <p className="m-0 py-3 text-center text-sm text-ink-3">
          {props.minSessions > 1
            ? t`No session above the minimum for this dimension`
            : t`No session for this dimension`}
        </p>
      </Panel>
    )
  }

  const footer = pageFooter(
    current.sorted.map((row) => ({ sessions: toNumber(row.sessions) })),
    current.page,
    PAGE_SIZE,
    props.base
  )
  // Exact counts: the two must read as parts of one another.
  const onPage = int(footer.onPage)
  const total = int(footer.base)

  const columns: ColumnsType<BreakdownRow> = [
    {
      title: props.label,
      dataIndex: props.dimension,
      key: 'value',
      render: (value: unknown, row) => {
        const share = shareOf(toNumber(row.sessions), props.base)
        const empty = value === null || value === '' || value === undefined
        return (
          <div className="relative">
            {/* The row's share of the one broken down, as on the dashboard tables. */}
            <div
              aria-hidden
              className="absolute inset-y-0 -left-1.5 rounded bg-primary opacity-[0.06]"
              style={{ width: `${Math.max(Math.min(share, 100), 2)}%` }}
            />
            <span className={`relative break-all ${empty ? 'text-ink-4' : 'text-ink'}`}>
              {formatDimensionValue(props.dimension, value, { emptyLabel: t`(empty)`, locale: i18n.locale })}
            </span>
          </div>
        )
      }
    },
    {
      title: t`Sessions`,
      dataIndex: 'sessions',
      key: 'sessions',
      align: 'right',
      width: 90,
      sorter: (a, b) => toNumber(a.sessions) - toNumber(b.sessions),
      render: (value: unknown) => formatNumber(toNumber(value))
    },
    {
      title: t`Share`,
      key: 'share',
      align: 'right',
      width: 70,
      sorter: (a, b) => toNumber(a.sessions) - toNumber(b.sessions),
      render: (_: unknown, row) => formatPercent(shareOf(toNumber(row.sessions), props.base))
    },
    {
      title: t`TimeScore`,
      dataIndex: 'median_duration',
      key: 'median_duration',
      align: 'right',
      width: 110,
      sorter: (a, b) => toNumber(a.median_duration) - toNumber(b.median_duration),
      render: (value: unknown) => (
        <div className="flex items-center justify-end gap-1.5">
          <span style={getHeatMapStyle(toNumber(value), best, TIMESCORE_REFERENCE_SECONDS)} />
          <span className="whitespace-nowrap">{formatDuration(toNumber(value))}</span>
        </div>
      )
    },
    {
      title: t`Bounce`,
      dataIndex: 'bounce_rate',
      key: 'bounce_rate',
      align: 'right',
      width: 75,
      sorter: (a, b) => toNumber(a.bounce_rate) - toNumber(b.bounce_rate),
      render: (value: unknown) => formatPercent(toNumber(value))
    },
    {
      title: t`Scroll`,
      dataIndex: 'median_scroll',
      key: 'median_scroll',
      align: 'right',
      width: 70,
      sorter: (a, b) => toNumber(a.median_scroll) - toNumber(b.median_scroll),
      render: (value: unknown) => formatPercent(toNumber(value))
    }
  ]

  return (
    <Panel
      title={title}
      extra={extra}
      flush
      footer={
        <span data-breakdown-footer>
          {footer.paginated
            ? t`${onPage} of ${total} sessions on this page`
            : t`${onPage} of ${total} sessions`}
        </span>
      }
    >
      <div className="overflow-x-auto">
        <Table<BreakdownRow>
          size="small"
          dataSource={rows}
          rowKey={(row) => `${positions.get(row) ?? 0}:${String(row[props.dimension] ?? '')}`}
          columns={columns}
          pagination={{
            current: current.page,
            pageSize: PAGE_SIZE,
            size: 'small',
            showSizeChanger: false,
            hideOnSinglePage: true
          }}
          onChange={(pagination, _filters, _sorter, extra) =>
            setView({ source: rows, sorted: extra.currentDataSource, page: pagination.current ?? 1 })
          }
        />
      </div>
    </Panel>
  )
}
