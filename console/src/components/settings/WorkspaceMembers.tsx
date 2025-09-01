import { useState } from 'react'
import {
  Table,
  Typography,
  Spin,
  Button,
  Modal,
  Form,
  Input,
  App,
  Tag,
  Alert,
  Space,
  Popconfirm,
  Tooltip
} from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faTrashCan } from '@fortawesome/free-regular-svg-icons'
import { faRefresh } from '@fortawesome/free-solid-svg-icons'
import { WorkspaceMember } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { Section } from './Section'

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
  const [inviteModalVisible, setInviteModalVisible] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviting, setInviting] = useState(false)
  const { message } = App.useApp()

  // API Key Modal states
  const [apiKeyModalVisible, setApiKeyModalVisible] = useState(false)
  const [apiKeyName, setApiKeyName] = useState('')
  const [creatingApiKey, setCreatingApiKey] = useState(false)
  const [apiKeyToken, setApiKeyToken] = useState('')
  const [removingMember, setRemovingMember] = useState(false)
  const [resendingInvitation, setResendingInvitation] = useState(false)

  const columns = [
    {
      title: 'Email',
      dataIndex: 'email',
      key: 'email',
      render: (email: string) => {
        return (
          <Space>
            <Text ellipsis>{email}</Text>
          </Space>
        )
      }
    },
    {
      title: 'Role',
      dataIndex: 'role',
      key: 'role',
      render: (role: string, record: WorkspaceMember) => {
        if (record.type === 'api_key') {
          return (
            <Tag bordered={false} color="purple">
              API Key
            </Tag>
          )
        }
        const roleDisplay = record.invitation_expires_at
          ? `Invitation sent`
          : role.charAt(0).toUpperCase() + role.slice(1)

        return (
          <Tag bordered={false} color={role === 'owner' ? 'gold' : 'blue'}>
            {roleDisplay}
          </Tag>
        )
      }
    },
    {
      title: 'Since',
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
            render: (_: any, record: WorkspaceMember) => {
              // Don't show remove button for the owner or for the current user
              if (record.role === 'owner') {
                return null
              }

              const isInvitation = record.invitation_expires_at

              return (
                <Space size="small">
                  {!isInvitation && (
                    <Popconfirm
                      title="Remove member"
                      description={`Are you sure you want to remove ${record.email}?${record.type === 'api_key' ? ' This API key will be permanently deleted.' : ''}`}
                      onConfirm={() => handleRemoveMember(record.user_id)}
                      okText="Yes"
                      cancelText="No"
                      okButtonProps={{ danger: true, loading: removingMember }}
                    >
                      <Tooltip title="Remove member" placement="left">
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
                        title="Delete invitation"
                        description={`Are you sure you want to delete the invitation for ${record.email}?`}
                        onConfirm={() => handleDeleteInvitation(record.invitation_id!)}
                        okText="Yes"
                        cancelText="No"
                        okButtonProps={{ danger: true, loading: removingMember }}
                      >
                        <Tooltip title="Delete invitation" placement="left">
                          <Button
                            icon={<FontAwesomeIcon icon={faTrashCan} />}
                            size="small"
                            type="text"
                            loading={removingMember}
                          />
                        </Tooltip>
                        <Tooltip title="Resend invitation" placement="left">
                          <Button
                            icon={<FontAwesomeIcon icon={faRefresh} />}
                            size="small"
                            type="text"
                            onClick={() => handleResendInvitation(record.email)}
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
      message.error('Please enter an email address')
      return
    }

    setInviting(true)
    try {
      // Call the API to invite the user - always with role "member"
      await workspaceService.inviteMember({
        workspace_id: workspaceId,
        email: inviteEmail,
        role: 'member' // Always set to member
      })

      message.success(`Invitation sent to ${inviteEmail}`)
      setInviteModalVisible(false)
      setInviteEmail('')

      // Refresh the members list
      onMembersChange()
    } catch (error) {
      console.error('Failed to invite member', error)
      message.error('Failed to invite member')
    } finally {
      setInviting(false)
    }
  }

  const handleCreateApiKey = async () => {
    if (!apiKeyName.trim()) {
      message.error('Please enter an API key name')
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
        email_prefix: snakeCaseName
      })

      setApiKeyToken(response.token)
      message.success('API key created successfully')

      // Refresh the members list
      onMembersChange()
    } catch (error: any) {
      console.error('Failed to create API key', error)
      message.error(error.message || 'Failed to create API key')
    } finally {
      setCreatingApiKey(false)
    }
  }

  const resetApiKeyModal = () => {
    setApiKeyModalVisible(false)
    setApiKeyName('')
    setApiKeyToken('')
  }

  const domainName = `${workspaceId}.${
    window.API_ENDPOINT?.replace(/^https?:\/\//, '').split('/')[0] || 'api.example.com'
  }`

  const handleRemoveMember = async (userId: string) => {
    if (!userId) return

    setRemovingMember(true)
    try {
      await workspaceService.removeMember({
        workspace_id: workspaceId,
        user_id: userId
      })

      message.success('Member removed successfully')
      onMembersChange()
    } catch (error) {
      console.error('Failed to remove member', error)
      message.error('Failed to remove member')
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

      message.success('Invitation deleted successfully')
      onMembersChange()
    } catch (error) {
      console.error('Failed to delete invitation', error)
      message.error('Failed to delete invitation')
    } finally {
      setRemovingMember(false)
    }
  }

  const handleResendInvitation = async (email: string) => {
    if (!email) return

    setResendingInvitation(true)
    try {
      // Reuse the inviteMember API which will update the existing invitation due to UPSERT logic
      await workspaceService.inviteMember({
        workspace_id: workspaceId,
        email: email,
        role: 'member' // Always use member role for resending
      })

      message.success(`Invitation resent to ${email}`)
      onMembersChange()
    } catch (error) {
      console.error('Failed to resend invitation', error)
      message.error('Failed to resend invitation')
    } finally {
      setResendingInvitation(false)
    }
  }

  return (
    <>
      <Section title="Members" description="Manage your workspace members">
        {isOwner && (
          <div className="flex justify-end mb-4">
            <Space size="middle">
              <Button type="primary" size="small" ghost onClick={() => setApiKeyModalVisible(true)}>
                Create API Key
              </Button>
              <Button type="primary" size="small" ghost onClick={() => setInviteModalVisible(true)}>
                Invite Member
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
            locale={{ emptyText: 'No members found' }}
            className="border border-gray-200 rounded-md"
          />
        )}
      </Section>

      <Modal
        title="Invite Member"
        open={inviteModalVisible}
        onCancel={() => setInviteModalVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setInviteModalVisible(false)}>
            Cancel
          </Button>,
          <Button key="invite" type="primary" onClick={handleInvite} loading={inviting}>
            Send Invitation
          </Button>
        ]}
      >
        <Form layout="vertical">
          <Form.Item
            label="Email Address"
            required
            rules={[{ required: true, message: 'Please enter an email address' }]}
          >
            <Input
              placeholder="Enter email address"
              value={inviteEmail}
              onChange={(e) => setInviteEmail(e.target.value)}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="Create API Key"
        open={apiKeyModalVisible}
        onCancel={resetApiKeyModal}
        footer={
          apiKeyToken
            ? [
                <Button key="close" type="primary" onClick={resetApiKeyModal}>
                  Close
                </Button>
              ]
            : [
                <Button key="cancel" onClick={resetApiKeyModal}>
                  Cancel
                </Button>,
                <Button
                  key="create"
                  type="primary"
                  onClick={handleCreateApiKey}
                  loading={creatingApiKey}
                >
                  Create API Key
                </Button>
              ]
        }
      >
        {!apiKeyToken ? (
          <Form layout="vertical">
            <Form.Item
              label="API Key Name"
              required
              rules={[{ required: true, message: 'Please enter an API key name' }]}
            >
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
                addonAfter={'@' + domainName}
              />
            </Form.Item>
          </Form>
        ) : (
          <>
            <Alert
              message="API Key Created Successfully"
              description="This token will only be displayed once. Please save it in a secure location. It cannot be retrieved again."
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
            />
            <Form layout="vertical">
              <Form.Item label="API Token">
                <Input.TextArea
                  value={apiKeyToken}
                  autoSize={{ minRows: 3, maxRows: 5 }}
                  readOnly
                />
              </Form.Item>
            </Form>
          </>
        )}
      </Modal>
    </>
  )
}
