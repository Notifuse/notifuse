import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { WorkspaceMembers } from './WorkspaceMembers'
import type { PermissionResource, WorkspaceMember } from '../../services/api/types'
import { ALL_PERMISSION_RESOURCES, isPermissionEnforced } from '../../services/api/permissions'

const api = vi.hoisted(() => ({
  createAPIKey: vi.fn(),
  setUserPermissions: vi.fn(),
  inviteMember: vi.fn(),
  removeMember: vi.fn(),
  deleteInvitation: vi.fn()
}))

vi.mock('../../services/api/workspace', () => ({ workspaceService: api }))

i18n.loadAndActivate({ locale: 'en', messages: {} })

beforeEach(() => {
  vi.clearAllMocks()
  api.createAPIKey.mockResolvedValue({ token: 'tok', email: 'sender@api.example.com' })
  api.setUserPermissions.mockResolvedValue({ status: 'ok', message: '' })
  api.inviteMember.mockResolvedValue({ status: 'ok', message: '' })
  // Carries a trailing path segment so the normalisation is exercised, not just the host.
  window.API_ENDPOINT = 'https://api.example.com/'
})

const apiKeyRow = (permissions: WorkspaceMember['permissions']): WorkspaceMember => ({
  user_id: 'key-1',
  workspace_id: 'ws1',
  role: 'member',
  email: 'sender@api.example.com',
  type: 'api_key',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  permissions
})

const renderMembers = (members: WorkspaceMember[]) =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <WorkspaceMembers
          workspaceId="ws1"
          members={members}
          loading={false}
          onMembersChange={vi.fn()}
          isOwner={true}
        />
      </App>
    </I18nProvider>
  )

// faUserCog is an alias; Font Awesome renders it under the canonical name.
const GEAR_ICON = '[data-icon="user-gear"]'

const rowFor = (member: WorkspaceMember) => {
  const row = screen.getByText(member.email).closest('tr')
  expect(row).not.toBeNull()
  return row as HTMLTableRowElement
}

// Every host of the editable matrix is a Drawer: the matrix carries fourteen expandable rows and
// an expanded one lists every endpoint the permission gates, which no centred dialog holds. antd
// gives the drawer panel role=dialog and labels it with its title, so the role query still names
// the host — the class assertion is what keeps a silent slide back to Modal from passing.
const openDrawer = async (name: string) => {
  const drawer = await screen.findByRole('dialog', { name })
  expect(drawer.className).toContain('ant-drawer-section')
  expect(drawer.closest('.ant-modal')).toBeNull()
  return drawer as HTMLElement
}

const footerOf = (drawer: HTMLElement) => {
  const footer = drawer.querySelector('.ant-drawer-footer')
  expect(footer).not.toBeNull()
  return footer as HTMLElement
}

const openPermissionsEditor = async (member: WorkspaceMember) => {
  const gear = rowFor(member).querySelector(GEAR_ICON)?.closest('button')
  expect(gear).not.toBeNull()
  fireEvent.click(gear as HTMLButtonElement)
  return openDrawer(`Edit Permissions for ${member.email}`)
}

const matrixRows = (dialog: HTMLElement) =>
  Array.from(dialog.querySelectorAll('tbody tr[data-row-key]'))

// The Read switch is the first of the row, the Write switch the second.
const switchesFor = (dialog: HTMLElement, resource: PermissionResource) => {
  const row = dialog.querySelector(`tr[data-row-key="${resource}"]`)
  expect(row).not.toBeNull()
  const [read, write] = Array.from(
    (row as HTMLElement).querySelectorAll<HTMLButtonElement>('button[role="switch"]')
  )
  return { read, write }
}

