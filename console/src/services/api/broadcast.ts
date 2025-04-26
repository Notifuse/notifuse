import { api } from './client'

export interface UTMParameters {
  source?: string
  medium?: string
  campaign?: string
  term?: string
  content?: string
}

export interface VariationMetrics {
  recipients: number
  delivered: number
  opens: number
  clicks: number
  open_rate: number
  click_rate: number
}

export interface BroadcastVariation {
  id: string
  name: string
  template_id: string
  template_version: number
  subject: string
  preview_text?: string
  from_name: string
  from_email: string
  reply_to?: string
  metrics?: VariationMetrics
}

export interface BroadcastTestSettings {
  enabled: boolean
  sample_percentage: number
  auto_send_winner: boolean
  auto_send_winner_metric?: 'open_rate' | 'click_rate'
  test_duration_hours?: number
  variations: BroadcastVariation[]
}

export interface AudienceSettings {
  lists?: string[]
  segments?: string[]
  exclude_unsubscribed: boolean
  skip_duplicate_emails: boolean
  rate_limit_per_minute?: number
}

export interface ScheduleSettings {
  send_immediately: boolean
  scheduled_time?: string
  use_recipient_timezone: boolean
  time_window_start?: string
  time_window_end?: string
}

export type BroadcastStatus =
  | 'draft'
  | 'scheduled'
  | 'sending'
  | 'paused'
  | 'sent'
  | 'cancelled'
  | 'failed'

export interface Broadcast {
  id: string
  workspace_id: string
  name: string
  status: BroadcastStatus
  audience: AudienceSettings
  schedule: ScheduleSettings
  test_settings: BroadcastTestSettings
  goal_id?: string
  tracking_enabled: boolean
  utm_parameters?: UTMParameters
  metadata?: Record<string, any>
  sent_count: number
  delivered_count: number
  failed_count: number
  winning_variation?: string
  test_sent_at?: string
  winner_sent_at?: string
  created_at: string
  updated_at: string
  scheduled_at?: string
  started_at?: string
  completed_at?: string
  cancelled_at?: string
  paused_at?: string
}

export interface CreateBroadcastRequest {
  workspace_id: string
  name: string
  audience: AudienceSettings
  schedule: ScheduleSettings
  test_settings: BroadcastTestSettings
  goal_id?: string
  tracking_enabled: boolean
  utm_parameters?: UTMParameters
  metadata?: Record<string, any>
}

export interface UpdateBroadcastRequest {
  workspace_id: string
  id: string
  name: string
  audience: AudienceSettings
  schedule: ScheduleSettings
  test_settings: BroadcastTestSettings
  goal_id?: string
  tracking_enabled: boolean
  utm_parameters?: UTMParameters
  metadata?: Record<string, any>
}

export interface ListBroadcastsRequest {
  workspace_id: string
  status?: BroadcastStatus
  limit?: number
  offset?: number
}

export interface ListBroadcastsResponse {
  broadcasts: Broadcast[]
  total_count: number
}

export interface GetBroadcastRequest {
  workspace_id: string
  id: string
}

export interface GetBroadcastResponse {
  broadcast: Broadcast
}

export interface ScheduleBroadcastRequest {
  workspace_id: string
  id: string
  scheduled_at?: string
  send_now: boolean
}

export interface PauseBroadcastRequest {
  workspace_id: string
  id: string
}

export interface ResumeBroadcastRequest {
  workspace_id: string
  id: string
}

export interface CancelBroadcastRequest {
  workspace_id: string
  id: string
}

export interface SendToIndividualRequest {
  workspace_id: string
  broadcast_id: string
  recipient_email: string
  variation_id?: string
}

export const broadcastApi = {
  list: async (params: ListBroadcastsRequest): Promise<ListBroadcastsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    if (params.status) searchParams.append('status', params.status)
    if (params.limit) searchParams.append('limit', params.limit.toString())
    if (params.offset) searchParams.append('offset', params.offset.toString())

    return api.get<ListBroadcastsResponse>(`/api/broadcasts.list?${searchParams.toString()}`)
  },

  get: async (params: GetBroadcastRequest): Promise<GetBroadcastResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('id', params.id)

    return api.get<GetBroadcastResponse>(`/api/broadcasts.get?${searchParams.toString()}`)
  },

  create: async (params: CreateBroadcastRequest): Promise<GetBroadcastResponse> => {
    return api.post<GetBroadcastResponse>('/api/broadcasts.create', params)
  },

  update: async (params: UpdateBroadcastRequest): Promise<GetBroadcastResponse> => {
    return api.post<GetBroadcastResponse>('/api/broadcasts.update', params)
  },

  schedule: async (params: ScheduleBroadcastRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/broadcasts.schedule', params)
  },

  pause: async (params: PauseBroadcastRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/broadcasts.pause', params)
  },

  resume: async (params: ResumeBroadcastRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/broadcasts.resume', params)
  },

  cancel: async (params: CancelBroadcastRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/broadcasts.cancel', params)
  },

  sendToIndividual: async (params: SendToIndividualRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/broadcasts.sendToIndividual', params)
  }
}
