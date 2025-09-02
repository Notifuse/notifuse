import { Layout, Menu, Select, Space, Button, Dropdown, message } from 'antd'
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
  faBarsStaggered
} from '@fortawesome/free-solid-svg-icons'
import { useAuth } from '../contexts/AuthContext'
import { Workspace } from '../services/api/types'
import { ContactsCsvUploadProvider } from '../components/contacts/ContactsCsvUploadProvider'
import { useState } from 'react'
import { FileManagerProvider } from '../components/file_manager/context'
import { FileManagerSettings } from '../components/file_manager/interfaces'
import { workspaceService } from '../services/api/workspace'
import { isRootUser } from '../services/api/auth'

const { Content, Sider } = Layout

export function WorkspaceLayout() {
  const { workspaceId } = useParams({ from: '/workspace/$workspaceId' })
  const { signout, workspaces, user, refreshWorkspaces } = useAuth()
  const navigate = useNavigate()
  const [collapsed, setCollapsed] = useState(false)

  // Use useMatches to determine the current route path
  const matches = useMatches()
  const currentPath = matches[matches.length - 1]?.pathname || ''

  // Determine which key should be selected based on the current path
  let selectedKey = 'broadcasts'
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
  }

  const handleWorkspaceChange = (workspaceId: string) => {
    if (workspaceId === 'new-workspace') {
      // Navigate to workspace creation page or open a modal
      navigate({ to: '/workspace/create' })
      return
    }

    navigate({
      to: '/workspace/$workspaceId/contacts',
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
    {
      key: 'contacts',
      icon: <FontAwesomeIcon icon={faUserGroup} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/contacts" params={{ workspaceId }}>
          Contacts
        </Link>
      )
    },
    {
      key: 'lists',
      icon: <FontAwesomeIcon icon={faFolderOpen} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/lists" params={{ workspaceId }}>
          Lists
        </Link>
      )
    },
    {
      key: 'templates',
      icon: <FontAwesomeIcon icon={faObjectGroup} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/templates" params={{ workspaceId }}>
          Templates
        </Link>
      )
    },
    {
      key: 'broadcasts',
      icon: <FontAwesomeIcon icon={faPaperPlane} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/broadcasts" params={{ workspaceId }}>
          Broadcasts
        </Link>
      )
    },
    {
      key: 'transactional-notifications',
      icon: <FontAwesomeIcon icon={faTerminal} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/transactional-notifications" params={{ workspaceId }}>
          Transactional
        </Link>
      )
    },
    {
      key: 'file-manager',
      icon: <FontAwesomeIcon icon={faImage} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/file-manager" params={{ workspaceId }}>
          File Manager
        </Link>
      )
    },
    {
      key: 'logs',
      icon: <FontAwesomeIcon icon={faBarsStaggered} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/logs" params={{ workspaceId }}>
          Logs
        </Link>
      )
    },
    {
      key: 'settings',
      icon: <FontAwesomeIcon icon={faGear} size="sm" style={{ opacity: 0.7 }} />,
      label: (
        <Link to="/workspace/$workspaceId/settings" params={{ workspaceId }}>
          Settings
        </Link>
      )
    }
  ]

  return (
    <ContactsCsvUploadProvider>
      <Layout style={{ minHeight: '100vh' }}>
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
              items={menuItems}
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
                settings={workspaces.find((w) => w.id === workspaceId)?.settings.file_manager}
                onUpdateSettings={handleUpdateWorkspaceSettings}
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
