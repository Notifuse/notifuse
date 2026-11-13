import { describe, expect, it } from 'vitest'
import {
  defaultAuditPeriod,
  defaultInboundPeriod,
  defaultMessagesPeriod,
  periodBounds,
  COUNT_MAX_PERIOD,
  canCountMessages
} from './period'

// 2026-11-12 14:34:27.512 UTC
const NOW = Date.UTC(2026, 10, 12, 14, 34, 27, 512)

describe('periodBounds', () => {
  it('gives a lower bound for each rolling period, rounded down to the minute', () => {
    expect(periodBounds('1h', NOW)).toEqual({ from: '2026-11-12T13:34:00.000Z' })
    expect(periodBounds('24h', NOW)).toEqual({ from: '2026-11-11T14:34:00.000Z' })
    expect(periodBounds('7d', NOW)).toEqual({ from: '2026-11-05T14:34:00.000Z' })
    expect(periodBounds('30d', NOW)).toEqual({ from: '2026-10-13T14:34:00.000Z' })
    expect(periodBounds('90d', NOW)).toEqual({ from: '2026-08-14T14:34:00.000Z' })
  })

  it('shares the bound within a minute', () => {
    expect(periodBounds('24h', NOW)).toEqual(periodBounds('24h', NOW + 30_000))
    expect(periodBounds('24h', NOW + 33_000)).not.toEqual(periodBounds('24h', NOW))
  })

  it('sends no bound for all time', () => {
    expect(periodBounds('all', NOW)).toEqual({})
  })

  it('passes the custom range through', () => {
    expect(
      periodBounds('custom', NOW, { from: '2026-11-01T00:00:00Z', to: '2026-11-02T00:00:00Z' })
    ).toEqual({ from: '2026-11-01T00:00:00Z', to: '2026-11-02T00:00:00Z' })
    expect(periodBounds('custom', NOW, {})).toEqual({})
  })
})

describe('default periods', () => {
  it('opens messages on all time with an indexed entity filter', () => {
    for (const key of [
      'id',
      'contact_email',
      'broadcast_id',
      'automation_id',
      'transactional_notification_id',
      'template_id'
    ] as const) {
      expect(defaultMessagesPeriod({ [key]: 'x' })).toBe('all')
    }
  })

  it('keeps the last 24 hours otherwise, list and external id included', () => {
    expect(defaultMessagesPeriod({})).toBe('24h')
    expect(defaultMessagesPeriod({ list_id: 'l1' })).toBe('24h')
    expect(defaultMessagesPeriod({ external_id: 'e1' })).toBe('24h')
    expect(defaultMessagesPeriod({ is_failed: true })).toBe('24h')
  })

  it('opens incoming webhooks on all time for a message or a recipient', () => {
    expect(defaultInboundPeriod({ message_id: 'm1' })).toBe('all')
    expect(defaultInboundPeriod({ recipient_email: 'a@b.co' })).toBe('all')
    expect(defaultInboundPeriod({ source: 'ses' })).toBe('24h')
  })

  it('opens the audit log on all time for a target, seven days otherwise', () => {
    expect(defaultAuditPeriod({ target_id: 't1' })).toBe('all')
    expect(defaultAuditPeriod({ actor_email: 'a@b.co' })).toBe('7d')
  })
})

describe('canCountMessages', () => {
  it('counts every rolling period up to COUNT_MAX_PERIOD', () => {
    expect(COUNT_MAX_PERIOD).toBe('7d')
    for (const period of ['1h', '24h', '7d'] as const) {
      expect(canCountMessages({}, period)).toBe(true)
    }
    expect(canCountMessages({}, '30d')).toBe(false)
    expect(canCountMessages({ broadcast_id: 'b1' }, '30d')).toBe(true)
  })

  it('counts all time only with an indexed filter', () => {
    expect(canCountMessages({}, 'all')).toBe(false)
    expect(canCountMessages({ external_id: 'x' }, 'all')).toBe(false)
    expect(canCountMessages({ list_id: 'l' }, 'all')).toBe(false)
    expect(canCountMessages({ template_id: 't' }, 'all')).toBe(true)
    expect(canCountMessages({ contact_email: 'ann@example.com' }, 'all')).toBe(true)
  })
})
