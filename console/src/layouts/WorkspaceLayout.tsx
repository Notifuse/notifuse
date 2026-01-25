import { Layout, Menu, Select, Space, Button, Dropdown, message, Avatar } from 'antd'
import { Outlet, Link, useParams, useMatches, useNavigate } from '@tanstack/react-router'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { useLingui } from '@lingui/react/macro'
import md5 from 'blueimp-md5'
import {
  faImage,
  faPaperPlane,
  faFileLines,
  faQuestionCircle
} from '@fortawesome/free-regular-svg-icons'
import {
  faPlus,
  faPowerOff,
  faTerminal,
  faBarsStaggered,
  faAngleLeft,
  faAngleRight
} from '@fortawesome/free-solid-svg-icons'
import { useAuth } from '../contexts/AuthContext'
import { LanguageSwitcher } from '../components/LanguageSwitcher'
import { Workspace, UserPermissions } from '../services/api/types'
import { ContactsCsvUploadProvider } from '../components/contacts/ContactsCsvUploadProvider'
import { useState, useEffect } from 'react'
import { FileManagerProvider } from '../components/file_manager/context'
import { FileManagerSettings } from '../components/file_manager/interfaces'
import { workspaceService } from '../services/api/workspace'
import { isRootUser } from '../services/api/auth'
import {
  FolderOpenOutlined,
  LineChartOutlined,
  SettingOutlined,
  WarningOutlined,
  DownOutlined
} from '@ant-design/icons'

const { Content, Sider, Header } = Layout

// Helper function to generate Gravatar URL from email
const getGravatarUrl = (email: string | undefined, size: number = 32): string => {
  if (!email) return ''
  const hash = md5(email.trim().toLowerCase())
  return `https://www.gravatar.com/avatar/${hash}?s=${size}&d=identicon`
}

