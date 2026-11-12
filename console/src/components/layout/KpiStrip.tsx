import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Skeleton, Tooltip } from 'antd'
import { ArrowRight } from 'lucide-react'

export type KpiTone = 'neutral' | 'positive' | 'negative' | 'warning'

export interface KpiItem {
  key: string
  label: ReactNode
  /** Already formatted by the screen (format.ts). */
  value: ReactNode
  sub?: ReactNode
  /** Colour of `sub`. */
  tone?: KpiTone
  /** A slot right of the value for a sparkline; the strip does not draw it. */
  trend?: ReactNode
  /** A tooltip on the value ("partial tracking"). */
  hint?: ReactNode
  /** Makes the cell a toggle button. */
  onClick?: () => void
  active?: boolean
}

export interface KpiStripProps {
  items: KpiItem[]
  /** An arrow on each separator, for a funnel. */
  connector?: 'arrow'
  /** An extra column on the right, on the subtle background. */
  aside?: ReactNode
  loading?: boolean
  /** Neither border nor shadow, for a strip inside a card or a header. */
  embedded?: boolean
}

/** Below this width a cell would cramp its value, and the strip wraps onto two rows. */
const MIN_CELL_WIDTH = 140

// Whole class names: Tailwind only generates the classes it finds written out in the source.
const SUB_TONE: Record<KpiTone, string> = {
  neutral: 'text-ink-3',
  positive: 'text-success-fg',
  negative: 'text-danger-fg',
  warning: 'text-warning-fg'
}

/**
 * Whether the strip wraps, from its own width. The aside counts as one more cell, so the decision
 * depends only on the strip's width and never on the layout it produces.
 */
function useWrapped(cells: number) {
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)

  useEffect(() => {
    const element = ref.current
    if (!element) return
    const observer = new ResizeObserver((entries) => {
      const entry = entries[0]
      if (entry) setWidth(entry.contentRect.width)
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  // Before the first measurement (and in jsdom) the strip is laid out on one row.
  const wrapped = width > 0 && width / cells < MIN_CELL_WIDTH
  return { ref, wrapped }
}

/** A row of KPIs, clickable or linked by arrows, that wraps onto two rows when narrow. */
export function KpiStrip({ items, connector, aside, loading, embedded }: KpiStripProps) {
  const count = items.length
  const { ref, wrapped } = useWrapped(count + (aside ? 1 : 0))
  const columns = wrapped ? Math.ceil(count / 2) : count
  const remainder = count % columns
  const arrows = connector === 'arrow' && !wrapped

  return (
    <div
      ref={ref}
      data-kpi-strip
      className={`flex gap-px overflow-hidden rounded-lg bg-line-soft ${wrapped ? 'flex-col' : ''} ${
        embedded ? '' : 'border border-line shadow-card'
      }`}
    >
      <div
        data-kpi-grid
        className="grid min-w-0 flex-1 gap-px"
        style={{ gridTemplateColumns: `repeat(${Math.max(columns, 1)}, minmax(0, 1fr))` }}
      >
        {items.map((item, index) => {
          const isLast = index === count - 1
          // The last cell takes the empty columns of the last row rather than leaving a grey gap.
          const span = wrapped && isLast && remainder !== 0 ? columns - remainder + 1 : undefined
          return (
            <KpiCell
              key={item.key}
              item={item}
              loading={loading}
              arrow={arrows && index > 0}
              gridColumn={span ? `span ${span} / span ${span}` : undefined}
            />
          )
        })}
      </div>
      {aside ? (
        <div data-kpi-aside className={`bg-subtle px-3.5 py-2 ${wrapped ? '' : 'shrink-0'}`}>
          {aside}
        </div>
      ) : null}
    </div>
  )
}

interface KpiCellProps {
  item: KpiItem
  loading?: boolean
  arrow: boolean
  gridColumn?: string
}

function KpiCell({ item, loading, arrow, gridColumn }: KpiCellProps) {
  const value = <span className="text-kpi font-semibold text-ink tabular-nums">{item.value}</span>

  const content = (
    <>
      {arrow ? (
        <span
          data-kpi-arrow
          aria-hidden="true"
          className="absolute top-1/2 -left-2 z-10 flex size-4 -translate-y-1/2 items-center justify-center rounded-full bg-surface text-ink-4"
        >
          <ArrowRight size={12} />
        </span>
      ) : null}
      <span className="mb-1 block truncate text-xs text-ink-3">{item.label}</span>
      <span className="flex min-h-6 items-end justify-between gap-2">
        {loading ? (
          <span className="flex">
            <Skeleton.Input active size="small" />
          </span>
        ) : item.hint ? (
          <Tooltip title={item.hint}>{value}</Tooltip>
        ) : (
          value
        )}
        {item.trend && !loading ? <span className="flex shrink-0 items-end">{item.trend}</span> : null}
      </span>
      {item.sub ? (
        <span className={`block truncate text-xs ${SUB_TONE[item.tone ?? 'neutral']}`}>{item.sub}</span>
      ) : null}
      {item.active ? (
        <span
          data-kpi-active-bar
          aria-hidden="true"
          className="absolute inset-x-0 bottom-0 h-0.5 bg-primary-500"
        />
      ) : null}
    </>
  )

  const base = 'relative flex min-w-0 flex-col px-3.5 py-2 text-left'

  if (item.onClick) {
    return (
      <button
        type="button"
        data-kpi-cell
        aria-pressed={Boolean(item.active)}
        onClick={item.onClick}
        style={{ gridColumn }}
        className={`${base} cursor-pointer border-0 focus-visible:z-20 focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none ${
          item.active ? 'bg-primary-50' : 'bg-surface hover:bg-fill'
        }`}
      >
        {content}
      </button>
    )
  }

  return (
    <div data-kpi-cell style={{ gridColumn }} className={`${base} bg-surface`}>
      {content}
    </div>
  )
}
