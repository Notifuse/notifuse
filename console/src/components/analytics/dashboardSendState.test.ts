import { describe, expect, it } from 'vitest'
import { sendStateOf } from './dashboardSendState'

// The period ends at the end of yesterday, Paris time.
const periodEndUtc = '2026-11-10T22:59:59.999Z'

describe('sendStateOf', () => {
  it('is loading while the latest message loads', () => {
    expect(sendStateOf({ latestAt: undefined, periodEndUtc, periodSent: 0 })).toBe('loading')
  })

  it('is never without any message', () => {
    expect(sendStateOf({ latestAt: null, periodEndUtc, periodSent: 0 })).toBe('never')
  })

  it('is today when the only messages are after the period', () => {
    expect(sendStateOf({ latestAt: '2026-11-11T08:00:00Z', periodEndUtc, periodSent: 0 })).toBe('today')
  })

  it('is counted when the period has sends too', () => {
    expect(sendStateOf({ latestAt: '2026-11-11T08:00:00Z', periodEndUtc, periodSent: 12 })).toBe('counted')
  })

  it('is counted when the latest message is inside the period', () => {
    expect(sendStateOf({ latestAt: '2026-11-02T08:00:00Z', periodEndUtc, periodSent: 0 })).toBe('counted')
  })

  it('is loading while the series loads, and counted when it failed', () => {
    expect(sendStateOf({ latestAt: '2026-11-11T08:00:00Z', periodEndUtc, periodSent: undefined })).toBe('loading')
    expect(sendStateOf({ latestAt: '2026-11-11T08:00:00Z', periodEndUtc, periodSent: undefined, seriesError: true })).toBe(
      'counted'
    )
  })

  it('is counted when the latest message failed to load', () => {
    expect(sendStateOf({ latestAt: undefined, latestError: true, periodEndUtc, periodSent: undefined })).toBe('counted')
  })

  it('counts a message exactly at the period end as inside it', () => {
    expect(sendStateOf({ latestAt: periodEndUtc, periodSent: 0, periodEndUtc })).toBe('counted')
  })

  it('compares instants, whatever the offset or precision the server writes', () => {
    // 23:30 in Paris on the 10th is still yesterday's period
    expect(sendStateOf({ latestAt: '2026-11-10T23:30:00+01:00', periodEndUtc, periodSent: 0 })).toBe('counted')
    // 00:30 in Paris on the 11th is today
    expect(sendStateOf({ latestAt: '2026-11-11T00:30:00+01:00', periodEndUtc, periodSent: 0 })).toBe('today')
    expect(sendStateOf({ latestAt: '2026-11-10T23:00:00.123456789Z', periodEndUtc, periodSent: 0 })).toBe('today')
  })
})
