import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { App, Button, Table, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Mail } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import dayjs from '../../../lib/dayjs'
import { automationApi } from '../../../services/api/automation'
import { broadcastApi, type Broadcast } from '../../../services/api/broadcast'
import type { MessageHistory } from '../../../services/api/messages_history'
import { templatesApi, type Template } from '../../../services/api/template'
import { transactionalNotificationsApi } from '../../../services/api/transactional_notifications'
import type { UserPermissions, Workspace } from '../../../services/api/types'
import { useNumberFormat } from '../../layout'
import { MESSAGE_ORIGIN_LABELS } from '../../logs/labels'
import { messageLanguage, resolveEmail } from '../../logs/messageTemplate'
import { messageOrigin, rowIsSmtp } from '../../logs/messageProvider'
import TemplatePreviewDrawer from '../../templates/TemplatePreviewDrawer'
import { BlockError } from '../BlockError'
import { MessageJourneySteps } from './MessageJourneySteps'
import { useDayLabel } from './sheetDates'
import { useContactMessages } from './useContactSheetQueries'

const FIVE_MINUTES = 5 * 60 * 1000

export interface ContactMessagesTabProps {
  workspace: Workspace
  email: string
  permissions: UserPermissions | null
  /** The contact's timezone, else the workspace's. */
  timezone: string
  /** Every message of the contact, when counted: « 9 of 48 messages ». */
  total?: number
}

/** The template a message was sent with, at its version; shared with the other previews. */
function useMessageTemplate(workspace: Workspace, message: MessageHistory, enabled: boolean) {
  const { t } = useLingui()
  return useQuery({
    queryKey: ['template', workspace.id, message.template_id, message.template_version],
    queryFn: async () => {
      const response = await templatesApi.get({
        workspace_id: workspace.id,
        id: message.template_id,
        version: message.template_version
      })
      if (!response.template) throw new Error(t`The server returned no template`)
      return response.template
    },
    enabled: enabled && !!message.template_id,
    staleTime: 60 * 60 * 1000,
    retry: 1
  })
}

/**
 * The email a message was sent with: the template's content in the language the contact received
 * it in, as the server picked it (the translation, else the default).
 */
function sentEmail(workspace: Workspace, message: MessageHistory, template: Template) {
  const email = resolveEmail(
    template,
    messageLanguage(message.message_data?.data),
    workspace.settings?.default_language ?? ''
  )
  return email ? { ...template, email } : template
}

/**
 * The subject the contact received: the one the send forced, else the template's in the language
 * sent, rendered with the message's data.
 */
function useRenderedSubject(workspace: Workspace, message: MessageHistory, template?: Template) {
  const forced = message.channel_options?.subject
  const subject = forced || (template ? sentEmail(workspace, message, template).email?.subject : undefined)
  return useQuery({
    queryKey: ['message-subject', message.id, subject],
    queryFn: async () => {
      if (!subject || !subject.includes('{')) return subject ?? ''
      // Loaded on demand: the Liquid engine is only needed here and in the editors.
      const { renderSecureLiquid } = await import('../../../utils/liquidConfig')
      const { html, errors } = await renderSecureLiquid(subject, message.message_data?.data ?? {})
      return errors.length > 0 ? subject : html
    },
    enabled: !!subject,
    staleTime: Infinity
  })
}

interface MessageCellProps {
  workspace: Workspace
  message: MessageHistory
  name?: string
  variant?: string
  canReadTemplates: boolean
}

function MessageCell({ workspace, message, name, variant, canReadTemplates }: MessageCellProps) {
  const { t } = useLingui()
  const template = useMessageTemplate(workspace, message, canReadTemplates)
  const subject = useRenderedSubject(workspace, message, template.data)
  const title = name || template.data?.name || message.template_id
  return (
    <div className="flex min-w-0 flex-col leading-tight">
      <span className="truncate font-medium text-ink">{title}</span>
      {/* The variant opens the second line: beside the name it left « Wee… » of it. */}
      {variant || subject.data ? (
        <span className="flex min-w-0 items-center gap-1.5">
          {variant ? (
            <span className="shrink-0 rounded-sm bg-fill px-1.5 text-2xs leading-4.5 font-medium text-ink-3">
              {t`Variant ${variant}`}
            </span>
          ) : null}
          {subject.data ? <span className="truncate text-xs text-ink-3">{subject.data}</span> : null}
        </span>
      ) : null}
    </div>
  )
}

