import { describe, it, expect } from 'vitest'
import { getEmailTrackingPolicy } from './emailTracking'

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
