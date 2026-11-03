import { useState } from 'react'
import {
  Table,
  Typography,
  Spin,
  Button,
  Drawer,
  Form,
  Input,
  App,
  Tag,
  Alert,
  Space,
  Popconfirm,
  Tooltip,
  Popover
} from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faTrashCan } from '@fortawesome/free-regular-svg-icons'
import { faRefresh, faUserCog } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import { WorkspaceMember, UserPermissions, StoredPermissions } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import {
  ENFORCED_PERMISSION_VERBS,
  OPT_IN_PERMISSION_RESOURCES,
  countEnforcedGrants,
  createEmptyPermissions,
  grantUnenforcedPermissions,
  createFullPermissions,
  grantsFullPermissions
} from '../../services/api/permissions'
import { ApiError } from '../../services/api/client'
import { EditPermissionsDrawer } from './EditPermissionsDrawer'
import { PermissionsMatrix } from './PermissionsMatrix'
import { PermissionScopePresets } from './PermissionScopePresets'
import { useLicense } from '../../hooks/useLicense'
import { LicenceGateNotice } from '../license/LicenceGateNotice'
import { SettingsSectionHeader } from './SettingsSectionHeader'

const { Text } = Typography

interface WorkspaceMembersProps {
  workspaceId: string
  members: WorkspaceMember[]
  loading: boolean
  onMembersChange: () => void
  isOwner: boolean
}

