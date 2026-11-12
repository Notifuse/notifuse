import { App } from 'antd'
import { useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { broadcastApi, type Broadcast } from '../../services/api/broadcast'

export type ConfirmedAction = 'pause' | 'resume' | 'cancel' | 'retryFailed'

const errorText = (error: unknown) => (error instanceof Error && error.message ? error.message : undefined)

/**
 * The lifecycle actions of a broadcast, with their current confirmations and messages. Each
 * success invalidates the broadcast, its stats, its task and every list of the workspace.
 */
export function useBroadcastActions(workspaceId: string) {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()

  const invalidate = (id: string) => {
    queryClient.invalidateQueries({ queryKey: ['broadcast', workspaceId, id] })
    queryClient.invalidateQueries({ queryKey: ['broadcast-stats', workspaceId, id] })
    queryClient.invalidateQueries({ queryKey: ['task', workspaceId, id] })
    queryClient.invalidateQueries({ queryKey: ['testResults', workspaceId, id] })
    queryClient.invalidateQueries({ queryKey: ['broadcast-tasks', workspaceId] })
    queryClient.invalidateQueries({ queryKey: ['broadcasts', workspaceId] })
  }

  const run = async (action: ConfirmedAction, broadcast: Broadcast) => {
    const params = { workspace_id: workspaceId, id: broadcast.id }
    const name = broadcast.name
    try {
      switch (action) {
        case 'pause':
          await broadcastApi.pause(params)
          message.success(t`Broadcast "${name}" paused successfully`)
          break
        case 'resume':
          await broadcastApi.resume(params)
          message.success(t`Broadcast "${name}" resumed successfully`)
          break
        case 'cancel':
          await broadcastApi.cancel(params)
          message.success(t`Broadcast "${name}" cancelled successfully`)
          break
        case 'retryFailed': {
          const { requeued } = await broadcastApi.retryFailed(params)
          message.success(
            t`${plural(requeued, {
              one: '# recipient queued for another attempt',
              other: '# recipients queued for another attempt'
            })}`
          )
          break
        }
      }
      invalidate(broadcast.id)
    } catch (error) {
      // The server names why, e.g. a chosen sender that left the integration.
      const reason = errorText(error)
      switch (action) {
        case 'pause':
          message.error(t`Failed to pause broadcast`)
          break
        case 'resume':
          message.error(reason ?? t`Failed to resume broadcast`)
          break
        case 'cancel':
          message.error(t`Failed to cancel broadcast`)
          break
        case 'retryFailed':
          message.error(reason ?? t`Failed to retry the recipients`)
          break
      }
    }
  }

  /** Asks the current confirmation, then runs the action. */
  const confirm = (action: ConfirmedAction, broadcast: Broadcast) => {
    const texts: Record<ConfirmedAction, { title: string; content: string; okText: string; cancelText: string }> = {
      pause: {
        title: t`Pause broadcast?`,
        content: t`The broadcast will stop sending and can be resumed later.`,
        okText: t`Yes, pause`,
        cancelText: t`Cancel`
      },
      resume: {
        title: t`Resume broadcast?`,
        content: t`The broadcast will continue sending from where it was paused.`,
        okText: t`Yes, resume`,
        cancelText: t`Cancel`
      },
      cancel: {
        title: t`Cancel broadcast?`,
        content: t`Queued emails will not be sent. In-flight sends will complete.`,
        okText: t`Yes, cancel`,
        cancelText: t`No`
      },
      retryFailed: {
        title: t`Retry the failed recipients?`,
        content: t`They will be sent the same email again. Fix whatever the provider objected to first, or they will fail a second time.`,
        okText: t`Yes, retry`,
        cancelText: t`Cancel`
      }
    }
    modal.confirm({ ...texts[action], onOk: () => run(action, broadcast) })
  }

  const selectWinner = (broadcast: Broadcast, templateId: string, templateName: string) => {
    modal.confirm({
      title: t`Pick as winner`,
      content: t`Are you sure you want to select "${templateName}" as the winner? The broadcast will be sent to the remaining recipients.`,
      okText: t`Yes`,
      cancelText: t`No`,
      onOk: async () => {
        try {
          await broadcastApi.selectWinner({ workspace_id: workspaceId, id: broadcast.id, template_id: templateId })
          message.success(t`Winner selected successfully! The broadcast will be sent to remaining recipients.`)
          invalidate(broadcast.id)
        } catch {
          message.error(t`Failed to select winner`)
        }
      }
    })
  }

  /** Deletes a draft; the caller confirms with DeleteBroadcastModal. */
  const remove = async (broadcast: Broadcast) => {
    await broadcastApi.delete({ workspace_id: workspaceId, id: broadcast.id })
    message.success(t`Broadcast "${broadcast.name}" deleted successfully`)
    // The detail of a deleted broadcast must not refetch into a 404 while it closes.
    queryClient.removeQueries({ queryKey: ['broadcast', workspaceId, broadcast.id] })
    queryClient.invalidateQueries({ queryKey: ['broadcasts', workspaceId] })
  }

  return { confirm, run, selectWinner, remove, invalidate }
}
