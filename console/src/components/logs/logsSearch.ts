/**
 * Search of the `/logs` route. The router's default parser runs JSON.parse on each value, so
 * `?is_failed=true` arrives as a boolean, `?template_id=2024` as a number and a string the page
 * once wrote as 'true' as the text "true". The route's `validateSearch` normalises it, and each tab
 * reads it through `parseLogsSearch` again: the router keeps the raw value of a key the parser
 * dropped. The old Messages and Incoming webhooks tabs still read the location themselves.
 */

import { searchString } from '../../lib/searchParams'
import { isFullEmail } from '../../lib/email'

export const LOGS_TABS = ['messages', 'incoming-webhooks', 'outgoing-webhooks', 'audit'] as const
export type LogsTab = (typeof LOGS_TABS)[number]

// Messages and incoming webhooks: a rolling window up to now.
export const LIVE_PERIODS = ['1h', '24h', '7d', '30d', 'all'] as const
export type LivePeriod = (typeof LIVE_PERIODS)[number]

export const AUDIT_PERIODS = ['24h', '7d', '30d', '90d', 'all', 'custom'] as const
export type AuditPeriod = (typeof AUDIT_PERIODS)[number]

export type LogsPeriod = LivePeriod | AuditPeriod

export const MESSAGE_ORIGINS = ['broadcast', 'automation', 'transactional', 'other'] as const
export type MessageOrigin = (typeof MESSAGE_ORIGINS)[number]

export const MESSAGE_CHANNELS = ['email', 'sms', 'push'] as const
export type MessageChannel = (typeof MESSAGE_CHANNELS)[number]

// Kept here rather than derived from the API types: the list is what the URL may carry.
export const WEBHOOK_SOURCES = [
  'ses',
  'postmark',
  'mailgun',
  'sparkpost',
  'mailjet',
  'smtp',
  'sendgrid',
  'supabase'
] as const
export type LogsWebhookSource = (typeof WEBHOOK_SOURCES)[number]

export const INBOUND_EVENT_TYPES = [
  'delivered',
  'bounce',
  'complaint',
  'unsubscribe',
  'reply',
  'auto_reply',
  'auth_email',
  'before_user_created'
] as const
export type InboundEventType = (typeof INBOUND_EVENT_TYPES)[number]

export const DELIVERY_STATUSES = ['pending', 'delivering', 'delivered', 'failed'] as const
export type DeliveryStatus = (typeof DELIVERY_STATUSES)[number]

export const AUDIT_OUTCOMES = ['success', 'failure', 'denied'] as const
export const AUDIT_SCOPES = ['workspace', 'deployment'] as const
export const AUDIT_ACTOR_TYPES = ['user', 'api_key', 'system', 'anonymous'] as const

export const MESSAGE_FLAGS = [
  'is_delivered',
  'is_failed',
  'is_opened',
  'is_clicked',
  'is_bounced',
  'is_complained',
  'is_unsubscribed'
] as const
export type MessageFlag = (typeof MESSAGE_FLAGS)[number]

const TEXT_KEYS = [
  // Messages
  'contact_email',
  'id',
  'external_id',
  'list_id',
  'template_id',
  'broadcast_id',
  'automation_id',
  'transactional_notification_id',
  'clicked_url',
  'message',
  // Incoming webhooks
  'recipient_email',
  'message_id',
  // Outgoing webhooks
  'subscription_id',
  // Audit log (`id` above is also the open entry)
  'from',
  'to',
  'actor_id',
  'actor_email',
  'categories',
  'actions',
  'target_id',
  'ip_address'
] as const
type TextKey = (typeof TEXT_KEYS)[number]

export type LogsSearch = {
  tab?: LogsTab
  period?: LogsPeriod
  origin?: MessageOrigin
  channel?: MessageChannel
  source?: LogsWebhookSource
  event_type?: InboundEventType
  status?: DeliveryStatus
  outcome?: (typeof AUDIT_OUTCOMES)[number]
  scope?: (typeof AUDIT_SCOPES)[number]
  actor_type?: (typeof AUDIT_ACTOR_TYPES)[number]
} & { [K in MessageFlag]?: boolean } & { [K in TextKey]?: string }

function oneOf<T extends string>(list: readonly T[], value: unknown): T | undefined {
  const text = searchString(value)
  return text !== undefined && (list as readonly string[]).includes(text) ? (text as T) : undefined
}

function flag(value: unknown): boolean | undefined {
  const single = Array.isArray(value) ? value[0] : value
  if (single === true || single === 'true') return true
  if (single === false || single === 'false') return false
  return undefined
}

const RFC3339 =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/

/** A time Go's `time.Parse(time.RFC3339, …)` reads: the shape, and each field in its range. */
function isRfc3339(value: string): boolean {
  const match = RFC3339.exec(value)
  if (!match) return false
  const [year, month, day, hour, minute, second] = match.slice(1, 7).map(Number)
  const [offsetHour, offsetMinute] = [match[7], match[8]].map((part) => Number(part ?? 0))
  const daysInMonth = new Date(Date.UTC(year, month, 0)).getUTCDate()
  return (
    month >= 1 &&
    month <= 12 &&
    day >= 1 &&
    day <= daysInMonth &&
    hour <= 23 &&
    minute <= 59 &&
    second <= 59 &&
    offsetHour <= 23 &&
    offsetMinute <= 59
  )
}

export function parseLogsSearch(search: Record<string, unknown>): LogsSearch {
  const out: LogsSearch = {}
  const tab = oneOf(LOGS_TABS, search.tab)
  if (tab) out.tab = tab

  // A tab without its own periods (outgoing webhooks) has none. Without a tab, the periods of the
  // messages apply: the first tab most members open.
  const periods: readonly string[] =
    tab === 'audit' ? AUDIT_PERIODS : tab === 'outgoing-webhooks' ? [] : LIVE_PERIODS
  const period = oneOf(periods as readonly LogsPeriod[], search.period)
  if (period) out.period = period

  const closed = {
    origin: oneOf(MESSAGE_ORIGINS, search.origin),
    channel: oneOf(MESSAGE_CHANNELS, search.channel),
    source: oneOf(WEBHOOK_SOURCES, search.source),
    event_type: oneOf(INBOUND_EVENT_TYPES, search.event_type),
    status: oneOf(DELIVERY_STATUSES, search.status),
    outcome: oneOf(AUDIT_OUTCOMES, search.outcome),
    scope: oneOf(AUDIT_SCOPES, search.scope),
    actor_type: oneOf(AUDIT_ACTOR_TYPES, search.actor_type)
  }
  for (const [key, value] of Object.entries(closed)) {
    if (value !== undefined) Object.assign(out, { [key]: value })
  }

  for (const key of MESSAGE_FLAGS) {
    const value = flag(search[key])
    if (value !== undefined) out[key] = value
  }

  for (const key of TEXT_KEYS) {
    const value = searchString(search[key])
    if (value !== undefined) out[key] = value
  }
  // The bounds of a custom period reach the server, which reads RFC 3339 and answers 400 to
  // anything else (a bare date included).
  for (const key of ['from', 'to'] as const) {
    if (out[key] !== undefined && !isRfc3339(out[key])) delete out[key]
  }
  // The server answers 400 to an email filter that is not a full address (a link cut short).
  for (const key of ['contact_email', 'recipient_email'] as const) {
    if (out[key] !== undefined && !isFullEmail(out[key])) delete out[key]
  }
  return out
}
