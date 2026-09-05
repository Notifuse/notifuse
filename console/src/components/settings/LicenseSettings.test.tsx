import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App as AntApp } from 'antd'
import { LicenseContext, LicenseContextValue, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import { LicenseSettings } from './LicenseSettings'
import type { Entitlements } from '../../types/license'

const { setLicense } = vi.hoisted(() => ({ setLicense: vi.fn() }))

vi.mock('../../services/api/license', () => ({
  licenseApi: {
    get: vi.fn(),
    set: setLicense
  }
}))

const entitlements = (overrides: Partial<Entitlements> = {}): Entitlements => ({
  tier: 'agency',
  org: 'ACME SAS',
  sub: 'billing@acme.com',
  max_workspaces: 15,
  features: ['rbac', 'ses_tenant'],
  state: 'active',
  expires_at: '2027-03-01T00:00:00Z',
  ...overrides
})

const renderSettings = (value: Partial<LicenseContextValue>) =>
  render(
    <AntApp>
      <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, ...value }}>
        <LicenseSettings />
      </LicenseContext.Provider>
    </AntApp>
  )

describe('LicenseSettings', () => {
  beforeEach(() => {
    setLicense.mockReset()
  })

  // Root, not owner: the server's licence endpoints are gated on requireRootUser, and the
  // licensee's organisation and billing address are not every member's to read.
  it('tells a non-root member who to ask instead of showing them the licensee', () => {
    renderSettings({ canManageLicense: false, entitlements: entitlements() })

    expect(screen.getByText(/Only an instance administrator/i)).toBeInTheDocument()
    expect(screen.queryByText(/ACME SAS/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Install licence key' })).not.toBeInTheDocument()
  })

  // The anti-sharing deterrent is social rather than cryptographic — nothing stops a key being
  // pasted into a second deployment except the name of whoever bought it being on the screen.
  it('shows the licensee prominently', () => {
    renderSettings({ canManageLicense: true, entitlements: entitlements() })

    expect(screen.getByText(/ACME SAS/)).toBeInTheDocument()
    expect(screen.getByText('billing@acme.com')).toBeInTheDocument()
  })

  it('lists the capabilities the key does and does not carry', () => {
    renderSettings({ canManageLicense: true, entitlements: entitlements() })

    // Granted and withheld are both listed, so an operator can see what a licence would add
    // without leaving for the price list.
    expect(screen.getByText('Custom permissions (RBAC)')).toBeInTheDocument()
    expect(screen.getByText('Single sign-on (SSO)')).toBeInTheDocument()
  })

  it('reports Community and an unlimited quota without inventing a date', () => {
    renderSettings({
      canManageLicense: true,
      entitlements: entitlements({
        state: 'none',
        tier: '',
        features: [],
        max_workspaces: 3,
        expires_at: '0001-01-01T00:00:00Z'
      })
    })

    expect(screen.getByText(/No licence/i)).toBeInTheDocument()
    expect(screen.queryByText(/0001/)).not.toBeInTheDocument()
  })

  it('installs a pasted key and adopts the state the server answered with', async () => {
    const adopt = vi.fn()
    const response = { entitlements: entitlements(), read_only: false }
    setLicense.mockResolvedValue(response)

    renderSettings({ canManageLicense: true, adopt })

    fireEvent.change(screen.getByPlaceholderText('NFUSE1....'), {
      target: { value: '  NFUSE1.payload.signature  ' }
    })
    fireEvent.click(screen.getByRole('button', { name: 'Install licence key' }))

    // Trimmed: a key copied out of an email arrives with whitespace, and the server would reject
    // the signature over it.
    await waitFor(() => expect(setLicense).toHaveBeenCalledWith('NFUSE1.payload.signature'))
    await waitFor(() => expect(adopt).toHaveBeenCalledWith(response))
  })

  it('offers nothing to buy while a licence is in force', () => {
    renderSettings({ canManageLicense: true, entitlements: entitlements() })

    expect(screen.queryByText('Buy a licence')).not.toBeInTheDocument()
    expect(screen.queryByText('Upgrade licence')).not.toBeInTheDocument()
  })

  it('offers the install button only once a key has been pasted', () => {
    renderSettings({ canManageLicense: true })

    expect(screen.queryByRole('button', { name: 'Install licence key' })).not.toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('NFUSE1....'), {
      target: { value: 'NFUSE1.payload.signature' }
    })

    expect(screen.getByRole('button', { name: 'Install licence key' })).toBeInTheDocument()
    expect(setLicense).not.toHaveBeenCalled()
  })

  it('offers to buy under the table, full width, only while there is no licence', () => {
    renderSettings({ canManageLicense: true })

    // Buying is not a step of installing; beside the Install button it read as one. antd
    // renders a Button with an href as a link, which is what a new tab wants.
    const buy = screen.getByRole('link', { name: 'Buy a licence' })
    expect(buy).toHaveClass('ant-btn-primary')
    expect(buy).toHaveClass('ant-btn-block')
    const table = screen.getByText('Status').closest('table') as HTMLElement
    expect(table.compareDocumentPosition(buy) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.queryByText('What a licence includes')).not.toBeInTheDocument()
    // Refresh sits in the Status row: that value is the one thing on the page that can be
    // stale, and a button next to it says what it re-reads.
    const statusRow = screen.getByText('Status').closest('tr') as HTMLElement
    expect(within(statusRow).getByRole('button', { name: 'Refresh' })).toBeInTheDocument()
  })
})

// The Refresh button is the one licence read a human asks for.
//
// It used to swallow: the provider caught every failure and logged it, which is right for
// the automatic read on mount and wrong here. A button that answers a failed read with
// nothing is indistinguishable from a button that is not wired — and this one is pressed
// exactly when the read has been failing, because that is the state it exists to escape.
describe('LicenseSettings refresh', () => {
  it('reports a failed read instead of doing nothing visible', async () => {
    const refresh = vi.fn().mockRejectedValue(new Error('licence endpoint unreachable'))

    renderSettings({ canManageLicense: true, refresh })

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))

    await waitFor(() =>
      expect(screen.getByText('licence endpoint unreachable')).toBeInTheDocument()
    )
  })

  it('says nothing when the read works', async () => {
    const refresh = vi.fn().mockResolvedValue(undefined)

    renderSettings({ canManageLicense: true, refresh })

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))

    await waitFor(() => expect(refresh).toHaveBeenCalled())
    expect(screen.queryByText(/could not read the licence state/i)).not.toBeInTheDocument()
  })
})
