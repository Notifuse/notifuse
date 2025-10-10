import { Layout, Menu, Select, Space, Button, Dropdown, message, Alert, Typography } from 'antd'
import { Outlet, Link, useParams, useMatches, useNavigate } from '@tanstack/react-router'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faImage,
  faFolderOpen,
  faObjectGroup,
  faPaperPlane
} from '@fortawesome/free-regular-svg-icons'
import {
  faGear,
  faPlus,
  faPowerOff,
  faRightFromBracket,
  faTerminal,
  faUserGroup,
  faBarsStaggered,
  faChartLine
} from '@fortawesome/free-solid-svg-icons'
import { useAuth } from '../contexts/AuthContext'
import { Workspace, UserPermissions } from '../services/api/types'
import { ContactsCsvUploadProvider } from '../components/contacts/ContactsCsvUploadProvider'
import { useState, useEffect } from 'react'
import { FileManagerProvider } from '../components/file_manager/context'
import { FileManagerSettings } from '../components/file_manager/interfaces'
import { workspaceService } from '../services/api/workspace'
import { isRootUser } from '../services/api/auth'
import { useQuery } from '@tanstack/react-query'

const { Content, Sider } = Layout
const { Paragraph } = Typography

interface CronStatusResponse {
  success: boolean
  last_run: string | null
  last_run_unix: number | null
  time_since_last_run: string | null
  time_since_last_run_seconds: number | null
  message?: string
}

function CronStatusBanner() {
  const [apiEndpoint, setApiEndpoint] = useState<string>('')

  useEffect(() => {
    // Get API endpoint from window object
    setApiEndpoint((window as any).API_ENDPOINT || '')
  }, [])

  const { data: cronStatus, isError } = useQuery<CronStatusResponse>({
    queryKey: ['cronStatus'],
    queryFn: async () => {
      const response = await fetch(`${apiEndpoint}/api/cron.status`)
      if (!response.ok) {
        throw new Error('Failed to fetch cron status')
      }
      return response.json()
    },
    refetchInterval: 3600000, // Refetch every hour
    enabled: !!apiEndpoint // Only run query if we have an API endpoint
  })

  // Don't show banner if we can't fetch status or if there's an error
  if (!cronStatus || isError) {
    return null
  }

  // Check if last run was more than 90 seconds ago
  const needsCronSetup =
    !cronStatus.last_run ||
    (cronStatus.time_since_last_run_seconds && cronStatus.time_since_last_run_seconds > 90)

  if (!needsCronSetup) {
    return null
  }

  const cronCommand = `* * * * * curl ${apiEndpoint}/api/cron > /dev/null 2>&1`

  const handleCopyCronCommand = () => {
    navigator.clipboard.writeText(cronCommand)
    message.success('Cron command copied to clipboard!')
  }

  return (
    <div
      style={{
        position: 'fixed',
        bottom: 16,
        right: 16,
        width: '800px',
        zIndex: 1000
      }}
    >
      <Alert
        message="Cron Job Setup Required"
        description={
          <Space direction="vertical" style={{ width: '100%' }}>
            <div>
              {cronStatus.last_run
                ? `Last cron run was ${Math.floor((cronStatus.time_since_last_run_seconds || 0) / 60)} minutes ago. `
                : 'No cron run detected. '}
              Add this cron job to your server to enable automatic task processing:{' '}
              <a
                href="https://docs.notifuse.com/installation#a-cron-scheduler"
                target="_blank"
                rel="noopener noreferrer"
                style={{ color: '#7763F1' }}
              >
                Learn more
              </a>
            </div>
            <Paragraph
              copyable={{
                text: cronCommand,
                onCopy: handleCopyCronCommand
              }}
              style={{
                backgroundColor: '#f5f5f5',
                padding: '8px',
                border: '1px solid #d9d9d9',
                borderRadius: '4px',
                fontFamily: 'monospace',
                fontSize: '12px',
                marginBottom: 0,
                whiteSpace: 'pre-wrap'
              }}
            >
              {cronCommand}
            </Paragraph>
          </Space>
        }
        type="warning"
        showIcon
        closable
      />
    </div>
  )
}

