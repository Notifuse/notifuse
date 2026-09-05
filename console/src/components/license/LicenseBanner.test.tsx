import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LicenseContext, LicenseContextValue, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import { LicenseBanner } from './LicenseBanner'
import { LICENSE_BANNER_HEIGHT_VAR } from './bannerOffset'
import type { Entitlements } from '../../types/license'

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'u1', email: 'root@example.com' },
    workspaces: [{ id: 'ws1', name: 'Test' }],
    isAuthenticated: true,
    loading: false
  })
}))

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

const renderBanner = (value: Partial<LicenseContextValue>) =>
  render(
    <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, ...value }}>
      <LicenseBanner />
    </LicenseContext.Provider>
  )

describe('LicenseBanner', () => {
  // The banner has exactly one trigger, and everything else must leave the console alone. An
  // unlicensed deployment in particular: Community is a supported way to run Notifuse, and a
  // permanent bar across a console that is doing nothing wrong is an advertisement.
  it.each([
    ['a current licence', entitlements()],
    ['an unlicensed deployment', entitlements({ state: 'none', tier: '', features: [] })],
    ['a licence that has fully expired', entitlements({ state: 'expired' })],
    ['a console that has not been told', null]
  ])('says nothing for %s', (_name, ent) => {
    const { container } = renderBanner({ entitlements: ent })

    expect(container).toBeEmptyDOMElement()
    // The layouts offset their fixed chrome by this variable; leaving it set would open a gap
    // above a console with no banner in it.
    expect(document.documentElement.style.getPropertyValue(LICENSE_BANNER_HEIGHT_VAR)).toBe('')
  })

  // Expiry is a warning, not a wall: everything the key grants keeps working through the grace
  // period, and the banner has to read that way or a customer being dunned thinks they are down.
  it('warns during the grace period without claiming anything is blocked', () => {
    renderBanner({ entitlements: entitlements({ state: 'grace' }) })

    // The headline and the reassurance below it both say "grace period", which is the point.
    expect(screen.getAllByText(/grace period/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/keeps working during the grace period/i)).toBeInTheDocument()
    // Renew, not buy: they already paid, and the payment is being retried.
    expect(screen.getByText('Renew licence')).toBeInTheDocument()
    expect(screen.queryByText('Buy a licence')).not.toBeInTheDocument()
  })

  it('dates the expiry it is warning about', () => {
    renderBanner({
      entitlements: entitlements({ state: 'grace', expires_at: '2026-03-01T12:00:00Z' })
    })

    expect(screen.getByText(/expired on/i)).toBeInTheDocument()
  })

  it('offers the licence page to a root user and tells everyone else who to ask', () => {
    const grace = entitlements({ state: 'grace' })

    const { unmount } = renderBanner({ entitlements: grace, canManageLicense: true })
    expect(screen.getByText('Enter a licence key')).toBeInTheDocument()
    expect(screen.queryByText(/ask the owner of this workspace/i)).not.toBeInTheDocument()
    unmount()

    renderBanner({ entitlements: grace, canManageLicense: false })
    expect(screen.queryByText('Enter a licence key')).not.toBeInTheDocument()
    expect(screen.getByText(/ask the owner of this workspace/i)).toBeInTheDocument()
  })

  it('publishes its height so the fixed chrome underneath can step aside', () => {
    renderBanner({ entitlements: entitlements({ state: 'grace' }) })

    // jsdom measures every box as 0, so the assertion is that the variable is SET, not what it
    // holds — the value comes from a real layout.
    expect(document.documentElement.style.getPropertyValue(LICENSE_BANNER_HEIGHT_VAR)).not.toBe('')
  })
})
