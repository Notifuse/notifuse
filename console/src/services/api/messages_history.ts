import { api } from './client'

/**
 * Channel-specific delivery options for email
 * Structure allows future extension for SMS/push without breaking changes
 */
export interface ChannelOptions {
  // Email options
  from_name?: string
  // Set when the send forced them (transactional email_options), as stored, Liquid included.
  subject?: string
  subject_preview?: string
  cc?: string[]
  bcc?: string[]
  reply_to?: string
}

export interface MessageData {
  data: Record<string, unknown>
  metadata?: Record<string, unknown>
}

export interface MessageAttachment {
  checksum: string
  filename: string
  content_type: string
  disposition: string
  content_id?: string
}

export interface MessageHistory {
  id: string
  external_id?: string
  contact_email: string
  broadcast_id?: string
  automation_id?: string
  transactional_notification_id?: string
  list_id?: string
  template_id: string
  template_version: number
  channel: string
  // The provider's own explanation of a failure. Named status_info because
  // that is the field the API returns; nothing was ever sent under the name
  // `error`, which is why this column read blank for every message.
  status_info?: string
  message_data: MessageData
  channel_options?: ChannelOptions
  attachments?: MessageAttachment[]

  // Event timestamps
  sent_at: string
  delivered_at?: string
  failed_at?: string
  opened_at?: string
  clicked_at?: string
  bounced_at?: string
  complained_at?: string
  unsubscribed_at?: string

  // Whether the email carried the open pixel and tracked links; absent on messages older than these
  // fields, which were tracked.
  opens_tracked?: boolean
  clicks_tracked?: boolean

  // System timestamps
  created_at: string
  updated_at: string
}

/** What sent a message: a broadcast, an automation, a transactional notification, or none of them. */
export type MessageOrigin = 'broadcast' | 'automation' | 'transactional' | 'other'

export interface MessageFilterParams {
  id?: string
  external_id?: string
  list_id?: string
  channel?: string
  contact_email?: string
  broadcast_id?: string
  // Messages sent by this automation's email steps.
  automation_id?: string
  template_id?: string
  transactional_notification_id?: string
  // Messages on which this exact URL was clicked; requires broadcast_id.
  clicked_url?: string
  is_sent?: boolean
  is_delivered?: boolean
  is_failed?: boolean
  is_opened?: boolean
  is_clicked?: boolean
  is_bounced?: boolean
  is_complained?: boolean
  is_unsubscribed?: boolean
  origin?: MessageOrigin

  // Time range filters; created_after and created_before are the indexed ones.
  created_after?: string
  created_before?: string
  sent_after?: string
  sent_before?: string
  updated_after?: string
  updated_before?: string
}

export interface MessageListParams extends MessageFilterParams {
  cursor?: string
  limit?: number
}

export interface MessageListResult {
  messages: MessageHistory[]
  next_cursor?: string
  has_more: boolean
}

// The filters messages.list and messages.count share.
const FILTER_KEYS = [
  'id',
  'external_id',
  'list_id',
  'channel',
  'contact_email',
  'broadcast_id',
  'automation_id',
  'template_id',
  'transactional_notification_id',
  'clicked_url',
  'is_sent',
  'is_delivered',
  'is_failed',
  'is_opened',
  'is_clicked',
  'is_bounced',
  'is_complained',
  'is_unsubscribed',
  'origin',
  'created_after',
  'created_before',
  'sent_after',
  'sent_before',
  'updated_after',
  'updated_before'
] as const satisfies readonly (keyof MessageFilterParams)[]

function filterQuery(workspaceId: string, params: MessageFilterParams): URLSearchParams {
  const query = new URLSearchParams()
  query.append('workspace_id', workspaceId)
  for (const key of FILTER_KEYS) {
    const value = params[key]
    // A false status filter is a filter; an empty text is none.
    if (typeof value === 'boolean') query.append(key, String(value))
    else if (value) query.append(key, value)
  }
  return query
}

/**
 * Lists message history with pagination and filtering
 */
export function listMessages(
  workspaceId: string,
  params: MessageListParams
): Promise<MessageListResult> {
  const queryParams = filterQuery(workspaceId, params)
  if (params.cursor) queryParams.append('cursor', params.cursor)
  if (params.limit) queryParams.append('limit', String(params.limit))
  return api.get<MessageListResult>(`/api/messages.list?${queryParams.toString()}`)
}

