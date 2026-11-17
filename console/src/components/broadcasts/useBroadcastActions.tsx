import { App } from 'antd'
import { useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { broadcastApi, type Broadcast } from '../../services/api/broadcast'
import { ApiError } from '../../services/api/errors'

export type ConfirmedAction = 'pause' | 'resume' | 'cancel' | 'unschedule' | 'retryFailed'

/** An action of a broadcast's menus: a confirmed action, or the choice between back to draft and cancel. */
export type BroadcastAction = ConfirmedAction | 'cancelOrDraft'

const errorText = (error: unknown) => (error instanceof Error && error.message ? error.message : undefined)

// A resume refused because a variation's chosen sender is unknown to the marketing integration, or there is no
// such integration: the server's English validation error (400) names a sender
const refusedForSender = (error: unknown) =>
  error instanceof ApiError && error.status === 400 && /\bsender\b/i.test(error.message)

// Back to draft refused because the sending has started, though the console saw no start yet
// (no start date, no progress read): the server's English validation error (400) says so.
const refusedAsStarted = (error: unknown) =>
  error instanceof ApiError && error.status === 400 && /started sending/i.test(error.message)

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
        case 'unschedule':
          await broadcastApi.unschedule(params)
          message.success(t`Broadcast "${name}" is back to draft`)
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
      // The server names why a retry failed; a resume refused for its sender gets a sentence of its own
      const reason = errorText(error)
      switch (action) {
        case 'pause':
          message.error(t`Failed to pause broadcast`)
          break
        case 'resume':
          message.error(
            refusedForSender(error)
              ? t`A variation's sender is no longer one of the marketing integration's senders. Choose its sender again, then resume.`
              : t`Failed to resume broadcast`
          )
          break
        case 'cancel':
          message.error(t`Failed to cancel broadcast`)
          break
        case 'unschedule':
          message.error(
            refusedAsStarted(error)
              ? t`This broadcast has started sending, so it can no longer go back to draft. Cancel it instead.`
              : t`Failed to send the broadcast back to draft`
          )
          // Read again: a broadcast found started shows its start date or progress, and drops Back to draft.
          invalidate(broadcast.id)
          break
        case 'retryFailed':
          message.error(reason ?? t`Failed to retry the recipients`)
          break
      }
    }
  }

  /**
   * Cancel, for a scheduled broadcast that has not started sending: it can still go back to draft,
   * which is offered first, as the usual way out. Choosing is the confirmation.
   */
  const chooseCancel = (broadcast: Broadcast) => {
    const choose = (action: 'unschedule' | 'cancel') => {
      instance.destroy()
      void run(action, broadcast)
    }
    // The two ways out are the buttons themselves: no footer. Closing (the cross, Escape, a click
    // outside) keeps the broadcast scheduled.
    const instance = modal.confirm({
      title: t`Cancel the scheduled send?`,
      icon: null,
      closable: true,
      mask: { closable: true },
      footer: null,
      content: (
        <div className="mt-3 flex flex-col gap-2">
          <button
            type="button"
            onClick={() => choose('unschedule')}
            className="flex w-full cursor-pointer flex-col gap-0.5 rounded-lg border border-primary-300 bg-primary-50 px-3.5 py-3 text-left transition-colors hover:border-primary-500 focus-visible:outline-2 focus-visible:outline-primary-500"
          >
            <span className="font-semibold text-primary-700">{t`Back to draft`}</span>
            <span className="text-sm text-ink-2">
              {t`It is not sent at its scheduled time. You can edit it and schedule it again; its date is kept.`}
            </span>
          </button>
          <button
            type="button"
            onClick={() => choose('cancel')}
            className="flex w-full cursor-pointer flex-col gap-0.5 rounded-lg border border-line bg-surface px-3.5 py-3 text-left transition-colors hover:border-danger-fg hover:bg-danger-bg focus-visible:outline-2 focus-visible:outline-danger-fg"
          >
            <span className="font-semibold text-danger-fg">{t`Cancel broadcast`}</span>
            <span className="text-sm text-ink-2">{t`It will never be sent. A cancelled broadcast can't be resumed.`}</span>
          </button>
        </div>
      )
    })
  }

  /** Asks the current confirmation, then runs the action; offers the choice for cancelOrDraft. */
  const confirm = (action: BroadcastAction, broadcast: Broadcast) => {
    if (action === 'cancelOrDraft') {
      chooseCancel(broadcast)
      return
    }
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
      unschedule: {
        title: t`Back to draft?`,
        content: t`The broadcast will not be sent at its scheduled time. It becomes a draft you can edit and schedule again, with its date kept.`,
        okText: t`Yes, back to draft`,
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
      title: t({ message: 'Pick as winner', context: 'newsletter A/B' }),
      content: t({
        message: `Are you sure you want to select "${templateName}" as the winner? The broadcast will be sent to the remaining recipients.`,
        context: 'newsletter A/B'
      }),
      okText: t`Yes`,
      cancelText: t`No`,
      onOk: async () => {
        try {
          await broadcastApi.selectWinner({ workspace_id: workspaceId, id: broadcast.id, template_id: templateId })
          message.success(
            t({
              message: 'Winner selected successfully! The broadcast will be sent to remaining recipients.',
              context: 'newsletter A/B'
            })
          )
          invalidate(broadcast.id)
        } catch {
          message.error(t({ message: 'Failed to select winner', context: 'newsletter A/B' }))
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
