import { Layout, Menu, Select, Space, Button, Dropdown, message, Avatar } from 'antd'
import type { MenuProps } from 'antd'
import { Outlet, Link, useParams, useMatches, useNavigate } from '@tanstack/react-router'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { useLingui } from '@lingui/react/macro'
import md5 from 'blueimp-md5'
import {
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
import { createEmptyPermissions, createFullPermissions } from '../services/api/permissions'
import { isRootUser } from '../services/api/auth'
import {
  AppstoreOutlined,
  FolderOpenOutlined,
  LineChartOutlined,
  SettingOutlined,
  WarningOutlined,
  DownOutlined
} from '@ant-design/icons'

const { Content, Sider, Header } = Layout

/** Web analytics sub-entries, mirroring the routes under /web-analytics. */
const WEB_ANALYTICS_SECTIONS = ['dashboard', 'live', 'explore', 'goals', 'filters', 'annotations']

/** Collapsible sidebar groups, and the path fragments living inside each. */
const MENU_GROUPS: Record<string, string[]> = {
  'web-analytics': ['/web-analytics'],
  content: ['/templates', '/blog', '/file-manager']
}

type MenuItem = NonNullable<MenuProps['items']>[number]

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

  // The web analytics settings live at /settings/web-analytics and belong to
  // the settings entry, so a settings path belongs to no group.
  const activeGroup = currentPath.includes('/settings')
    ? null
    : (Object.entries(MENU_GROUPS).find(([, fragments]) =>
        fragments.some((fragment) => currentPath.includes(fragment))
      )?.[0] ?? null)
  const inWebAnalytics = activeGroup === 'web-analytics'

  const [openKeys, setOpenKeys] = useState<string[]>(activeGroup ? [activeGroup] : [])

  // Entering a group reveals it and leaving reclaims the rows it costs, so the
  // sidebar only pays for the group you are actually in. One group at a time,
  // and depending on the two flags rather than the path leaves a manual toggle
  // alone for as long as you stay put. While collapsed the rail shows flyouts
  // and rc-menu drives openKeys itself; this restores them on expanding back.
  useEffect(() => {
    if (collapsed) return
    setOpenKeys(activeGroup ? [activeGroup] : [])
  }, [activeGroup, collapsed])

  // Fetch user permissions for the current workspace
  useEffect(() => {
    const fetchUserPermissions = async () => {
      if (!user || !workspaceId) {
        setLoadingPermissions(false)
        return
      }

      // If user is root, they have full permissions
      if (isRootUser(user.email)) {
        setUserPermissions(createFullPermissions())
        setLoadingPermissions(false)
        return
      }

      try {
        const response = await workspaceService.getMembers(workspaceId)
        const currentUserMember = response.members.find((member) => member.user_id === user.id)

        if (currentUserMember) {
          // The stored map may be partial or null; a resource it does not mention is denied,
          // which is what the empty base spells out.
          setUserPermissions({ ...createEmptyPermissions(), ...currentUserMember.permissions })
        } else {
          // User is not a member of this workspace, set empty permissions
          setUserPermissions(createEmptyPermissions())
        }
      } catch (error) {
        console.error('Failed to fetch user permissions', error)
        // On error, assume no permissions
        setUserPermissions(createEmptyPermissions())
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

  // Group titles are plain text, so a click on one only toggles it. Opening a
  // group also lands on its first entry, which is what the click was reaching
  // for; closing it goes nowhere, so the caret keeps its meaning. The collapsed
  // rail opens these submenus on hover, where navigating would follow the mouse.
  const handleOpenChange: MenuProps['onOpenChange'] = (keys) => {
    const opened = keys.find((key) => !openKeys.includes(key))
    setOpenKeys(keys)
    if (collapsed || !opened) return
    if (opened === 'web-analytics') {
      navigate({
        to: '/console/workspace/$workspaceId/web-analytics/$tab',
        params: { workspaceId, tab: 'dashboard' }
      })
    } else if (opened === 'content') {
      // Mirrors the first child the group actually renders.
      if (hasAccess('templates')) {
        navigate({ to: '/console/workspace/$workspaceId/templates', params: { workspaceId } })
      } else {
        navigate({ to: '/console/workspace/$workspaceId/blog', params: { workspaceId } })
      }
    }
  }

  // Determine which key should be selected based on the current path
  let selectedKey = 'analytics' // Default to analytics/dashboard
  if (currentPath.includes('/settings')) {
    // Must be checked before '/web-analytics': the web analytics settings live
    // at /settings/web-analytics and belong to the settings entry.
    selectedKey = 'settings'
  } else if (inWebAnalytics) {
    // /web-analytics alone redirects to the dashboard, so an unrecognized
    // trailing segment lands on the same entry the user ends up looking at.
    const section = currentPath.split('/web-analytics/')[1]?.split('/')[0] ?? ''
    selectedKey = WEB_ANALYTICS_SECTIONS.includes(section)
      ? `web-analytics-${section}`
      : 'web-analytics-dashboard'
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

  // Templates, Blog and File Manager are the material you author and reuse.
  // Built here rather than inline so the group can be dropped entirely when a
  // member can reach none of it, instead of showing an empty expandable row.
  // Children carry no icons, matching the Web Analytics submenu.
  const contentChildren: MenuItem[] = []
  if (hasAccess('templates')) {
    contentChildren.push({
      key: 'templates',
      label: (
        <Link to="/console/workspace/$workspaceId/templates" params={{ workspaceId }}>
          {t`Templates`}
        </Link>
      )
    })
  }
  if (hasAccess('workspace')) {
    contentChildren.push(
      {
        key: 'blog',
        label: (
          <Link to="/console/workspace/$workspaceId/blog" params={{ workspaceId }}>
            {t`Blog`}
          </Link>
        )
      },
      {
        key: 'file-manager',
        label: (
          <Link to="/console/workspace/$workspaceId/file-manager" params={{ workspaceId }}>
            {t`File Manager`}
          </Link>
        )
      }
    )
  }

  const menuItems = [
    hasAccess('message_history') && {
      key: 'analytics',
      // icon: <FontAwesomeIcon icon={faChartLine} size="sm" style={{ opacity: 0.7 }} />,
      icon: <AppstoreOutlined />,
      label: (
        <Link to="/console/workspace/$workspaceId" params={{ workspaceId }}>
          {t`Dashboard`}
        </Link>
      )
    },
    hasAccess('web_analytics') && {
      key: 'web-analytics',
      icon: <LineChartOutlined />,
      // A submenu rather than a link: the caret on the right toggles the
      // section, and handleOpenChange lands on the dashboard when it opens.
      label: t`Web Analytics`,
      children: [
        {
          key: 'web-analytics-dashboard',
          label: (
            <Link
              to="/console/workspace/$workspaceId/web-analytics/$tab"
              params={{ workspaceId, tab: 'dashboard' }}
            >
              {t`Dashboard`}
            </Link>
          )
        },
        {
          key: 'web-analytics-live',
          label: (
            <Link to="/console/workspace/$workspaceId/web-analytics/live" params={{ workspaceId }}>
              {t`Live`}
            </Link>
          )
        },
        {
          key: 'web-analytics-explore',
          label: (
            <Link
              to="/console/workspace/$workspaceId/web-analytics/$tab"
              params={{ workspaceId, tab: 'explore' }}
            >
              {t`Explore`}
            </Link>
          )
        },
        {
          key: 'web-analytics-goals',
          label: (
            <Link
              to="/console/workspace/$workspaceId/web-analytics/$tab"
              params={{ workspaceId, tab: 'goals' }}
            >
              {t`Goals`}
            </Link>
          )
        },
        {
          key: 'web-analytics-filters',
          label: (
            <Link
              to="/console/workspace/$workspaceId/web-analytics/$tab"
              params={{ workspaceId, tab: 'filters' }}
            >
              {t`Filters`}
            </Link>
          )
        },
        {
          key: 'web-analytics-annotations',
          label: (
            <Link
              to="/console/workspace/$workspaceId/web-analytics/$tab"
              params={{ workspaceId, tab: 'annotations' }}
            >
              {t`Annotations`}
            </Link>
          )
        }
      ]
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
    contentChildren.length > 0 && {
      key: 'content',
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
          className="lucide lucide-files-icon lucide-files opacity-70"
        >
          <path d="M20 7h-3a2 2 0 0 1-2-2V2" />
          <path d="M9 18a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h7l4 4v10a2 2 0 0 1-2 2Z" />
          <path d="M3 7.6v12.8A1.6 1.6 0 0 0 4.6 22h9.8" />
        </svg>
      ),
      label: t`Content`,
      children: contentChildren
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
              // The nav inside owns the scrolling; the panel must not also
              // scroll, or the logo and the collapse button travel with it.
              overflow: 'hidden',
              zIndex: 10,
              backgroundColor: '#F9F9F9'
            }}
            collapsible
            collapsed={collapsed}
            trigger={null}
            className="workspace-sider border-r border-gray-200"
          >
            <div
              style={{
                flex: '0 0 auto',
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
            <div className="workspace-sider-nav">
              <Menu
                mode="inline"
                selectedKeys={[selectedKey]}
                openKeys={openKeys}
                onOpenChange={handleOpenChange}
                style={{
                  borderRight: 0,
                  backgroundColor: '#F9F9F9',
                  fontSize: '13px',
                  // Item labels are <Link> anchors, which index.css pins to 500.
                  // Submenu titles are plain text and inherit this instead, so it
                  // has to match or the group rows read heavier than the rest.
                  fontWeight: 500
                }}
                items={loadingPermissions ? [] : menuItems}
                theme="light"
              />
            </div>
            <div
              style={{
                flex: '0 0 auto',
                padding: '16px',
                borderTop: '1px solid #f0f0f0',
                backgroundColor: '#F9F9F9'
              }}
            >
              <div
                style={{
                  textAlign: 'center',
                  fontSize: '9px',
                  color: '#000',
                  opacity: 0.7,
                  marginBottom: '8px'
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