export function WorkspaceLayout() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { signout, workspaces, user, refreshWorkspaces } = useAuth()
  const navigate = useNavigate()
  const [collapsed, setCollapsed] = useState(false)
  const [userPermissions, setUserPermissions] = useState<UserPermissions | null>(null)
  const [loadingPermissions, setLoadingPermissions] = useState(true)

  // Use useMatches to determine the current route path
  const matches = useMatches()
  const currentPath = matches[matches.length - 1]?.pathname || ''
  const isSettingsPage = currentPath.includes('/settings') || currentPath.includes('/blog')

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
          message_history: { read: true, write: true },
          blog: { read: true, write: true },
          automations: { read: true, write: true }
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
            message_history: { read: false, write: false },
            blog: { read: false, write: false },
            automations: { read: false, write: false }
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
          message_history: { read: false, write: false },
          blog: { read: false, write: false },
          automations: { read: false, write: false }
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
    return permissions?.read || permissions?.write || false
  }

  // Determine which key should be selected based on the current path
  let selectedKey = 'analytics' // Default to analytics/dashboard
  if (currentPath.includes('/settings')) {
    selectedKey = 'settings'
  } else if (currentPath.includes('/lists')) {
    selectedKey = 'lists'
  } else if (currentPath.includes('/templates')) {
    selectedKey = 'templates'
  } else if (currentPath.includes('/blog')) {
    selectedKey = 'blog'
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
  } else if (currentPath.includes('/automations')) {
    selectedKey = 'automations'
  }

  const handleWorkspaceChange = (workspaceId: string) => {
    if (workspaceId === 'new-workspace') {
      // Navigate to workspace creation page or open a modal
      navigate({ to: '/console/workspace/create' })
      return
    }

    navigate({
      to: '/console/workspace/$workspaceId',
      params: { workspaceId }
    })
  }

  // Function to handle workspace settings update
  const handleUpdateWorkspaceSettings = async (settings: FileManagerSettings): Promise<void> => {
    const workspace = workspaces.find((w) => w.id === workspaceId)
    if (!workspace) {
      message.error(t`Workspace not found`)
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

      message.success(t`Workspace settings updated successfully`)
    } catch (error: unknown) {
      console.error('Error updating workspace settings:', error)
      const errorMessage = error instanceof Error ? error.message : t`Unknown error`
      message.error(t`Failed to update workspace settings: ${errorMessage}`)
    }
  }

  const menuItems = [
    hasAccess('message_history') && {
      key: 'analytics',
      // icon: <FontAwesomeIcon icon={faChartLine} size="sm" style={{ opacity: 0.7 }} />,
      icon: <LineChartOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId" params={{ workspaceId }}>
          {t`Dashboard`}
        </Link>
      )
    },
    hasAccess('contacts') && {
      key: 'contacts',
      // icon: <ContactsOutlined />,
      icon: (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="lucide lucide-square-user-round-icon lucide-square-user-round opacity-70"
        >
          <path d="M18 21a6 6 0 0 0-12 0" />
          <circle cx="12" cy="11" r="4" />
          <rect width="18" height="18" x="3" y="3" rx="2" />
        </svg>
      ),
      label: (
        <Link to="/console/workspace/$workspaceId/contacts" params={{ workspaceId }}>
          {t`Contacts`}
        </Link>
      )
    },
    hasAccess('lists') && {
      key: 'lists',
      // icon: <FontAwesomeIcon icon={faFolderOpen} size="sm" style={{ opacity: 0.7 }} />,
      icon: <FolderOpenOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId/lists" params={{ workspaceId }}>
          {t`Lists`}
        </Link>
      )
    },
    hasAccess('templates') && {
      key: 'templates',
      icon: (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="lucide lucide-layout-panel-top-icon lucide-layout-panel-top opacity-70"
        >
          <rect width="18" height="7" x="3" y="3" rx="1" />
          <rect width="7" height="7" x="3" y="14" rx="1" />
          <rect width="7" height="7" x="14" y="14" rx="1" />
        </svg>
      ),
      label: (
        <Link to="/console/workspace/$workspaceId/templates" params={{ workspaceId }}>
          {t`Templates`}
        </Link>
      )
    },
    hasAccess('broadcasts') && {
      key: 'broadcasts',
      icon: <FontAwesomeIcon icon={faPaperPlane} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/console/workspace/$workspaceId/broadcasts" params={{ workspaceId }}>
          {t`Broadcasts`}
        </Link>
      )
    },
    hasAccess('automations') && {
      key: 'automations',
      icon: (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="lucide lucide-workflow-icon lucide-workflow opacity-70"
        >
          <rect width="8" height="8" x="3" y="3" rx="2" />
          <path d="M7 11v4a2 2 0 0 0 2 2h4" />
          <rect width="8" height="8" x="13" y="13" rx="2" />
        </svg>
      ),
      label: (
        <Link to="/console/workspace/$workspaceId/automations" params={{ workspaceId }}>
          {t`Automations`}
        </Link>
      )
    },
    hasAccess('transactional') && {
      key: 'transactional-notifications',
      icon: <FontAwesomeIcon icon={faTerminal} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link
          to="/console/workspace/$workspaceId/transactional-notifications"
          params={{ workspaceId }}
        >
          {t`Transactional`}
        </Link>
      )
    },
    hasAccess('workspace') && {
      key: 'blog',
      icon: (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="lucide lucide-pen-line-icon lucide-pen-line"
        >
          <path d="M13 21h8" />
          <path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z" />
        </svg>
      ),
      label: (
        <Link to="/console/workspace/$workspaceId/blog" params={{ workspaceId }}>
          {t`Blog`}
        </Link>
      )
    },
    hasAccess('workspace') && {
      key: 'file-manager',
      icon: <FontAwesomeIcon icon={faImage} size="sm" style={{ opacity: 0.6 }} />,
      // icon: (
      //   <svg
      //     xmlns="http://www.w3.org/2000/svg"
      //     width="16"
      //     height="16"
      //     viewBox="0 0 24 24"
      //     fill="none"
      //     stroke="currentColor"
      //     strokeWidth="2"
      //     strokeLinecap="round"
      //     strokeLinejoin="round"
      //     className="lucide lucide-image-icon lucide-image opacity-70"
      //   >
      //     <rect width="18" height="18" x="3" y="3" rx="2" ry="2" />
      //     <circle cx="9" cy="9" r="2" />
      //     <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />
      //   </svg>
      // ),
      label: (
        <Link to="/console/workspace/$workspaceId/file-manager" params={{ workspaceId }}>
          {t`File Manager`}
        </Link>
      )
    },
    hasAccess('message_history') && {
      key: 'logs',
      icon: <FontAwesomeIcon icon={faBarsStaggered} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/console/workspace/$workspaceId/logs" params={{ workspaceId }}>
          {t`Logs`}
        </Link>
      )
    },
    hasAccess('workspace') && {
      key: 'settings',
      icon: <SettingOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId/settings" params={{ workspaceId }}>
          {t`Settings`}
        </Link>
      )
    }
  ].filter((item) => Boolean(item)) as Array<{ key: string; icon: React.ReactNode; label: React.ReactNode }>

  return (
    <ContactsCsvUploadProvider>
      <Layout style={{ minHeight: '100vh', backgroundColor: '#F9F9F9' }}>
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
              zIndex: 10,
              backgroundColor: '#F9F9F9'
            }}
            collapsible
            collapsed={collapsed}
            trigger={null}
            className="border-r border-gray-200"
          >
            <div
              style={{
                padding: '16px 0 16px 27px',
                textAlign: 'center',
                borderBottom: '1px solid #f0f0f0'
              }}
            >
              <img
                src={collapsed ? '/console/icon.png' : '/console/logo.png'}
                alt=""
                style={{
                  height: '31px',
                  width: 'auto',
                  transition: 'height 0.2s'
                }}
              />
            </div>
            <Menu
              mode="inline"
              selectedKeys={[selectedKey]}
              style={{
                height: 'calc(100% - 120px)',
                borderRight: 0,
                backgroundColor: '#F9F9F9',
                fontSize: '13px',
                fontWeight: 600
              }}
              items={loadingPermissions ? [] : menuItems}
              theme="light"
            />
            <div
              style={{
                position: 'fixed',
                bottom: 0,
                left: 0,
                width: collapsed ? '80px' : '249px',
                padding: '16px',
                // backgroundColor: '#F9F9F9',
                zIndex: 1
              }}
            >
              <div
                style={{
                  borderBottom: '1px solid #f0f0f0',
                  textAlign: 'center',
                  fontSize: '9px',
                  color: '#000',
                  opacity: 0.7,
                  marginBottom: '8px',
                  paddingBottom: '8px'
                }}
              >
                v{window.VERSION || '1.0'}
              </div>
              <Button
                type="text"
                block
                icon={<FontAwesomeIcon icon={collapsed ? faAngleRight : faAngleLeft} />}
                onClick={() => setCollapsed(!collapsed)}
              >
                {!collapsed && t`Collapse`}
              </Button>
            </div>
          </Sider>
          <Header
            style={{
              position: 'fixed',
              top: 0,
              right: 0,
              width: `calc(100% - ${collapsed ? '80px' : '250px'})`,
              height: '64px',
              backgroundColor: '#F9F9F9',
              borderBottom: '1px solid #f0f0f0',
              padding: '0 24px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              zIndex: 9,
              transition: 'width 0.2s'
            }}
          >
            <Select
              value={workspaceId}
              variant="filled"
              onChange={handleWorkspaceChange}
              style={{ width: '200px' }}
              placeholder={t`Select workspace`}
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
                            <FontAwesomeIcon icon={faPlus} /> {t`New workspace`}
                          </Space>
                        ),
                        value: 'new-workspace'
                      }
                    ]
                  : [])
              ]}
            />
            <Space size="middle">
              <Dropdown
                trigger={['click']}
                menu={{
                  items: [
                    {
                      key: 'docs',
                      label: (
                        <a
                          href="https://docs.notifuse.com/"
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          <FontAwesomeIcon icon={faFileLines} className="mr-2" /> {t`Documentation`}
                        </a>
                      )
                    },
                    {
                      key: 'report-issue',
                      label: (
                        <a
                          href="https://github.com/notifuse/notifuse/issues"
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          <WarningOutlined className="mr-2" />
                          {t`Report An Issue`}
                        </a>
                      )
                    }
                  ]
                }}
                placement="bottomRight"
              >
                <Button
                  color="default"
                  variant="filled"
                  icon={<FontAwesomeIcon icon={faQuestionCircle} />}
                >
                  {t`Help`}
                </Button>
              </Dropdown>
              <LanguageSwitcher />
              <Dropdown
                menu={{
                  items: [
                    {
                      key: 'logout',
                      label: (
                        <Space>
                          <FontAwesomeIcon icon={faPowerOff} size="sm" style={{ opacity: 0.7 }} />
                          {t`Logout`}
                        </Space>
                      ),
                      onClick: () => signout()
                    }
                  ]
                }}
                trigger={['click']}
                placement="bottomRight"
              >
                <Button type="text">
                  <Space size="small">
                    <Avatar src={getGravatarUrl(user?.email)} size={24} />
                    {user?.email}
                    <DownOutlined style={{ fontSize: '10px' }} />
                  </Space>
                </Button>
              </Dropdown>
            </Space>
          </Header>
          <Layout
            style={{
              marginLeft: collapsed ? '80px' : '250px',
              marginTop: '64px',
              padding: isSettingsPage ? '0' : '24px',
              transition: 'margin-left 0.2s',
              backgroundColor: '#F9F9F9'
            }}
          >
            <Content style={{ backgroundColor: '#F9F9F9' }}>
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
