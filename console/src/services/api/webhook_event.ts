import { api } from './client'

export type EmailEventType = 'delivered' | 'bounce' | 'complaint'
export type EmailProviderKind = 'ses' | 'sparkpost' | 'mailgun' | 'mailjet' | 'postmark'

export interface WebhookEvent {
  id: string
  type: EmailEventType
  email_provider_kind: EmailProviderKind
  integration_id: string
  recipient_email: string
  message_id: string
  transactional_id?: string
  broadcast_id?: string
  timestamp: string
  raw_payload: string

  // Bounce specific fields
  bounce_type?: string
  bounce_category?: string
  bounce_diagnostic?: string

  // Complaint specific fields
  complaint_feedback_type?: string

  created_at: string
}

export interface WebhookEventListParams {
  cursor?: string
  limit?: number

  // Filters
  event_type?: EmailEventType
  recipient_email?: string
  message_id?: string
  transactional_id?: string
  broadcast_id?: string

  // Time range filters
  timestamp_after?: string
  timestamp_before?: string
}

export interface WebhookEventListResult {
  events: WebhookEvent[]
  next_cursor?: string
  has_more: boolean
}

/**
 * Lists webhook events with pagination and filtering
 */
export function listWebhookEvents(
  workspaceId: string,
  params: WebhookEventListParams
): Promise<WebhookEventListResult> {
  // Convert params object to URLSearchParams for query string
  const queryParams = new URLSearchParams()
  queryParams.append('workspace_id', workspaceId)

  // Add all other params that are defined
  if (params.cursor) queryParams.append('cursor', params.cursor)
  if (params.limit) queryParams.append('limit', String(params.limit))
  if (params.event_type) queryParams.append('event_type', params.event_type)
  if (params.recipient_email) queryParams.append('recipient_email', params.recipient_email)
  if (params.message_id) queryParams.append('message_id', params.message_id)
  if (params.transactional_id) queryParams.append('transactional_id', params.transactional_id)
  if (params.broadcast_id) queryParams.append('broadcast_id', params.broadcast_id)
  if (params.timestamp_after) queryParams.append('timestamp_after', params.timestamp_after)
  if (params.timestamp_before) queryParams.append('timestamp_before', params.timestamp_before)

  return api.get<WebhookEventListResult>(`/api/webhookEvents.list?${queryParams.toString()}`)
}
