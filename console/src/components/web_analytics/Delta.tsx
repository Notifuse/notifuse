import { ChevronDown, ChevronUp } from 'lucide-react'
import { deltaTone, hasChange } from './lib/delta'
import { formatPercent } from './lib/format'

interface DeltaProps {
  /** Percentage change; nothing is rendered at exactly zero. */
  change?: number
  /** Lower is better (bounce rate), so a drop reads as an improvement. */
  invertTrend?: boolean
  /** `text-2xs` instead of `text-xs`, for a tile or a dense cell. */
  small?: boolean
  decimals?: number
}

/**
 * Period-over-period change.
 *
 * The arrow always follows the raw sign — a number that went down points down
 * — while the colour follows whether that movement is good. Tying both to
 * "good/bad" would make a falling bounce rate render as a green up arrow,
 * which reads as growth.
 */
export function Delta({ change, invertTrend, small, decimals = 0 }: DeltaProps) {
  if (!hasChange(change)) return null

  const Icon = change > 0 ? ChevronUp : ChevronDown
  const tone = deltaTone(change, invertTrend) === 'positive' ? 'text-success-fg' : 'text-danger-fg'

  return (
    <span className={`inline-flex items-center font-medium ${small ? 'text-2xs' : 'text-xs'} ${tone}`}>
      <Icon size={small ? 11 : 12} />
      {formatPercent(Math.abs(change), decimals)}
    </span>
  )
}
