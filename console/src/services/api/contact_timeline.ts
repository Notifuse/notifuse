import { api } from './client'

// Entity data types for each entity type
export interface ContactEntityData {
  email: string
  first_name?: string
  last_name?: string
  external_id?: string
}

export interface ContactListEntityData {
  id: string
  name: string
  status: string
}

export interface MessageHistoryEntityData {
  id: string
  template_id: string
  template_version: number
  template_name?: string | null
  template_category?: string | null
  template_email?: Record<string, any> | null
  channel: string
  sent_at: string
  delivered_at?: string
  opened_at?: string
  clicked_at?: string
  message_data?: Record<string, any>
}

export interface WebhookEventEntityData {
  id: string
  type: string // delivered, bounce, complaint, opened, clicked
  message_id: string
  timestamp: string
  bounce_type?: string | null
  bounce_category?: string | null
  bounce_diagnostic?: string | null
  complaint_feedback_type?: string | null
  template_id?: string | null
  template_version?: number | null
  template_name?: string | null
}

export type EntityData =
  | ContactEntityData
  | ContactListEntityData
  | MessageHistoryEntityData
  | WebhookEventEntityData

export interface ContactTimelineEntry {
  id: string
  email: string
  operation: 'insert' | 'update' | 'delete'
  entity_type: 'contact' | 'contact_list' | 'message_history' | 'webhook_event'
  kind: string // operation_entityType (e.g., 'insert_contact', 'update_message_history')
  changes: Record<string, any>
  entity_id?: string // NULL for contact, list_id for contact_list, message_id for message_history and webhook_event
  entity_data?: EntityData // Joined entity data with contact, list, message, or webhook event details
  created_at: string // Can be set to historical data
  db_created_at: string // Timestamp when record was inserted into database
}

export interface TimelineListRequest {
  workspace_id: string
  email: string
  limit?: number // Default 50, max 100
  cursor?: string
}

export interface TimelineListResponse {
  timeline: ContactTimelineEntry[]
  next_cursor?: string
}

export const contactTimelineApi = {
  /**
   * List timeline entries for a contact with pagination
   */
  list: async (params: TimelineListRequest): Promise<TimelineListResponse> => {
    const searchParams = new URLSearchParams()

    // Add required params
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('email', params.email)

    // Add optional params if they exist
    if (params.limit !== undefined) {
      searchParams.append('limit', params.limit.toString())
    }
    if (params.cursor) {
      searchParams.append('cursor', params.cursor)
    }

    return api.get<TimelineListResponse>(`/api/timeline.list?${searchParams.toString()}`)
  }
}
