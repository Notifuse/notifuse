import { isPartiallyTracked } from '../../lib/emailTracking'
import type { KpiTone } from '../layout'

/** The values of the analytics origin dimension; other is a template test or a double opt-in confirmation. */
export type MessageOrigin = 'broadcast' | 'automation' | 'transactional' | 'other'
export const MESSAGE_ORIGINS: MessageOrigin[] = ['broadcast', 'transactional', 'automation', 'other']

/** The message_history measures the dashboard reads, per day or summed. */
export interface MessageTotals {
  sent: number
  delivered: number
  bounced: number
  complained: number
  opened: number
  clicked: number
  opensTracked: number
  clicksTracked: number
}

export interface SeriesPoint extends MessageTotals {
  /** YYYY-MM-DD, the workspace's local day. */
  day: string
}

/** Measure names in the analytics schema, by field. */
export const MEASURES: Record<keyof MessageTotals, string> = {
  sent: 'count_sent',
  delivered: 'count_delivered',
  bounced: 'count_bounced',
  complained: 'count_complained',
  opened: 'count_opened',
  clicked: 'count_clicked',
  opensTracked: 'count_opens_tracked',
  clicksTracked: 'count_clicks_tracked'
}

const FIELDS = Object.keys(MEASURES) as (keyof MessageTotals)[]

export function emptyTotals(): MessageTotals {
  return {
    sent: 0,
    delivered: 0,
    bounced: 0,
    complained: 0,
    opened: 0,
    clicked: 0,
    opensTracked: 0,
    clicksTracked: 0
  }
}

export function toNumber(value: unknown): number {
  if (typeof value === 'number') return Number.isFinite(value) ? value : 0
  if (typeof value === 'string') {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : 0
  }
  return 0
}

export function readTotals(row: Record<string, unknown> | undefined): MessageTotals {
  const totals = emptyTotals()
  if (!row) return totals
  for (const field of FIELDS) totals[field] = toNumber(row[MEASURES[field]])
  return totals
}

/**
 * Rows of a daily series. The engine names the bucket `created_at_day` and writes it
 * "YYYY-MM-DDT00:00:00Z", already the local day: the date is its first ten characters, with no
 * timezone conversion.
 */
export function parseSeries(rows: Record<string, unknown>[] | undefined): SeriesPoint[] {
  return (rows ?? [])
    .map((row) => ({
      day: String(row.created_at_day ?? '').slice(0, 10),
      ...readTotals(row)
    }))
    .filter((point) => /^\d{4}-\d{2}-\d{2}$/.test(point.day))
}

export function sumSeries(points: MessageTotals[]): MessageTotals {
  const sum = emptyTotals()
  for (const point of points) for (const field of FIELDS) sum[field] += point[field]
  return sum
}

const ratio = (numerator: number, denominator: number): number | null =>
  denominator > 0 ? numerator / denominator : null

/** Points of difference within which a rate counts as unchanged. */
const POINTS_BAND = 0.05
const BOUNCE_WARNING = 0.02
const COMPLAINT_WARNING = 0.001

export interface RateKpi {
  rate: number | null
  /** Current minus previous rate, in percentage points. */
  deltaPoints: number | null
  tone: KpiTone
  /** Emails the rate is computed over. */
  tracked: number
  partial: boolean
}

export interface DashboardKpis {
  contacts: { total: number; added: number; tone: KpiTone }
  sent: { total: number; broadcastShare: number | null }
  delivered: {
    rate: number | null
    bounceRate: number | null
    complaintRate: number | null
    tone: KpiTone
    /** Emails left but no provider reported a delivery: the webhooks are missing. */
    noDeliveryEvents: boolean
  }
  openRate: RateKpi
  clickRate: RateKpi
  visits: { value: number | null; deltaRatio: number | null; tone: KpiTone }
}

export interface DashboardKpiInput {
  series: Record<MessageOrigin, MessageTotals>
  previous?: MessageTotals
  contactsTotal?: number
  contactsNew?: number
  visits?: number
  previousVisits?: number
}

function rateKpi(
  numerator: number,
  tracked: number,
  sent: number,
  previous?: { numerator: number; tracked: number }
): RateKpi {
  const rate = ratio(numerator, tracked)
  const previousRate = previous ? ratio(previous.numerator, previous.tracked) : null
  const deltaPoints = rate !== null && previousRate !== null ? (rate - previousRate) * 100 : null
  const tone: KpiTone =
    deltaPoints === null
      ? 'neutral'
      : deltaPoints > POINTS_BAND
        ? 'positive'
        : deltaPoints < -POINTS_BAND
          ? 'negative'
          : 'neutral'
  return {
    rate,
    deltaPoints,
    tone,
    tracked,
    partial: isPartiallyTracked(sent, tracked)
  }
}

/** The six KPIs, as numbers and tones; the strip formats them in the console's language. */
export function computeDashboardKpis(input: DashboardKpiInput): DashboardKpis {
  const total = sumSeries(Object.values(input.series))
  const { previous } = input

  const added = input.contactsNew ?? 0
  const bounceRate = ratio(total.bounced, total.sent)
  const complaintRate = ratio(total.complained, total.sent)
  const noDeliveryEvents = total.sent > 0 && total.delivered === 0

  let visitsDelta: number | null = null
  if (input.visits !== undefined && input.previousVisits !== undefined && input.previousVisits > 0) {
    visitsDelta = (input.visits - input.previousVisits) / input.previousVisits
  }
  // The tone of the figure shown, one decimal of a percent: "+0.0 %" is never green.
  const shownDelta = visitsDelta === null ? 0 : Math.round(visitsDelta * 1000)

  return {
    contacts: {
      total: input.contactsTotal ?? 0,
      added,
      tone: added > 0 ? 'positive' : 'neutral'
    },
    sent: {
      total: total.sent,
      broadcastShare: ratio(input.series.broadcast.sent, total.sent)
    },
    delivered: {
      rate: noDeliveryEvents ? null : ratio(total.delivered, total.sent),
      bounceRate,
      complaintRate,
      tone: (bounceRate ?? 0) > BOUNCE_WARNING || (complaintRate ?? 0) > COMPLAINT_WARNING ? 'warning' : 'neutral',
      noDeliveryEvents
    },
    openRate: rateKpi(
      total.opened,
      total.opensTracked,
      total.sent,
      previous && {
        numerator: previous.opened,
        tracked: previous.opensTracked
      }
    ),
    clickRate: rateKpi(
      total.clicked,
      total.clicksTracked,
      total.sent,
      previous && {
        numerator: previous.clicked,
        tracked: previous.clicksTracked
      }
    ),
    visits: {
      value: input.visits ?? null,
      deltaRatio: visitsDelta,
      tone: shownDelta > 0 ? 'positive' : shownDelta < 0 ? 'negative' : 'neutral'
    }
  }
}