/**
 * The email a message was sent with, in the preview drawer, once its template is loaded. A
 * template that cannot be loaded (deleted version, server error) says so instead of opening
 * nothing.
 */
function MessagePreview({
  workspace,
  message,
  onLoaded,
  onClose
}: {
  workspace: Workspace
  message: MessageHistory
  /** The template is in: the drawer opens. */
  onLoaded: () => void
  onClose: () => void
}) {
  const { t } = useLingui()
  const { message: toast } = App.useApp()
  const template = useMessageTemplate(workspace, message, true)
  const failed = template.isError || (template.isSuccess && !template.data)
  useEffect(() => {
    if (!failed) return
    toast.error(t`Could not load the email sent`)
    onClose()
  }, [failed, toast, t, onClose])
  const loaded = !!template.data
  useEffect(() => {
    if (loaded) onLoaded()
  }, [loaded, onLoaded])
  if (!template.data) return null
  return (
    <TemplatePreviewDrawer
      open
      onClose={onClose}
      record={sentEmail(workspace, message, template.data)}
      workspace={workspace}
      templateData={message.message_data?.data ?? {}}
      messageHistory={message}
    />
  )
}

/** The broadcast's A/B variation a message was sent from, by its template. */
function variationOf(broadcast: Broadcast | undefined, message: MessageHistory): string | undefined {
  const settings = broadcast?.test_settings
  if (!settings?.enabled || (settings.variations?.length ?? 0) < 2) return undefined
  return settings.variations.find((variation) => variation.template_id === message.template_id)?.variation_name
}

