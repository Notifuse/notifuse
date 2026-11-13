import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Empty, Result, Segmented, Skeleton, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useNavigate } from '@tanstack/react-router'
import {
  Ban,
  CircleCheck,
  CircleX,
  CornerUpLeft,
  Eye,
  Flag,
  MousePointerClick,
  Send,
  UserRound
} from 'lucide-react'
import dayjs from '../../lib/dayjs'
import { useAuth } from '../../contexts/AuthContext'
import { ApiError } from '../../services/api/errors'
import { listMessages, type MessageHistory } from '../../services/api/messages_history'
import { templatesApi, type Template } from '../../services/api/template'
import type { UserPermissions, Workspace } from '../../services/api/types'
import { SUPPORTED_LANGUAGES } from '../../lib/languages'
import { messageStatus } from '../../lib/messageStatus'
import { compileTemplatePreview } from '../templates/compileTemplatePreview'
import type { CompileTemplateResponse } from '../../services/api/template'
import { KeyValueList, Mono, Pill, SidePanel, type DrawerNav } from '../layout'
import { CodeBlock } from './CodeBlock'
import { MESSAGE_STATUS_LABELS } from './labels'
import { formatLogDateTime } from './logTime'
import { ERASED_EMAIL, messageOrigin } from './messageProvider'
import { messageLanguage, resolveEmail } from './messageTemplate'

export interface MessageNames {
  broadcast: (id: string) => string | undefined
  automation: (id: string) => string | undefined
  list: (id: string) => string | undefined
}

export interface MessageDetailProps {
  workspaceId: string
  /** The member's rights, as the Logs page loaded them. */
  permissions: UserPermissions | null
  /** The open message, from the URL. */
  id?: string
  /** The loaded row of that message, shown while it loads. */
  row?: MessageHistory
  names: MessageNames
  nav?: DrawerNav
  onClose: () => void
}

const isErased = (message: MessageHistory) =>
  message.contact_email === ERASED_EMAIL ||
  Object.keys(message.message_data?.data ?? {}).length === 0

/** Logs → Messages: one message, opened by `message` in the URL. */
export function MessageDetail({
  workspaceId,
  permissions,
  id,
  row,
  names,
  nav,
  onClose
}: MessageDetailProps) {
  const { t } = useLingui()

  // D1: by id and without a period, so a copied link opens the message wherever it is.
  const messageQuery = useQuery({
    queryKey: ['logs', 'message', workspaceId, id],
    queryFn: async () => (await listMessages(workspaceId, { id, limit: 1 })).messages[0] ?? null,
    enabled: id !== undefined,
    placeholderData: row,
    retry: false
  })

  if (id === undefined) return <SidePanel open={false} onClose={onClose} title="" />

  const message = messageQuery.data
  if (!message) {
    return (
      <SidePanel open onClose={onClose} title={t`Message`} copyLink={false} nav={nav}>
        {messageQuery.isPending ? (
          <Skeleton active paragraph={{ rows: 6 }} />
        ) : messageQuery.isError ? (
          <Result
            status="error"
            title={t`Couldn't load the message`}
            subTitle={messageQuery.error instanceof Error ? messageQuery.error.message : undefined}
            extra={<Button onClick={() => void messageQuery.refetch()}>{t`Retry`}</Button>}
          />
        ) : (
          <div className="py-10">
            <Empty description={t`Message not found`}>
              <Button onClick={onClose}>{t`Close`}</Button>
            </Empty>
          </div>
        )}
      </SidePanel>
    )
  }

  return (
    <LoadedMessage
      workspaceId={workspaceId}
      permissions={permissions}
      message={message}
      names={names}
      nav={nav}
      onClose={onClose}
    />
  )
}

interface LoadedMessageProps {
  workspaceId: string
  permissions: UserPermissions | null
  message: MessageHistory
  names: MessageNames
  nav?: DrawerNav
  onClose: () => void
}

