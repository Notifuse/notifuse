import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import {
  Bot,
  Eye,
  Flag,
  Globe,
  KeyRound,
  Layers,
  ListMinus,
  ListPlus,
  Mail,
  MailCheck,
  MailX,
  MousePointerClick,
  Play,
  Reply,
  Send,
  ShieldCheck,
  UserPen,
  UserPlus,
  UserX,
  Webhook,
  Zap
} from 'lucide-react'
import { Trans, useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import dayjs from '../../../lib/dayjs'
import type {
  AutomationEventEntityData,
  ContactListEntityData,
  ContactTimelineEntry,
  InboundWebhookEventEntityData,
  MessageHistoryEntityData
} from '../../../services/api/contact_timeline'
import type { Segment } from '../../../services/api/segment'
import type { Workspace } from '../../../services/api/types'
import { getProviderName } from '../../integrations/EmailProviders'
import { MESSAGE_ORIGIN_LABELS } from '../../logs/labels'
import { messageOrigin } from '../../logs/messageProvider'
import { segmentDotColor } from '../../segment/segmentColor'
import { changeNew, changeOld, webPageURL } from '../../timeline/webPageUrl'
import { LIST_STATUS_LABEL } from '../contactDisplay'
import { formatDuration } from '../visits/duration'
import { ActivityCode as Code, ActivityLink as LinkButton, ActivityStrong as Strong } from './activityParts'
import { useContactFieldLabel } from './contactFieldLabels'

export type ActivityTone = 'primary' | 'green' | 'amber' | 'gray' | 'red' | 'blue'

/** One entry of the activity feed: an icon, a one-line sentence, an optional detail, its source. */
export interface ActivityDescription {
  icon: LucideIcon
  tone: ActivityTone
  title: ReactNode
  meta?: ReactNode
  source: string
}

export interface ActivityDescriberOptions {
  workspace: Workspace
  segments: Segment[]
  timezone: string
  /** Opens the email an entry names, when the entry carries its template. */
  onPreviewEmail: (data: MessageHistoryEntityData) => void
  /** Opens the website page an entry names. */
  onPreviewPage: (url: string) => void
}

// Fields a contact update records that are not the contact's own data.
const SYSTEM_FIELDS = new Set(['created_at', 'updated_at', 'db_created_at', 'db_updated_at'])
// How many changed fields one profile entry spells out before « +N ».
const FIELDS_SHOWN = 3

const ISO_DATE = /^\d{4}-\d{2}-\d{2}T/

/** Describes timeline entries in the words of the activity feed. */
export function useActivityDescriber({
  workspace,
  segments,
  timezone,
  onPreviewEmail,
  onPreviewPage
}: ActivityDescriberOptions): (entry: ContactTimelineEntry) => ActivityDescription {
  const { t, i18n } = useLingui()
  const fieldLabel = useContactFieldLabel(workspace)

  const statusLabel = (status: string) =>
    LIST_STATUS_LABEL[status] ? i18n._(LIST_STATUS_LABEL[status]) : status

  // A value of a profile change, as written beside its old one: numbers with the separators of the
  // console's language and every decimal they carry, dates in the contact's timezone.
  const changedValue = (value: unknown): string => {
    if (value === null || value === undefined || value === '') return '—'
    if (typeof value === 'number') return value.toLocaleString(i18n.locale, { maximumFractionDigits: 20 })
    if (typeof value === 'string' && ISO_DATE.test(value)) return dayjs(value).tz(timezone).format('lll')
    if (typeof value === 'object') {
      const json = JSON.stringify(value)
      return json.length > 40 ? `${json.slice(0, 39)}…` : json
    }
    return String(value)
  }

  const emailName = (data?: MessageHistoryEntityData, fallback?: string): ReactNode => {
    const name = data?.template_name || data?.template_id || fallback || t`an email`
    return data?.template_email ? (
      <LinkButton label={name} onClick={() => onPreviewEmail(data)} />
    ) : (
      <Strong>{name}</Strong>
    )
  }

  const message = (entry: ContactTimelineEntry): ActivityDescription => {
    const data = entry.entity_data as MessageHistoryEntityData | undefined
    const name = emailName(data, entry.entity_id)
    // What sent it, as the Messages tab names it; an entry without it (written before the
    // timeline returned it) by its template's category.
    const origin = data
      ? messageOrigin({
          broadcast_id: data.broadcast_id ?? undefined,
          automation_id: data.automation_id ?? undefined,
          transactional_notification_id: data.transactional_notification_id ?? undefined
        })
      : 'other'
    const source =
      origin !== 'other'
        ? i18n._(MESSAGE_ORIGIN_LABELS[origin])
        : data?.template_category === 'transactional'
          ? t`Transactional`
          : t`Email`
    const changed = (key: string) => entry.changes?.[key] !== undefined
    if (entry.kind === 'email.link_clicked') {
      const url = changeNew<string>(entry, 'clicked_url')
      return {
        icon: MousePointerClick,
        tone: 'primary',
        title: <Trans>Clicked a link in {name}</Trans>,
        meta: url ? (
          <span className="block truncate" title={url}>
            {url}
          </span>
        ) : undefined,
        source
      }
    }
    if (changed('failed_at')) return { icon: MailX, tone: 'red', title: <Trans>Email failed: {name}</Trans>, source }
    if (changed('bounced_at')) return { icon: MailX, tone: 'red', title: <Trans>Email bounced: {name}</Trans>, source }
    if (changed('complained_at'))
      return { icon: MailX, tone: 'red', title: <Trans>Marked as spam: {name}</Trans>, source }
    if (changed('unsubscribed_at'))
      return { icon: MailX, tone: 'red', title: <Trans>Unsubscribed from {name}</Trans>, source }
    if (changed('clicked_at'))
      return { icon: MousePointerClick, tone: 'primary', title: <Trans>Clicked in {name}</Trans>, source }
    if (changed('opened_at')) return { icon: Eye, tone: 'primary', title: <Trans>Opened {name}</Trans>, source }
    if (changed('delivered_at'))
      return { icon: MailCheck, tone: 'green', title: <Trans>Email delivered: {name}</Trans>, source }
    if (entry.operation === 'insert') return { icon: Send, tone: 'gray', title: <Trans>Email sent: {name}</Trans>, source }
    return { icon: Mail, tone: 'gray', title: <Trans>Email updated: {name}</Trans>, source }
  }

  const list = (entry: ContactTimelineEntry): ActivityDescription => {
    const data = entry.entity_data as ContactListEntityData | undefined
    const name = <Strong>{data?.name || changeNew<string>(entry, 'list_id') || entry.entity_id || t`a list`}</Strong>
    const source = t`List`
    // What the entry recorded, never the subscription's current status (entity_data.status): a
    // removal keeps its status on the row, and a later change would rewrite every past entry.
    // The kind the trigger writes says it; rows without one fall back to their changes.
    const removed =
      entry.kind === 'list.removed' || entry.operation === 'delete' || !!changeNew<string>(entry, 'deleted_at')
    if (removed) return { icon: ListMinus, tone: 'gray', title: <Trans>Removed from {name}</Trans>, source }
    const raw = entry.changes?.status
    const status = changeNew<string>(entry, 'status') ?? (typeof raw === 'string' ? raw : undefined)
    const old = changeOld<string>(entry, 'status')
    const meta = old && status && old !== status ? `${statusLabel(old)} → ${statusLabel(status)}` : undefined
    const kind = entry.kind.startsWith('list.') ? entry.kind.slice('list.'.length) : undefined
    switch (kind ?? status) {
      case 'confirmed':
        return { icon: ShieldCheck, tone: 'green', title: <Trans>Confirmed their subscription to {name}</Trans>, meta, source }
      case 'resubscribed':
        return { icon: ListPlus, tone: 'green', title: <Trans>Subscribed again to {name}</Trans>, meta, source }
      case 'subscribed':
      case 'active':
        return old === 'pending'
          ? { icon: ShieldCheck, tone: 'green', title: <Trans>Confirmed their subscription to {name}</Trans>, meta, source }
          : { icon: ListPlus, tone: 'green', title: <Trans>Subscribed to {name}</Trans>, meta, source }
      case 'pending':
        return { icon: ListPlus, tone: 'amber', title: <Trans>Added to {name}, awaiting confirmation</Trans>, meta, source }
      case 'unsubscribed':
        return { icon: MailX, tone: 'red', title: <Trans>Unsubscribed from {name}</Trans>, meta, source }
      case 'bounced':
        return { icon: MailX, tone: 'red', title: <Trans>Bounced in {name}</Trans>, meta, source }
      case 'complained':
        return { icon: MailX, tone: 'red', title: <Trans>Complained in {name}</Trans>, meta, source }
      default:
        return meta
          ? { icon: ListPlus, tone: 'gray', title: <Trans>Subscription to {name} changed</Trans>, meta, source }
          : { icon: ListPlus, tone: 'gray', title: <Trans>Subscription to {name} updated</Trans>, source }
    }
  }

  const segment = (entry: ContactTimelineEntry): ActivityDescription => {
    const id = entry.entity_id || changeNew<string>(entry, 'segment_id') || ''
    const found = segments.find((item) => item.id === id)
    const chip = (
      <span className="inline-flex h-5 items-center gap-1 rounded-full border border-line bg-surface px-1.5 align-middle text-xs font-medium text-ink-2">
        <span
          aria-hidden="true"
          className="size-1.5 rounded-full"
          // The segment's own colour, chosen by the user: not a theme token.
          style={{ backgroundColor: segmentDotColor(found?.color) }}
        />
        {found?.name ?? id}
      </span>
    )
    return entry.kind === 'segment.left'
      ? { icon: Layers, tone: 'gray', title: <Trans>Left segment {chip}</Trans>, source: t`Segment` }
      : { icon: Layers, tone: 'blue', title: <Trans>Joined segment {chip}</Trans>, source: t`Segment` }
  }

  const contact = (entry: ContactTimelineEntry): ActivityDescription => {
    const source = t`Profile`
    if (entry.operation === 'insert') return { icon: UserPlus, tone: 'green', title: t`Contact created`, source }
    if (entry.operation === 'delete') return { icon: UserX, tone: 'red', title: t`Contact deleted`, source }
    const fields = Object.keys(entry.changes ?? {}).filter((key) => !SYSTEM_FIELDS.has(key))
    const shown = fields.slice(0, FIELDS_SHOWN)
    const more = fields.length - shown.length
    const diffs = (
      <>
        {shown.map((key, index) => {
          const raw = entry.changes[key]
          const enveloped = typeof raw === 'object' && raw !== null && ('old' in raw || 'new' in raw)
          const before = enveloped ? changeOld<unknown>(entry, key) : undefined
          const after = enveloped ? changeNew<unknown>(entry, key) : raw
          return (
            <span key={key}>
              {index > 0 ? ', ' : null}
              <Strong>{fieldLabel(key)}</Strong>{' '}
              {enveloped && before !== undefined ? (
                <>
                  <span className="text-ink-3 line-through decoration-gray-300">{changedValue(before)}</span>
                  {' → '}
                </>
              ) : null}
              <span className="font-semibold text-success-fg">{changedValue(after)}</span>
            </span>
          )
        })}
        {more > 0 ? <span className="text-ink-3">{`, +${more}`}</span> : null}
      </>
    )
    return {
      icon: UserPen,
      tone: 'gray',
      title: fields.length > 0 ? <Trans>Profile updated: {diffs}</Trans> : t`Profile updated`,
      source
    }
  }

  const customEvent = (entry: ContactTimelineEntry): ActivityDescription => {
    // The kind the trigger writes is 'custom_event.<name>'; the timeline joins no entity data for
    // custom events, so the name and the goal come from the kind and the changes.
    const eventName = changeNew<string>(entry, 'event_name') || entry.kind.replace(/^custom_event\./, '')
    const goalName = changeNew<string>(entry, 'goal_name')
    const goalValue = changeNew<number>(entry, 'goal_value')
    const value =
      goalValue !== undefined && goalValue !== null
        ? goalValue.toLocaleString(i18n.locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
        : undefined
    const code = <Code>{eventName}</Code>
    const metaParts = [
      goalName && goalName !== eventName ? t`Goal ${goalName}` : null,
      value ? t`Value ${value}` : null
    ].filter(Boolean)
    return {
      icon: Zap,
      tone: 'amber',
      title: entry.operation === 'update' ? <Trans>Event {code} updated</Trans> : <Trans>Event {code}</Trans>,
      meta: metaParts.length > 0 ? metaParts.join(' · ') : undefined,
      source: t`Event`
    }
  }

  const webhook = (entry: ContactTimelineEntry): ActivityDescription => {
    const data = entry.entity_data as InboundWebhookEventEntityData | undefined
    // The entry's own changes first: the join can match another webhook event of the same message.
    const type = changeNew<string>(entry, 'type') || data?.type || ''
    const provider = changeNew<string>(entry, 'source') || data?.source
    const source = provider ? getProviderName(provider) : t`Webhook`
    const name = data?.template_name ? <Strong>{data.template_name}</Strong> : null
    switch (type) {
      case 'delivered':
        return {
          icon: MailCheck,
          tone: 'green',
          title: name ? <Trans>Email delivered: {name}</Trans> : t`Email delivered`,
          source
        }
      case 'bounce':
        return {
          icon: MailX,
          tone: 'red',
          title: name ? <Trans>Email bounced: {name}</Trans> : t`Email bounced`,
          meta:
            ['bounce_type', 'bounce_category', 'bounce_diagnostic']
              .map((key) => changeNew<string>(entry, key) || data?.[key as 'bounce_type'])
              .filter(Boolean)
              .join(' · ') || undefined,
          source
        }
      case 'complaint':
        return {
          icon: MailX,
          tone: 'red',
          title: name ? <Trans>Marked as spam: {name}</Trans> : t`Marked as spam`,
          meta: changeNew<string>(entry, 'complaint_feedback_type') || data?.complaint_feedback_type || undefined,
          source
        }
      case 'unsubscribe':
        return {
          icon: MailX,
          tone: 'red',
          title: name ? <Trans>Unsubscribed from {name}</Trans> : t`Unsubscribed through the email provider`,
          source
        }
      case 'reply':
        return { icon: Reply, tone: 'gray', title: name ? <Trans>Replied to {name}</Trans> : t`Replied to an email`, source }
      case 'auto_reply':
        return { icon: Bot, tone: 'gray', title: name ? <Trans>Auto-reply to {name}</Trans> : t`Auto-reply to an email`, source }
      case 'auth_email':
        return { icon: KeyRound, tone: 'green', title: t`Authentication email sent via Supabase`, source }
      case 'before_user_created':
        return { icon: UserPlus, tone: 'green', title: t`User created and synced from Supabase`, source }
      default: {
        const kind = <Code>{type || entry.kind}</Code>
        return { icon: Webhook, tone: 'gray', title: <Trans>Webhook event {kind}</Trans>, source }
      }
    }
  }

  const webSession = (entry: ContactTimelineEntry): ActivityDescription => {
    const pageviews = changeNew<number>(entry, 'pageview_count')
    const durationMs = changeNew<number>(entry, 'duration_ms')
    const landing = changeNew<string>(entry, 'landing_path')
    const exit = changeNew<string>(entry, 'exit_path')
    const goals = changeNew<number>(entry, 'goal_count')
    const pages = pageviews !== undefined ? <Strong>{plural(pageviews, { one: '# page', other: '# pages' })}</Strong> : null
    // A real visit never reads « 0 s »: under a second is still a second.
    const duration = durationMs !== undefined && durationMs > 0 ? formatDuration(Math.max(1000, durationMs), i18n) : null
    const title =
      pages && duration ? (
        <Trans>
          Website visit: {pages}, {duration}
        </Trans>
      ) : pages ? (
        <Trans>Website visit: {pages}</Trans>
      ) : (
        t`Website visit`
      )
    // The UTM chain in the order a campaign is read: source, medium, campaign, content.
    const utm = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_content']
      .map((key) => changeNew<string>(entry, key))
      .filter(Boolean)
      .join(' / ')
    const path = [landing, exit && exit !== landing ? exit : null].filter(Boolean).join(' → ')
    const referrer = changeNew<string>(entry, 'referrer_domain')
    const meta = [
      path || null,
      referrer ? t`from ${referrer}` : null,
      utm || null,
      goals ? plural(goals, { one: '# goal', other: '# goals' }) : null,
      [changeNew<string>(entry, 'device'), changeNew<string>(entry, 'country')].filter(Boolean).join(', ') || null
    ].filter(Boolean)
    return {
      icon: Globe,
      tone: 'blue',
      title,
      // Truncated to one line, whole on hover.
      meta:
        meta.length > 0 ? (
          <span className="block truncate" title={meta.join(' · ')}>
            {meta.join(' · ')}
          </span>
        ) : undefined,
      source: t`Web analytics`
    }
  }

  const webPage = (entry: ContactTimelineEntry): ActivityDescription => {
    const path = changeNew<string>(entry, 'path') || t`an unknown page`
    const url = webPageURL(entry, workspace)
    const page = url ? (
      <LinkButton label={path} onClick={() => onPreviewPage(url)} />
    ) : (
      <Strong>{path}</Strong>
    )
    const durationMs = changeNew<number>(entry, 'duration_ms')
    const scroll = changeNew<number>(entry, 'max_scroll')
    const meta = [
      durationMs !== undefined && durationMs > 0 ? formatDuration(Math.max(1000, durationMs), i18n) : null,
      scroll ? t`${scroll}% scrolled` : null,
      changeNew<boolean>(entry, 'is_landing') ? t`entry page` : null,
      changeNew<boolean>(entry, 'is_exit') ? t`exit page` : null
    ].filter(Boolean)
    return {
      icon: Eye,
      tone: 'blue',
      title: <Trans>Viewed {page}</Trans>,
      meta: meta.length > 0 ? meta.join(' · ') : undefined,
      source: t`Web analytics`
    }
  }

  const automation = (entry: ContactTimelineEntry): ActivityDescription => {
    const data = entry.entity_data as AutomationEventEntityData | undefined
    const name = <Strong>{data?.name || entry.entity_id || t`an automation`}</Strong>
    const source = t`Automation`
    if (entry.kind === 'automation.start') {
      return { icon: Play, tone: 'amber', title: <Trans>Entered the automation {name}</Trans>, source }
    }
    const reason = changeNew<string>(entry, 'exit_reason')
    if (reason === 'failed') return { icon: Flag, tone: 'red', title: <Trans>Failed in the automation {name}</Trans>, source }
    if (reason && reason !== 'completed') {
      return { icon: Flag, tone: 'gray', title: <Trans>Left the automation {name}</Trans>, meta: <Code>{reason}</Code>, source }
    }
    return { icon: Flag, tone: 'green', title: <Trans>Completed the automation {name}</Trans>, source }
  }

  return (entry) => {
    switch (entry.entity_type) {
      case 'message_history':
        return message(entry)
      case 'contact_list':
        return list(entry)
      case 'contact_segment':
        return segment(entry)
      case 'contact':
        return contact(entry)
      case 'custom_event':
        return customEvent(entry)
      case 'inbound_webhook_event':
        return webhook(entry)
      case 'web_session':
        return webSession(entry)
      case 'web_page':
        return webPage(entry)
      case 'automation':
        return automation(entry)
      default: {
        const kind = <Code>{entry.kind}</Code>
        return { icon: Zap, tone: 'gray', title: <Trans>Activity {kind}</Trans>, source: entry.entity_type }
      }
    }
  }
}
