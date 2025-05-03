import { useState } from 'react'
import { Button, Modal, Input, Space, Typography, App } from 'antd'
import { ExclamationCircleOutlined } from '@ant-design/icons'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { Section } from './Section'

const { Text } = Typography

interface DeleteWorkspaceSectionProps {
  workspace: Workspace
  onDeleteSuccess: () => void
}

export function DeleteWorkspaceSection({
  workspace,
  onDeleteSuccess
}: DeleteWorkspaceSectionProps) {
  const [deletingWorkspace, setDeletingWorkspace] = useState(false)
  const [confirmWorkspaceId, setConfirmWorkspaceId] = useState('')
  const { message } = App.useApp()

  const handleDeleteWorkspace = async () => {
    if (confirmWorkspaceId !== workspace.id) {
      message.error('Workspace ID does not match')
      return
    }

    setDeletingWorkspace(true)
    try {
      await workspaceService.delete({ id: workspace.id })
      message.success('Workspace deleted successfully')
      onDeleteSuccess()
    } catch (error) {
      console.error('Failed to delete workspace', error)
      message.error('Failed to delete workspace')
    } finally {
      setDeletingWorkspace(false)
    }
  }

  return (
    <>
      <Section title="Delete Workspace" description="This action cannot be undone.">
        <div>
          <p style={{ marginTop: 16 }}>
            This will permanently delete the workspace "{workspace.name}" and all of its data. To
            confirm, please enter the workspace ID: <Text code>{workspace.id}</Text>
          </p>

          <Input
            value={confirmWorkspaceId}
            onChange={(e) => setConfirmWorkspaceId(e.target.value)}
            placeholder="Enter workspace ID"
            style={{ marginBottom: 16 }}
          />

          <Button
            danger
            type="primary"
            loading={deletingWorkspace}
            disabled={confirmWorkspaceId !== workspace.id}
            onClick={handleDeleteWorkspace}
          >
            Delete Workspace
          </Button>
        </div>
      </Section>
    </>
  )
}