function LoadedMessage({
  workspaceId,
  permissions,
  message,
  names,
  nav,
  onClose
}: LoadedMessageProps) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { workspaces } = useAuth()
  const workspace = workspaces.find((candidate) => candidate.id === workspaceId)
  const timezone = workspace?.settings.timezone
  const canReadTemplates = Boolean(permissions?.templates?.read)
  const canReadContacts = Boolean(permissions?.contacts?.read)
  // The tab shown, back to Events for each message; the preview compiles once "Sent content" was
  // opened, and the subject then comes from it. Reset here rather than by remounting the panel, so
  // J and K keep it and the focus on its header buttons; only its body is remounted (bodyKey),
  // which scrolls it back to the top and drops a focus held inside it.
  const [view, setView] = useState({ id: message.id, tab: 'events', previewWanted: false })
  if (view.id !== message.id) setView({ id: message.id, tab: 'events', previewWanted: false })
  const tab = view.id === message.id ? view.tab : 'events'
  const previewWanted = view.id === message.id && view.previewWanted

  // D2: the template at the version sent.
  const templateQuery = useQuery({
    queryKey: ['template', workspaceId, message.template_id, message.template_version],
    queryFn: async () => {
      const response = await templatesApi.get({
        workspace_id: workspaceId,
        id: message.template_id,
        version: message.template_version
      })
      if (!response.template) throw new Error('Failed to load template')
      return response.template
    },
    enabled: canReadTemplates,
    staleTime: 60 * 60 * 1000,
    retry: (count, error) => !(error instanceof ApiError && error.status === 404) && count < 1
  })
  const template = templateQuery.data
  const templateMissing =
    templateQuery.error instanceof ApiError && templateQuery.error.status === 404

  // D3: the template compiled again with the message's data, in the language it was sent in. An
  // erased message has no data, and is not compiled.
  const erased = isErased(message)
  const data = message.message_data?.data ?? {}
  // The recorded pixel URL is live and signed: rendering it would record an open (as the web
  // version does, it is blanked).
  const previewData = { ...data, tracking_opens_url: '' }
  const language = messageLanguage(data)
  const email = template
    ? resolveEmail(template, language, workspace?.settings?.default_language ?? '')
    : undefined
  const preview = useQuery<CompileTemplateResponse>({
    queryKey: [
      'message-preview',
      workspaceId,
      message.template_id,
      message.template_version,
      language ?? null,
      message.id
    ],
    queryFn: () => compileTemplatePreview({ workspace: workspace!, email, testData: previewData }),
    enabled: previewWanted && Boolean(workspace) && Boolean(template) && !erased,
    staleTime: 5 * 60_000,
    retry: false
  })

  const origin = messageOrigin(message)
  const broadcastName = message.broadcast_id ? names.broadcast(message.broadcast_id) : undefined
  const automationName = message.automation_id ? names.automation(message.automation_id) : undefined
  const title: ReactNode =
    origin === 'broadcast' ? (
      (broadcastName ?? <Mono>{message.broadcast_id}</Mono>)
    ) : origin === 'automation' ? (
      (automationName ?? <Mono>{message.automation_id}</Mono>)
    ) : origin === 'transactional' ? (
      <Mono>{message.transactional_notification_id}</Mono>
    ) : (
      <Mono>{message.template_id}</Mono>
    )
  const status = messageStatus(message)
  const recipient = message.contact_email

  const at = (value: string) => {
    const date = timezone ? dayjs(value).tz(timezone) : dayjs(value)
    const today = timezone ? dayjs().tz(timezone) : dayjs()
    if (date.isSame(today, 'day')) {
      const time = date.format('HH:mm:ss')
      return t`today, ${time}`
    }
    return formatLogDateTime(value, timezone)
  }

  const templateLabel = template
    ? template.deleted_at
      ? t`${template.name} · deleted`
      : template.name
    : message.template_id

  const tabs = {
    activeKey: tab,
    onChange: (key: string) =>
      setView((current) => ({
        ...current,
        tab: key,
        previewWanted: current.previewWanted || key === 'content'
      })),
    items: [
      {
        key: 'events',
        label: t`Events`,
        children: (
          <div className="flex flex-col gap-4">
            <Timeline
              message={message}
              timezone={timezone}
              createdTitle={
                origin === 'broadcast'
                  ? t`Sent by the newsletter ${broadcastName ?? message.broadcast_id ?? ''}`
                  : origin === 'automation'
                    ? t`Sent by the automation ${automationName ?? message.automation_id ?? ''}`
                    : origin === 'transactional'
                      ? t`Sent by the transactional notification ${message.transactional_notification_id ?? ''}`
                      : t`Created`
              }
              createdDetail={t`Template ${templateLabel} · v${message.template_version}`}
            />
            <MessageFields
              message={message}
              workspaceId={workspaceId}
              workspace={workspace}
              template={canReadTemplates ? template : undefined}
              email={canReadTemplates ? email : undefined}
              language={language}
              permissions={permissions}
              templateMissing={templateMissing}
              canReadTemplates={canReadTemplates}
              compiledSubject={
                preview.data && !preview.data.error ? preview.data.subject : undefined
              }
              compiledPreview={
                preview.data && !preview.data.error ? preview.data.subject_preview : undefined
              }
              names={names}
            />
          </div>
        )
      },
      {
        key: 'content',
        label: t`Sent content`,
        children: (
          <SentContent
            canReadTemplates={canReadTemplates}
            erased={erased}
            templateMissing={templateMissing}
            templateError={
              templateQuery.isError && !templateMissing
                ? templateQuery.error instanceof Error
                  ? templateQuery.error.message
                  : ''
                : undefined
            }
            templateLoading={templateQuery.isPending}
            preview={preview}
          />
        )
      },
      {
        key: 'data',
        label: t`Data`,
        children: <MessageData message={message} />
      }
    ]
  }

  const contactBlocked = !canReadContacts || message.contact_email === ERASED_EMAIL
  const openContact = (
    <Button
      icon={<UserRound size={14} />}
      disabled={contactBlocked}
      onClick={() =>
        void navigate({
          to: '/console/workspace/$workspaceId/contacts',
          params: { workspaceId },
          search: { contact: message.contact_email }
        })
      }
    >
      {t`Open contact`}
    </Button>
  )

  return (
    <SidePanel
      open
      onClose={onClose}
      title={title}
      status={<Pill tone={status.tone}>{i18n._(MESSAGE_STATUS_LABELS[status.key])}</Pill>}
      meta={[
        recipient === ERASED_EMAIL ? t`Contact erased` : t`To ${recipient}`,
        at(message.created_at)
      ]}
      nav={nav}
      tabs={tabs}
      bodyKey={message.id}
      footer={
        !canReadContacts ? (
          <Tooltip title={t`You need access to contacts`}>
            <span>{openContact}</span>
          </Tooltip>
        ) : (
          openContact
        )
      }
    />
  )
}

