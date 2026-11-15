/** The fill colour per tone, written whole for Tailwind. */
const FILL = { default: 'bg-primary-400', partial: 'bg-primary-300', warning: 'bg-primary-400' } as const

export interface FillBarProps {
  /** How full the bar is, from 0 to 1; outside that range it is clamped, NaN draws it empty. */
  ratio: number
  tone?: keyof typeof FILL
  /** Its width: a fixed one in a column of rates, the room left in a row. */
  className?: string
}

/** A thin bar filled to a ratio: the mini bar of the rate columns and of the result rows. */
export function FillBar({ ratio, tone = 'default', className = 'w-6.5 shrink-0' }: FillBarProps) {
  const width = Number.isNaN(ratio) ? 0 : Math.min(1, Math.max(0, ratio)) * 100
  return (
    <span aria-hidden="true" className={`h-1 overflow-hidden rounded-xs bg-line-soft ${className}`}>
      <span
        data-rate-fill
        className={`block h-full ${FILL[tone]}`}
        style={{ width: `${Math.round(width * 10) / 10}%` }}
      />
    </span>
  )
}
