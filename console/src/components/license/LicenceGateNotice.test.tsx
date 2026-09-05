import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LicenseContext, LicenseContextValue, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import { LicenceGateNotice } from './LicenceGateNotice'
import type { Entitlements, LicenseFeature } from '../../types/license'

const entitlements = (features: Entitlements['features']): Entitlements => ({
  tier: 'Studio',
  org: 'ACME SAS',
  sub: 'billing@acme.com',
  max_workspaces: 5,
  features,
  state: 'active',
  expires_at: '2027-01-01T00:00:00Z'
})

// `where` is an object rather than an optional string: passing undefined to a defaulted
// parameter takes the default, so "no workspace" needs its own spelling.
const renderNotice = (
  feature: LicenseFeature,
  value: Partial<LicenseContextValue> = {},
  where: { workspaceId?: string } = { workspaceId: 'ws1' }
) =>
  render(
    <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, ...value }}>
      <LicenceGateNotice feature={feature} workspaceId={where.workspaceId} />
    </LicenseContext.Provider>
  )

describe('LicenceGateNotice', () => {
  it('names the capability and the plan, and leads with what still works', () => {
    renderNotice('rbac', { entitlements: entitlements(['template_i18n']) })

    expect(
      screen.getByText('Custom permissions require a Notifuse Studio licence.')
    ).toBeInTheDocument()
    // A full grant is never gated, and the person reading this needs to know that before they
    // are told what to buy.
    expect(screen.getByText(/can still be given full access/i)).toBeInTheDocument()
  })

  it('names the plan from the table the server also uses', () => {
    renderNotice('sso', { entitlements: entitlements(['rbac']) })

    expect(
      screen.getByText('Single sign-on requires a Notifuse Enterprise licence.')
    ).toBeInTheDocument()
  })

  it('offers one button, to the workspace licence settings, whoever is looking', () => {
    // The licence page already offers root the key box and the price list, and already tells a
    // member who to ask; the block repeats neither, and never shows "Buy" to someone who cannot
    // install what they would buy.
    renderNotice('rbac', { entitlements: entitlements([]), canManageLicense: false })

    expect(screen.getByRole('button', { name: 'Licence settings' })).toBeInTheDocument()
    expect(screen.queryByText('Buy a licence')).not.toBeInTheDocument()
    expect(screen.queryByText(/ask the person who runs/i)).not.toBeInTheDocument()
  })

  it('offers no button without a workspace to route through', () => {
    renderNotice('rbac', { entitlements: entitlements([]) }, {})

    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('says nothing when the licence covers the capability', () => {
    renderNotice('rbac', { entitlements: entitlements(['rbac']) })

    expect(screen.queryByText(/licence/i)).not.toBeInTheDocument()
  })

  // Unknown entitlements read as licensed, like every other advisory check in this console. A
  // deployment that has paid must never be told it has not because the console was told nothing.
  it('says nothing when the console has not been told what the licence is', () => {
    renderNotice('rbac')

    expect(screen.queryByText(/licence/i)).not.toBeInTheDocument()
  })
})