describe('WorkspaceMembers permissions column', () => {
  it('renders an API key grant as a count instead of an unconditional Full Access', () => {
    const member = apiKeyRow({ transactional: { read: false, write: true } })
    renderMembers([member])

    const row = rowFor(member)
    expect(row.textContent).toContain(`1/${ALL_PERMISSION_RESOURCES.length * 2}`)
    expect(row.textContent).not.toContain('Full Access')
  })

  it('counts an API key holding every resource as Full Access', () => {
    const granted = Object.fromEntries(
      ALL_PERMISSION_RESOURCES.map((resource) => [resource, { read: true, write: true }])
    ) as WorkspaceMember['permissions']
    const member = apiKeyRow(granted)
    renderMembers([member])

    expect(rowFor(member).textContent).toContain('Full Access')
  })

  it('uses the whole resource list as the badge denominator, not the row key count', () => {
    expect(ALL_PERMISSION_RESOURCES).toHaveLength(14)

    // A row that grants read+write on three resources is not Full Access.
    const member = apiKeyRow({
      contacts: { read: true, write: true },
      lists: { read: true, write: true },
      templates: { read: true, write: true }
    })
    renderMembers([member])

    const row = rowFor(member)
    expect(row.textContent).toContain('6/28')
    expect(row.textContent).not.toContain('Full Access')
  })

  it('renders a row whose permissions are undefined without throwing', () => {
    const member = apiKeyRow(undefined)
    expect(() => renderMembers([member])).not.toThrow()
    expect(rowFor(member).textContent).toContain('No Access')
  })

  it('renders a row whose permissions are null without throwing', () => {
    const member = apiKeyRow(null as unknown as WorkspaceMember['permissions'])
    expect(() => renderMembers([member])).not.toThrow()
    expect(rowFor(member).textContent).toContain('No Access')
  })

  it('renders a row whose permissions are an empty map without throwing', () => {
    const member = apiKeyRow({})
    expect(() => renderMembers([member])).not.toThrow()
    expect(rowFor(member).textContent).toContain('No Access')
  })
})

describe('WorkspaceMembers actions', () => {
  it('offers the edit-permissions control on an API key row', () => {
    const member = apiKeyRow({ transactional: { read: false, write: true } })
    renderMembers([member])

    expect(rowFor(member).querySelector(GEAR_ICON)).not.toBeNull()
  })

  it('offers the edit-permissions control on a human member row', () => {
    const member: WorkspaceMember = {
      ...apiKeyRow({ contacts: { read: true, write: false } }),
      user_id: 'user-1',
      email: 'member@example.com',
      type: 'user'
    }
    renderMembers([member])

    expect(rowFor(member).querySelector(GEAR_ICON)).not.toBeNull()
  })
})

describe('permissions matrix', () => {
  it('renders a row per resource when the stored map holds fewer', async () => {
    const member = apiKeyRow({ transactional: { read: false, write: true } })
    renderMembers([member])
    const dialog = await openPermissionsEditor(member)

    expect(matrixRows(dialog)).toHaveLength(ALL_PERMISSION_RESOURCES.length)
    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(dialog.querySelector(`tr[data-row-key="${resource}"]`)).not.toBeNull()
    }
  })

  it('renders the full grid for a key whose stored map is empty', async () => {
    const member = apiKeyRow({})
    renderMembers([member])
    const dialog = await openPermissionsEditor(member)

    expect(matrixRows(dialog)).toHaveLength(ALL_PERMISSION_RESOURCES.length)
    expect(switchesFor(dialog, 'segments').read).toHaveAttribute('aria-checked', 'false')
  })

  it('renders the full grid for a row carrying no permissions at all', async () => {
    const member = apiKeyRow(null as unknown as WorkspaceMember['permissions'])
    renderMembers([member])
    const dialog = await openPermissionsEditor(member)

    expect(matrixRows(dialog)).toHaveLength(ALL_PERMISSION_RESOURCES.length)
  })

  it('grants a resource the stored map does not mention', async () => {
    const member = apiKeyRow({ transactional: { read: false, write: true } })
    renderMembers([member])
    const dialog = await openPermissionsEditor(member)

    fireEvent.click(switchesFor(dialog, 'segments').write)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save Permissions' }))

    await waitFor(() => expect(api.setUserPermissions).toHaveBeenCalled())
    const { permissions } = api.setUserPermissions.mock.calls[0][0]
    expect(permissions.segments).toEqual({ read: false, write: true })
    // The resource the row already held keeps its grant, and nothing else is widened.
    expect(permissions.transactional).toEqual({ read: false, write: true })
    expect(permissions.contacts).toEqual({ read: false, write: false })
    expect(Object.keys(permissions).sort()).toEqual([...ALL_PERMISSION_RESOURCES].sort())
  })

  it('locks the unenforceable verbs on rather than persisting them off', async () => {
    const member = apiKeyRow({})
    renderMembers([member])
    const dialog = await openPermissionsEditor(member)

    const llm = switchesFor(dialog, 'llm')
    expect(llm.read).toBeDisabled()
    expect(llm.read).toHaveAttribute('aria-checked', 'true')
    expect(llm.write).toBeEnabled()
    expect(llm.write).toHaveAttribute('aria-checked', 'false')

    const messageHistory = switchesFor(dialog, 'message_history')
    expect(messageHistory.write).toBeDisabled()
    expect(messageHistory.write).toHaveAttribute('aria-checked', 'true')
    expect(messageHistory.read).toBeEnabled()
    expect(messageHistory.read).toHaveAttribute('aria-checked', 'false')

    // Saving an untouched form must not freeze them at false: a backfill only ever adds keys a
    // row is missing, so a stored false would never be widened.
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save Permissions' }))
    await waitFor(() => expect(api.setUserPermissions).toHaveBeenCalled())
    const { permissions } = api.setUserPermissions.mock.calls[0][0]
    expect(permissions.llm).toEqual({ read: true, write: false })
    expect(permissions.message_history).toEqual({ read: false, write: true })
  })
})