export function WorkspaceLayout() {
  const { workspaceId } = useParams({ from: '/workspace/$workspaceId' })
  const { signout, workspaces, user, refreshWorkspaces } = useAuth()
  const navigate = useNavigate()
  const [collapsed, setCollapsed] = useState(false)
  const [userPermissions, setUserPermissions] = useState<UserPermissions | null>(null)
  const [loadingPermissions, setLoadingPermissions] = useState(true)

  // Use useMatches to determine the current route path
  const matches = useMatches()
  const currentPath = matches[matches.length - 1]?.pathname || ''

  // Fetch user permissions for the current workspace
  useEffect(() => {
    const fetchUserPermissions = async () => {
      if (!user || !workspaceId) {
        setLoadingPermissions(false)
        return
      }

      // If user is root, they have full permissions
      if (isRootUser(user.email)) {
        setUserPermissions({
          contacts: { read: true, write: true },
          lists: { read: true, write: true },
          templates: { read: true, write: true },
          broadcasts: { read: true, write: true },
          transactional: { read: true, write: true },
          workspace: { read: true, write: true },
          message_history: { read: true, write: true }
        })
        setLoadingPermissions(false)
        return
      }

      try {
        const response = await workspaceService.getMembers(workspaceId)
        const currentUserMember = response.members.find((member) => member.user_id === user.id)

        if (currentUserMember) {
          setUserPermissions(currentUserMember.permissions)
        } else {
          // User is not a member of this workspace, set empty permissions
          setUserPermissions({
            contacts: { read: false, write: false },
            lists: { read: false, write: false },
            templates: { read: false, write: false },
            broadcasts: { read: false, write: false },
            transactional: { read: false, write: false },
            workspace: { read: false, write: false },
            message_history: { read: false, write: false }
          })
        }
      } catch (error) {
        console.error('Failed to fetch user permissions', error)
        // On error, assume no permissions
        setUserPermissions({
          contacts: { read: false, write: false },
          lists: { read: false, write: false },
          templates: { read: false, write: false },
          broadcasts: { read: false, write: false },
          transactional: { read: false, write: false },
          workspace: { read: false, write: false },
          message_history: { read: false, write: false }
        })
      } finally {
        setLoadingPermissions(false)
      }
    }

    fetchUserPermissions()
  }, [workspaceId, user])

  // Helper function to check if user has access to a resource
  const hasAccess = (resource: keyof UserPermissions): boolean => {
    if (!userPermissions) return false
    // User needs at least read or write permission to access the resource
    const permissions = userPermissions[resource]
    return permissions.read || permissions.write
  }

  // Determine which key should be selected based on the current path
  let selectedKey = 'analytics' // Default to analytics/dashboard
  if (currentPath.includes('/settings')) {
    selectedKey = 'settings'
  } else if (currentPath.includes('/lists')) {
    selectedKey = 'lists'
  } else if (currentPath.includes('/templates')) {
    selectedKey = 'templates'
  } else if (currentPath.includes('/contacts')) {
    selectedKey = 'contacts'
  } else if (currentPath.includes('/file-manager')) {
    selectedKey = 'file-manager'
  } else if (currentPath.includes('/transactional-notifications')) {
    selectedKey = 'transactional-notifications'
  } else if (currentPath.includes('/logs')) {
    selectedKey = 'logs'
  } else if (currentPath.includes('/broadcasts')) {
    selectedKey = 'broadcasts'
  }

  const handleWorkspaceChange = (workspaceId: string) => {
    if (workspaceId === 'new-workspace') {
      // Navigate to workspace creation page or open a modal
      navigate({ to: '/workspace/create' })
      return
    }

    navigate({
      to: '/workspace/$workspaceId',
      params: { workspaceId }
    })
  }

  // Function to handle workspace settings update
  const handleUpdateWorkspaceSettings = async (settings: FileManagerSettings): Promise<void> => {
    const workspace = workspaces.find((w) => w.id === workspaceId)
    if (!workspace) {
      message.error('Workspace not found')
      return
    }

    try {
      // Update workspace using workspace service
      await workspaceService.update({
        id: workspace.id,
        name: workspace.name,
        settings: {
          ...workspace.settings,
          file_manager: settings
        }
      })

      // Refresh workspaces from context
      await refreshWorkspaces()

      message.success('Workspace settings updated successfully')
    } catch (error: any) {
      console.error('Error updating workspace settings:', error)
      message.error(`Failed to update workspace settings: ${error.message}`)
    }
  }

  const menuItems = [
    hasAccess('message_history') && {
      key: 'analytics',
      icon: <FontAwesomeIcon icon={faChartLine} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId" params={{ workspaceId }}>
          Dashboard
        </Link>
      )
    },
    hasAccess('contacts') && {
      key: 'contacts',
      icon: <FontAwesomeIcon icon={faUserGroup} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/contacts" params={{ workspaceId }}>
          Contacts
        </Link>
      )
    },
    hasAccess('lists') && {
      key: 'lists',
      icon: <FontAwesomeIcon icon={faFolderOpen} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/lists" params={{ workspaceId }}>
          Lists
        </Link>
      )
    },
    hasAccess('templates') && {
      key: 'templates',
      icon: <FontAwesomeIcon icon={faObjectGroup} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/templates" params={{ workspaceId }}>
          Templates
        </Link>
      )
    },
    hasAccess('broadcasts') && {
      key: 'broadcasts',
      icon: <FontAwesomeIcon icon={faPaperPlane} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/broadcasts" params={{ workspaceId }}>
          Broadcasts
        </Link>
      )
    },
    hasAccess('transactional') && {
      key: 'transactional-notifications',
      icon: <FontAwesomeIcon icon={faTerminal} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/transactional-notifications" params={{ workspaceId }}>
          Transactional
        </Link>
      )
    },
    hasAccess('workspace') && {
      key: 'file-manager',
      icon: <FontAwesomeIcon icon={faImage} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/file-manager" params={{ workspaceId }}>
          File Manager
        </Link>
      )
    },
    hasAccess('message_history') && {
      key: 'logs',
      icon: <FontAwesomeIcon icon={faBarsStaggered} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/logs" params={{ workspaceId }}>
          Logs
        </Link>
      )
    },
    hasAccess('workspace') && {
      key: 'settings',
      icon: <FontAwesomeIcon icon={faGear} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/settings" params={{ workspaceId }}>
          Settings
        </Link>
      )
    }
  ].filter(Boolean) as any[]

  return (
    <ContactsCsvUploadProvider>
      <Layout style={{ minHeight: '100vh' }}>
        <CronStatusBanner />
        <Layout>
          <Sider
            width={250}
            theme="light"
            style={{
              position: 'fixed',
              height: '100vh',
              left: 0,
              top: 0,
              overflow: 'auto',
              zIndex: 10
            }}
            collapsible
            collapsed={collapsed}
            trigger={null}
            className="border-r border-gray-200"
          >
            <div className="flex items-center gap-2 p-6">
              {!collapsed && (
                <Select
                  value={workspaceId}
                  onChange={handleWorkspaceChange}
                  style={{ width: '100%' }}
                  placeholder="Select workspace"
                  options={[
                    ...workspaces.map((workspace: Workspace) => ({
                      label: (
                        <Space size="small">
                          {workspace.settings.logo_url && (
                            <img
                              src={workspace.settings.logo_url}
                              alt=""
                              style={{
                                height: '14px',
                                width: '14px',
                                objectFit: 'contain',
                                verticalAlign: 'middle',
                                display: 'inline-block'
                              }}
                            />
                          )}
                          {workspace.name}
                        </Space>
                      ),
                      value: workspace.id
                    })),
                    ...(isRootUser(user?.email)
                      ? [
                          {
                            label: (
                              <Space className="text-indigo-500">
                                <FontAwesomeIcon icon={faPlus} /> New workspace
                              </Space>
                            ),
                            value: 'new-workspace'
                          }
                        ]
                      : [])
                  ]}
                />
              )}
              <Button
                type="text"
                icon={
                  collapsed ? (
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      width="15"
                      height="15"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      className="text-gray-500"
                    >
                      <rect width="18" height="18" x="3" y="3" rx="2" />
                      <path d="M9 3v18" />
                      <path d="m14 9 3 3-3 3" />
                    </svg>
                  ) : (
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      width="15"
                      height="15"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      className="text-gray-500"
                    >
                      <rect width="18" height="18" x="3" y="3" rx="2" />
                      <path d="M9 3v18" />
                      <path d="m16 15-3-3 3-3" />
                    </svg>
                  )
                }
                onClick={() => setCollapsed(!collapsed)}
              />
            </div>
            <Menu
              mode="inline"
              selectedKeys={[selectedKey]}
              style={{ height: 'calc(100% - 120px)', borderRight: 0, backgroundColor: '#fdfdfd' }}
              items={loadingPermissions ? [] : menuItems}
              theme="light"
            />
            <div
              style={{
                position: 'fixed',
                bottom: 0,
                left: 0,
                width: collapsed ? '80px' : '224px',
                padding: '16px',
                borderTop: '1px solid #f0f0f0',
                zIndex: 1,
                transition: 'width 0.2s'
              }}
            >
              {!collapsed && (
                <>
                  <Dropdown
                    menu={{
                      items: [
                        {
                          key: 'logout',
                          label: (
                            <Space>
                              <FontAwesomeIcon
                                icon={faRightFromBracket}
                                size="sm"
                                style={{ opacity: 0.7 }}
                              />
                              Logout
                            </Space>
                          ),
                          onClick: () => signout()
                        }
                      ]
                    }}
                    trigger={['click']}
                    placement="bottomRight"
                  >
                    <Button type="text" block>
                      <div style={{ padding: '4px 8px', color: '#595959', cursor: 'pointer' }}>
                        {user?.email}
                      </div>
                    </Button>
                  </Dropdown>
                  <div
                    style={{
                      textAlign: 'center',
                      marginTop: '8px',
                      fontSize: '9px',
                      color: '#000',
                      opacity: 0.7
                    }}
                  >
                    v{window.VERSION || '1.0'}
                  </div>
                </>
              )}
              {collapsed && (
                <Button
                  type="text"
                  icon={<FontAwesomeIcon icon={faPowerOff} size="sm" style={{ opacity: 0.7 }} />}
                  onClick={() => signout()}
                  style={{ width: '100%' }}
                />
              )}
            </div>
          </Sider>
          <Layout
            style={{
              marginLeft: collapsed ? '80px' : '250px',
              padding: '24px',
              transition: 'margin-left 0.2s'
            }}
          >
            <Content>
              <FileManagerProvider
                key={`fm-${workspaceId}-${!userPermissions?.templates?.write}`}
                settings={workspaces.find((w) => w.id === workspaceId)?.settings.file_manager}
                onUpdateSettings={handleUpdateWorkspaceSettings}
                readOnly={!userPermissions?.templates?.write}
              >
                <Outlet />
              </FileManagerProvider>
            </Content>
          </Layout>
        </Layout>
      </Layout>
    </ContactsCsvUploadProvider>
  )
}
