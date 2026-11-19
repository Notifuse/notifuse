import { Layout, Menu, Select, Space, Button, Dropdown, message, Avatar } from 'antd'
import { Outlet, Link, useParams, useMatches, useNavigate } from '@tanstack/react-router'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { useLingui } from '@lingui/react/macro'
import md5 from 'blueimp-md5'
import {
  faImage,
  faPaperPlane
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
  DownOutlined
} from '@ant-design/icons'
import {
  getBrandHiddenWorkspaceNavKeys,
  isHiddenWorkspaceNavPath,
  isMailStudioAdmin
} from '../lib/hidden-mail-studio-nav'

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
  const [isMobile, setIsMobile] = useState(typeof window !== 'undefined' ? window.innerWidth < 768 : false)
  const [userPermissions, setUserPermissions] = useState<UserPermissions | null>(null)
  const [loadingPermissions, setLoadingPermissions] = useState(true)

  // Handle window resize for mobile breakpoint
  useEffect(() => {
    const handleResize = () => {
      const mobile = window.innerWidth < 768
      setIsMobile(mobile)
      if (mobile) {
        setCollapsed(true)
      }
    }
    handleResize()
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  // Use useMatches to determine the current route path
  const matches = useMatches()
  const currentPath = matches[matches.length - 1]?.pathname || ''
  const isSettingsPage = currentPath.includes('/settings') || currentPath.includes('/blog')
  const isAdmin = isMailStudioAdmin(user?.email)
  const showTopBar = isAdmin
  const brandHiddenNavKeys = getBrandHiddenWorkspaceNavKeys(user?.email)

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

  // Brand-hidden Mail Studio features: no menu entry; typed URL sends home.
  useEffect(() => {
    if (!workspaceId || !isHiddenWorkspaceNavPath(currentPath, user?.email)) return
    navigate({
      to: '/console/workspace/$workspaceId',
      params: { workspaceId },
      replace: true
    })
  }, [currentPath, workspaceId, navigate, user?.email])

  // Determine which key should be selected based on the current path
  let selectedKey = 'analytics' // Default to analytics/dashboard
  if (currentPath.includes('/email-senders')) {
    selectedKey = 'email-senders'
  } else if (currentPath.includes('/settings')) {
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
          {/* Plain string: Lingui hash (eOlBfg) showed when catalog missed Transactional. Brands hide this nav. */}
          Transactional
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
      key: 'email-senders',
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
          className="opacity-70"
        >
          <rect width="20" height="16" x="2" y="4" rx="2" />
          <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
        </svg>
      ),
      label: (
        <Link to="/console/workspace/$workspaceId/email-senders" params={{ workspaceId }}>
          {t`Mail address`}
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
  ].filter(
    (item) =>
      Boolean(item) &&
      !brandHiddenNavKeys.includes((item as { key: string }).key)
  ) as Array<{ key: string; icon: React.ReactNode; label: React.ReactNode }>

  return (
    <ContactsCsvUploadProvider>
      <Layout style={{ minHeight: '100vh', backgroundColor: '#f7f9ff' }}>
        <Layout>
          {/* Mobile backdrop shadow when drawer is open */}
          {isMobile && !collapsed && (
            <div
              className="fixed inset-0 bg-[#0b1433]/20 z-20 backdrop-blur-xs transition-opacity"
              onClick={() => setCollapsed(true)}
            />
          )}

          <Sider
            width={250}
            theme="light"
            style={{
              position: 'fixed',
              height: '100vh',
              left: 0,
              top: 0,
              overflow: 'auto',
              zIndex: 30,
              backgroundColor: '#ffffff',
              borderRight: '1px solid #e6ecf8',
              boxShadow: '2px 0 10px 0 rgba(11, 20, 51, 0.06)',
              transform: isMobile && collapsed ? 'translateX(-100%)' : 'translateX(0)',
              transition: 'transform 0.2s ease-in-out, width 0.2s ease-in-out'
            }}
            collapsible
            collapsed={collapsed}
            trigger={null}
          >
            <div
              style={{
                height: '64px',
                padding: collapsed ? '0 16px' : '0 20px',
                display: 'flex',
                alignItems: 'center',
                justify: collapsed ? 'center' : 'flex-start',
                borderBottom: '1px solid #e6ecf8'
              }}
            >
              {collapsed ? (
                <div className="w-9 h-9 rounded-xl bg-gradient-to-tr from-[#2457d6] to-[#c63fe0] flex items-center justify-center text-white shadow-lg shadow-[0_8px_24px_rgba(11,20,51,0.12)]">
                  <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <rect width="20" height="16" x="2" y="4" rx="2"/>
                    <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7"/>
                  </svg>
                </div>
              ) : (
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-xl bg-gradient-to-tr from-[#2457d6] to-[#c63fe0] flex items-center justify-center text-white shadow-lg shadow-[0_8px_24px_rgba(11,20,51,0.12)]">
                    <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                      <rect width="20" height="16" x="2" y="4" rx="2"/>
                      <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7"/>
                    </svg>
                  </div>
                  <div>
                    <div className="flex items-center gap-1.5">
                      <span className="font-bold text-[#0b1433] text-base tracking-tight leading-none">Mail Studio</span>
                      <span className="bg-[#e8efff] text-[#2457d6] text-[10px] px-1.5 py-0.5 rounded font-semibold border border-[#c7d2e8] uppercase tracking-wider">
                        PRO
                      </span>
                      {isAdmin ? (
                        <span
                          className="bg-[#ecfdf3] text-[#067647] text-[10px] px-1.5 py-0.5 rounded font-semibold border border-[#abefc6] uppercase tracking-wider"
                          title={user?.email || 'ROOT_EMAIL'}
                        >
                          Admin
                        </span>
                      ) : null}
                    </div>
                    <span className="text-[11px] text-[#5e6b8c] font-medium leading-tight block mt-0.5">Automated Dispatch</span>
                  </div>
                </div>
              )}
            </div>
            <Menu
              mode="inline"
              selectedKeys={[selectedKey]}
              style={{
                height: 'calc(100% - 130px)',
                borderRight: 0,
                backgroundColor: '#ffffff',
                fontSize: '13px',
                fontWeight: 500,
                padding: '12px 8px'
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
                padding: '12px 16px',
                backgroundColor: '#ffffff',
                borderTop: '1px solid #e6ecf8',
                zIndex: 1
              }}
            >
              <div className="flex flex-col gap-2">
                {!showTopBar && (
                  <Button
                    type="text"
                    size="small"
                    block={!collapsed}
                    className="text-red-500 hover:text-red-600 hover:bg-red-50 justify-start"
                    icon={<FontAwesomeIcon icon={faPowerOff} size="sm" />}
                    onClick={() => signout()}
                  >
                    {!collapsed && t`Logout`}
                  </Button>
                )}
                <div className="flex items-center justify-between">
                  {!collapsed && (
                    <span className="text-[11px] font-medium text-[#5e6b8c]">
                      Mail Studio v{window.VERSION || '2.4'}
                    </span>
                  )}
                  <Button
                    type="text"
                    size="small"
                    className="text-[#5e6b8c] hover:text-[#0b1433] hover:bg-[#f2f6ff]"
                    icon={<FontAwesomeIcon icon={collapsed ? faAngleRight : faAngleLeft} />}
                    onClick={() => setCollapsed(!collapsed)}
                  >
                    {!collapsed && t`Collapse`}
                  </Button>
                </div>
              </div>
            </div>
          </Sider>
          {!showTopBar && isMobile && collapsed && (
            <Button
              type="primary"
              className="fixed top-3 left-3 z-40 shadow-md"
              icon={<FontAwesomeIcon icon={faBarsStaggered} />}
              onClick={() => setCollapsed(false)}
            />
          )}
          {showTopBar ? (
          <Header
            style={{
              position: 'fixed',
              top: 0,
              right: 0,
              width: isMobile ? '100%' : `calc(100% - ${collapsed ? '80px' : '250px'})`,
              height: '64px',
              backgroundColor: '#ffffff',
              borderBottom: '1px solid #e6ecf8',
              padding: isMobile ? '0 12px' : '0 24px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              zIndex: 10,
              transition: 'width 0.2s',
              boxShadow: '0 2px 10px 0 rgba(11, 20, 51, 0.06)'
            }}
          >
            <div className="flex items-center gap-2 sm:gap-3">
              {isMobile && (
                <Button
                  type="text"
                  className="text-[#5e6b8c] hover:text-[#0b1433]"
                  icon={<FontAwesomeIcon icon={faBarsStaggered} />}
                  onClick={() => setCollapsed(!collapsed)}
                />
              )}

              <Select
                value={workspaceId}
                variant="filled"
                onChange={handleWorkspaceChange}
                style={{ width: isMobile ? '140px' : '210px' }}
                placeholder={t`Select workspace`}
                className="rounded-lg text-xs sm:text-sm"
                options={[
                  ...workspaces.map((workspace: Workspace) => ({
                    label: (
                      <Space size="small" className="font-medium text-[#0b1433]">
                        {workspace.settings.logo_url ? (
                          <img
                            src={workspace.settings.logo_url}
                            alt=""
                            style={{
                              height: '16px',
                              width: '16px',
                              objectFit: 'contain',
                              verticalAlign: 'middle',
                              display: 'inline-block',
                              borderRadius: '3px'
                            }}
                          />
                        ) : (
                          <span className="w-4 h-4 rounded bg-[#e8efff] text-[#2457d6] flex items-center justify-center text-[10px] font-bold">
                            {workspace.name.substring(0, 1)}
                          </span>
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
                            <Space className="text-[#2457d6] font-medium">
                              <FontAwesomeIcon icon={faPlus} /> {t`New workspace`}
                            </Space>
                          ),
                          value: 'new-workspace'
                        }
                      ]
                    : [])
                ]}
              />

              <div className="hidden md:flex items-center gap-2 px-3 py-1.5 rounded-full bg-[#e8f8ef] text-[#1a7f4b] border border-[#b8e0c8] text-xs font-medium">
                <span className="pulse-dot-green"></span>
                <span>System Operational</span>
              </div>
            </div>

            <Space size={isMobile ? 'small' : 'middle'}>
              <LanguageSwitcher />
              <Dropdown
                menu={{
                  items: [
                    {
                      key: 'logout',
                      label: (
                        <Space className="text-red-500">
                          <FontAwesomeIcon icon={faPowerOff} size="sm" />
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
                <button className="flex items-center gap-2 py-1 px-2.5 rounded-lg border border-[#e6ecf8] bg-[#f2f6ff] hover:bg-[#e8efff] transition-all cursor-pointer">
                  <Avatar src={getGravatarUrl(user?.email)} size={26} className="ring-2 ring-[#c7d2e8]" />
                  {!isMobile && (
                    <span className="text-xs font-semibold text-[#0b1433] max-w-[140px] truncate">{user?.email}</span>
                  )}
                  <DownOutlined style={{ fontSize: '9px' }} className="text-[#5e6b8c]" />
                </button>
              </Dropdown>
            </Space>
          </Header>
          ) : null}
          <Layout
            style={{
              marginLeft: isMobile ? '0' : (collapsed ? '80px' : '250px'),
              marginTop: showTopBar ? '64px' : '0',
              padding: isSettingsPage ? '0' : (isMobile ? '12px 8px' : '24px'),
              transition: 'margin-left 0.2s',
              backgroundColor: '#f7f9ff'
            }}
          >
            <Content style={{ backgroundColor: '#f7f9ff' }}>
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
