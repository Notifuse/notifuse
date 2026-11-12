import { describe, it, expect } from 'vitest'
import dayjs from '../../lib/dayjs'
import {
  availableActions,
  deliveryVerdict,
  progressStatsFrom,
  statusPill,
  isFollowedRow,
  isLiveRow,
  getRemainingTestTime,
  type ProgressStats
} from './broadcastStatus'
import type { Broadcast } from '../../services/api/broadcast'
import type { BroadcastStatsResult } from '../../services/api/messages_history'

const broadcast = (over: Partial<Broadcast>): Broadcast =>
  ({
    id: 'bc-1',
    name: 'Newsletter',
    status: 'draft',
    test_settings: { enabled: false, sample_percentage: 50, auto_send_winner: false, variations: [] },
    ...over
  }) as unknown as Broadcast

const stats = (over: Partial<ProgressStats>): ProgressStats => ({
  remaining: 0,
  processed: 68,
  enqueuedCount: 68,
  sentCount: 45,
  failedCount: 23,
  ...over
})

const statsResult = (over: Partial<BroadcastStatsResult> = {}): BroadcastStatsResult =>
  ({
    broadcast_id: 'bc-1',
    stats: {
      total_sent: 45,
      total_delivered: 40,
      total_opened: 10,
      total_clicked: 2,
      total_failed: 23,
      total_bounced: 0,
      total_complained: 0,
      total_unsubscribed: 0
    },
    ...over
  }) as BroadcastStatsResult

// A broadcast is finished when its queue is empty, not when the arithmetic on message
// history happens to reach zero: sent_at is stamped on the first attempt whatever its
// outcome, so a refused recipient counted as both sent and failed.
describe('deliveryVerdict', () => {
  it('is still sending while anything is in flight', () => {
    expect(deliveryVerdict(stats({ inFlight: 12, failedTerminal: 0 }))).toEqual({ kind: 'sending', remaining: 12 })
  })

  it('is complete only when the queue is empty and nothing was given up on', () => {
    expect(deliveryVerdict(stats({ inFlight: 0, failedTerminal: 0, failedCount: 0 }))).toEqual({ kind: 'complete' })
  })

  it('names the recipients it gave up on', () => {
    expect(deliveryVerdict(stats({ inFlight: 0, failedTerminal: 23 }))).toEqual({
      kind: 'completedWithFailures',
      failed: 23
    })
  })

  it('counts a row still retrying as in flight, not as given up on', () => {
    expect(deliveryVerdict(stats({ inFlight: 2, failedTerminal: 0, failedCount: 2 }))).toEqual({
      kind: 'sending',
      remaining: 2
    })
  })

  it('falls back to the message-history arithmetic when the queue could not be read', () => {
    expect(deliveryVerdict(stats({ inFlight: undefined, failedTerminal: undefined, remaining: 5 }))).toEqual({
      kind: 'sending',
      remaining: 5
    })
    expect(deliveryVerdict(undefined)).toEqual({ kind: 'complete' })
  })

  it('still names the failures once the abandoned rows have been swept', () => {
    // Terminal queue rows are deleted after their retention window; the count comes from
    // message history so the campaign is not repainted green a week later.
    expect(deliveryVerdict(stats({ inFlight: 0, failedTerminal: 0, failedCount: 23 }))).toEqual({
      kind: 'completedWithFailures',
      failed: 23
    })
  })
})

describe('progressStatsFrom', () => {
  it('maps the queue to in-flight and given-up rows', () => {
    const result = progressStatsFrom(
      statsResult({ queue: { pending: 1, processing: 2, paused: 1, failed_retrying: 2, failed_terminal: 23 } }),
      100
    )
    expect(result).toEqual({
      remaining: 32,
      processed: 68,
      enqueuedCount: 100,
      sentCount: 45,
      failedCount: 23,
      inFlight: 6,
      failedTerminal: 23
    })
  })

  it('leaves both undefined when the queue is unreadable', () => {
    const result = progressStatsFrom(statsResult(), 68)
    expect(result.inFlight).toBeUndefined()
    expect(result.failedTerminal).toBeUndefined()
    expect(result.failedCount).toBe(23)
  })

  it('reports no remainder without an enqueued count', () => {
    const result = progressStatsFrom(
      statsResult({ queue: { pending: 3, processing: 0, paused: 0, failed_retrying: 0, failed_terminal: 0 } })
    )
    expect(result.remaining).toBe(0)
    expect(result.enqueuedCount).toBe(0)
    expect(result.inFlight).toBe(3)
  })
})

describe('statusPill', () => {
  it.each([
    ['draft', 'neutral', false, 'draft'],
    ['scheduled', 'info', false, 'scheduled'],
    ['testing', 'primary', true, 'testing'],
    ['test_completed', 'info', false, 'pickWinner'],
    ['processing', 'primary', true, 'preparing'],
    ['winner_selected', 'primary', true, 'winnerSending'],
    ['paused', 'warning', false, 'paused'],
    ['cancelled', 'neutral', false, 'cancelled'],
    ['failed', 'danger', false, 'failed']
  ] as const)('%s is a %s pill', (status, tone, pulse, kind) => {
    expect(statusPill(broadcast({ status }))).toMatchObject({ tone, pulse, kind })
  })

  it('follows the verdict for a followed processed row', () => {
    const b = broadcast({ status: 'processed' })
    expect(statusPill(b, { verdict: { kind: 'sending', remaining: 12 } })).toEqual({
      tone: 'primary',
      pulse: true,
      kind: 'sending',
      count: 12
    })
    expect(statusPill(b, { verdict: { kind: 'complete' } })).toMatchObject({ tone: 'success', kind: 'sent' })
    expect(statusPill(b, { verdict: { kind: 'completedWithFailures', failed: 4 } })).toEqual({
      tone: 'warning',
      pulse: false,
      kind: 'someFailed',
      count: 4
    })
  })

  it('reads the failures of another processed row from the 30-day stats', () => {
    const b = broadcast({ status: 'processed' })
    expect(statusPill(b, { l2Failed: 7 })).toEqual({ tone: 'warning', pulse: false, kind: 'someFailed', count: 7 })
    expect(statusPill(b, { l2Failed: 0 })).toMatchObject({ tone: 'success', kind: 'sent' })
    expect(statusPill(b)).toMatchObject({ tone: 'success', kind: 'sent' })
  })

  it('uses the fallback rule for a followed row whose queue is not loaded yet', () => {
    // No verdict yet: the rule of the other rows applies.
    expect(statusPill(broadcast({ status: 'processed' }), { l2Failed: 2 })).toMatchObject({
      kind: 'someFailed',
      count: 2
    })
  })
})