/** Every message sent to the contact: what it was, its journey and when it was sent. */
export function ContactMessagesTab({ workspace, email, permissions, timezone, total }: ContactMessagesTabProps) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()
  const dayLabel = useDayLabel(timezone)
  const messages = useContactMessages(workspace.id, email, true)
  const workspaceId = workspace.id

  // The message whose email is previewed: opened from its row or from its icon.
  const [previewing, setPreviewing] = useState<MessageHistory | null>(null)
  const [previewLoaded, setPreviewLoaded] = useState(false)
  const openPreview = (message: MessageHistory) => {
    setPreviewLoaded(false)
    setPreviewing(message)
  }
  const closePreview = useCallback(() => setPreviewing(null), [])
  const markPreviewLoaded = useCallback(() => setPreviewLoaded(true), [])
  const canPreview = (message: MessageHistory) => !!permissions?.templates?.read && !!message.template_id

  const canReadBroadcasts = !!permissions?.broadcasts?.read
  const canReadAutomations = !!permissions?.automations?.read
  const canReadTransactional = !!permissions?.transactional?.read
  const canReadTemplates = !!permissions?.templates?.read

  // The names of what sent the messages, on the same keys as the pages that list them.
  const broadcastsQuery = useQuery({
    queryKey: ['broadcasts', workspaceId],
    queryFn: () => broadcastApi.list({ workspace_id: workspaceId, limit: 1000 }),
    enabled: canReadBroadcasts,
    staleTime: FIVE_MINUTES
  })
  const automationsQuery = useQuery({
    queryKey: ['automations', workspaceId],
    queryFn: () => automationApi.list({ workspace_id: workspaceId, with_journey_counts: true }),
    enabled: canReadAutomations,
    staleTime: FIVE_MINUTES
  })
  const notificationsQuery = useQuery({
    queryKey: ['transactional-notifications', workspaceId],
    queryFn: () => transactionalNotificationsApi.list({ workspace_id: workspaceId }),
    enabled: canReadTransactional,
    staleTime: FIVE_MINUTES
  })
  const broadcasts = useMemo(
    () => new Map((broadcastsQuery.data?.broadcasts ?? []).map((item) => [item.id, item])),
    [broadcastsQuery.data]
  )
  const automations = useMemo(
    () => new Map((automationsQuery.data?.automations ?? []).map((item) => [item.id, item.name])),
    [automationsQuery.data]
  )
  const notifications = useMemo(
    () => new Map((notificationsQuery.data?.notifications ?? []).map((item) => [item.id, item.name])),
    [notificationsQuery.data]
  )

  if (messages.isError && !messages.data) return <BlockError onRetry={() => messages.refetch()} />

  const pages = messages.data?.pages ?? []
  const rows = pages.flatMap((page) => page.messages ?? [])

  const nameOf = (message: MessageHistory): string | undefined => {
    if (message.broadcast_id) return broadcasts.get(message.broadcast_id)?.name
    if (message.automation_id) return automations.get(message.automation_id)
    if (message.transactional_notification_id) {
      return notifications.get(message.transactional_notification_id) ?? message.transactional_notification_id
    }
    return undefined
  }

  // The day and time, or only the day with its year when the label carries one (an earlier year,
  // before yesterday): the tooltip has the time.
  const sentLabel = (message: MessageHistory) => {
    const at = message.sent_at || message.created_at
    const local = dayjs(at).tz(timezone)
    const now = dayjs().tz(timezone)
    const withYear = local.year() !== now.year() && local.isBefore(now.subtract(1, 'day').startOf('day'))
    return withYear ? dayLabel(at) : `${dayLabel(at)}, ${local.format('LT')}`
  }

  const columns: ColumnsType<MessageHistory> = [
    {
      title: t`Message`,
      key: 'message',
      render: (_, message) => (
        <MessageCell
          workspace={workspace}
          message={message}
          name={nameOf(message)}
          variant={message.broadcast_id ? variationOf(broadcasts.get(message.broadcast_id), message) : undefined}
          canReadTemplates={canReadTemplates}
        />
      )
    },
    {
      title: t`Type`,
      key: 'type',
      width: 112,
      render: (_, message) => (
        <span className="text-xs whitespace-nowrap text-ink-2">
          {i18n._(MESSAGE_ORIGIN_LABELS[messageOrigin(message)])}
        </span>
      )
    },
    {
      title: t`Journey`,
      key: 'journey',
      width: 252,
      render: (_, message) => (
        <MessageJourneySteps message={message} smtp={rowIsSmtp(message, workspace)} timezone={timezone} />
      )
    },
    {
      title: t`Sent`,
      key: 'sent',
      width: 132,
      align: 'right',
      render: (_, message) => {
        const full = dayjs(message.sent_at || message.created_at)
          .tz(timezone)
          .format('LLLL')
        return (
          <Tooltip title={t`${full} (${timezone})`}>
            <span className="text-xs whitespace-nowrap text-ink-3 tabular-nums">{sentLabel(message)}</span>
          </Tooltip>
        )
      }
    },
    {
      key: 'actions',
      width: 44,
      align: 'right',
      render: (_, message) =>
        canPreview(message) ? (
          <Tooltip title={t`View the sent email`}>
            <Button
              type="text"
              aria-label={t`View the sent email`}
              // While its template loads, before the drawer opens.
              loading={previewing?.id === message.id && !previewLoaded}
              icon={<Mail size={14} aria-hidden="true" />}
              onClick={(event) => {
                event.stopPropagation()
                openPreview(message)
              }}
            />
          </Tooltip>
        ) : null
    }
  ]

  const shown = rows.length
  const footer =
    total !== undefined && total >= shown
      ? plural(total, { one: `${int(shown)} of # message`, other: `${int(shown)} of # messages` })
      : plural(shown, { one: '# message', other: '# messages' })

  return (
    <div className="flex flex-col">
      <Table<MessageHistory>
        rowKey="id"
        size="small"
        tableLayout="fixed"
        // Below this width the table scrolls in its column rather than squeezing the message to nothing.
        scroll={{ x: 656 }}
        columns={columns}
        dataSource={rows}
        loading={messages.isLoading}
        pagination={false}
        locale={{ emptyText: t`No message sent to this contact yet` }}
        // The whole row opens the email sent, as the icon at its end does.
        rowClassName={(message) => (canPreview(message) ? 'cursor-pointer' : '')}
        onRow={(message) =>
          canPreview(message)
            ? {
                onClick: (event) => {
                  // A click in the preview drawer bubbles here through the portal.
                  if (!event.currentTarget.contains(event.target as Node)) return
                  openPreview(message)
                }
              }
            : {}
        }
      />
      {previewing ? (
        <MessagePreview
          key={previewing.id}
          workspace={workspace}
          message={previewing}
          onLoaded={markPreviewLoaded}
          onClose={closePreview}
        />
      ) : null}
      {rows.length > 0 ? (
        <div className="flex items-center justify-between gap-3 border-t border-line-soft px-1 pt-2.5">
          <span className="text-xs text-ink-3">{footer}</span>
          {messages.hasNextPage ? (
            <Button size="small" onClick={() => messages.fetchNextPage()} loading={messages.isFetchingNextPage}>
              {t`Load more`}
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
