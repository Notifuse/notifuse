import { App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { useQueryClient } from '@tanstack/react-query'
import { useNumberFormat } from '../layout'
import {
  transactionalNotificationsApi,
  type TransactionalNotification
} from '../../services/api/transactional_notifications'
import type { UserPermissions, Workspace } from '../../services/api/types'
import { canEditTransactionalNotification, canSendTestEmail } from '../../services/api/permissions'
import type { TemplateState } from './PreviewPanel'

/** The search of the message log, filtered on one notification (`extra`: a KPI's flag). */
export function logsSearch(notificationId: string, extra: Record<string, string | boolean> = {}) {
  return { tab: 'messages', transactional_notification_id: notificationId, ...extra }
}

export function templateIdOf(notification: Pick<TransactionalNotification, 'channels'>): string | undefined {
  return notification.channels?.email?.template_id || undefined
}

/** Why an action is disabled, or undefined when it is allowed. */
export interface ActionRules {
  test?: string
  edit?: string
  preview?: string
  logs?: string
  remove?: string
}

/** The rules of spec 7: actions without the right are shown disabled, with the missing right. */
export function useActionRules() {
  const { t } = useLingui()
  return (
    notification: TransactionalNotification,
    permissions: UserPermissions | null,
    template: TemplateState | undefined,
    workspace: Workspace
  ): ActionRules => {
    const write = Boolean(permissions?.transactional?.write)
    const integration = notification.integration_id
      ? workspace.integrations?.find((item) => item.id === notification.integration_id)
      : undefined
    const writeMissing = t`You need Transactional write permission`
    return {
      test: !canSendTestEmail(permissions)
        ? t`You need Templates read and Transactional write permissions to send test emails`
        : !templateIdOf(notification)
          ? t`This notification has no email template`
          : template?.status === 'notFound'
            ? t`Template not found`
            : undefined,
      edit: canEditTransactionalNotification(permissions, notification)
        ? undefined
        : write
          ? t`You need Templates read permission`
          : writeMissing,
      preview: !permissions?.templates?.read
        ? t`You need access to templates`
        : template?.status === 'notFound'
          ? t`Template not found`
          : template?.status !== 'ok'
            ? ''
            : undefined,
      logs: permissions?.message_history?.read ? undefined : t`You need access to message history`,
      remove: !write
        ? writeMissing
        : notification.integration_id
          ? integration
            ? t`Managed by ${integration.name}: it can't be deleted`
            : t`Managed by an integration: it can't be deleted`
          : undefined
    }
  }
}

/** Asks, deletes, says so, and refreshes the list; `onDeleted` closes the sheet. */
export function useDeleteNotification(workspaceId: string, onDeleted?: (id: string) => void) {
  const { t } = useLingui()
  const { modal, message } = App.useApp()
  const queryClient = useQueryClient()
  return (notification: TransactionalNotification) => {
    modal.confirm({
      title: t`Delete the notification?`,
      content: t`This cannot be undone.`,
      okText: t`Yes, Delete`,
      okButtonProps: { danger: true },
      cancelText: t`Cancel`,
      onOk: async () => {
        try {
          await transactionalNotificationsApi.delete({ workspace_id: workspaceId, id: notification.id })
          message.success(t`Transactional notification deleted successfully`)
          onDeleted?.(notification.id)
          await queryClient.invalidateQueries({ queryKey: ['transactional-notifications', workspaceId] })
        } catch (error) {
          console.error('Failed to delete notification:', error)
          message.error(t`Failed to delete notification`)
        }
      }
    })
  }
}

/** « {n} notifications », or « {n} of {total} notifications » while the search filters. */
export function useNotificationCount() {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  return (shown: number, total: number) => {
    const all = int(total)
    if (shown === total) return t`${plural(total, { one: `${all} notification`, other: `${all} notifications` })}`
    const part = int(shown)
    return t`${plural(total, { one: `${part} of ${all} notification`, other: `${part} of ${all} notifications` })}`
  }
}
