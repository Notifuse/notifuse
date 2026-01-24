import { useState } from 'react'
import { Button, Input, Typography, App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { SettingsSectionHeader } from './SettingsSectionHeader'

const { Text } = Typography

interface DeleteWorkspaceSectionProps {
  workspace: Workspace
  onDeleteSuccess: () => void
}

export function DeleteWorkspaceSection({
  workspace,
  onDeleteSuccess
}: DeleteWorkspaceSectionProps) {
  const { t } = useLingui()
  const [deletingWorkspace, setDeletingWorkspace] = useState(false)
  const [confirmWorkspaceId, setConfirmWorkspaceId] = useState('')
  const { message } = App.useApp()

  const handleDeleteWorkspace = async () => {
    if (confirmWorkspaceId !== workspace.id) {
      message.error(t`Workspace ID does not match`)
      return
    }

    setDeletingWorkspace(true)
    try {
      await workspaceService.delete({ id: workspace.id })
      message.success(t`Workspace deleted successfully`)
      onDeleteSuccess()
    } catch (error) {
      console.error('Failed to delete workspace', error)
      message.error(t`Failed to delete workspace`)
    } finally {
      setDeletingWorkspace(false)
    }
  }

  return (
    <>
      <SettingsSectionHeader title={t`Danger Zone`} description={t`This action cannot be undone.`} />

      <div>
        <p style={{ marginTop: 16 }}>
          {t`This will permanently delete the workspace "${workspace.name}" and all of its data. To confirm, please enter the workspace ID:`} <Text code>{workspace.id}</Text>
        </p>

        <Input
          value={confirmWorkspaceId}
          onChange={(e) => setConfirmWorkspaceId(e.target.value)}
          placeholder={t`Enter workspace ID`}
          style={{ marginBottom: 16 }}
        />

        <Button
          danger
          type="primary"
          loading={deletingWorkspace}
          disabled={confirmWorkspaceId !== workspace.id}
          onClick={handleDeleteWorkspace}
        >
          {t`Delete Workspace`}
        </Button>
      </div>
    </>
  )
}
