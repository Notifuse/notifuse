import { describe, it, expect } from 'vitest'
import {
  Entitlements,
  LICENSE_FEATURES,
  UNLIMITED_WORKSPACES,
  hasLicensedFeature,
  isLicensed,
  licenseExpiry
} from './license'

const entitlements = (overrides: Partial<Entitlements> = {}): Entitlements => ({
  tier: 'studio',
  org: 'ACME SAS',
  sub: 'billing@acme.com',
  max_workspaces: 5,
  features: ['rbac', 'ses_tenant'],
  state: 'active',
  expires_at: '2027-01-01T00:00:00Z',
  ...overrides
})

describe('isLicensed', () => {
  it('counts the grace period as licensed', () => {
    expect(isLicensed(entitlements({ state: 'active' }))).toBe(true)
    expect(isLicensed(entitlements({ state: 'grace' }))).toBe(true)
  })

  it('does not count an expired or absent licence', () => {
    expect(isLicensed(entitlements({ state: 'expired' }))).toBe(false)
    expect(isLicensed(entitlements({ state: 'none' }))).toBe(false)
    expect(isLicensed(null)).toBe(false)
  })
})

describe('hasLicensedFeature', () => {
  it('answers from the feature list, never from the tier', () => {
    const studio = entitlements()

    expect(hasLicensedFeature(studio, 'rbac')).toBe(true)
    expect(hasLicensedFeature(studio, 'sso')).toBe(false)
  })

  // The console never enforces — the backend refuses with 402 whatever this says — so the only
  // thing a false here would achieve is hiding a paid capability from the deployment that bought
  // it, on a console that simply had not been told yet.
  it('does not hide a capability while the state is unknown', () => {
    for (const feature of LICENSE_FEATURES) {
      expect(hasLicensedFeature(null, feature)).toBe(true)
    }
  })
})

describe('licenseExpiry', () => {
  it('reads the expiry of a real key', () => {
    expect(licenseExpiry(entitlements())?.toISOString()).toBe('2027-01-01T00:00:00.000Z')
  })

  // Go marshals a zero time as year 1 rather than omitting the field, so printing it verbatim
  // would tell an operator their licence ran out during the Roman Empire.
  it('reports nothing for Go’s zero time', () => {
    expect(licenseExpiry(entitlements({ expires_at: '0001-01-01T00:00:00Z' }))).toBeNull()
  })

  it('reports nothing for an absent or unparseable date', () => {
    expect(licenseExpiry(entitlements({ expires_at: '' }))).toBeNull()
    expect(licenseExpiry(entitlements({ expires_at: 'soon' }))).toBeNull()
    expect(licenseExpiry(null)).toBeNull()
  })
})

describe('the unlimited sentinel', () => {
  // Negative rather than 0: 0 is what an unfilled struct holds, and a quota check that read it as
  // "no ceiling" would hand an unlicensed deployment more than a licensed one.
  it('is negative, so an unfilled quota can never read as unlimited', () => {
    expect(UNLIMITED_WORKSPACES).toBeLessThan(0)
  })
})

// A grant whose feature list is not an array must not take the console down.
//
// The Go side returns an empty slice on purpose so this marshals to [], but "the server
// always sends []" is a promise made by a different process — possibly an older or a newer
// one, possibly a proxy that rewrote the body. `.includes` on null throws inside a render,
// which is a white screen for the whole console, caused by the one subsystem whose entire
// design is that its failures cost features and never the product.
describe('hasLicensedFeature with a malformed grant', () => {
  it('does not throw when features is null', () => {
    const grant = { features: null } as unknown as Entitlements
    expect(() => hasLicensedFeature(grant, 'sso')).not.toThrow()
  })

  it('reads unknown as licensed, like every other advisory check', () => {
    const grant = { features: null } as unknown as Entitlements
    expect(hasLicensedFeature(grant, 'sso')).toBe(true)
  })

  it('still answers correctly for a well-formed grant', () => {
    const grant = { features: ['rbac'] } as unknown as Entitlements
    expect(hasLicensedFeature(grant, 'rbac')).toBe(true)
    expect(hasLicensedFeature(grant, 'sso')).toBe(false)
  })
})
