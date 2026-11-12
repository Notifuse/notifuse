/**
 * Search of the `/logs` route. The router's default parser runs JSON.parse on each value, so
 * `?is_failed=true` arrives as a boolean, `?template_id=2024` as a number and a string the page
 * once wrote as 'true' as the text "true". Everything is normalised here, once, by the route's
 * `validateSearch`: the tabs read typed values and never the raw location.
 */

import { searchString } from '../../lib/searchParams'

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

export function parseLogsSearch(search: Record<string, unknown>): LogsSearch {
  const out: LogsSearch = {}
  const tab = oneOf(LOGS_TABS, search.tab)
  if (tab) out.tab = tab

  // A tab without its own periods (outgoing webhooks) has none; without a tab the page opens the
  // messages, so theirs apply.
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
  return out
}