type TimelineTone = 'neutral' | 'danger' | 'success' | 'warning' | 'info' | 'primary'
const TIMELINE_TONES: Record<TimelineTone, string> = {
  neutral: 'bg-neutral-bg text-neutral-fg',
  danger: 'bg-danger-bg text-danger-fg',
  success: 'bg-success-bg text-success-fg',
  warning: 'bg-warning-bg text-warning-fg',
  info: 'bg-info-bg text-info-fg',
  primary: 'bg-primary-100 text-primary-700'
}

interface TimelineProps {
  message: MessageHistory
  timezone?: string
  createdTitle: string
  createdDetail: string
}

/** One line per event of the message, the oldest first. */
function Timeline({ message, timezone, createdTitle, createdDetail }: TimelineProps) {
  const { t } = useLingui()
  // In the order of the spec's table: events of the same instant keep it.
  const candidates: {
    key: string
    at?: string
    title: string
    detail?: string
    tone: TimelineTone
    icon: typeof Send
  }[] = [
    {
      key: 'created',
      at: message.created_at,
      title: createdTitle,
      detail: createdDetail,
      tone: 'neutral',
      icon: Send
    },
    {
      key: 'failed',
      at: message.failed_at,
      title: t`Failed`,
      detail: message.status_info,
      tone: 'danger',
      icon: CircleX
    },
    {
      key: 'delivered',
      at: message.delivered_at,
      title: t`Delivered`,
      tone: 'success',
      icon: CircleCheck
    },
    {
      key: 'bounced',
      at: message.bounced_at,
      title: t`Bounced`,
      detail: message.status_info,
      tone: 'warning',
      icon: CornerUpLeft
    },
    {
      key: 'complained',
      at: message.complained_at,
      title: t`Marked as spam`,
      tone: 'danger',
      icon: Flag
    },
    {
      key: 'opened',
      at: message.opened_at,
      title: t`Opened`,
      detail: t`First open`,
      tone: 'info',
      icon: Eye
    },
    {
      key: 'clicked',
      at: message.clicked_at,
      title: t`Clicked`,
      detail: t`First click`,
      tone: 'primary',
      icon: MousePointerClick
    },
    {
      key: 'unsubscribed',
      at: message.unsubscribed_at,
      title: t`Unsubscribed`,
      tone: 'warning',
      icon: Ban
    }
  ]
  const events = candidates
    .map((event, order) => ({ ...event, order }))
    .filter((event): event is typeof event & { at: string } => Boolean(event.at))
    .sort((a, b) => new Date(a.at).getTime() - new Date(b.at).getTime() || a.order - b.order)

  const zoned = (value: string) => (timezone ? dayjs(value).tz(timezone) : dayjs(value))
  const createdDay = zoned(message.created_at)

  return (
    <ol data-unreset className="m-0 flex list-none flex-col gap-3 p-0" aria-label={t`Events`}>
      {events.map((event) => {
        const Icon = event.icon
        const time = zoned(event.at)
        return (
          <li key={event.key} data-event={event.key} className="flex items-start gap-3">
            <span className="w-20 shrink-0 pt-0.5 text-xs text-ink-3 tabular-nums">
              {time.isSame(createdDay, 'day')
                ? time.format('HH:mm:ss')
                : time.format('D MMM, HH:mm:ss')}
            </span>
            <span
              aria-hidden="true"
              className={`flex size-[22px] shrink-0 items-center justify-center rounded-full ${TIMELINE_TONES[event.tone]}`}
            >
              <Icon size={12} />
            </span>
            <span className="flex min-w-0 flex-col">
              <span className="text-sm text-ink">{event.title}</span>
              {event.detail ? (
                <span className="text-xs break-words text-ink-3">{event.detail}</span>
              ) : null}
            </span>
          </li>
        )
      })}
    </ol>
  )
}

