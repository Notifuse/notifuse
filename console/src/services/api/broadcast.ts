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
  bounced: number
  complained: number
  unsubscribed: number
}

// Define the EmailTemplate interface
export interface EmailTemplate {
  sender_id: string
  reply_to?: string
  subject: string
  subject_preview?: string
  compiled_preview: string
  visual_editor_tree: any
  text?: string
}

// Define the Template interface
export interface Template {
  id: string
  name: string
  version: number
  channel: string
  email: EmailTemplate
  category: string
  template_macro_id?: string
  integration_id?: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  test_data?: Record<string, any>
  settings?: Record<string, any>
  created_at: string
  updated_at: string
  deleted_at?: string
}

export interface BroadcastVariation {
  variation_name: string
  template_id: string
  metrics?: VariationMetrics
  template?: Template // Template joined from server when with_templates is true
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
  list?: string
  segments?: string[]
  exclude_unsubscribed: boolean
}

export interface ScheduleSettings {
  is_scheduled: boolean
  scheduled_date?: string // Format: YYYY-MM-dd
  scheduled_time?: string // Format: HH:mm
  timezone?: string // IANA timezone format, e.g. "America/New_York"
  use_recipient_timezone: boolean
}

export type BroadcastStatus =
  | 'draft'
  | 'scheduled'
  | 'sending'
  | 'paused'
  | 'sent'
  | 'cancelled'
  | 'failed'
  | 'testing'
  | 'test_completed'
  | 'winner_selected'

export interface BroadcastChannels {
  email: boolean
  web: boolean
}

export interface WebPublicationSettings {
  slug?: string
  meta_title?: string
  meta_description?: string
  og_title?: string
  og_description?: string
  og_image?: string
  canonical_url?: string
  keywords?: string[]
}

export interface Broadcast {
  id: string
  workspace_id: string
  name: string
  status: BroadcastStatus
  audience: AudienceSettings
  schedule: ScheduleSettings
  test_settings: BroadcastTestSettings
  utm_parameters?: UTMParameters
  metadata?: Record<string, any>
  channels: BroadcastChannels
  web_publication_settings?: WebPublicationSettings
  web_published_at?: string
  sent_count: number
  delivered_count: number
  failed_count: number
  total_opens?: number
  total_clicks?: number
  total_bounced?: number
  total_complained?: number
  total_unsubscribed?: number
  winning_template?: string
  test_sent_at?: string
  winner_sent_at?: string
  created_at: string
  updated_at: string
  started_at?: string
  completed_at?: string
  cancelled_at?: string
  paused_at?: string
  pause_reason?: string
}

export interface CreateBroadcastRequest {
  workspace_id: string
  name: string
  audience: AudienceSettings
  schedule: ScheduleSettings
  test_settings: BroadcastTestSettings
  utm_parameters?: UTMParameters
  metadata?: Record<string, any>
  channels: BroadcastChannels
  web_publication_settings?: WebPublicationSettings
}

export interface UpdateBroadcastRequest {
  workspace_id: string
  id: string
  name: string
  audience: AudienceSettings
  schedule: ScheduleSettings
  test_settings: BroadcastTestSettings
  utm_parameters?: UTMParameters
  metadata?: Record<string, any>
  channels: BroadcastChannels
  web_publication_settings?: WebPublicationSettings
}

export interface ListBroadcastsRequest {
  workspace_id: string
  status?: BroadcastStatus
  limit?: number
  offset?: number
  with_templates?: boolean
}

export interface ListBroadcastsResponse {
  broadcasts: Broadcast[]
  total_count: number
}

export interface GetBroadcastRequest {
  workspace_id: string
  id: string
  with_templates?: boolean
}

export interface GetBroadcastResponse {
  broadcast: Broadcast
}

export interface ScheduleBroadcastRequest {
  workspace_id: string
  id: string
  send_now: boolean
  scheduled_date?: string
  scheduled_time?: string
  timezone?: string
  use_recipient_timezone?: boolean
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
  template_id?: string
}

export interface DeleteBroadcastRequest {
  workspace_id: string
  id: string
}

export interface GetTestResultsRequest {
  workspace_id: string
  id: string
}

export interface SelectWinnerRequest {
  workspace_id: string
  id: string
  template_id: string
}

export interface VariationResult {
  template_id: string
  template_name: string
  recipients: number
  delivered: number
  opens: number
  clicks: number
  open_rate: number
  click_rate: number
}

export interface TestResultsResponse {
  broadcast_id: string
  status: string
  test_started_at?: string
  test_completed_at?: string
  variation_results: Record<string, VariationResult>
  recommended_winner?: string
  winning_template?: string
  is_auto_send_winner: boolean
}

export const broadcastApi = {
  list: async (params: ListBroadcastsRequest): Promise<ListBroadcastsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    if (params.status) searchParams.append('status', params.status)
    if (params.limit) searchParams.append('limit', params.limit.toString())
    if (params.offset) searchParams.append('offset', params.offset.toString())
    if (params.with_templates !== undefined)
      searchParams.append('with_templates', params.with_templates.toString())

    return api.get<ListBroadcastsResponse>(`/api/broadcasts.list?${searchParams.toString()}`)
  },

  get: async (params: GetBroadcastRequest): Promise<GetBroadcastResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('id', params.id)
    if (params.with_templates !== undefined)
      searchParams.append('with_templates', params.with_templates.toString())

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
  },

  delete: async (params: DeleteBroadcastRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/broadcasts.delete', params)
  },

  getTestResults: async (params: GetTestResultsRequest): Promise<TestResultsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('id', params.id)

    return api.get<TestResultsResponse>(`/api/broadcasts.getTestResults?${searchParams.toString()}`)
  },

  selectWinner: async (params: SelectWinnerRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/broadcasts.selectWinner', params)
  }
}
