import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LicenseContext, LicenseContextValue, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import { SsoLicenceNotice } from './SsoLicenceNotice'
import type { Entitlements } from '../../types/license'

const entitlements = (features: Entitlements['features']): Entitlements => ({
  tier: 'studio',
  org: 'ACME SAS',
  sub: 'billing@acme.com',
  max_workspaces: 5,
  features,
  state: 'active',
  expires_at: '2027-01-01T00:00:00Z'
})

const renderNotice = (oidcEnabled: boolean, value: Partial<LicenseContextValue> = {}) =>
  render(
    <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, ...value }}>
      <SsoLicenceNotice oidcEnabled={oidcEnabled} workspaceId="ws1" />
    </LicenseContext.Provider>
  )

const notice = () => screen.queryByText(/licence does not include it/i)

describe('SsoLicenceNotice', () => {
  it('says so when SSO is on and the licence does not cover it', () => {
    renderNotice(true, { entitlements: entitlements(['rbac', 'ses_tenant']) })

    expect(notice()).toBeInTheDocument()
    // What still works comes before what to buy: the failure being described is a missing
    // button, not a broken login, and an operator reading this needs to know that first.
    expect(screen.getByText(/nobody is locked out/i)).toBeInTheDocument()
    // The same block, and the same one button, as every other licence message in the console.
    expect(screen.getByRole('button', { name: 'Licence settings' })).toBeInTheDocument()
    expect(screen.queryByText('Buy a licence')).not.toBeInTheDocument()
  })

  it('says nothing when the licence covers SSO', () => {
    renderNotice(true, { entitlements: entitlements(['rbac', 'sso']) })

    expect(notice()).not.toBeInTheDocument()
  })

  it('says nothing when SSO is switched off', () => {
    renderNotice(false, { entitlements: entitlements([]) })

    expect(notice()).not.toBeInTheDocument()
  })

  // Unknown entitlements read as licensed, like every other advisory check in this console. A
  // deployment that has paid must never be told it has not because the console was told
  // nothing — and this notice is shown to the one person who could act on it wrongly.
  it('says nothing when the console has not been told what the licence is', () => {
    renderNotice(true)

    expect(notice()).not.toBeInTheDocument()
  })
})
