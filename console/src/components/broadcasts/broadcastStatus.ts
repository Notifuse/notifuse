import type { Dayjs } from 'dayjs'
import dayjs from '../../lib/dayjs'
import type { Broadcast, BroadcastStatus, TestResultsResponse } from '../../services/api/broadcast'
import type { BroadcastStatsResult } from '../../services/api/messages_history'
import type { PillTone } from '../layout'

export interface ProgressStats {
  remaining: number
  processed: number
  enqueuedCount: number
  sentCount: number
  failedCount: number
  /**
   * Rows still on their way out: pending, processing, paused, or failed with attempts
   * left. Undefined when the queue could not be read — callers fall back to the
   * message-history arithmetic rather than claiming the campaign is done.
   */
  inFlight?: number
  /** Rows that spent every attempt. These recipients were given up on. */
  failedTerminal?: number
}

/** The progress of a broadcast from `messages.broadcastStats` and the task's enqueued count. */
export function progressStatsFrom(result: BroadcastStatsResult, enqueuedCount?: number): ProgressStats {
  const { stats, queue } = result
  const processed = stats.total_sent + stats.total_failed
  return {
    remaining: enqueuedCount ? Math.max(0, enqueuedCount - processed) : 0,
    processed,
    enqueuedCount: enqueuedCount ?? 0,
    sentCount: stats.total_sent,
    failedCount: stats.total_failed,
    inFlight: queue && queue.pending + queue.processing + queue.paused + queue.failed_retrying,
    failedTerminal: queue?.failed_terminal
  }
}

// Whether a campaign has finished, and whether it reached everyone, is a question for the
// queue. Message history cannot answer it: sent_at is stamped on the first attempt whatever
// its outcome. The counts are undefined when the queue could not be read: that is
// "unknown", not "finished", so the old arithmetic stands in.
export type DeliveryVerdict =
  | { kind: 'sending'; remaining: number }
  | { kind: 'complete' }
  | { kind: 'completedWithFailures'; failed: number }

export function deliveryVerdict(progressStats?: ProgressStats): DeliveryVerdict {
  if (progressStats?.inFlight === undefined) {
    const remaining = progressStats?.remaining ?? 0
    return remaining > 0 ? { kind: 'sending', remaining } : { kind: 'complete' }
  }
  if (progressStats.inFlight > 0) return { kind: 'sending', remaining: progressStats.inFlight }
  // How many were given up on comes from message history, not from the queue: abandoned
  // rows are swept after their retention window, and reading the verdict from them alone
  // would repaint a campaign green a week later.
  if (progressStats.failedCount > 0) return { kind: 'completedWithFailures', failed: progressStats.failedCount }
  return { kind: 'complete' }
}

/** Which label and tooltip the status pill shows; `BroadcastStatusPill` words them. */
export type StatusPillKind =
  | 'draft'
  | 'scheduled'
  | 'testing'
  | 'pickWinner'
  | 'preparing'
  | 'winnerSending'
  | 'paused'
  | 'sending'
  | 'sent'
  | 'someFailed'
  | 'cancelled'
  | 'failed'
  | 'unknown'

export interface StatusPillSpec {
  tone: PillTone
  pulse: boolean
  kind: StatusPillKind
  /** Recipients still to go (`sending`) or given up on (`someFailed`). */
  count?: number
}

export interface StatusPillContext {
  /** The verdict of a followed row whose queue is loaded. */
  verdict?: DeliveryVerdict
  /** `count_failed` of the row in the 30-day stats (L2), for rows without a verdict. */
  l2Failed?: number
}

const pill = (tone: PillTone, kind: StatusPillKind, pulse = false, count?: number): StatusPillSpec =>
  count === undefined ? { tone, pulse, kind } : { tone, pulse, kind, count }

