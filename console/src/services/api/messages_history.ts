import { api } from './client'

export interface MessageData {
  data: Record<string, any>
  metadata?: Record<string, any>
}

export interface MessageHistory {
  id: string
  external_id?: string
  contact_email: string
  broadcast_id?: string
  list_ids?: string[]
  template_id: string
  template_version: number
  channel: string
  error?: string
  message_data: MessageData

  // Event timestamps
  sent_at: string
  delivered_at?: string
  failed_at?: string
  opened_at?: string
  clicked_at?: string
  bounced_at?: string
  complained_at?: string
  unsubscribed_at?: string

  // System timestamps
  created_at: string
  updated_at: string
}

export interface MessageListParams {
  cursor?: string
  limit?: number

  // Filters
  id?: string
  external_id?: string
  list_id?: string
  channel?: string
  contact_email?: string
  broadcast_id?: string
  template_id?: string
  has_error?: boolean
  is_sent?: boolean
  is_delivered?: boolean
  is_failed?: boolean
  is_opened?: boolean
  is_clicked?: boolean
  is_bounced?: boolean
  is_complained?: boolean
  is_unsubscribed?: boolean

  // Time range filters
  sent_after?: string
  sent_before?: string
  updated_after?: string
  updated_before?: string
}

export interface MessageListResult {
  messages: MessageHistory[]
  next_cursor?: string
  has_more: boolean
}

/**
 * Lists message history with pagination and filtering
 */
export function listMessages(
  workspaceId: string,
  params: MessageListParams
): Promise<MessageListResult> {
  // Convert params object to URLSearchParams for query string
  const queryParams = new URLSearchParams()
  queryParams.append('workspace_id', workspaceId)

  // Add all other params that are defined
  if (params.cursor) queryParams.append('cursor', params.cursor)
  if (params.limit) queryParams.append('limit', String(params.limit))
  if (params.id) queryParams.append('id', params.id)
  if (params.external_id) queryParams.append('external_id', params.external_id)
  if (params.list_id) queryParams.append('list_id', params.list_id)
  if (params.channel) queryParams.append('channel', params.channel)
  if (params.contact_email) queryParams.append('contact_email', params.contact_email)
  if (params.broadcast_id) queryParams.append('broadcast_id', params.broadcast_id)
  if (params.template_id) queryParams.append('template_id', params.template_id)
  if (params.has_error !== undefined) queryParams.append('has_error', String(params.has_error))
  if (params.is_sent !== undefined) queryParams.append('is_sent', String(params.is_sent))
  if (params.is_delivered !== undefined)
    queryParams.append('is_delivered', String(params.is_delivered))
  if (params.is_failed !== undefined) queryParams.append('is_failed', String(params.is_failed))
  if (params.is_opened !== undefined) queryParams.append('is_opened', String(params.is_opened))
  if (params.is_clicked !== undefined) queryParams.append('is_clicked', String(params.is_clicked))
  if (params.is_bounced !== undefined) queryParams.append('is_bounced', String(params.is_bounced))
  if (params.is_complained !== undefined)
    queryParams.append('is_complained', String(params.is_complained))
  if (params.is_unsubscribed !== undefined)
    queryParams.append('is_unsubscribed', String(params.is_unsubscribed))
  if (params.sent_after) queryParams.append('sent_after', params.sent_after)
  if (params.sent_before) queryParams.append('sent_before', params.sent_before)
  if (params.updated_after) queryParams.append('updated_after', params.updated_after)
  if (params.updated_before) queryParams.append('updated_before', params.updated_before)

  return api.get<MessageListResult>(`/api/messages.list?${queryParams.toString()}`)
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
}

/**
 * Response from the broadcast stats endpoint
 */
export interface BroadcastStatsResult {
  broadcast_id: string
  stats: MessageHistoryStatusSum
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
