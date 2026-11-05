import type { MessageHistoryStatusSum } from '../../services/api/messages_history'
import { trackedBase } from '../../lib/emailTracking'

/**
 * What the Variations table shows for one variation. Open and click rates are
 * shares of the emails that carried the pixel or tracked links; opens_tracked
 * and clicks_tracked are those totals, absent when the source predates them.
 */
export interface VariationRowMetrics {
  recipients: number
  opens: number
  clicks: number
  open_rate: number
  click_rate: number
  opens_tracked?: number
  clicks_tracked?: number
}

/**
 * Maps a variation's aggregate stats into the Variations table's metrics. Returns
 * undefined for an unsent variation (0 sent) so those rows stay N/A instead of
 * showing 0% / NaN.
 */
export function toVariationMetrics(s?: MessageHistoryStatusSum): VariationRowMetrics | undefined {
  if (!s || !s.total_sent) return undefined
  const opensTracked = trackedBase(s.total_sent, s.total_opens_tracked)
  const clicksTracked = trackedBase(s.total_sent, s.total_clicks_tracked)
  return {
    recipients: s.total_sent,
    opens: s.total_opened,
    open_rate: opensTracked ? s.total_opened / opensTracked : 0,
    opens_tracked: opensTracked,
    clicks: s.total_clicked,
    click_rate: clicksTracked ? s.total_clicked / clicksTracked : 0,
    clicks_tracked: clicksTracked
  }
}

/** Below this share of tracked recipients, a variation's rate is flagged as weak. */
export const LOW_TRACKED_SHARE = 0.5

export interface VariationRate {
  /** The rate as shown, '-' when no email of the variation was tracked. */
  text: string
  /** Emails the rate is computed over. */
  tracked: number
  /** Share of the variation's recipients that were tracked, when not all were. */
  trackedShare?: number
  /** True when under half the recipients were tracked. */
  lowTrackedShare: boolean
}

/** Describes an open or click rate cell of the Variations table. */
export function describeVariationRate(
  rate: number,
  recipients: number,
  tracked: number | undefined
): VariationRate {
  const base = trackedBase(recipients, tracked)
  if (!base) {
    return { text: '-', tracked: 0, trackedShare: recipients ? 0 : undefined, lowTrackedShare: recipients > 0 }
  }
  const partial = recipients > 0 && base < recipients
  const trackedShare = partial ? base / recipients : undefined
  return {
    text: `${(rate * 100).toFixed(1)}%`,
    tracked: base,
    trackedShare,
    lowTrackedShare: trackedShare !== undefined && trackedShare < LOW_TRACKED_SHARE
  }
}
