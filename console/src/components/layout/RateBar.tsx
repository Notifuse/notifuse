import { useNumberFormat } from './format'

export interface RateBarProps {
  /** A ratio from 0 to 1, or null when there is none. */
  rate: number | null
  /** The ratio that fills the bar (1 by default; a screen passes the column's maximum). */
  scale?: number
  /** `partial`: incomplete data (sending in progress); `warning`: above a threshold. */
  tone?: 'default' | 'partial' | 'warning'
}

const FILL = { default: 'bg-primary-400', partial: 'bg-primary-300', warning: 'bg-primary-400' } as const
const TEXT = { default: 'text-ink', partial: 'text-ink-3', warning: 'text-warning-fg' } as const

/** A small bar and the rate it shows, for a column of rates. */
export function RateBar({ rate, scale = 1, tone = 'default' }: RateBarProps) {
  const { percent } = useNumberFormat()
  if (rate === null || Number.isNaN(rate)) return <span className="text-sm text-ink-3">—</span>
  const width = scale > 0 ? Math.min(100, Math.max(0, (rate / scale) * 100)) : 0
  return (
    <span className="inline-flex items-center gap-2">
      <span aria-hidden="true" className="h-1 w-6.5 shrink-0 overflow-hidden rounded-xs bg-line-soft">
        <span data-rate-fill className={`block h-full ${FILL[tone]}`} style={{ width: `${Math.round(width * 10) / 10}%` }} />
      </span>
      <span className={`text-sm tabular-nums ${TEXT[tone]}`}>{percent(rate)}</span>
    </span>
  )
}