describe('create API key drawer', () => {
  const openCreateApiKey = async () => {
    renderMembers([])
    fireEvent.click(screen.getByRole('button', { name: 'Create API Key' }))
    return openDrawer('Create API Key')
  }

  it('caps the email prefix at the length the server accepts', async () => {
    const drawer = await openCreateApiKey()

    expect(within(drawer).getByRole('textbox')).toHaveAttribute('maxlength', '64')
  })

  it('posts the scoped map the owner selected', async () => {
    const drawer = await openCreateApiKey()

    fireEvent.change(within(drawer).getByRole('textbox'), { target: { value: 'sender' } })
    fireEvent.click(switchesFor(drawer, 'contacts').write)
    fireEvent.click(switchesFor(drawer, 'broadcasts').read)
    fireEvent.click(
      within(footerOf(drawer)).getByRole('button', {
        name: 'Create API Key'
      })
    )

    await waitFor(() => expect(api.createAPIKey).toHaveBeenCalled())
    const request = api.createAPIKey.mock.calls[0][0]
    expect(request.email_prefix).toBe('sender')
    expect(request.permissions.contacts).toEqual({ read: true, write: false })
    expect(request.permissions.broadcasts).toEqual({ read: false, write: true })
    // Everything untouched keeps the full access an unscoped key has always had.
    expect(request.permissions.transactional).toEqual({ read: true, write: true })
    expect(Object.keys(request.permissions).sort()).toEqual([...ALL_PERMISSION_RESOURCES].sort())
  })

  it('offers a switch for every resource in the canonical list', async () => {
    const drawer = await openCreateApiKey()

    expect(matrixRows(drawer)).toHaveLength(ALL_PERMISSION_RESOURCES.length)
    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(drawer.querySelector(`tr[data-row-key="${resource}"]`)).not.toBeNull()
    }
  })

  it('scopes a key down to the integration resources without touching every other switch', async () => {
    const drawer = await openCreateApiKey()
    const scoped: PermissionResource[] = ['webhook_subscriptions', 'contacts', 'lists']

    fireEvent.change(within(drawer).getByRole('textbox'), { target: { value: 'zapier' } })
    fireEvent.click(within(drawer).getByRole('button', { name: 'Revoke all' }))
    for (const resource of scoped) {
      fireEvent.click(switchesFor(drawer, resource).read)
      fireEvent.click(switchesFor(drawer, resource).write)
    }
    fireEvent.click(within(footerOf(drawer)).getByRole('button', { name: 'Create API Key' }))

    await waitFor(() => expect(api.createAPIKey).toHaveBeenCalled())
    const { permissions } = api.createAPIKey.mock.calls[0][0]
    for (const resource of scoped) {
      expect(permissions[resource]).toEqual({ read: true, write: true })
    }
    // Everything else is denied, except the verbs no gate can enforce — those stay granted so a
    // stored `false` no backfill would widen never reaches the row.
    for (const resource of ALL_PERMISSION_RESOURCES) {
      if (scoped.includes(resource)) continue
      const granted = permissions[resource]
      expect(granted.read).toBe(!isPermissionEnforced(resource, 'read'))
      expect(granted.write).toBe(!isPermissionEnforced(resource, 'write'))
    }
  })

  it('restores the full grant from the revoked state', async () => {
    const drawer = await openCreateApiKey()

    fireEvent.change(within(drawer).getByRole('textbox'), { target: { value: 'sender' } })
    fireEvent.click(within(drawer).getByRole('button', { name: 'Revoke all' }))
    fireEvent.click(within(drawer).getByRole('button', { name: 'Grant all' }))
    fireEvent.click(within(footerOf(drawer)).getByRole('button', { name: 'Create API Key' }))

    await waitFor(() => expect(api.createAPIKey).toHaveBeenCalled())
    const { permissions } = api.createAPIKey.mock.calls[0][0]
    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(permissions[resource]).toEqual({ read: true, write: true })
    }
  })

  it('advertises the address the server actually mints, without a workspace id in it', async () => {
    const drawer = await openCreateApiKey()

    // The suffix sits in a disabled button beside the prefix field.
    expect(within(drawer).getByText('@api.example.com')).toBeInTheDocument()
    expect(drawer.textContent).not.toContain('ws1.api.example.com')
  })

  it('swaps the drawer footer for a single Close once the one-time token is shown', async () => {
    const drawer = await openCreateApiKey()

    fireEvent.change(within(drawer).getByRole('textbox'), { target: { value: 'sender' } })
    fireEvent.click(within(footerOf(drawer)).getByRole('button', { name: 'Create API Key' }))

    await waitFor(() => expect(api.createAPIKey).toHaveBeenCalled())
    // The token replaces the form, so the only textbox left is the one holding it.
    await waitFor(() => expect(within(drawer).getByRole('textbox')).toHaveValue('tok'))

    // Nothing is left to submit or to cancel: one button, and it says Close.
    const footer = footerOf(drawer)
    expect(within(footer).getAllByRole('button')).toHaveLength(1)
    expect(within(footer).getByRole('button', { name: 'Close' })).toBeInTheDocument()
    expect(within(drawer).queryByRole('table')).toBeNull()
  })
})

