import { App } from 'antd'
import { useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { automationApi } from '../../../services/api/automation'
import { ApiError } from '../../../services/api/errors'

/** Activate, pause and delete, shared by the list and the editor, with today's messages. */
export function useAutomationActions(workspaceId: string) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()

  // Activate and pause answer without the automation: the cached one is read again.
  const refresh = (id: string) => {
    void queryClient.invalidateQueries({ queryKey: ['automations', workspaceId] })
    void queryClient.invalidateQueries({ queryKey: ['automation', workspaceId, id] })
  }

  const activate = async (id: string): Promise<boolean> => {
    try {
      await automationApi.activate({ workspace_id: workspaceId, automation_id: id })
      message.success(t`Automation activated successfully`)
      return true
    } catch (error) {
      // A rejected trigger configuration comes back as a 400 carrying the database's own
      // explanation: the only thing that tells the user which condition to change.
      const reason = error instanceof ApiError && error.status === 400 ? error.message : ''
      message.error(reason || t`Failed to activate automation`)
      return false
    } finally {
      refresh(id)
    }
  }

  const pause = async (id: string): Promise<boolean> => {
    try {
      await automationApi.pause({ workspace_id: workspaceId, automation_id: id })
      message.success(t`Automation paused successfully`)
      return true
    } catch {
      message.error(t`Failed to pause automation`)
      return false
    } finally {
      refresh(id)
    }
  }

  const remove = async (id: string): Promise<boolean> => {
    try {
      await automationApi.delete({ workspace_id: workspaceId, automation_id: id })
      message.success(t`Automation deleted successfully`)
      // Removed rather than invalidated: a refetch would answer 404 while the editor closes.
      queryClient.removeQueries({ queryKey: ['automation', workspaceId, id] })
      queryClient.removeQueries({ queryKey: ['automation-node-stats', workspaceId, id] })
      void queryClient.invalidateQueries({ queryKey: ['automations', workspaceId] })
      return true
    } catch {
      message.error(t`Failed to delete automation`)
      return false
    }
  }

  return { activate, pause, remove }
}