/** The counts of messages.count; each follows the is_*=true filter of the same name. */
export interface MessageCounts {
  total: number
  delivered: number
  opened: number
  clicked: number
  bounced: number
  failed: number
  complained: number
}

/**
 * Counts the messages of the same filters as listMessages, in total and by status. The server
 * refuses (400) a count without created_after and without an indexed filter.
 */
export function countMessages(
  workspaceId: string,
  params: MessageFilterParams
): Promise<MessageCounts> {
  return api.get<MessageCounts>(
    `/api/messages.count?${filterQuery(workspaceId, params).toString()}`
  )
}

/**
 * Stats for each message status in a broadcast
 * Matches MessageHistoryStatusSum from the backend
 */
export interface MessageHistoryStatusSum {
  total_sent: number
  total_delivered: number
  total_bounced: number
  total_complained: number
  total_failed: number
  total_opened: number
  total_clicked: number
  total_unsubscribed: number
  // Sent emails that carried the open pixel / tracked links: the denominators
  // of the open and click rates.
  total_opens_tracked?: number
  total_clicks_tracked?: number
}

/**
 * Response from the broadcast stats endpoint
 */
// What the queue is still holding for one broadcast. Absent when the queue could not
// be read, which the caller must treat as "unknown" rather than as "nothing left".
export interface BroadcastQueueCounts {
  pending: number
  processing: number
  paused: number
  /** Still has attempts left; it will be picked up again on its own. */
  failed_retrying: number
  /** Has spent every attempt. Only an explicit retry revives it. */
  failed_terminal: number
}

export interface BroadcastStatsResult {
  broadcast_id: string
  stats: MessageHistoryStatusSum
  queue?: BroadcastQueueCounts
}

/**
 * Gets statistics for a specific broadcast
 */
export function getBroadcastStats(
  workspaceId: string,
  broadcastId: string
): Promise<BroadcastStatsResult> {
  const queryParams = new URLSearchParams()
  queryParams.append('workspace_id', workspaceId)
  queryParams.append('broadcast_id', broadcastId)

  return api.get<BroadcastStatsResult>(`/api/messages.broadcastStats?${queryParams.toString()}`)
}

/**
 * Response from the broadcast variation stats endpoint (one A/B variation)
 */
export interface BroadcastVariationStatsResult {
  broadcast_id: string
  template_id: string
  stats: MessageHistoryStatusSum
}

/**
 * Gets aggregate statistics for a single variation (template) of a broadcast.
 * Available regardless of broadcast status, so it powers per-variation metrics after a
 * broadcast has finished sending (when the A/B testResults endpoint no longer applies).
 */
export function getBroadcastVariationStats(
  workspaceId: string,
  broadcastId: string,
  templateId: string
): Promise<BroadcastVariationStatsResult> {
  const queryParams = new URLSearchParams()
  queryParams.append('workspace_id', workspaceId)
  queryParams.append('broadcast_id', broadcastId)
  queryParams.append('template_id', templateId)

  return api.get<BroadcastVariationStatsResult>(
    `/api/messages.broadcastVariationStats?${queryParams.toString()}`
  )
}

/**
 * Per-URL click stats for a broadcast
 * Matches LinkClickStats from the backend
 */
export interface LinkClickStats {
  url: string
  total_clicks: number
  unique_clicks: number
}

/**
 * Response from the broadcast link stats endpoint
 */
export interface BroadcastLinkStatsResult {
  link_stats: LinkClickStats[]
}

/**
 * Gets per-link click statistics for a specific broadcast.
 * Pass templateId to scope the breakdown to a single A/B variation (the template its
 * messages were sent with); omit it for the whole broadcast.
 */
export function getBroadcastLinkStats(
  workspaceId: string,
  broadcastId: string,
  templateId?: string
): Promise<BroadcastLinkStatsResult> {
  const queryParams = new URLSearchParams()
  queryParams.append('workspace_id', workspaceId)
  queryParams.append('broadcast_id', broadcastId)
  if (templateId) queryParams.append('template_id', templateId)

  return api.get<BroadcastLinkStatsResult>(
    `/api/messages.broadcastLinkStats?${queryParams.toString()}`
  )
}
