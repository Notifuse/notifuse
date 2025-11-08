import { useState, useEffect } from 'react'
import { useParams, useNavigate } from '@tanstack/react-router'
import { Layout } from 'antd'
import { workspaceService } from '../services/api/workspace'
import { Workspace, WorkspaceMember } from '../services/api/types'
import { WorkspaceMembers } from '../components/settings/WorkspaceMembers'
import { GeneralSettings } from '../components/settings/GeneralSettings'
import { SMTPRelaySettings } from '../components/settings/SMTPRelaySettings'
import { Integrations } from '../components/settings/Integrations'
import { CustomFieldsConfiguration } from '../components/settings/CustomFieldsConfiguration'
import { WebPublicationSettings } from '../components/settings/WebPublicationSettings'
import { useAuth } from '../contexts/AuthContext'
import { DeleteWorkspaceSection } from '../components/settings/DeleteWorkspace'
import { SettingsSidebar, SettingsSection } from '../components/settings/SettingsSidebar'

const { Sider, Content } = Layout

export function WorkspaceSettingsPage() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/settings' })
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [members, setMembers] = useState<WorkspaceMember[]>([])
  const [loadingMembers, setLoadingMembers] = useState(false)
  const [isOwner, setIsOwner] = useState(false)
  const [activeSection, setActiveSection] = useState<SettingsSection>('team')
  const { refreshWorkspaces, user, workspaces } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    // Find the workspace from the auth context
    const currentWorkspace = workspaces.find((w) => w.id === workspaceId) || null
    setWorkspace(currentWorkspace)

    fetchMembers()
  }, [workspaceId, workspaces])

  const fetchMembers = async () => {
    setLoadingMembers(true)
    try {
      const response = await workspaceService.getMembers(workspaceId)
      setMembers(response.members)

      // Check if current user is an owner
      if (user) {
        const currentUserMember = response.members.find((member) => member.user_id === user.id)
        setIsOwner(currentUserMember?.role === 'owner')
      }
    } catch (error) {
      console.error('Failed to fetch workspace members', error)
    } finally {
      setLoadingMembers(false)
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

  const renderSection = () => {
    switch (activeSection) {
      case 'team':
        return (
          <WorkspaceMembers
            workspaceId={workspaceId}
            members={members}
            loading={loadingMembers}
            onMembersChange={fetchMembers}
            isOwner={isOwner}
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
      case 'custom-fields':
        return (
          <CustomFieldsConfiguration
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            isOwner={isOwner}
          />
        )
      case 'smtp-relay':
        return <SMTPRelaySettings />
      case 'general':
        return (
          <GeneralSettings
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            isOwner={isOwner}
          />
        )
      case 'web-publications':
        return (
          <WebPublicationSettings
            workspace={workspace}
            onWorkspaceUpdate={handleWorkspaceUpdate}
            isOwner={isOwner}
          />
        )
      case 'danger-zone':
        return workspace && isOwner ? (
          <DeleteWorkspaceSection workspace={workspace} onDeleteSuccess={handleWorkspaceDelete} />
        ) : null
      default:
        return null
    }
  }

  return (
    <Layout
      style={{
        background: '#fff',
        minHeight: 'calc(100vh - 48px)'
      }}
    >
      <Sider
        width={250}
        style={{
          background: '#fff',
          borderRight: '1px solid #f0f0f0',
          overflow: 'auto'
        }}
      >
        <SettingsSidebar
          activeSection={activeSection}
          onSectionChange={setActiveSection}
          isOwner={isOwner}
        />
      </Sider>
      <Layout style={{ background: '#fff' }}>
        <Content>
          <div style={{ maxWidth: '700px', padding: '24px' }}>{renderSection()}</div>
        </Content>
      </Layout>
    </Layout>
  )
}
