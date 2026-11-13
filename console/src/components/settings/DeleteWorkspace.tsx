import { useState } from 'react'
import { Button, Input, App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { Mono, Panel } from '../layout'

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
      <SettingsSectionHeader title={t`Danger zone`} description={t`This action cannot be undone.`} />

      <Panel title={t`Delete this workspace`} tone="danger">
        <div className="flex flex-col items-start gap-3">
          <p className="m-0 text-sm text-ink-2">
            {t`This will permanently delete the workspace "${workspace.name}" and all of its data. To confirm, please enter the workspace ID:`}{' '}
            <Mono copy>{workspace.id}</Mono>
          </p>

          <div className="w-full max-w-[420px]">
            <Input
              value={confirmWorkspaceId}
              onChange={(e) => setConfirmWorkspaceId(e.target.value)}
              placeholder={t`Enter workspace ID`}
              aria-label={t`Workspace ID`}
            />
          </div>

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
      </Panel>
    </>
  )
}
