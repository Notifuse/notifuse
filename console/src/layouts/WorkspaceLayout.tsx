import { useEffect, useState } from 'react'
import { message, type MenuProps } from 'antd'
import { Outlet, useMatches, useNavigate, useParams } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { useAuth } from '../contexts/AuthContext'
import type { UserPermissions } from '../services/api/types'
import { ContactsCsvUploadProvider } from '../components/contacts/ContactsCsvUploadProvider'
import { FileManagerProvider } from '../components/file_manager/context'
import type { FileManagerSettings } from '../components/file_manager/interfaces'
import { workspaceService } from '../services/api/workspace'
import { createEmptyPermissions, createFullPermissions } from '../services/api/permissions'
import { isRootUser } from '../services/api/auth'
import { withBannerOffset } from '../components/license/bannerOffset'
import { AppSidebar } from './app-shell/AppSidebar'
import type { AccountRole } from './app-shell/AccountMenu'
import {
  activeGroup as groupOf,
  buildMenuItems,
  firstPageOfGroup,
  selectedMenuKey
} from './app-shell/menuItems'
import { useSidebarCollapsed } from './app-shell/useSidebarCollapsed'
import { useAuditLogAccess } from '../hooks/useAuditLogAccess'

export type PageLayout = 'page' | 'rail'

export function WorkspaceLayout() {
  const { t, i18n } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { signout, workspaces, user, refreshWorkspaces } = useAuth()
  const navigate = useNavigate()
  const [userPermissions, setUserPermissions] = useState<UserPermissions | null>(null)
  const [role, setRole] = useState<AccountRole | undefined>()
  const [loadingPermissions, setLoadingPermissions] = useState(true)

  const matches = useMatches()
  const currentPath = matches[matches.length - 1]?.pathname || ''
  // The deepest route that declares a layout decides it; a route declaring none is a plain page.
  const pageLayout: PageLayout =
    [...matches].reverse().find((match) => match.staticData?.layout)?.staticData?.layout ?? 'page'
  const rail = pageLayout === 'rail'

  // A rail page always folds the sidebar; its expand button peeks over the rail instead, which
  // neither moves the content nor touches the stored choice.
  const [storedCollapsed, setStoredCollapsed] = useSidebarCollapsed()
  const [peek, setPeek] = useState(false)
  const menuCollapsed = rail ? !peek : storedCollapsed
  const offsetCollapsed = rail || storedCollapsed

  // Every navigation folds the peek back.
  useEffect(() => {
    setPeek(false)
  }, [currentPath])

  const activeGroup = groupOf(currentPath)
  const [openKeys, setOpenKeys] = useState<string[]>(activeGroup ? [activeGroup] : [])

  // Entering a group reveals it and leaving reclaims the rows it costs, so the sidebar only pays
  // for the group you are actually in. One group at a time, and depending on the group and the
  // fold rather than the path leaves a manual toggle alone for as long as you stay put. Folded,
  // the menu shows flyouts and rc-menu drives openKeys itself; this restores them on unfolding.
  useEffect(() => {
    if (menuCollapsed) return
    setOpenKeys(activeGroup ? [activeGroup] : [])
  }, [activeGroup, menuCollapsed])

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
        setRole('root')
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
          setRole(currentUserMember.role === 'owner' ? 'owner' : 'member')
        } else {
          // User is not a member of this workspace, set empty permissions
          setUserPermissions(createEmptyPermissions())
          setRole(undefined)
        }
      } catch (error) {
        console.error('Failed to fetch user permissions', error)
        // On error, assume no permissions
        setUserPermissions(createEmptyPermissions())
        setRole(undefined)
      } finally {
        setLoadingPermissions(false)
      }
    }

    fetchUserPermissions()
  }, [workspaceId, user])

  // Helper function to check if user has access to a resource
  const auditAccess = useAuditLogAccess(workspaceId)

  const hasAccess = (resource: keyof UserPermissions): boolean => {
    if (!userPermissions) return false
    // User needs at least read or write permission to access the resource
    const permissions = userPermissions[resource]
    return permissions?.read || permissions?.write || false
  }

  // Group titles are plain text, so a click on one only toggles it. Opening a group also lands on
  // its first entry, which is what the click was reaching for; closing it goes nowhere, so the
  // caret keeps its meaning. The folded menu opens these groups on hover, where navigating would
  // follow the mouse.
  const handleOpenChange: MenuProps['onOpenChange'] = (keys) => {
    const opened = keys.find((key) => !openKeys.includes(key))
    setOpenKeys(keys)
    if (menuCollapsed || !opened) return
    const page = firstPageOfGroup(opened, hasAccess, workspaceId)
    if (page) navigate(page)
  }

  const handleWorkspaceChange = (nextWorkspaceId: string) => {
    navigate({ to: '/console/workspace/$workspaceId', params: { workspaceId: nextWorkspaceId } })
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

  // The Logs entry shows when one of its tabs can open: each needs its read right, and the audit
  // log its own access (owner, grant or root).
  const canRead = (resource: keyof UserPermissions) => Boolean(userPermissions?.[resource]?.read)
  const canOpenLogs =
    canRead('message_history') ||
    canRead('webhook_events') ||
    canRead('webhook_subscriptions') ||
    auditAccess.canRead

  const menuItems = buildMenuItems({ hasAccess, workspaceId, i18n, canOpenLogs })

  return (
    <ContactsCsvUploadProvider>
      <div className="min-h-screen bg-canvas">
        <AppSidebar
          collapsed={menuCollapsed}
          peeking={rail && peek}
          rail={rail}
          onToggle={() => setStoredCollapsed(!storedCollapsed)}
          onPeekChange={setPeek}
          menuItems={loadingPermissions ? [] : menuItems}
          selectedKey={selectedMenuKey(currentPath)}
          openKeys={openKeys}
          onOpenChange={handleOpenChange}
          workspaces={workspaces}
          workspaceId={workspaceId}
          isRoot={isRootUser(user?.email)}
          onSelectWorkspace={handleWorkspaceChange}
          onCreateWorkspace={() => navigate({ to: '/console/workspace/create' })}
          user={user}
          role={role}
          onSignout={signout}
        />
        <main
          data-page-layout={pageLayout}
          // isolate: the z-indexes of the page (sticky table columns at 12 and 13) stay inside it,
          // under the fixed sidebar (z-10), whose menus and tooltips render inside the sidebar.
          className={`isolate motion-safe:transition-[margin] motion-safe:duration-180 ${
            offsetCollapsed ? 'ml-sidebar-collapsed' : 'ml-sidebar'
          }`}
          // The page starts under the licence banner, which is 0px tall on a licensed console.
          style={{ paddingTop: withBannerOffset('0px') }}
        >
          <div data-page-content className={pageLayout === 'page' ? 'px-5 pt-4 pb-6' : undefined}>
            <FileManagerProvider
              key={`fm-${workspaceId}-${!userPermissions?.templates?.write}`}
              workspaceId={workspaceId}
              settings={workspaces.find((w) => w.id === workspaceId)?.settings.file_manager}
              onUpdateSettings={handleUpdateWorkspaceSettings}
              readOnly={!userPermissions?.templates?.write}
            >
              <Outlet />
            </FileManagerProvider>
          </div>
        </main>
      </div>
    </ContactsCsvUploadProvider>
  )
}
