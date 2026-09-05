import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App as AntApp } from 'antd'
import { EditPermissionsDrawer } from './EditPermissionsDrawer'
import type { WorkspaceMember } from '../../services/api/types'
import { LicenseContext, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import type { Entitlements } from '../../types/license'

const { setUserPermissions } = vi.hoisted(() => ({ setUserPermissions: vi.fn() }))

vi.mock('../../services/api/workspace', () => ({
  workspaceService: { setUserPermissions }
}))

const member = {
  user_id: 'u1',
  email: 'member@example.com',
  type: 'user',
  permissions: {},
  created_at: '2026-01-01T00:00:00Z'
} as unknown as WorkspaceMember

function renderDrawer() {
  return render(
    <AntApp>
      <EditPermissionsDrawer
        open
        member={member}
        workspaceId="ws1"
        onClose={vi.fn()}
        onSuccess={vi.fn()}
      />
    </AntApp>
  )
}

async function save() {
  fireEvent.click(screen.getByRole('button', { name: /save permissions/i }))
}

/**
 * Granular permissions are a licensed capability and this drawer IS that capability — the
 * server calls SetUserPermissions "the canonical permission editor". So this is the surface
 * where a licence refusal is most likely to be met, and it was the one surface that threw the
 * explanation away.
 *
 * client.ts has already turned the 402 into a readable sentence naming the capability and the
 * plan by the time it reaches the catch here. Replacing it with a fixed "Failed to update
 * permissions" showed the most-hit paid gate as a malfunction, with nothing anywhere else in
 * the console mentioning a licence at all — so the operator files a bug instead of buying.
 */
describe('EditPermissionsDrawer error reporting', () => {
  beforeEach(() => {
    setUserPermissions.mockReset()
  })

  it('shows the licence refusal the server explained', async () => {
    setUserPermissions.mockRejectedValue(
      new Error('Custom permissions require a Notifuse Studio licence.')
    )

    renderDrawer()
    await save()

    await waitFor(() =>
      expect(
        screen.getByText('Custom permissions require a Notifuse Studio licence.')
      ).toBeInTheDocument()
    )
    expect(screen.queryByText('Failed to update permissions')).not.toBeInTheDocument()
  })

  // The same rule serves the other refusal on this control: a 403 has already been turned
  // into "You do not have write access to…" and is equally worth showing.
  it('shows a permission denial the same way', async () => {
    setUserPermissions.mockRejectedValue(
      new Error('You do not have write access to Members.')
    )

    renderDrawer()
    await save()

    await waitFor(() =>
      expect(screen.getByText('You do not have write access to Members.')).toBeInTheDocument()
    )
  })

  // The fallback still has to exist: a failure with nothing to say must not render an empty
  // toast, which reads as "the click did nothing".
  it('falls back to a fixed sentence when the failure carries no message', async () => {
    setUserPermissions.mockRejectedValue(new Error(''))

    renderDrawer()
    await save()

    await waitFor(() =>
      expect(screen.getByText('Failed to update permissions')).toBeInTheDocument()
    )
  })
})

/**
 * The same gate, met before Save instead of after it. The server's 402 and its sentence are
 * untouched; what changes is that an owner on an unlicensed deployment sees a locked matrix
 * that names the plan, rather than a live one that refuses on Save.
 */
describe('EditPermissionsDrawer under an unlicensed deployment', () => {
  const licensedFor = (features: Entitlements['features']): Entitlements => ({
    tier: 'Studio',
    org: 'ACME SAS',
    sub: 'billing@acme.com',
    max_workspaces: 5,
    features,
    state: 'active',
    expires_at: '2027-01-01T00:00:00Z'
  })

  const renderUnder = (features: Entitlements['features']) =>
    render(
      <AntApp>
        <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: licensedFor(features) }}>
          <EditPermissionsDrawer
            open
            member={member}
            workspaceId="ws1"
            onClose={vi.fn()}
            onSuccess={vi.fn()}
          />
        </LicenseContext.Provider>
      </AntApp>
    )

  it('locks the matrix and the save button, and says what to buy', () => {
    renderUnder(['template_i18n'])

    expect(
      screen.getByText('Custom permissions require a Notifuse Studio licence.')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /save permissions/i })).toBeDisabled()
    const switches = screen.getAllByRole('switch')
    expect(switches.length).toBeGreaterThan(0)
    expect(switches.every((s) => (s as HTMLButtonElement).disabled)).toBe(true)
  })

  it('leaves everything live when the licence covers custom permissions', () => {
    renderUnder(['rbac'])

    expect(screen.queryByText(/requires? a Notifuse/i)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /save permissions/i })).toBeEnabled()
    expect(
      screen.getAllByRole('switch').some((s) => !(s as HTMLButtonElement).disabled)
    ).toBe(true)
  })
})