interface MessageFieldsProps {
  message: MessageHistory
  permissions: UserPermissions | null
  /** The template's email in the message's language. */
  email?: Template['email']
  language?: string
  workspaceId: string
  workspace?: Workspace
  template?: Template
  templateMissing: boolean
  canReadTemplates: boolean
  compiledSubject?: string
  compiledPreview?: string
  names: MessageNames
}

/**
 * The From address as the send most likely picked it: the email's sender, else the default one, of
 * the provider that sent it. A notification goes through the transactional provider; a newsletter
 * or an automation through the marketing one. A message of no origin is a double opt-in
 * confirmation (marketing provider, opt-in template) or a template test, whose provider is read as
 * the test dialog's default: the marketing one for a marketing template, the transactional one
 * otherwise, and the first email integration when that default is not set or no longer exists. An
 * unknown template (no templates.read, still loading, deleted) counts as marketing. Not recorded on
 * the message, so not shown here: the integration picked in a test dialog and its sender (a test
 * goes out from the dialog's sender, never the template's, so the address shown for a test is a
 * guess), an automation email node's own integration, a newsletter variation's sender, and a
 * sender_id passed to a transactional send.
 */
function senderOf(
  workspace: Workspace | undefined,
  message: MessageHistory,
  template: Template | undefined,
  email: Template['email'] | undefined
) {
  const origin = messageOrigin(message)
  const marketingTemplate = template?.category === 'marketing' || template?.category === 'opt_in'
  const transactional =
    origin === 'transactional' ||
    (origin === 'other' && template !== undefined && !marketingTemplate)
  const providerId = transactional
    ? workspace?.settings?.transactional_email_provider_id
    : workspace?.settings?.marketing_email_provider_id
  const integrations = workspace?.integrations ?? []
  const integration =
    integrations.find((candidate) => candidate.id === providerId) ??
    (origin === 'other'
      ? integrations.find(
          (candidate) => candidate.type === 'email' && candidate.email_provider?.kind
        )
      : undefined)
  const provider = integration?.email_provider
  return (
    provider?.senders.find((sender) => sender.id === email?.sender_id) ??
    provider?.senders.find((sender) => sender.is_default)
  )
}

