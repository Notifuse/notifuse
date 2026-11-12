import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate, useSearch } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { workspaceService } from '../services/api/workspace'
import { Workspace, WorkspaceMember } from '../services/api/types'
import { WorkspaceMembers } from '../components/settings/WorkspaceMembers'
import { GeneralSettings } from '../components/settings/GeneralSettings'
import { SMTPBridgeSettings } from '../components/settings/SMTPBridgeSettings'
import { Integrations } from '../components/settings/Integrations'
import { CustomFieldsConfiguration } from '../components/settings/CustomFieldsConfiguration'
import { BlogSettings } from '../components/settings/BlogSettings'
import { WebAnalyticsSettings } from '../components/settings/WebAnalyticsSettings'
import { WebhooksSettings } from '../components/settings/WebhooksSettings'
import { useAuth } from '../contexts/AuthContext'
import { isRootUser } from '../services/api/auth'
import { AuditLogsSettings } from '../components/settings/audit_logs/AuditLogsSettings'
import { LicenseSettings } from '../components/settings/LicenseSettings'
import { DeleteWorkspaceSection } from '../components/settings/DeleteWorkspace'
import { Skeleton } from 'antd'
import { Panel, RailLayout } from '../components/layout'
import {
  SettingsSidebar,
  SETTINGS_SECTIONS,
  DEFAULT_SETTINGS_SECTION,
  SettingsSection
} from '../components/settings/SettingsSidebar'

/** A panel-shaped placeholder, while what decides the section's view is not known yet. */
function SettingsSkeleton() {
  return (
    <div data-testid="settings-skeleton" className="flex flex-col gap-3">
      <Panel>
        <Skeleton active paragraph={{ rows: 4 }} />
      </Panel>
      <Panel>
        <Skeleton active paragraph={{ rows: 3 }} />
      </Panel>
    </div>
  )
}

// Sections whose view depends on who the member is: their read-only view must not show before
// the members answer and then switch to the form.
const DEPENDS_ON_ROLE: ReadonlySet<SettingsSection> = new Set<SettingsSection>([
  'general',
  'integrations',
  'custom-fields',
  'webhooks',
  'blog',
  'web-analytics',
  'audit-logs',
  'danger-zone'
])