export function WorkspaceMembers({
  workspaceId,
  members,
  loading,
  onMembersChange,
  isOwner
}: WorkspaceMembersProps) {
  const { t } = useLingui()
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
  const [removingMember, setRemovingMember] = useState(false)
  const [resendingInvitation, setResendingInvitation] = useState(false)

  // Permissions drawer states
  const [permissionsDrawerOpen, setPermissionsDrawerOpen] = useState(false)
  const [editingMember, setEditingMember] = useState<WorkspaceMember | null>(null)

  const columns = [
    {
      title: t`Email`,
      dataIndex: 'email',
      key: 'email',
      render: (email: string) => {
        return <Text className="break-all">{email}</Text>
      }
    },
    {
      title: t`Role`,
      dataIndex: 'role',
      key: 'role',
      render: (role: string, record: WorkspaceMember) => {
        if (record.type === 'api_key') {
          return (
            <Tag variant="filled" color="purple">
              {t`API Key`}
            </Tag>
          )
        }
        const roleDisplay = record.invitation_expires_at
          ? t`Invitation sent`
          : role.charAt(0).toUpperCase() + role.slice(1)

        return (
          <Tag variant="filled" color={role === 'owner' ? 'gold' : 'blue'}>
            {roleDisplay}
          </Tag>
        )
      }
    },
    {
      title: t`Permissions`,
      key: 'permissions',
      render: (record: WorkspaceMember) => {
        if (record.invitation_expires_at) {
          return <Tag color="orange">{t`Pending`}</Tag>
        }

        // An API key is a member row, so its grant is counted the same way. The row's own map
        // may be partial or null — a resource it does not mention is denied.
        const permissions: StoredPermissions = record.permissions ?? createEmptyPermissions()

        // Full Access is the server's definition, read the way the editor reads a row: the
        // unenforced verbs count as granted, as the editor shows and saves them, so a row stored
        // with one of them off is still full access. It is checked first and never inferred from
        // the count. The count is over the verbs that gate something, canonical and opt-in alike:
        // the console stores the unenforced verbs as granted on every save, and counting them put
        // a full row above the total, made a row one verb short read as full, and kept a revoked
        // row from ever reading No Access.
        const totalPermissions = ENFORCED_PERMISSION_VERBS.length
        const activePermissions = countEnforcedGrants(permissions)
        const details = <PermissionsMatrix value={permissions} />

        if (
          grantsFullPermissions(
            grantUnenforcedPermissions({ ...createEmptyPermissions(), ...permissions })
          )
        ) {
          const tag = <Tag color="green">{t`Full Access`}</Tag>
          // An opt-in grant on top of full access (Audit logs read) is not implied by it, so it
          // stays one hover away rather than hidden behind the same tag as a plain full grant.
          const holdsOptIn = ENFORCED_PERMISSION_VERBS.some(
            ([resource, verb]) =>
              OPT_IN_PERMISSION_RESOURCES.includes(resource) && permissions[resource]?.[verb]
          )
          if (!holdsOptIn) return tag
          return (
            <Popover content={details} title={t`Permission Details`} trigger="hover">
              <span className="cursor-pointer">{tag}</span>
            </Popover>
          )
        }

        if (activePermissions === 0) {
          return (
            <Popover content={details} title={t`Permission Details`} trigger="hover">
              <Tag color="red" className="cursor-pointer">
                {t`No Access`}
              </Tag>
            </Popover>
          )
        }
        return (
          <Popover content={details} title={t`Permission Details`} trigger="hover">
            <Tag color="blue" className="cursor-pointer">
              {activePermissions}/{totalPermissions}
            </Tag>
          </Popover>
        )
      }
    },
    {
      title: t`Since`,
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => new Date(date).toLocaleDateString()
    },
    // Only add the action column if the user is an owner
    ...(isOwner
      ? [
          {
            title: '',
            key: 'action',
            width: 100,
            render: (_: unknown, record: WorkspaceMember) => {
              // Don't show remove button for the owner or for the current user
              if (record.role === 'owner') {
                return null
              }

              const isInvitation = record.invitation_expires_at

              return (
                <Space size="small">
                  {!isInvitation && record.role !== 'owner' && (
                    <Tooltip title={t`Edit permissions`} placement="left">
                      <Button
                        icon={<FontAwesomeIcon icon={faUserCog} />}
                        size="small"
                        type="text"
                        onClick={() => handleEditPermissions(record)}
                      />
                    </Tooltip>
                  )}
                  {!isInvitation && (
                    <Popconfirm
                      title={t`Remove member`}
                      description={t`Are you sure you want to remove ${record.email}?${record.type === 'api_key' ? ' This API key will be permanently deleted.' : ''}`}
                      onConfirm={() => handleRemoveMember(record.user_id)}
                      okText={t`Yes`}
                      cancelText={t`No`}
                      okButtonProps={{ danger: true, loading: removingMember }}
                    >
                      <Tooltip title={t`Remove member`} placement="left">
                        <Button
                          icon={<FontAwesomeIcon icon={faTrashCan} />}
                          size="small"
                          type="text"
                          loading={removingMember}
                        />
                      </Tooltip>
                    </Popconfirm>
                  )}
                  {isInvitation && (
                    <>
                      <Popconfirm
                        title={t`Delete invitation`}
                        description={t`Are you sure you want to delete the invitation for ${record.email}?`}
                        onConfirm={() => handleDeleteInvitation(record.invitation_id!)}
                        okText={t`Yes`}
                        cancelText={t`No`}
                        okButtonProps={{ danger: true, loading: removingMember }}
                      >
                        <Tooltip title={t`Delete invitation`} placement="left">
                          <Button
                            icon={<FontAwesomeIcon icon={faTrashCan} />}
                            size="small"
                            type="text"
                            loading={removingMember}
                          />
                        </Tooltip>
                        <Tooltip title={t`Resend invitation`} placement="left">
                          <Button
                            icon={<FontAwesomeIcon icon={faRefresh} />}
                            size="small"
                            type="text"
                            onClick={() => handleResendInvitation(record)}
                            loading={resendingInvitation}
                          />
                        </Tooltip>
                      </Popconfirm>
                    </>
                  )}
                </Space>
              )
            }
          }
        ]
      : [])
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
      <SettingsSectionHeader title={t`Team`} description={t`Manage your workspace members`} />

      {isOwner && (
        <div className="flex justify-end mb-4">
          <Space size="middle">
            <Button type="primary" size="small" ghost onClick={() => setApiKeyDrawerOpen(true)}>
              {t`Create API Key`}
            </Button>
            <Button type="primary" size="small" ghost onClick={() => setInviteDrawerOpen(true)}>
              {t`Invite Member`}
            </Button>
          </Space>
        </div>
      )}

      {loading ? (
        <div style={{ textAlign: 'center', padding: '20px' }}>
          <Spin />
        </div>
      ) : (
        <Table
          dataSource={members}
          columns={columns}
          rowKey="user_id"
          pagination={false}
          locale={{ emptyText: t`No members found` }}
          className="border border-gray-200 rounded-md"
        />
      )}

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