function MessageFields({
  message,
  permissions,
  email,
  language,
  workspaceId,
  workspace,
  template,
  templateMissing,
  canReadTemplates,
  compiledSubject,
  compiledPreview,
  names
}: MessageFieldsProps) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const options = message.channel_options

  const sender = senderOf(workspace, message, template, email)
  const fromName = options?.from_name ?? sender?.name
  const from =
    fromName || sender?.email ? `${fromName ?? ''} <${sender?.email ?? '—'}>`.trim() : undefined

  const forced = (value: string | undefined) =>
    value ? (
      <span className="flex flex-wrap items-baseline gap-2">
        <span className="break-words">{value}</span>
        <span className="text-xs text-ink-3">{t`Set at send time`}</span>
      </span>
    ) : undefined
  const subject =
    forced(options?.subject) ?? (canReadTemplates ? (compiledSubject ?? email?.subject) : undefined)
  const subjectPreview =
    forced(options?.subject_preview) ??
    (canReadTemplates ? (compiledPreview ?? email?.subject_preview) : undefined)

  const tracked = (value?: boolean) => (value === false ? t`off` : t`on`)
  const tracking =
    message.opens_tracked === undefined && message.clicks_tracked === undefined
      ? t`On (default)`
      : t`Opens ${tracked(message.opens_tracked)} · clicks ${tracked(message.clicks_tracked)}`

  const templateValue =
    template && !templateMissing ? (
      <span className="flex items-center gap-2">
        <span>{template.deleted_at ? t`${template.name} · deleted` : template.name}</span>
        <span className="text-xs text-ink-3">v{message.template_version}</span>
      </span>
    ) : (
      <span className="flex items-center gap-2">
        <Mono>{message.template_id}</Mono>
        <span className="text-xs text-ink-3">v{message.template_version}</span>
      </span>
    )

  const origin = messageOrigin(message)
  let sentBy: ReactNode
  if (origin === 'broadcast' && message.broadcast_id) {
    const id = message.broadcast_id
    sentBy = (
      <SourceLink
        label={names.broadcast(id) ?? id}
        enabled={Boolean(permissions?.broadcasts?.read)}
        onOpen={() =>
          void navigate({
            to: '/console/workspace/$workspaceId/broadcasts',
            params: { workspaceId },
            search: { broadcast: id }
          })
        }
      />
    )
  } else if (origin === 'automation' && message.automation_id) {
    const id = message.automation_id
    sentBy = (
      <SourceLink
        label={names.automation(id) ?? id}
        enabled={Boolean(permissions?.automations?.read)}
        onOpen={() =>
          void navigate({
            to: '/console/workspace/$workspaceId/automations',
            params: { workspaceId },
            search: { automation: id }
          })
        }
      />
    )
  } else if (origin === 'transactional' && message.transactional_notification_id) {
    const id = message.transactional_notification_id
    sentBy = (
      <SourceLink
        label={id}
        enabled={Boolean(permissions?.transactional?.read)}
        onOpen={() =>
          void navigate({
            to: '/console/workspace/$workspaceId/transactional-notifications',
            params: { workspaceId },
            search: { notification: id }
          })
        }
      />
    )
  }

  const listId = message.list_id
  const attachments = message.attachments ?? []

  return (
    <section className="flex flex-col gap-2">
      <h4 data-unreset className="m-0 text-sm font-semibold text-ink">
        {t`Message`}
      </h4>
      <KeyValueList
        items={[
          { label: t`Message ID`, value: <Mono copy>{message.id}</Mono> },
          {
            label: t`External ID`,
            value: message.external_id ? <Mono copy>{message.external_id}</Mono> : '—'
          },
          { label: t`Template`, value: templateValue },
          { label: t`From`, value: from },
          ...(subject ? [{ label: t`Subject`, value: subject }] : []),
          ...(subjectPreview ? [{ label: t`Subject preview`, value: subjectPreview }] : []),
          ...(language
            ? [{ label: t`Language`, value: SUPPORTED_LANGUAGES[language] ?? language }]
            : []),
          ...(options?.reply_to ? [{ label: t`Reply-To`, value: options.reply_to }] : []),
          ...(options?.cc?.length ? [{ label: t`CC`, value: options.cc.join(', ') }] : []),
          ...(options?.bcc?.length ? [{ label: t`BCC`, value: options.bcc.join(', ') }] : []),
          { label: t`Tracking`, value: tracking },
          ...(listId
            ? [{ label: t`List`, value: names.list(listId) ?? <Mono>{listId}</Mono> }]
            : []),
          ...(attachments.length
            ? [
                {
                  label: t`Attachments`,
                  value: (
                    <span className="flex flex-col">
                      {attachments.map((attachment) => (
                        <span key={attachment.checksum + attachment.filename}>
                          {attachment.filename}{' '}
                          <span className="text-xs text-ink-3">{attachment.content_type}</span>
                        </span>
                      ))}
                    </span>
                  )
                }
              ]
            : []),
          ...(sentBy ? [{ label: t`Sent by`, value: sentBy }] : [])
        ]}
      />
    </section>
  )
}

