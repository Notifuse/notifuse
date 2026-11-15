import { useNumberFormat } from './format'
import { FillBar } from './FillBar'

export interface RateBarProps {
  /** A ratio from 0 to 1, or null when there is none. */
  rate: number | null
  /** The ratio that fills the bar (1 by default; a screen passes the column's maximum). */
  scale?: number
  /** `partial`: incomplete data (sending in progress); `warning`: above a threshold. */
  tone?: 'default' | 'partial' | 'warning'
}

const TEXT = { default: 'text-ink', partial: 'text-ink-3', warning: 'text-warning-fg' } as const

/** A small bar and the rate it shows, for a column of rates. */
export function RateBar({ rate, scale = 1, tone = 'default' }: RateBarProps) {
  const { percent } = useNumberFormat()
  if (rate === null || Number.isNaN(rate)) return <span className="text-sm text-ink-3">—</span>
  return (
    <span className="inline-flex items-center gap-2">
      <FillBar ratio={scale > 0 ? rate / scale : 0} tone={tone} />
      <span className={`text-sm tabular-nums ${TEXT[tone]}`}>{percent(rate)}</span>
    </span>
  )
}
