import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { LicenseContext, UNKNOWN_LICENSE } from '../../contexts/licenseState'
import type { Entitlements } from '../../types/license'
import { WorkspaceMembers } from './WorkspaceMembers'
import { ApiError } from '../../services/api/errors'
import type { PermissionResource, WorkspaceMember } from '../../services/api/types'
import {
  ALL_PERMISSION_RESOURCES,
  OPT_IN_PERMISSION_RESOURCES,
  ENFORCED_PERMISSION_VERBS,
  createEmptyPermissions,
  createFullPermissions,
  grantUnenforcedPermissions,
  isPermissionEnforced
} from '../../services/api/permissions'

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

// `licensedFor` omitted means the console has not been told, and every advisory read answers
// "licensed" — the state every other test here runs in.
interface RenderOptions {
  isOwner?: boolean
  currentUserId?: string
  error?: boolean
  onMembersChange?: () => void
}

const renderMembers = (
  members: WorkspaceMember[],
  licensedFor?: Entitlements['features'],
  options: RenderOptions = {}
) =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <LicenseContext.Provider
          value={{
            ...UNKNOWN_LICENSE,
            entitlements: licensedFor
              ? {
                  tier: 'Studio',
                  org: 'ACME SAS',
                  sub: 'billing@acme.com',
                  max_workspaces: 5,
                  features: licensedFor,
                  state: 'active',
                  expires_at: '2027-01-01T00:00:00Z'
                }
              : null
          }}
        >
          <WorkspaceMembers
            workspaceId="ws1"
            members={members}
            loading={false}
            error={options.error ?? false}
            onMembersChange={options.onMembersChange ?? vi.fn()}
            isOwner={options.isOwner ?? true}
            currentUserId={options.currentUserId}
          />
        </LicenseContext.Provider>
      </App>
    </I18nProvider>
  )

// A member row shows the email; an API key row shows its name, the part before "@".
const rowFor = (member: WorkspaceMember) => {
  const cell = screen.queryAllByText(member.email)[0] ?? screen.getAllByText(member.email.split('@')[0])[0]
  const row = cell.closest('tr')
  expect(row).not.toBeNull()
  return row as HTMLTableRowElement
}

// The row menu: its button, then the item of the dropdown antd renders in the body.
const openRowMenu = (member: WorkspaceMember) => {
  const trigger = within(rowFor(member)).queryByRole('button', { name: 'Actions' })
  expect(trigger).not.toBeNull()
  fireEvent.click(trigger as HTMLElement)
}

