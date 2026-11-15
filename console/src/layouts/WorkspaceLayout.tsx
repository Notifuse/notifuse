import { useEffect, useState } from 'react'
import { message, type MenuProps } from 'antd'
import { Outlet, useMatches, useNavigate, useParams } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import type { UserPermissions } from '../services/api/types'
import { ContactsCsvUploadProvider } from '../components/contacts/ContactsCsvUploadProvider'
import { FileManagerProvider } from '../components/file_manager/context'
import type { FileManagerSettings } from '../components/file_manager/interfaces'
import { workspaceService } from '../services/api/workspace'
import { isRootUser } from '../services/api/auth'
import { withBannerOffset } from '../components/license/bannerOffset'
import { AppSidebar } from './app-shell/AppSidebar'
import {
  activeGroup as groupOf,
  buildMenuItems,
  firstPageOfGroup,
  homePage,
  selectedMenuKey
} from './app-shell/menuItems'
import { useSidebarPinned } from './app-shell/useSidebarPinned'
import { useAuditLogAccess } from '../hooks/useAuditLogAccess'

export type PageLayout = 'page' | 'rail'

export function WorkspaceLayout() {
  const { t, i18n } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { signout, workspaces, user, refreshWorkspaces } = useAuth()
  const navigate = useNavigate()
  // Keyed by the workspace: the layout stays mounted on a switch, and shows no entry until the new
  // workspace's permissions arrive.
  const {
    permissions: userPermissions,
    role,
    loading: loadingPermissions
  } = useWorkspacePermissions(workspaceId)

  const matches = useMatches()
  const currentPath = matches[matches.length - 1]?.pathname || ''
  // The deepest route that declares a layout decides it; a route declaring none is a plain page.
  const pageLayout: PageLayout =
    [...matches].reverse().find((match) => match.staticData?.layout)?.staticData?.layout ?? 'page'

  // Pinned, the sidebar sits in the page flow; unpinned, it is the folded rail and opens over the
  // content.
  const [storedPinned, setStoredPinned] = useSidebarPinned()

  // Contacts and Broadcasts need the width: they open with the sidebar unpinned, and a pin made
  // there holds until the visit ends, without touching the stored choice.
  const widePage = [...matches].reverse().find((match) => match.staticData?.unpinSidebar)?.routeId ?? null
  const [visitPinned, setVisitPinned] = useState(false)
  const [visitedPage, setVisitedPage] = useState(widePage)
  if (visitedPage !== widePage) {
    // Adjusted while rendering, as React recommends for state that follows a value.
    setVisitedPage(widePage)
    setVisitPinned(false)
  }
  const pinned = widePage ? visitPinned : storedPinned
  const setPinned = (next: boolean) => (widePage ? setVisitPinned(next) : setStoredPinned(next))

  const activeGroup = groupOf(currentPath)
  const [openKeys, setOpenKeys] = useState<string[]>(activeGroup ? [activeGroup] : [])

  // Entering a group reveals it and leaving reclaims the rows it costs, so the sidebar only pays
  // for the group you are actually in. One group at a time, and depending on the group rather than
  // the path leaves a manual toggle alone for as long as you stay put, folds and hovers included
  // (AppSidebar keeps rc-menu from clearing the groups when it folds).
  useEffect(() => {
    setOpenKeys(activeGroup ? [activeGroup] : [])
  }, [activeGroup])

  const auditAccess = useAuditLogAccess(workspaceId)

  // Helper function to check if user has access to a resource
  const hasAccess = (resource: keyof UserPermissions): boolean => {
    if (!userPermissions) return false
    // User needs at least read or write permission to access the resource
    const permissions = userPermissions[resource]
    return permissions?.read || permissions?.write || false
  }

  // Group titles are plain text, so a click on one only toggles it. Pinned, opening a group also
  // lands on its first entry, which is what the click was reaching for; closing it goes nowhere, so
  // the caret keeps its meaning. Unpinned, it only opens: a navigation folds the sidebar, which
  // would hide the group's other entries before you could pick one.
  const handleOpenChange: MenuProps['onOpenChange'] = (keys) => {
    const opened = keys.find((key) => !openKeys.includes(key))
    setOpenKeys(keys)
    if (!pinned || !opened) return
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
          pinned={pinned}
          onPinnedChange={setPinned}
          navigationKey={currentPath}
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
          home={homePage(userPermissions, loadingPermissions, workspaceId, i18n)}
        />
        <main
          data-page-layout={pageLayout}
          // isolate: the z-indexes of the page (sticky table columns at 12 and 13) stay inside it,
          // under the fixed sidebar (z-10). The workspace, help and account menus render inside the
          // sidebar; the tooltips and submenus of the folded navigation render in the body.
          className={`isolate ${pinned ? 'ml-sidebar' : 'ml-sidebar-collapsed'}`}
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
