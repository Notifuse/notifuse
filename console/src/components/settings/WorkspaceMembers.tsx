import { useState, type ReactNode } from 'react'
import {
  Table,
  Button,
  Drawer,
  Form,
  Input,
  App,
  Alert,
  Space,
  Tooltip,
  Popover,
  Dropdown,
  Result,
  Skeleton
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { AtSign, KeyRound, MoreHorizontal, Plus } from 'lucide-react'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import { WorkspaceMember, UserPermissions, StoredPermissions } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import {
  createEmptyPermissions,
  createFullPermissions,
  grantsFullPermissions
} from '../../services/api/permissions'
import { ApiError } from '../../services/api/client'
import { isRootUser } from '../../services/api/auth'
import { EditPermissionsDrawer } from './EditPermissionsDrawer'
import { PermissionsMatrix } from './PermissionsMatrix'
import { PermissionScopePresets } from './PermissionScopePresets'
import { useLicense } from '../../hooks/useLicense'
import { LicenceGateNotice } from '../license/LicenceGateNotice'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { Panel, Pill } from '../layout'
import { initialsFor } from '../../layouts/app-shell/gravatar'
import { summarizeAccess } from './accessSummary'
import { PERMISSION_RESOURCE_LABELS } from './permissionLabels'

const DAY = 24 * 60 * 60 * 1000

interface WorkspaceMembersProps {
  workspaceId: string
  members: WorkspaceMember[]
  loading: boolean
  /** The members could not load: the panel offers to retry (onMembersChange). */
  error?: boolean
  onMembersChange: () => void
  isOwner: boolean
  /** The signed-in user, marked "(you)". */
  currentUserId?: string
}

/** The Access cell: a pill, or the resources of a partial grant, with the matrix on hover. */
function AccessCell({ member }: { member: WorkspaceMember }) {
  const { t, i18n } = useLingui()
  const summary = summarizeAccess(member)
  const permissions: StoredPermissions = member.permissions ?? createEmptyPermissions()

  const withDetails = (content: ReactNode, count?: ReactNode) => (
    <Popover
      title={t`Permission details`}
      trigger="hover"
      content={
        <div className="flex flex-col gap-2">
          {count ? <div className="text-xs text-ink-3">{count}</div> : null}
          <PermissionsMatrix value={permissions} />
        </div>
      }
    >
      <span className="cursor-help">{content}</span>
    </Popover>
  )

  switch (summary.kind) {
    case 'owner':
      return (
        <Pill tone="primary" dot={false}>
          {t`Owner`}
        </Pill>
      )
    case 'pending':
      return <Pill tone="warning">{t`Pending`}</Pill>
    case 'none':
      return (
        <Pill tone="danger" dot={false}>
          {t`No access`}
        </Pill>
      )
    case 'full': {
      const pill = (
        <Pill tone="success" dot={false}>
          {t`Full access`}
        </Pill>
      )
      // An opt-in grant on top of full access (Audit logs read) is not implied by it, so it
      // stays one hover away rather than hidden behind the same pill as a plain full grant.
      return summary.optIn ? withDetails(pill) : pill
    }
    case 'partial': {
      const names = summary.resources.map((resource) => i18n._(PERMISSION_RESOURCE_LABELS[resource])).join(', ')
      const more = summary.more
      const active = summary.active
      const total = summary.total
      return withDetails(
        <span className="text-sm text-ink-2">
          {names}
          {more > 0 ? ` +${more}` : null}
          {summary.readOnly ? <span className="text-ink-3"> {t`· read only`}</span> : null}
        </span>,
        t`${active} of ${total} permissions granted`
      )
    }
  }
}

function Avatar({ children }: { children: ReactNode }) {
  return (
    <span className="grid size-6 shrink-0 place-items-center rounded-full bg-primary-100 text-2xs font-semibold text-primary-700">
      {children}
    </span>
  )
}

export function WorkspaceMembers({
  workspaceId,
  members,
  loading,
  error = false,
  onMembersChange,
  isOwner,
  currentUserId
}: WorkspaceMembersProps) {
  const { t, i18n } = useLingui()
  const [inviteDrawerOpen, setInviteDrawerOpen] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviting, setInviting] = useState(false)
  const [invitePermissions, setInvitePermissions] =
    useState<UserPermissions>(createFullPermissions)
  const { has } = useLicense()
  // Without the rbac licence the server accepts full access and refuses anything narrower, on
  // invitations and API keys alike: the choices that would narrow a set are greyed, and the
  // submit button waits until the set is full access. Advisory: the 402 stays the real gate.
  const rbacLocked = !has('rbac')
  const { message } = App.useApp()

  // API Key drawer states
  const [apiKeyDrawerOpen, setApiKeyDrawerOpen] = useState(false)
  const [apiKeyName, setApiKeyName] = useState('')
  // Defaults to a full grant, which is what an unscoped key has always been.
  const [apiKeyPermissions, setApiKeyPermissions] =
    useState<UserPermissions>(createFullPermissions)
  const [creatingApiKey, setCreatingApiKey] = useState(false)
  const [apiKeyToken, setApiKeyToken] = useState('')
  const [, setRemovingMember] = useState(false)
  const [, setResendingInvitation] = useState(false)

  // Permissions drawer states
  const [permissionsDrawerOpen, setPermissionsDrawerOpen] = useState(false)
  const [editingMember, setEditingMember] = useState<WorkspaceMember | null>(null)

  const { modal } = App.useApp()
  const shortDate = (value: string) =>
    new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium' }).format(new Date(value))

  const isInvitation = (member: WorkspaceMember) => Boolean(member.invitation_expires_at)
  // Platform admins (ROOT_EMAIL) are surfaced to an owner as owner rows the server builds; their
  // created_at is the time of the request, not a date they joined.
  const isPlatformAdmin = (member: WorkspaceMember) => member.role === 'owner' && isRootUser(member.email)

  const memberRows = members
    .filter((member) => member.type !== 'api_key')
    .sort((a, b) => {
      if ((a.role === 'owner') !== (b.role === 'owner')) return a.role === 'owner' ? -1 : 1
      return a.created_at.localeCompare(b.created_at)
    })
  const apiKeyRows = members
    .filter((member) => member.type === 'api_key')
    .sort((a, b) => b.created_at.localeCompare(a.created_at))

  const confirmRemove = (member: WorkspaceMember) => {
    const email = member.email
    modal.confirm({
      title: member.type === 'api_key' ? t`Delete this API key?` : t`Remove member`,
      content:
        member.type === 'api_key'
          ? t`Are you sure you want to remove ${email}? This API key will be permanently deleted.`
          : t`Are you sure you want to remove ${email}?`,
      okText: member.type === 'api_key' ? t`Delete` : t`Remove`,
      cancelText: t`Cancel`,
      okButtonProps: { danger: true },
      onOk: () => handleRemoveMember(member.user_id)
    })
  }

  const confirmDeleteInvitation = (member: WorkspaceMember) => {
    const email = member.email
    modal.confirm({
      title: t`Delete invitation`,
      content: t`Are you sure you want to delete the invitation for ${email}?`,
      okText: t`Delete`,
      cancelText: t`Cancel`,
      okButtonProps: { danger: true },
      onOk: () => handleDeleteInvitation(member.invitation_id!)
    })
  }

  const actionsFor = (member: WorkspaceMember) => {
    if (!isOwner || member.role === 'owner') return null
    const items = isInvitation(member)
      ? [
          { key: 'resend', label: t`Resend invitation`, onClick: () => handleResendInvitation(member) },
          { key: 'delete', label: t`Delete invitation`, danger: true, onClick: () => confirmDeleteInvitation(member) }
        ]
      : [
          { key: 'edit', label: t`Edit permissions`, onClick: () => handleEditPermissions(member) },
          member.type === 'api_key'
            ? { key: 'remove', label: t`Delete key`, danger: true, onClick: () => confirmRemove(member) }
            : { key: 'remove', label: t`Remove from workspace`, danger: true, onClick: () => confirmRemove(member) }
        ]
    return (
      <Dropdown trigger={['click']} menu={{ items }} placement="bottomRight">
        <Button
          type="text"
          size="small"
          aria-label={t`Actions`}
          icon={<MoreHorizontal size={16} aria-hidden="true" />}
        />
      </Dropdown>
    )
  }

  const invitationLine = (member: WorkspaceMember) => {
    const expiresAt = new Date(member.invitation_expires_at as string).getTime()
    const remaining = expiresAt - Date.now()
    if (remaining <= 0) return <span className="text-warning-fg">{t`Invitation expired`}</span>
    const sent = shortDate(member.created_at)
    const days = Math.ceil(remaining / DAY)
    const expires = plural(days, { one: 'expires in # day', other: 'expires in # days' })
    return t`Invitation sent ${sent}, ${expires}`
  }

  const memberColumns: ColumnsType<WorkspaceMember> = [
    {
      title: t`Member`,
      key: 'member',
      render: (_, member) => {
        const invitation = isInvitation(member)
        const you = member.user_id !== '' && member.user_id === currentUserId
        return (
          <div className="flex min-w-0 items-center gap-2.5">
            <Avatar>{invitation ? <AtSign size={12} aria-hidden="true" /> : initialsFor(member)}</Avatar>
            <div className="flex min-w-0 flex-col">
              <div className="flex min-w-0 items-center gap-1.5">
                <span className="truncate font-medium text-ink">{member.name || member.email}</span>
                {you ? <span className="shrink-0 text-ink-3">{t`(you)`}</span> : null}
                {isPlatformAdmin(member) ? (
                  <Pill tone="neutral" dot={false}>
                    {t`Platform admin`}
                  </Pill>
                ) : null}
              </div>
              {member.name && !invitation ? <span className="truncate text-xs text-ink-3">{member.email}</span> : null}
              {invitation ? <span className="truncate text-xs text-ink-3">{invitationLine(member)}</span> : null}
            </div>
          </div>
        )
      }
    },
    { title: t`Access`, key: 'access', width: 260, render: (_, member) => <AccessCell member={member} /> },
    {
      title: t`Joined`,
      key: 'joined',
      width: 130,
      render: (_, member) =>
        isInvitation(member) || isPlatformAdmin(member) ? null : (
          <span className="text-ink-3">{shortDate(member.created_at)}</span>
        )
    },
    { title: '', key: 'actions', width: 48, align: 'right', render: (_, member) => actionsFor(member) }
  ]

  const apiKeyColumns: ColumnsType<WorkspaceMember> = [
    {
      title: t`Name`,
      key: 'name',
      render: (_, member) => (
        // createAPIKey builds the address from the name typed: the part before "@" is that name.
        <Tooltip title={member.email}>
          <span className="font-medium text-ink">{member.email.split('@')[0]}</span>
        </Tooltip>
      )
    },
    { title: t`Access`, key: 'access', width: 260, render: (_, member) => <AccessCell member={member} /> },
    {
      title: t`Created`,
      key: 'created',
      width: 130,
      render: (_, member) => <span className="text-ink-3">{shortDate(member.created_at)}</span>
    },
    { title: '', key: 'actions', width: 48, align: 'right', render: (_, member) => actionsFor(member) }
  ]

  const handleInvite = async () => {
    if (!inviteEmail.trim()) {
      message.error(t`Please enter an email address`)
      return
    }

    setInviting(true)
    try {
      // Call the API to invite the user with permissions
      await workspaceService.inviteMember({
        workspace_id: workspaceId,
        email: inviteEmail,
        permissions: invitePermissions
      })

      message.success(t`Invitation sent to ${inviteEmail}`)
      setInviteDrawerOpen(false)
      setInviteEmail('')

      // Refresh the members list
      onMembersChange()
    } catch (error) {
      if (error instanceof ApiError && error.status === 403 && error.message.includes('team member limit')) {
        message.error(t`Team member limit reached. Please upgrade your plan to add more members.`)
      } else {
        const msg = error instanceof Error ? error.message : t`Failed to invite member`
        message.error(msg)
      }
    } finally {
      setInviting(false)
    }
  }

  const handleCreateApiKey = async () => {
    if (!apiKeyName.trim()) {
      message.error(t`Please enter an API key name`)
      return
    }

    // Convert to snake_case
    const snakeCaseName = apiKeyName
      .trim()
      .toLowerCase()
      .replace(/\s+/g, '_')
      .replace(/[^a-z0-9_]/g, '')

    setCreatingApiKey(true)
    try {
      const response = await workspaceService.createAPIKey({
        workspace_id: workspaceId,
        email_prefix: snakeCaseName,
        permissions: apiKeyPermissions
      })

      setApiKeyToken(response.token)
      message.success(t`API key created successfully`)

      // Refresh the members list
      onMembersChange()
    } catch (error: unknown) {
      console.error('Failed to create API key', error)
      message.error((error as Error).message || t`Failed to create API key`)
    } finally {
      setCreatingApiKey(false)
    }
  }

  const resetApiKeyDrawer = () => {
    setApiKeyDrawerOpen(false)
    setApiKeyName('')
    setApiKeyToken('')
    setApiKeyPermissions(createFullPermissions())
  }

  // The server mints the address as `prefix@{apiHost}` — the workspace id is not part of it, so
  // prefixing one here advertised an address that does not exist and cannot receive anything.
  const domainName =
    window.API_ENDPOINT?.replace(/^https?:\/\//, '').split('/')[0] || 'api.example.com'

  const handleRemoveMember = async (userId: string) => {
    if (!userId) return

    setRemovingMember(true)
    try {
      await workspaceService.removeMember({
        workspace_id: workspaceId,
        user_id: userId
      })

      message.success(t`Member removed successfully`)
      onMembersChange()
    } catch (error) {
      console.error('Failed to remove member', error)
      message.error(t`Failed to remove member`)
    } finally {
      setRemovingMember(false)
    }
  }

  const handleDeleteInvitation = async (invitationId: string) => {
    if (!invitationId) return

    setRemovingMember(true)
    try {
      await workspaceService.deleteInvitation({
        invitation_id: invitationId
      })

      message.success(t`Invitation deleted successfully`)
      onMembersChange()
    } catch (error) {
      console.error('Failed to delete invitation', error)
      message.error(t`Failed to delete invitation`)
    } finally {
      setRemovingMember(false)
    }
  }

  const handleResendInvitation = async (invitation: WorkspaceMember) => {
    const email = invitation.email
    if (!email) return

    setResendingInvitation(true)
    try {
      // Reuse the inviteMember API which will update the existing invitation due to UPSERT logic.
      // Resend the grant the invitation already carries — resending must not widen it. Anything
      // the stored map leaves out stays denied, hence the empty base.
      const permissions: UserPermissions = {
        ...createEmptyPermissions(),
        ...invitation.permissions
      }

      await workspaceService.inviteMember({
        workspace_id: workspaceId,
        email: email,
        permissions
      })

      message.success(t`Invitation resent to ${email}`)
      onMembersChange()
    } catch (error) {
      console.error('Failed to resend invitation', error)
      // The server's sentence, not a fixed one: a resend replays the invitation's stored
      // permissions through the licence gate, and "this needs a licence" is something to
      // buy, where "Failed to resend invitation" is a bug report.
      const msg = error instanceof Error && error.message ? error.message : t`Failed to resend invitation`
      message.error(msg)
    } finally {
      setResendingInvitation(false)
    }
  }

  const handleEditPermissions = (member: WorkspaceMember) => {
    setEditingMember(member)
    setPermissionsDrawerOpen(true)
  }

  const handlePermissionsDrawerClose = () => {
    setPermissionsDrawerOpen(false)
    setEditingMember(null)
  }

  const handlePermissionsSuccess = () => {
    onMembersChange()
  }

  return (
    <>
      <SettingsSectionHeader title={t`Team & API keys`} description={t`Who can do what in this workspace`} />

      <Panel
        flush
        title={t`Members`}
        extra={
          isOwner ? (
            <Button
              type="primary"
              size="small"
              icon={<Plus size={14} aria-hidden="true" />}
              onClick={() => setInviteDrawerOpen(true)}
            >
              {t`Invite`}
            </Button>
          ) : null
        }
      >
        {error ? (
          <Result
            status="error"
            title={t`Couldn't load the members`}
            extra={<Button onClick={onMembersChange}>{t`Retry`}</Button>}
          />
        ) : loading ? (
          <div className="px-3.5 py-3">
            <Skeleton active title={false} paragraph={{ rows: 4 }} />
          </div>
        ) : (
          <Table
            size="small"
            dataSource={memberRows}
            columns={memberColumns}
            // Invitations have no user id: keyed by user id alone, they would all share "".
            rowKey={(member) => member.invitation_id ?? member.user_id}
            pagination={false}
            locale={{ emptyText: t`No members found` }}
          />
        )}
      </Panel>

      <Panel
        flush
        title={t`API keys`}
        extra={
          isOwner ? (
            <Button size="small" icon={<KeyRound size={14} aria-hidden="true" />} onClick={() => setApiKeyDrawerOpen(true)}>
              {t`New key`}
            </Button>
          ) : null
        }
      >
        {error ? null : loading ? (
          <div className="px-3.5 py-3">
            <Skeleton active title={false} paragraph={{ rows: 2 }} />
          </div>
        ) : apiKeyRows.length === 0 ? (
          <div className="px-3.5 py-4 text-sm text-ink-3">
            {t`No API keys yet. Create one to call the API or connect Zapier.`}
          </div>
        ) : (
          <Table
            size="small"
            dataSource={apiKeyRows}
            columns={apiKeyColumns}
            rowKey="user_id"
            pagination={false}
          />
        )}
      </Panel>

      {/* Drawers rather than modals: the permissions matrix carries fourteen expandable rows, and
          an expanded one lists every endpoint the permission gates — Contacts alone gates 23. */}
      <Drawer
        title={t`Invite Member`}
        open={inviteDrawerOpen}
        onClose={() => setInviteDrawerOpen(false)}
        placement="right"
        size={720}
        styles={{ wrapper: { maxWidth: '100%' } }}
        footer={
          <div className="flex justify-end">
            <Space>
              <Button onClick={() => setInviteDrawerOpen(false)}>{t`Cancel`}</Button>
              <Button
                type="primary"
                onClick={handleInvite}
                loading={inviting}
                disabled={rbacLocked && !grantsFullPermissions(invitePermissions)}
              >
                {t`Send Invitation`}
              </Button>
            </Space>
          </div>
        }
      >
        <Form layout="vertical">
          <Form.Item
            label={t`Email Address`}
            required
            rules={[{ required: true, message: t`Please enter an email address` }]}
          >
            <Input
              placeholder={t`Enter email address`}
              value={inviteEmail}
              onChange={(e) => setInviteEmail(e.target.value)}
            />
          </Form.Item>

          <Form.Item label={t`Permissions`}>
            {/* An invite starts from a full grant, which is never gated, so it can be sent as it
                is without the licence; only narrowing it needs one. */}
            <LicenceGateNotice feature="rbac" workspaceId={workspaceId} />
            <PermissionScopePresets
              value={invitePermissions}
              onChange={setInvitePermissions}
              fullAccessOnly={rbacLocked}
            />
            <PermissionsMatrix
              value={invitePermissions}
              onChange={setInvitePermissions}
              fullAccessOnly={rbacLocked}
            />
          </Form.Item>
        </Form>
      </Drawer>

      <Drawer
        title={t`Create API Key`}
        open={apiKeyDrawerOpen}
        onClose={resetApiKeyDrawer}
        placement="right"
        size={720}
        styles={{ wrapper: { maxWidth: '100%' } }}
        // Two states share the drawer: the creation form, then the one-time token. Once the
        // token is on screen there is nothing left to submit or to cancel, so the footer
        // collapses to a single Close — which is also what discards the token.
        footer={
          <div className="flex justify-end">
            {apiKeyToken ? (
              <Button type="primary" onClick={resetApiKeyDrawer}>
                {t`Close`}
              </Button>
            ) : (
              <Space>
                <Button onClick={resetApiKeyDrawer}>{t`Cancel`}</Button>
                <Button
                  type="primary"
                  onClick={handleCreateApiKey}
                  loading={creatingApiKey}
                  disabled={rbacLocked && !grantsFullPermissions(apiKeyPermissions)}
                >
                  {t`Create API Key`}
                </Button>
              </Space>
            )}
          </div>
        }
      >
        {!apiKeyToken ? (
          <Form layout="vertical">
            <Form.Item
              label={t`API Key Name`}
              required
              rules={[{ required: true, message: t`Please enter an API key name` }]}
            >
              <Space.Compact style={{ width: '100%' }}>
                <Input
                  value={apiKeyName}
                  onChange={(e) => {
                    // Convert to snake_case on change
                    const snakeCaseName = e.target.value
                      .toLowerCase()
                      .replace(/\s+/g, '_')
                      .replace(/[^a-z0-9_]/g, '')
                    setApiKeyName(snakeCaseName)
                  }}
                  maxLength={64}
                  style={{ flex: 1 }}
                />
                <Button disabled style={{ pointerEvents: 'none', color: 'rgba(0, 0, 0, 0.88)' }}>
                  {'@' + domainName}
                </Button>
              </Space.Compact>
            </Form.Item>

            <Form.Item label={t`Permissions`}>
              {/* Same rule as the invitation: a key starts from full access, which is never
                  gated; scoping it down needs the licence. */}
              <LicenceGateNotice feature="rbac" workspaceId={workspaceId} />
              <PermissionScopePresets
                value={apiKeyPermissions}
                onChange={setApiKeyPermissions}
                fullAccessOnly={rbacLocked}
              />
              <PermissionsMatrix
                value={apiKeyPermissions}
                onChange={setApiKeyPermissions}
                fullAccessOnly={rbacLocked}
              />
            </Form.Item>
          </Form>
        ) : (
          <>
            <Alert
              title={t`API Key Created Successfully`}
              description={t`This token will only be displayed once. Please save it in a secure location. It cannot be retrieved again.`}
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
            />
            <Form layout="vertical">
              <Form.Item label={t`API Token`}>
                <Input.TextArea
                  value={apiKeyToken}
                  autoSize={{ minRows: 3, maxRows: 5 }}
                  readOnly
                />
              </Form.Item>
            </Form>
          </>
        )}
      </Drawer>

      <EditPermissionsDrawer
        open={permissionsDrawerOpen}
        member={editingMember}
        workspaceId={workspaceId}
        onClose={handlePermissionsDrawerClose}
        onSuccess={handlePermissionsSuccess}
      />
    </>
  )
}
