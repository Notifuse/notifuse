import { describe, it, expect } from 'vitest'
import { deriveInstallState, InstallProbe } from './installStatus'

const probe = (overrides: Partial<InstallProbe> = {}): InstallProbe => ({
  hasSettings: true,
  enabled: true,
  checkTraffic: true,
  ...overrides
})

describe('deriveInstallState', () => {
  it('reports a workspace with no web analytics settings', () => {
    expect(deriveInstallState(probe({ hasSettings: false, enabled: false }))).toBe('not_configured')
  })

  it('reports settings that exist but are switched off', () => {
    expect(deriveInstallState(probe({ enabled: false }))).toBe('disabled')
  })

  it('prefers the missing-settings answer over the disabled one', () => {
    expect(deriveInstallState(probe({ hasSettings: false, enabled: true }))).toBe('not_configured')
  })

  it('skips the traffic checks on config screens', () => {
    expect(deriveInstallState(probe({ checkTraffic: false }))).toBe('ok')
  })

  it('lets a config screen through while collection is still switched off', () => {
    // Attribution rules are written before the snippet goes live; gating them
    // on `enabled` would make the rules editor unreachable.
    expect(deriveInstallState(probe({ enabled: false, checkTraffic: false }))).toBe('ok')
  })

  it('still gates a config screen on a workspace that has no settings at all', () => {
    expect(deriveInstallState(probe({ hasSettings: false, checkTraffic: false }))).toBe(
      'not_configured'
    )
  })

  it('waits while the 24-hour probe is in flight', () => {
    expect(deriveInstallState(probe())).toBe('loading')
  })

  it('passes through as soon as the last day has traffic', () => {
    expect(deriveInstallState(probe({ sessionsLast24h: 3 }))).toBe('ok')
  })

  it('does not run the lifetime check when the last day has traffic', () => {
    // sessionsEver stays undefined because the second probe never fires.
    expect(deriveInstallState(probe({ sessionsLast24h: 1, sessionsEver: undefined }))).toBe('ok')
  })

  it('waits while the lifetime probe is in flight', () => {
    expect(deriveInstallState(probe({ sessionsLast24h: 0 }))).toBe('loading')
  })

  it('asks for an install when nothing was ever recorded', () => {
    expect(deriveInstallState(probe({ sessionsLast24h: 0, sessionsEver: 0 }))).toBe('never_received')
  })

  it('reports a quiet day on a workspace that has history', () => {
    expect(deriveInstallState(probe({ sessionsLast24h: 0, sessionsEver: 42 }))).toBe('stalled')
  })

  it('never hides a page behind an install screen when a probe failed', () => {
    expect(deriveInstallState(probe({ failed: true }))).toBe('ok')
    expect(deriveInstallState(probe({ failed: true, sessionsLast24h: 0, sessionsEver: 0 }))).toBe(
      'ok'
    )
  })

  it('still reports a configuration problem when a probe failed', () => {
    expect(deriveInstallState(probe({ hasSettings: false, failed: true }))).toBe('not_configured')
    expect(deriveInstallState(probe({ enabled: false, failed: true }))).toBe('disabled')
  })
})