describe('isFollowedRow and isLiveRow', () => {
  const now = dayjs('2026-11-12T12:00:00Z')

  it.each(['testing', 'test_completed', 'processing', 'winner_selected', 'paused', 'failed'] as const)(
    'follows a %s row',
    (status) => {
      expect(isFollowedRow(broadcast({ status }), now)).toBe(true)
    }
  )

  it.each(['draft', 'scheduled', 'cancelled'] as const)('does not follow a %s row', (status) => {
    expect(isFollowedRow(broadcast({ status }), now)).toBe(false)
  })

  it('follows a processed row for three days after it completed', () => {
    expect(
      isFollowedRow(broadcast({ status: 'processed', completed_at: '2026-11-10T12:00:00Z' }), now)
    ).toBe(true)
    expect(
      isFollowedRow(broadcast({ status: 'processed', completed_at: '2026-11-08T12:00:00Z' }), now)
    ).toBe(false)
  })

  it('follows a processed row without completed_at, which may still be draining', () => {
    expect(isFollowedRow(broadcast({ status: 'processed' }), now)).toBe(true)
  })

  it('calls a row live while it is sending', () => {
    expect(isLiveRow(broadcast({ status: 'testing' }))).toBe(true)
    expect(isLiveRow(broadcast({ status: 'processing' }))).toBe(true)
    expect(isLiveRow(broadcast({ status: 'winner_selected' }))).toBe(true)
    expect(isLiveRow(broadcast({ status: 'processed' }), { kind: 'sending', remaining: 3 })).toBe(true)
    expect(isLiveRow(broadcast({ status: 'processed' }), { kind: 'complete' })).toBe(false)
    expect(isLiveRow(broadcast({ status: 'paused' }))).toBe(false)
  })
})

describe('getRemainingTestTime', () => {
  const now = dayjs('2026-11-12T12:00:00Z')
  const testing = (over: Partial<Broadcast> = {}) =>
    broadcast({
      status: 'testing',
      test_sent_at: '2026-11-12T10:00:00Z',
      test_settings: {
        enabled: true,
        sample_percentage: 20,
        auto_send_winner: true,
        test_duration_hours: 4,
        variations: []
      },
      ...over
    })

  it('returns the duration alone, with no sentence around it', () => {
    expect(getRemainingTestTime(testing(), undefined, now)).toBe('2 hours')
  })

  it('prefers the start time of the test results', () => {
    expect(getRemainingTestTime(testing(), { test_started_at: '2026-11-12T11:00:00Z' }, now)).toBe('3 hours')
  })

  it('returns null once the test has ended, or outside a test', () => {
    expect(getRemainingTestTime(testing({ test_sent_at: '2026-11-12T06:00:00Z' }), undefined, now)).toBeNull()
    expect(getRemainingTestTime(testing({ status: 'processing' }), undefined, now)).toBeNull()
    expect(getRemainingTestTime(testing({ test_sent_at: undefined }), undefined, now)).toBeNull()
  })
})

describe('availableActions', () => {
  const sending = { kind: 'sending', remaining: 3 } as const
  const failed = { kind: 'completedWithFailures', failed: 2 } as const
  const progress = { remaining: 0, processed: 10, enqueuedCount: 10, sentCount: 8, failedCount: 2, inFlight: 0, failedTerminal: 2 }
  const on = (set: ReturnType<typeof availableActions>) => Object.entries(set).filter(([, v]) => v).map(([k]) => k)

  it('offers edit and delete on a draft', () => {
    expect(on(availableActions({ status: 'draft' }))).toEqual(['edit', 'delete'])
  })
  it('offers edit and cancel on a scheduled broadcast', () => {
    expect(on(availableActions({ status: 'scheduled' }))).toEqual(['edit', 'cancel'])
  })
  it('offers pause and cancel while sending, whether processing or draining', () => {
    expect(on(availableActions({ status: 'processing' }))).toEqual(['pause', 'cancel'])
    expect(on(availableActions({ status: 'processed' }, sending))).toEqual(['pause', 'cancel'])
  })
  it('offers resume and cancel when paused', () => {
    expect(on(availableActions({ status: 'paused' }))).toEqual(['resume', 'cancel'])
  })
  it('offers retry only for recipients given up on', () => {
    expect(on(availableActions({ status: 'processed' }, failed, progress))).toEqual(['retryFailed'])
    expect(on(availableActions({ status: 'failed' }, failed, progress))).toEqual(['retryFailed'])
    expect(on(availableActions({ status: 'processed' }, failed, { ...progress, failedTerminal: 0 }))).toEqual([])
    expect(on(availableActions({ status: 'processed' }, { kind: 'complete' }, progress))).toEqual([])
  })
})