function SourceLink({
  label,
  enabled,
  onOpen
}: {
  label: string
  enabled: boolean
  onOpen: () => void
}) {
  if (!enabled) return <span>{label}</span>
  return (
    <a
      href="#"
      onClick={(event) => {
        event.preventDefault()
        onOpen()
      }}
    >
      {label}
    </a>
  )
}

interface SentContentProps {
  canReadTemplates: boolean
  erased: boolean
  templateMissing: boolean
  /** Why the template could not be loaded, other than a 404; undefined when it loaded. */
  templateError?: string
  templateLoading: boolean
  preview: {
    data?: CompileTemplateResponse
    isLoading: boolean
    isError: boolean
    error: unknown
  }
}

function SentContent({
  canReadTemplates,
  erased,
  templateMissing,
  templateError,
  templateLoading,
  preview
}: SentContentProps) {
  const { t } = useLingui()
  const [view, setView] = useState<'html' | 'text' | 'mjml'>('html')
  const note = (text: string) => (
    <p data-unreset className="m-0 text-sm text-ink-3">
      {text}
    </p>
  )

  if (!canReadTemplates) return note(t`You need access to templates`)
  if (erased) return note(t`The contact was erased; the message data is gone`)
  if (templateMissing) return note(t`The template behind this message isn't available`)
  if (templateError !== undefined) {
    return (
      <Alert
        type="error"
        showIcon
        title={t`Couldn't load the template`}
        description={templateError || undefined}
      />
    )
  }
  if (templateLoading || preview.isLoading) return <Skeleton active paragraph={{ rows: 6 }} />

  const failure =
    preview.data?.error?.message ??
    (preview.error instanceof Error ? preview.error.message : undefined)
  if (preview.isError || preview.data?.error) {
    return (
      <Alert type="error" showIcon title={t`Couldn't render this message`} description={failure} />
    )
  }
  const compiled = preview.data
  if (!compiled) return null

  return (
    <div className="flex flex-col gap-3">
      {note(
        t`Rendered again from the template and the message data; Notifuse doesn't store the sent HTML`
      )}
      <Segmented<'html' | 'text' | 'mjml'>
        size="small"
        value={view}
        onChange={setView}
        options={[
          { label: t`HTML`, value: 'html' },
          { label: t`Text version`, value: 'text' },
          { label: t`MJML Source`, value: 'mjml' }
        ]}
      />
      {view === 'html' ? (
        <iframe
          title={t`Sent content`}
          srcDoc={compiled.html}
          sandbox=""
          className="h-[600px] w-full rounded-md border border-line-soft bg-surface"
        />
      ) : view === 'text' ? (
        compiled.text ? (
          <CodeBlock value={compiled.text} label={t`text`} />
        ) : (
          note(t`This email is sent as HTML only`)
        )
      ) : (
        <CodeBlock value={compiled.mjml} label={t`MJML`} />
      )}
    </div>
  )
}

function MessageData({ message }: { message: MessageHistory }) {
  const { t } = useLingui()
  const data = message.message_data?.data ?? {}
  const metadata = message.message_data?.metadata ?? {}
  const hasData = Object.keys(data).length > 0
  const hasMetadata = Object.keys(metadata).length > 0
  if (!hasData && !hasMetadata) {
    return (
      <div className="py-10">
        <Empty description={t`No data`} />
      </div>
    )
  }
  return (
    <div className="flex flex-col gap-4">
      {hasData ? (
        <section className="flex flex-col gap-2">
          <h4 data-unreset className="m-0 text-sm font-semibold text-ink">
            {t`Template data`}
          </h4>
          <CodeBlock value={data} label={t`template data`} />
        </section>
      ) : null}
      {hasMetadata ? (
        <section className="flex flex-col gap-2">
          <h4 data-unreset className="m-0 text-sm font-semibold text-ink">
            {t`Metadata`}
          </h4>
          <CodeBlock value={metadata} label={t`metadata`} />
        </section>
      ) : null}
    </div>
  )
}
