import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App as AntApp } from 'antd'
import { EditPermissionsDrawer } from './EditPermissionsDrawer'
import type { WorkspaceMember } from '../../services/api/types'
import { LicenseContext, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import type { Entitlements } from '../../types/license'
import { ALL_PERMISSION_RESOURCES } from '../../services/api/permissions'

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
 * Granular permissions are a licensed capability and this drawer is where they are edited, so
 * this is the surface where a licence refusal is most likely to be met, and it was the one
 * surface that threw the explanation away.
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
 * The same gate, met before Save instead of after it. Without the rbac licence the server accepts
 * full access and refuses any narrower set, so the drawer offers exactly that: Grant all stays
 * live, every choice that would narrow the grant is greyed, and Save is enabled only while the
 * set is full access.
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

  const restrictedMember = {
    ...member,
    permissions: { contacts: { read: true, write: false } }
  } as unknown as WorkspaceMember

  const fullMember = {
    ...member,
    permissions: Object.fromEntries(
      ALL_PERMISSION_RESOURCES.map((resource) => [resource, { read: true, write: true }])
    )
  } as unknown as WorkspaceMember

  const renderUnder = (features: Entitlements['features'], who: WorkspaceMember = member) =>
    render(
      <AntApp>
        <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: licensedFor(features) }}>
          <EditPermissionsDrawer
            open
            member={who}
            workspaceId="ws1"
            onClose={vi.fn()}
            onSuccess={vi.fn()}
          />
        </LicenseContext.Provider>
      </AntApp>
    )

  const switchFor = (resource: string, verb: 'read' | 'write') => {
    const row = document.querySelector(`tr[data-row-key="${resource}"]`)
    expect(row).not.toBeNull()
    const [read, write] = Array.from(
      (row as HTMLElement).querySelectorAll<HTMLButtonElement>('button[role="switch"]')
    )
    return verb === 'read' ? read : write
  }

  beforeEach(() => {
    setUserPermissions.mockReset()
    setUserPermissions.mockResolvedValue({ status: 'success' })
  })

  it('says what to buy, and blocks saving a restricted member as they are', () => {
    renderUnder(['template_i18n'], restrictedMember)

    expect(
      screen.getByText('Custom permissions require a Notifuse Studio licence.')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /save permissions/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Revoke all' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Grant all' })).toBeEnabled()
  })

  it('puts a restricted member back on full access with Grant all', async () => {
    renderUnder(['template_i18n'], restrictedMember)

    fireEvent.click(screen.getByRole('button', { name: 'Grant all' }))
    const saveButton = screen.getByRole('button', { name: /save permissions/i })
    expect(saveButton).toBeEnabled()
    fireEvent.click(saveButton)

    await waitFor(() => expect(setUserPermissions).toHaveBeenCalled())
    const { permissions } = setUserPermissions.mock.calls[0][0]
    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(permissions[resource]).toEqual({ read: true, write: true })
    }
  })

  it('greys every switch that would narrow a full grant, and lets it be saved', () => {
    renderUnder(['template_i18n'], fullMember)

    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(switchFor(resource, 'read').disabled).toBe(true)
      expect(switchFor(resource, 'write').disabled).toBe(true)
    }
    // Audit logs is opt-in and never part of full access, so granting it narrows nothing.
    expect(switchFor('audit_logs', 'read').disabled).toBe(false)
    expect(screen.getByRole('button', { name: /save permissions/i })).toBeEnabled()
  })

  it('leaves live the one switch that completes full access', () => {
    const permissions = Object.fromEntries(
      ALL_PERMISSION_RESOURCES.map((resource) => [resource, { read: true, write: true }])
    )
    permissions.contacts = { read: true, write: false }
    renderUnder(['template_i18n'], { ...member, permissions } as unknown as WorkspaceMember)

    expect(switchFor('contacts', 'write').disabled).toBe(false)
    expect(switchFor('contacts', 'read').disabled).toBe(true)
    expect(switchFor('lists', 'write').disabled).toBe(true)
    expect(screen.getByRole('button', { name: /save permissions/i })).toBeDisabled()

    fireEvent.click(switchFor('contacts', 'write'))
    expect(screen.getByRole('button', { name: /save permissions/i })).toBeEnabled()
  })

  // Grant all keeps the opt-in grants the member already holds: replacing the map would strip
  // Audit logs read from them without anybody choosing to.
  it('keeps an opt-in grant when Grant all is pressed', async () => {
    renderUnder(['template_i18n'], {
      ...member,
      permissions: { contacts: { read: true, write: false }, audit_logs: { read: true, write: true } }
    } as unknown as WorkspaceMember)

    fireEvent.click(screen.getByRole('button', { name: 'Grant all' }))
    fireEvent.click(screen.getByRole('button', { name: /save permissions/i }))

    await waitFor(() => expect(setUserPermissions).toHaveBeenCalled())
    const { permissions } = setUserPermissions.mock.calls[0][0]
    expect(permissions.audit_logs).toEqual({ read: true, write: true })
    expect(permissions.contacts).toEqual({ read: true, write: true })
  })

  it('leaves everything live when the licence covers custom permissions', () => {
    renderUnder(['rbac'], restrictedMember)

    expect(screen.queryByText(/requires? a Notifuse/i)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /save permissions/i })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Revoke all' })).toBeEnabled()
    expect(switchFor('lists', 'read').disabled).toBe(false)
  })
})
