import { describe, it, expect } from 'vitest'
import { getEmailTrackingPolicy, isPartiallyTracked, trackedBase } from './emailTracking'

describe('getEmailTrackingPolicy', () => {
  it('reads the detailed settings when present', () => {
    expect(
      getEmailTrackingPolicy({
        email_tracking_enabled: true,
        email_tracking: { opens_enabled: false, clicks_enabled: true, unknown_consent: 'no_track' }
      })
    ).toEqual({ opens_enabled: false, clicks_enabled: true, unknown_consent: 'no_track' })
  })

  it('falls back to the single switch, tracking unknown consent', () => {
    expect(getEmailTrackingPolicy({ email_tracking_enabled: true })).toEqual({
      opens_enabled: true,
      clicks_enabled: true,
      unknown_consent: 'track'
    })
    expect(getEmailTrackingPolicy({ email_tracking_enabled: false })).toEqual({
      opens_enabled: false,
      clicks_enabled: false,
      unknown_consent: 'track'
    })
  })

  it('treats a missing workspace as tracking nothing', () => {
    expect(getEmailTrackingPolicy(undefined).opens_enabled).toBe(false)
  })
})

describe('trackedBase', () => {
  it('uses the tracked total when the server sends one', () => {
    expect(trackedBase(100, 40)).toBe(40)
    expect(trackedBase(100, 0)).toBe(0)
  })

  it('falls back to the sent total when it does not', () => {
    expect(trackedBase(100, undefined)).toBe(100)
  })
})

describe('isPartiallyTracked', () => {
  it('is true only when fewer emails were tracked than sent', () => {
    expect(isPartiallyTracked(100, 40)).toBe(true)
    expect(isPartiallyTracked(100, 100)).toBe(false)
    expect(isPartiallyTracked(100, undefined)).toBe(false)
  })
})