export function statusPill(broadcast: Broadcast, { verdict, l2Failed }: StatusPillContext = {}): StatusPillSpec {
  switch (broadcast.status) {
    case 'draft':
      return pill('neutral', 'draft')
    case 'scheduled':
      return pill('info', 'scheduled')
    case 'testing':
      return pill('primary', 'testing', true)
    case 'test_completed':
      return pill('info', 'pickWinner')
    case 'processing':
      return pill('primary', 'preparing', true)
    case 'winner_selected':
      return pill('primary', 'winnerSending', true)
    case 'paused':
      return pill('warning', 'paused')
    case 'processed':
      if (verdict) {
        if (verdict.kind === 'sending') return pill('primary', 'sending', true, verdict.remaining)
        if (verdict.kind === 'completedWithFailures') return pill('warning', 'someFailed', false, verdict.failed)
        return pill('success', 'sent')
      }
      return l2Failed ? pill('warning', 'someFailed', false, l2Failed) : pill('success', 'sent')
    case 'cancelled':
      return pill('neutral', 'cancelled')
    case 'failed':
      return pill('danger', 'failed')
    default:
      return pill('neutral', 'unknown')
  }
}

const FOLLOWED_STATUSES: ReadonlySet<BroadcastStatus> = new Set([
  'testing',
  'test_completed',
  'processing',
  'winner_selected',
  'paused',
  'failed'
])

const FOLLOW_PROCESSED_DAYS = 3

/**
 * A row whose queue and task the list reads: in progress, or processed less than three days
 * ago (a later retry is unlikely). A processed row without `completed_at` may still be draining.
 */
export function isFollowedRow(broadcast: Broadcast, now: Dayjs = dayjs()): boolean {
  if (FOLLOWED_STATUSES.has(broadcast.status)) return true
  if (broadcast.status !== 'processed') return false
  if (!broadcast.completed_at) return true
  return now.diff(dayjs(broadcast.completed_at), 'day', true) < FOLLOW_PROCESSED_DAYS
}

/** A row whose numbers move by themselves: the list and its stats refetch while one is loaded. */
export function isLiveRow(broadcast: Broadcast, verdict?: DeliveryVerdict): boolean {
  if (broadcast.status === 'testing' || broadcast.status === 'processing' || broadcast.status === 'winner_selected') {
    return true
  }
  return broadcast.status === 'processed' && verdict?.kind === 'sending'
}

/**
 * The time left in an A/B test, as a duration alone ("2 hours"): the sentence around it is
 * a translated message of the caller. Null outside a running test or once it has ended.
 */
export function getRemainingTestTime(
  broadcast: Broadcast,
  testResults?: Pick<TestResultsResponse, 'test_started_at'>,
  now: Dayjs = dayjs()
): string | null {
  const hours = broadcast.test_settings?.test_duration_hours
  if (broadcast.status !== 'testing' || !broadcast.test_settings?.enabled || !hours) return null
  const start = testResults?.test_started_at || broadcast.test_sent_at
  if (!start) return null
  const end = dayjs(start).add(hours, 'hour')
  if (now.isAfter(end)) return null
  return now.to(end, true)
}

/** The lifecycle actions a broadcast offers, by status and delivery verdict (list and detail). */
export interface BroadcastActionSet {
  edit: boolean
  pause: boolean
  resume: boolean
  cancel: boolean
  /** Recipients given up on can be queued again. */
  retryFailed: boolean
  delete: boolean
}

export function availableActions(
  broadcast: Pick<Broadcast, 'status'>,
  verdict?: DeliveryVerdict,
  progress?: ProgressStats
): BroadcastActionSet {
  const { status } = broadcast
  const sendingNow = status === 'processing' || (status === 'processed' && verdict?.kind === 'sending')
  return {
    edit: status === 'draft' || status === 'scheduled',
    pause: sendingNow,
    resume: status === 'paused',
    cancel: sendingNow || status === 'scheduled' || status === 'paused',
    retryFailed:
      (status === 'processed' || status === 'failed') &&
      verdict?.kind === 'completedWithFailures' &&
      (progress?.failedTerminal ?? 0) > 0,
    delete: status === 'draft'
  }
}