export function WorkspaceSettingsPage() {
  const { t } = useLingui()
  const { workspaceId, section } = useParams({
    from: '/console/workspace/$workspaceId/settings/$section'
  })
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [members, setMembers] = useState<WorkspaceMember[]>([])
  // The workspace whose members have answered once. A refetch (a General save refreshes the
  // workspaces, which refetches the members) keeps the sections mounted: only another workspace
  // brings the skeletons back.
  const [membersLoadedFor, setMembersLoadedFor] = useState<string | null>(null)
  const membersLoaded = membersLoadedFor === workspaceId
  const currentWorkspaceId = useRef(workspaceId)
  currentWorkspaceId.current = workspaceId
  const [membersError, setMembersError] = useState(false)
  const [isOwner, setIsOwner] = useState(false)
  const [canManageWebhooks, setCanManageWebhooks] = useState(false)
  const [canManageCustomFields, setCanManageCustomFields] = useState(false)
  const [canManageBlog, setCanManageBlog] = useState(false)
  const [canManageWebAnalytics, setCanManageWebAnalytics] = useState(false)
  const { refreshWorkspaces, user, workspaces } = useAuth()
  const navigate = useNavigate()
  const search = useSearch({ from: '/console/workspace/$workspaceId/settings/$section' })

  // The panel of a webhook lives in the URL; opening and closing it replaces the entry.
  const openWebhook = (id?: string) =>
    navigate({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId, section: 'webhooks' },
      search: id ? { webhook: id } : {},
      replace: true
    })

  // The section of the URL, General when it names none.
  const activeSection: SettingsSection = SETTINGS_SECTIONS.includes(section as SettingsSection)
    ? (section as SettingsSection)
    : DEFAULT_SETTINGS_SECTION

  useEffect(() => {
    if (!SETTINGS_SECTIONS.includes(section as SettingsSection)) {
      navigate({
        to: '/console/workspace/$workspaceId/settings/$section',
        params: { workspaceId, section: DEFAULT_SETTINGS_SECTION },
        replace: true
      })
    }
  }, [section, workspaceId, navigate])

  useEffect(() => {
    // Find the workspace from the auth context
    const currentWorkspace = workspaces.find((w) => w.id === workspaceId) || null
    setWorkspace(currentWorkspace)

    fetchMembers()
  // eslint-disable-next-line react-hooks/exhaustive-deps -- fetchMembers is stable
  }, [workspaceId, workspaces])

  const fetchMembers = async () => {
    const requested = workspaceId
    try {
      const response = await workspaceService.getMembers(requested)
      // An answer for a workspace the page has left would apply its rights here.
      if (currentWorkspaceId.current !== requested) return
      setMembers(response.members)
      setMembersError(false)

      // Check if current user is an owner
      if (user) {
        const currentUserMember = response.members.find((member) => member.user_id === user.id)
        setIsOwner(currentUserMember?.role === 'owner')
        // Custom fields can be managed by owners or members with workspace:write permission
        // (mirrors the backend HasPermission(workspace, write) check).
        setCanManageCustomFields(
          currentUserMember?.role === 'owner' ||
            currentUserMember?.permissions?.workspace?.write === true
        )
        // Blog settings can be managed by owners or members with blog:write permission
        // (mirrors the backend HasPermission(blog, write) check).
        setCanManageBlog(
          currentUserMember?.role === 'owner' ||
            currentUserMember?.permissions?.blog?.write === true
        )
        // Web analytics settings can be managed by owners or members with
        // web_analytics:write permission (mirrors the backend
        // HasPermission(web_analytics, write) check).
        setCanManageWebAnalytics(
          currentUserMember?.role === 'owner' ||
            currentUserMember?.permissions?.web_analytics?.write === true
        )
        // Webhook subscriptions can be managed by owners or members with
        // webhook_subscriptions:write (the server's check on every write endpoint).
        setCanManageWebhooks(
          currentUserMember?.role === 'owner' ||
            currentUserMember?.permissions?.webhook_subscriptions?.write === true
        )
      }
    } catch (error) {
      if (currentWorkspaceId.current !== requested) return
      // The other sections still show, with the rights of a member who may not write.
      setMembersError(true)
      console.error(t`Failed to fetch workspace members`, error)
    } finally {
      if (currentWorkspaceId.current === requested) {
        setMembersLoadedFor(requested)
      }
    }
  }

  const handleWorkspaceUpdate = async (updatedWorkspace: Workspace) => {
    setWorkspace(updatedWorkspace)
    // Refresh the workspaces in auth context to stay in sync
    await refreshWorkspaces()
  }

  const handleWorkspaceDelete = async () => {
    navigate({ to: '/console' })
    await refreshWorkspaces()
  }

  const handleSectionChange = (newSection: SettingsSection) => {
    navigate({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId, section: newSection }
    })
  }

  const renderSection = () => {
    if (activeSection !== 'licence' && activeSection !== 'smtp-bridge' && !workspace) return <SettingsSkeleton />
    if (!membersLoaded && DEPENDS_ON_ROLE.has(activeSection)) return <SettingsSkeleton />
    switch (activeSection) {
      case 'team':
        return (
          <WorkspaceMembers
            workspaceId={workspaceId}
            members={members}
            loading={!membersLoaded}
            error={membersError}
            onMembersChange={fetchMembers}
            isOwner={isOwner}
            currentUserId={user?.id}
          />
        )
      case 'integrations':
        return (
          <Integrations
            workspace={workspace}
            loading={false}
            onSave={handleWorkspaceUpdate}
            isOwner={isOwner}
          />
        )
      case 'webhooks':
        return workspace ? (
          <WebhooksSettings
            workspaceId={workspace.id}
            canManage={canManageWebhooks}
            openWebhookId={search.webhook}
            onOpenWebhook={openWebhook}
          />
        ) : null
      case 'custom-fields':
        return (
          <CustomFieldsConfiguration
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            canManage={canManageCustomFields}
          />
        )
      case 'smtp-bridge':
        return <SMTPBridgeSettings />
      case 'general':
        return (
          <GeneralSettings
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            isOwner={isOwner}
          />
        )
      case 'blog':
        return (
          <BlogSettings
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            canManage={canManageBlog}
          />
        )
      case 'web-analytics':
        return (
          <WebAnalyticsSettings
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            canManage={canManageWebAnalytics}
          />
        )
      case 'audit-logs':
        return workspace && isOwner ? (
          <AuditLogsSettings workspace={workspace} isOwner={isOwner} onWorkspaceUpdate={handleWorkspaceUpdate} />
        ) : null
      case 'licence':
        // LicenseSettings renders its own non-root notice rather than nothing: a member who
        // followed the banner here needs to be told who can act, not shown a blank panel.
        return <LicenseSettings />
      case 'danger-zone':
        return workspace && isOwner ? (
          <DeleteWorkspaceSection workspace={workspace} onDeleteSuccess={handleWorkspaceDelete} />
        ) : null
      default:
        return null
    }
  }

  return (
    <RailLayout
      title={t`Settings`}
      rail={
        <SettingsSidebar
          activeSection={activeSection}
          onSectionChange={handleSectionChange}
          isOwner={isOwner}
          isRoot={isRootUser(user?.email)}
        />
      }
    >
      <div className="flex max-w-[900px] flex-col gap-3">{renderSection()}</div>
    </RailLayout>
  )
}