describe('invite member drawer', () => {
  const openInvite = async () => {
    renderMembers([])
    fireEvent.click(screen.getByRole('button', { name: 'Invite Member' }))
    return openDrawer('Invite Member')
  }

  it('sends a grant naming every canonical resource', async () => {
    const drawer = await openInvite()

    fireEvent.change(within(drawer).getByRole('textbox'), {
      target: { value: 'member@example.com' }
    })
    fireEvent.click(switchesFor(drawer, 'webhook_subscriptions').write)
    fireEvent.click(within(footerOf(drawer)).getByRole('button', { name: 'Send Invitation' }))

    await waitFor(() => expect(api.inviteMember).toHaveBeenCalled())
    const { permissions } = api.inviteMember.mock.calls[0][0]
    // A resource the map omits is denied on the server, so an invite that never names segments,
    // webhook_subscriptions or webhook_events silently strips them.
    expect(Object.keys(permissions).sort()).toEqual([...ALL_PERMISSION_RESOURCES].sort())
    expect(permissions.segments).toEqual({ read: true, write: true })
    expect(permissions.webhook_events).toEqual({ read: true, write: true })
    expect(permissions.webhook_subscriptions).toEqual({ read: true, write: false })
  })

  it('offers a switch for every resource in the canonical list', async () => {
    const drawer = await openInvite()

    expect(matrixRows(drawer)).toHaveLength(ALL_PERMISSION_RESOURCES.length)
    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(drawer.querySelector(`tr[data-row-key="${resource}"]`)).not.toBeNull()
    }
  })
})