const chooseInMenu = async (item: string) => {
  fireEvent.click(await screen.findByRole('menuitem', { name: item }))
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
  openRowMenu(member)
  await chooseInMenu('Edit permissions')
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

describe('WorkspaceMembers access column', () => {
  // The shapes the console itself saves: every set leaves the drawer through
  // grantUnenforcedPermissions, so it carries the unenforced verbs, Audit logs write included.
  const savedFull = () => grantUnenforcedPermissions(createFullPermissions())
  // A row with details to show wraps its summary in a Popover, marked cursor-help; one without
  // renders the pill alone. Read synchronously, so a missing popover costs no timeout.
  const hasPopover = (member: WorkspaceMember) => rowFor(member).querySelector('.cursor-help') !== null

  it('names the resources of a partial grant instead of an unconditional Full access', () => {
    const member = apiKeyRow({ transactional: { read: false, write: true } })
    renderMembers([member])

    const row = rowFor(member)
    expect(row.textContent).toContain('Transactional')
    expect(row.textContent).not.toContain('read only')
    expect(row.textContent).not.toContain('Full access')
    expect(hasPopover(member)).toBe(true)
  })

  it('counts an API key holding every resource as Full access', () => {
    const granted = Object.fromEntries(
      ALL_PERMISSION_RESOURCES.map((resource) => [resource, { read: true, write: true }])
    ) as WorkspaceMember['permissions']
    const member = apiKeyRow(granted)
    renderMembers([member])

    expect(rowFor(member).textContent).toContain('Full access')
  })

  it('names two resources, then the number of the others, and keeps the count of verbs in the details', async () => {
    // Pinned here and in permissions.test.ts: fourteen canonical resources and Audit logs read,
    // less the three canonical verbs no endpoint enforces.
    expect(ENFORCED_PERMISSION_VERBS).toHaveLength(26)

    const member = apiKeyRow({
      contacts: { read: true, write: true },
      lists: { read: true, write: true },
      templates: { read: true, write: true }
    })
    renderMembers([member])

    const row = rowFor(member)
    expect(row.textContent).toContain('Contacts, Lists')
    expect(row.textContent).toContain('+1')
    expect(row.textContent).not.toContain('Full access')

    fireEvent.mouseEnter(row.querySelector('.cursor-help') as HTMLElement)
    expect(await screen.findByText('Permission details')).toBeInTheDocument()
    expect(await screen.findByText('6 of 26 permissions granted')).toBeInTheDocument()
  })

  it('says read only for a grant without any write', () => {
    const member = apiKeyRow({ contacts: { read: true, write: false }, lists: { read: true, write: false } })
    renderMembers([member])

    expect(rowFor(member).textContent).toContain('· read only')
  })

  it('shows Full access for a full set as the editor saves it, without details', () => {
    const member = apiKeyRow(savedFull())
    renderMembers([member])

    expect(rowFor(member).textContent).toContain('Full access')
    expect(hasPopover(member)).toBe(false)
  })

  it('shows Full access for a full set that can also read audit logs, and keeps that grant visible', async () => {
    const member = apiKeyRow({ ...savedFull(), audit_logs: { read: true, write: true } })
    renderMembers([member])

    expect(rowFor(member).textContent).toContain('Full access')
    expect(hasPopover(member)).toBe(true)

    fireEvent.mouseEnter(rowFor(member).querySelector('.cursor-help') as HTMLElement)
    expect(await screen.findByText('Permission details')).toBeInTheDocument()
  })

  it('does not call a set one verb short of full Full access', () => {
    const permissions = savedFull()
    permissions.contacts = { read: true, write: false }
    const member = apiKeyRow(permissions)
    renderMembers([member])

    expect(rowFor(member).textContent).not.toContain('Full access')
    expect(rowFor(member).textContent).toContain('Contacts, Segments')
  })

  // Older consoles and API callers could store an unenforced verb as false. It gates nothing, the
  // editor shows it granted and saves it so, so the row is full access.
  it('shows Full access for a full set stored with an unenforced verb off', () => {
    const member = apiKeyRow({ ...createFullPermissions(), llm: { read: false, write: true } })
    renderMembers([member])

    expect(rowFor(member).textContent).toContain('Full access')
  })

  it('shows No access for the set Revoke all saves, without details', () => {
    const member = apiKeyRow(grantUnenforcedPermissions(createEmptyPermissions()))
    renderMembers([member])

    expect(rowFor(member).textContent).toContain('No access')
    expect(hasPopover(member)).toBe(false)
  })

  it('names Audit logs for a key that can only read them instead of calling it No access', () => {
    const member = apiKeyRow({ audit_logs: { read: true, write: true } })
    renderMembers([member])

    expect(rowFor(member).textContent).not.toContain('No access')
    expect(rowFor(member).textContent).toContain('Audit Logs')
  })

  it.each([
    ['undefined', undefined],
    ['null', null as unknown as WorkspaceMember['permissions']],
    ['an empty map', {}]
  ])('renders a row whose permissions are %s as No access', (_label, permissions) => {
    const member = apiKeyRow(permissions)
    expect(() => renderMembers([member])).not.toThrow()
    expect(rowFor(member).textContent).toContain('No access')
  })
})

describe('WorkspaceMembers tables', () => {
  const human = (overrides: Partial<WorkspaceMember>): WorkspaceMember => ({
    user_id: 'user-1',
    workspace_id: 'ws1',
    role: 'member',
    email: 'member@example.com',
    type: 'user',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    permissions: { contacts: { read: true, write: false } },
    ...overrides
  })

  const tableOf = (title: string) => {
    const panel = screen.getByText(title).closest('section')
    expect(panel).not.toBeNull()
    return panel as HTMLElement
  }

  it('lists members and invitations under Members, API keys under API keys', () => {
    const key = apiKeyRow({ contacts: { read: true, write: false } })
    renderMembers([
      human({}),
      human({ user_id: '', email: 'guest@example.com', invitation_id: 'inv-1', invitation_expires_at: '2099-01-01T00:00:00Z' }),
      key
    ])

    const members = tableOf('Members')
    expect(within(members).getByText('member@example.com')).toBeInTheDocument()
    expect(within(members).getByText('guest@example.com')).toBeInTheDocument()
    expect(within(members).queryByText('sender')).not.toBeInTheDocument()

    const keys = tableOf('API keys')
    expect(within(keys).getByText('sender')).toBeInTheDocument()
    expect(within(keys).queryByText('member@example.com')).not.toBeInTheDocument()
  })

  it('shows the name with the email under it, and the email alone without a name', () => {
    renderMembers([
      human({ name: 'Julie Martin', email: 'julie@example.com' }),
      human({ user_id: 'user-2', email: 'noname@example.com' })
    ])

    const julie = rowFor({ ...human({}), email: 'julie@example.com' })
    expect(within(julie).getByText('Julie Martin')).toBeInTheDocument()
    expect(within(julie).getByText('JM')).toBeInTheDocument()
    expect(within(rowFor({ ...human({}), email: 'noname@example.com' })).getByText('NO')).toBeInTheDocument()
  })

  it('marks the signed-in member with (you)', () => {
    renderMembers([human({ user_id: 'me', name: 'Ann', email: 'ann@example.com' })], undefined, { currentUserId: 'me' })

    expect(rowFor({ ...human({}), email: 'ann@example.com' }).textContent).toContain('(you)')
  })

  it('dates an invitation and says when it expires', () => {
    const now = Date.now()
    renderMembers([
      human({
        user_id: '',
        email: 'guest@example.com',
        invitation_id: 'inv-1',
        created_at: new Date(now - 60 * 60 * 1000).toISOString(),
        invitation_expires_at: new Date(now + 6 * 24 * 60 * 60 * 1000 + 60_000).toISOString()
      })
    ])

    const row = rowFor({ ...human({}), email: 'guest@example.com' })
    expect(row.textContent).toMatch(/Invitation sent .*, expires in 7 days/)
    expect(row.textContent).toContain('Pending')
  })

  it('says an invitation expired while the page was open', () => {
    renderMembers([
      human({
        user_id: '',
        email: 'guest@example.com',
        invitation_id: 'inv-1',
        invitation_expires_at: new Date(Date.now() - 1000).toISOString()
      })
    ])

    expect(rowFor({ ...human({}), email: 'guest@example.com' }).textContent).toContain('Invitation expired')
  })

  it('keeps two invitations apart, though neither has a user id', () => {
    renderMembers([
      human({ user_id: '', email: 'a@example.com', invitation_id: 'inv-1', invitation_expires_at: '2099-01-01T00:00:00Z' }),
      human({ user_id: '', email: 'b@example.com', invitation_id: 'inv-2', invitation_expires_at: '2099-01-01T00:00:00Z' })
    ])

    const keys = Array.from(document.querySelectorAll('tr[data-row-key]')).map((row) => row.getAttribute('data-row-key'))
    expect(keys).toContain('inv-1')
    expect(keys).toContain('inv-2')
  })

  it('shows a platform admin without a joined date', () => {
    window.ROOT_EMAIL = 'root@example.com'
    renderMembers([human({ role: 'owner', email: 'root@example.com', name: 'Root', created_at: '2026-01-01T00:00:00Z' })])

    const row = rowFor({ ...human({}), email: 'root@example.com' })
    expect(row.textContent).toContain('Platform admin')
    expect(row.textContent).not.toContain('2026')
    window.ROOT_EMAIL = ''
  })

  it('names an API key after its address, and gives the address in a tooltip', async () => {
    renderMembers([apiKeyRow({ contacts: { read: true, write: false } })])

    const name = within(tableOf('API keys')).getByText('sender')
    fireEvent.mouseEnter(name)
    expect(await screen.findByText('sender@api.example.com')).toBeInTheDocument()
  })

  it('says there is no API key yet', () => {
    renderMembers([human({})])

    expect(screen.getByText('No API keys yet. Create one to call the API or connect Zapier.')).toBeInTheDocument()
  })

  it('offers no menu to a member who is not owner, nor on an owner row', () => {
    const { unmount } = renderMembers([human({})], undefined, { isOwner: false })
    expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Invite' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'New key' })).not.toBeInTheDocument()
    unmount()

    renderMembers([human({ role: 'owner', email: 'owner@example.com' })])
    expect(within(rowFor({ ...human({}), email: 'owner@example.com' })).queryByRole('button', { name: 'Actions' })).toBeNull()
  })

  it('removes a member after confirmation', async () => {
    api.removeMember.mockResolvedValue({})
    const member = human({})
    renderMembers([member])

    openRowMenu(member)
    await chooseInMenu('Remove from workspace')
    fireEvent.click(await screen.findByRole('button', { name: 'Remove' }))

    await waitFor(() => expect(api.removeMember).toHaveBeenCalledWith({ workspace_id: 'ws1', user_id: 'user-1' }))
  })

  it('deletes an API key after its confirmation', async () => {
    api.removeMember.mockResolvedValue({})
    const key = apiKeyRow({ contacts: { read: true, write: false } })
    renderMembers([key])

    openRowMenu(key)
    await chooseInMenu('Delete key')
    expect(await screen.findByText(/This API key will be permanently deleted/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(api.removeMember).toHaveBeenCalledWith({ workspace_id: 'ws1', user_id: 'key-1' }))
  })

  it('resends an invitation with the grant it carries', async () => {
    const invitation = human({ user_id: '', email: 'guest@example.com', invitation_id: 'inv-1', invitation_expires_at: '2099-01-01T00:00:00Z' })
    renderMembers([invitation])

    openRowMenu(invitation)
    await chooseInMenu('Resend invitation')

    await waitFor(() => expect(api.inviteMember).toHaveBeenCalled())
    expect(api.inviteMember.mock.calls[0][0].permissions.contacts).toEqual({ read: true, write: false })
    expect(api.inviteMember.mock.calls[0][0].permissions.lists).toEqual({ read: false, write: false })
  })

  it('deletes an invitation after confirmation', async () => {
    api.deleteInvitation.mockResolvedValue({})
    const invitation = human({ user_id: '', email: 'guest@example.com', invitation_id: 'inv-1', invitation_expires_at: '2099-01-01T00:00:00Z' })
    renderMembers([invitation])

    openRowMenu(invitation)
    await chooseInMenu('Delete invitation')
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(api.deleteInvitation).toHaveBeenCalledWith({ invitation_id: 'inv-1' }))
  })

  it('tells the owner when the plan has no room for another member', async () => {
    api.inviteMember.mockRejectedValue(new ApiError('team member limit reached', 403))
    renderMembers([])
    fireEvent.click(screen.getByRole('button', { name: 'Invite' }))
    const drawer = await openDrawer('Invite Member')
    fireEvent.change(within(drawer).getByRole('textbox'), { target: { value: 'new@example.com' } })
    fireEvent.click(within(footerOf(drawer)).getByRole('button', { name: 'Send Invitation' }))

    expect(
      await screen.findByText('Team member limit reached. Please upgrade your plan to add more members.')
    ).toBeInTheDocument()
  })

  it('offers to retry when the members could not load', () => {
    const onRetry = vi.fn()
    renderMembers([], undefined, { error: true, onMembersChange: onRetry })

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalled()
  })
})

describe('permissions matrix', () => {
  it('renders a row per resource when the stored map holds fewer', async () => {
    const member = apiKeyRow({ transactional: { read: false, write: true } })
    renderMembers([member])
    const dialog = await openPermissionsEditor(member)

    expect(matrixRows(dialog)).toHaveLength(
      ALL_PERMISSION_RESOURCES.length + OPT_IN_PERMISSION_RESOURCES.length
    )
    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(dialog.querySelector(`tr[data-row-key="${resource}"]`)).not.toBeNull()
    }
  })

  it('renders the full grid for a key whose stored map is empty', async () => {
    const member = apiKeyRow({})
    renderMembers([member])
    const dialog = await openPermissionsEditor(member)

    expect(matrixRows(dialog)).toHaveLength(
      ALL_PERMISSION_RESOURCES.length + OPT_IN_PERMISSION_RESOURCES.length
    )
    expect(switchesFor(dialog, 'segments').read).toHaveAttribute('aria-checked', 'false')
  })

  it('renders the full grid for a row carrying no permissions at all', async () => {
    const member = apiKeyRow(null as unknown as WorkspaceMember['permissions'])
    renderMembers([member])
    const dialog = await openPermissionsEditor(member)

    expect(matrixRows(dialog)).toHaveLength(
      ALL_PERMISSION_RESOURCES.length + OPT_IN_PERMISSION_RESOURCES.length
    )
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
    expect(Object.keys(permissions).sort()).toEqual([...ALL_PERMISSION_RESOURCES, ...OPT_IN_PERMISSION_RESOURCES].sort())
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
    fireEvent.click(screen.getByRole('button', { name: 'New key' }))
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
    expect(Object.keys(request.permissions).sort()).toEqual([...ALL_PERMISSION_RESOURCES, ...OPT_IN_PERMISSION_RESOURCES].sort())
  })

  it('offers a switch for every resource in the canonical list', async () => {
    const drawer = await openCreateApiKey()

    expect(matrixRows(drawer)).toHaveLength(ALL_PERMISSION_RESOURCES.length + OPT_IN_PERMISSION_RESOURCES.length)
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

  // The same rule as the invitation: a full key is never gated, a scoped one is. The matrix used
  // to stay fully live without the licence, so the refusal only came as a 402 on Create.
  it('greys every choice that narrows the key under an unlicensed deployment, and creates a full key', async () => {
    renderMembers([], ['template_i18n'])
    fireEvent.click(screen.getByRole('button', { name: 'New key' }))
    const drawer = await openDrawer('Create API Key')

    expect(
      within(drawer).getByText('Custom permissions require a Notifuse Studio licence.')
    ).toBeInTheDocument()
    for (const resource of ALL_PERMISSION_RESOURCES) {
      const { read, write } = switchesFor(drawer, resource)
      expect(read.disabled).toBe(true)
      expect(write.disabled).toBe(true)
    }
    expect(within(drawer).getByRole('button', { name: 'Revoke all' })).toBeDisabled()

    fireEvent.change(within(drawer).getByRole('textbox'), { target: { value: 'sender' } })
    const create = within(footerOf(drawer)).getByRole('button', { name: 'Create API Key' })
    expect(create).toBeEnabled()
    fireEvent.click(create)

    await waitFor(() => expect(api.createAPIKey).toHaveBeenCalled())
    const { permissions } = api.createAPIKey.mock.calls[0][0]
    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(permissions[resource]).toEqual({ read: true, write: true })
    }
  })

  it('leaves the key matrix live when the licence covers custom permissions', async () => {
    renderMembers([], ['rbac'])
    fireEvent.click(screen.getByRole('button', { name: 'New key' }))
    const drawer = await openDrawer('Create API Key')

    expect(within(drawer).queryByText(/requires? a Notifuse/i)).not.toBeInTheDocument()
    expect(switchesFor(drawer, 'contacts').write.disabled).toBe(false)
    expect(within(drawer).getByRole('button', { name: 'Revoke all' })).toBeEnabled()
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
  const openInvite = async (licensedFor?: Entitlements['features']) => {
    renderMembers([], licensedFor)
    fireEvent.click(screen.getByRole('button', { name: 'Invite' }))
    return openDrawer('Invite Member')
  }

  // An invite starts from a full grant, which the server never gates, so it can be sent as it
  // is. What is locked is narrowing it — the licensed capability — and it is said here, before
  // Send, rather than as a 402 after it.
  it('greys every choice that narrows the grant under an unlicensed deployment, and says what to buy', async () => {
    const drawer = await openInvite(['template_i18n'])

    expect(
      within(drawer).getByText('Custom permissions require a Notifuse Studio licence.')
    ).toBeInTheDocument()
    for (const resource of ALL_PERMISSION_RESOURCES) {
      const { read, write } = switchesFor(drawer, resource)
      expect(read.disabled).toBe(true)
      expect(write.disabled).toBe(true)
    }
    expect(within(drawer).getByRole('button', { name: 'Revoke all' })).toBeDisabled()
    expect(within(drawer).getByRole('button', { name: 'Grant all' })).toBeEnabled()
    expect(within(footerOf(drawer)).getByRole('button', { name: 'Send Invitation' })).toBeEnabled()
  })

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
    expect(Object.keys(permissions).sort()).toEqual([...ALL_PERMISSION_RESOURCES, ...OPT_IN_PERMISSION_RESOURCES].sort())
    expect(permissions.segments).toEqual({ read: true, write: true })
    expect(permissions.webhook_events).toEqual({ read: true, write: true })
    expect(permissions.webhook_subscriptions).toEqual({ read: true, write: false })
  })

  it('offers a switch for every resource in the canonical list', async () => {
    const drawer = await openInvite()

    expect(matrixRows(drawer)).toHaveLength(ALL_PERMISSION_RESOURCES.length + OPT_IN_PERMISSION_RESOURCES.length)
    for (const resource of ALL_PERMISSION_RESOURCES) {
      expect(drawer.querySelector(`tr[data-row-key="${resource}"]`)).not.toBeNull()
    }
  })
})
