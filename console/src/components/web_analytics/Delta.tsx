import { ChevronDown, ChevronUp } from 'lucide-react'
import { NEGATIVE_COLOR, POSITIVE_COLOR } from './lib/types'

interface DeltaProps {
  /** Percentage change; nothing is rendered at exactly zero. */
  change?: number
  /** Lower is better (bounce rate), so a drop reads as an improvement. */
  invertTrend?: boolean
  size?: number
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
export function Delta({ change, invertTrend, size = 12, decimals = 0 }: DeltaProps) {
  if (change === undefined || !Number.isFinite(change) || change === 0) return null

  const better = invertTrend ? change <= 0 : change >= 0
  const Icon = change >= 0 ? ChevronUp : ChevronDown

  return (
    <span
      className="inline-flex items-center font-medium"
      style={{ color: better ? POSITIVE_COLOR : NEGATIVE_COLOR, fontSize: size }}
    >
      <Icon size={size} />
      {Math.abs(change).toFixed(decimals)}%
    </span>
  )
}
