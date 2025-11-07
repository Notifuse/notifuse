import { useState, useEffect } from 'react'
import { useParams, useNavigate } from '@tanstack/react-router'
import { workspaceService } from '../services/api/workspace'
import { Workspace, WorkspaceMember } from '../services/api/types'
import { WorkspaceMembers } from '../components/settings/WorkspaceMembers'
import { WorkspaceSettings } from '../components/settings/WorkspaceSettings'
import { Integrations } from '../components/settings/Integrations'
import { CustomFieldsConfiguration } from '../components/settings/CustomFieldsConfiguration'
import { useAuth } from '../contexts/AuthContext'
import { DeleteWorkspaceSection } from '../components/settings/DeleteWorkspace'

export function WorkspaceSettingsPage() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/settings' })
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [members, setMembers] = useState<WorkspaceMember[]>([])
  const [loadingMembers, setLoadingMembers] = useState(false)
  const [isOwner, setIsOwner] = useState(false)
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
    navigate({ to: '/console/' })
    await refreshWorkspaces()
  }

  return (
    <div className="max-w-[1200px] p-6">
      <WorkspaceMembers
        workspaceId={workspaceId}
        members={members}
        loading={loadingMembers}
        onMembersChange={fetchMembers}
        isOwner={isOwner}
      />
      <Integrations
        workspace={workspace}
        loading={false}
        onSave={handleWorkspaceUpdate}
        isOwner={isOwner}
      />
      <CustomFieldsConfiguration
        workspace={workspace}
        onWorkspaceUpdate={handleWorkspaceUpdate}
        isOwner={isOwner}
      />
      <WorkspaceSettings
        workspace={workspace}
        loading={false}
        onWorkspaceUpdate={handleWorkspaceUpdate}
        onWorkspaceDelete={handleWorkspaceDelete}
        isOwner={isOwner}
      />

      {workspace && isOwner && (
        <DeleteWorkspaceSection workspace={workspace} onDeleteSuccess={handleWorkspaceDelete} />
      )}
    </div>
  )
}
