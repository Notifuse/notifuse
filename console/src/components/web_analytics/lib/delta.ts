import type { KpiTone } from '../../layout'

/** Whether a change is worth printing: a Delta renders nothing for zero, missing or NaN. */
export function hasChange(change: number | undefined): change is number {
  return change !== undefined && Number.isFinite(change) && change !== 0
}

/**
 * The tone of a change for `KpiStrip`'s `sub`: positive when it is an improvement, negative when it
 * is a deterioration, neutral when there is nothing to print. `invertTrend` is for a measure where
 * lower is better (bounce rate).
 */
export function deltaTone(change: number | undefined, invertTrend?: boolean): KpiTone {
  if (!hasChange(change)) return 'neutral'
  const better = invertTrend ? change < 0 : change > 0
  return better ? 'positive' : 'negative'
}
