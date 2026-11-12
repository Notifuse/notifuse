import { api } from './client'

// Types mirror internal/domain/web_analytics.go.

export type WebFilterOperator =
  | 'equals'
  | 'not_equals'
  | 'contains'
  | 'not_contains'
  | 'is_empty'
  | 'is_not_empty'
  | 'regex'

export type WebFilterAction = 'set_value' | 'unset_value' | 'set_default_value'

export interface WebFilterCondition {
  field: string
  operator: WebFilterOperator
  value?: string
}

export interface WebFilterOperation {
  dimension: string
  action: WebFilterAction
  value?: string
}

export interface WebFilter {
  id: string
  name: string
  priority: number
  order: number
  tags?: string[]
  conditions: WebFilterCondition[]
  operations: WebFilterOperation[]
  enabled: boolean
  version?: string
  created_at?: string
  updated_at?: string
}

export interface WebAnalyticsSettings {
  enabled: boolean
  allowed_domains?: string[]
  bounce_threshold_seconds?: number
  filters?: WebFilter[]
  filters_version?: string
  custom_dimension_labels?: Record<string, string>
  /** Off by default; letting Notifuse identify recipients from tracked email links is opt-in. */
  identify_from_email_links?: boolean
  geo_enabled: boolean
  geo_store_city: boolean
  geo_store_region: boolean
  geo_coordinates_precision: number
}

export interface WebAnalyticsBackfillState {
  filters_version: string
  partitions?: string[]
  partition_index: number
  rows_updated: number
}

export interface WebAnalyticsBackfillStatus {
  task_id: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'paused'
  progress: number
  state?: WebAnalyticsBackfillState
  error_message?: string
}

export const WEB_FILTER_SOURCE_FIELDS = [
  'utm_source',
  'utm_medium',
  'utm_campaign',
  'utm_term',
  'utm_content',
  'utm_id',
  'utm_id_from',
  'referrer',
  'referrer_domain',
  'referrer_path',
  'is_direct',
  'landing_page',
  'landing_domain',
  'landing_path',
  'path',
  'device',
  'browser',
  'browser_type',
  'os',
  'user_agent',
  'connection_type',
  'language',
  'timezone'
] as const

export const WEB_FILTER_WRITABLE_DIMENSIONS = [
  'channel',
  'channel_group',
  'custom_1',
  'custom_2',
  'custom_3',
  'custom_4',
  'custom_5',
  'custom_6',
  'custom_7',
  'custom_8',
  'custom_9',
  'custom_10',
  'utm_source',
  'utm_medium',
  'utm_campaign',
  'utm_term',
  'utm_content',
  'referrer_domain',
  'is_direct'
] as const

export type ContactVisitsFilter = 'all' | 'goal' | 'email'

/** One web session of a contact (webAnalytics.contactVisits). */
export interface ContactVisitSession {
  id: string
  /** YYYY-MM-DD, what contactSessionPages takes. */
  session_date: string
  created_at: string
  duration_ms: number
  pageview_count: number
  landing_path: string
  exit_path: string
  channel: string
  channel_group: string
  referrer_domain: string
  utm_source: string
  utm_campaign: string
  device: string
  os: string
  browser: string
  country: string
  goal_count: number
  goal_value: number
  first_goal_name: string | null
}

export interface ContactVisitsSummary {
  sessions: number
  goal_sessions: number
  email_sessions: number
  median_duration_ms: number
  first_session: ContactVisitSession | null
  goals: { goal_name: string; count: number; value: number }[]
  top_pages: { path: string; views: number }[]
}

export interface ContactVisitsRequest {
  workspace_id: string
  email: string
  filter?: ContactVisitsFilter
  limit?: number
  cursor?: string
}

export interface ContactVisitsResponse {
  /** Only on the first page, over all of the contact's sessions whatever the filter. */
  summary?: ContactVisitsSummary
  sessions: ContactVisitSession[]
  next_cursor?: string
}

export interface ContactSessionPagesRequest {
  workspace_id: string
  email: string
  session_id: string
  session_date: string
}

export interface ContactSessionPagesResponse {
  pages: { entered_at: string; path: string; duration_ms: number; page_number: number }[]
  goals: { goal_at: string; goal_name: string; goal_value: number; path: string }[]
}

export const webAnalyticsService = {
  /** A contact's identified web sessions, newest first. */
  async contactVisits(params: ContactVisitsRequest): Promise<ContactVisitsResponse> {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('email', params.email)
    if (params.filter) searchParams.append('filter', params.filter)
    if (params.limit !== undefined) searchParams.append('limit', String(params.limit))
    if (params.cursor) searchParams.append('cursor', params.cursor)
    return api.get<ContactVisitsResponse>(`/api/webAnalytics.contactVisits?${searchParams.toString()}`)
  },

  /** The pages and goals of one of the contact's sessions. */
  async contactSessionPages(params: ContactSessionPagesRequest): Promise<ContactSessionPagesResponse> {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('email', params.email)
    searchParams.append('session_id', params.session_id)
    searchParams.append('session_date', params.session_date)
    return api.get<ContactSessionPagesResponse>(
      `/api/webAnalytics.contactSessionPages?${searchParams.toString()}`
    )
  },


  async setSettings(workspaceId: string, settings: WebAnalyticsSettings | null): Promise<void> {
    await api.post('/api/workspaces.setWebAnalyticsSettings', {
      workspace_id: workspaceId,
      settings
    })
  },

  async backfillStart(workspaceId: string): Promise<WebAnalyticsBackfillStatus> {
    const response = await api.post<{ backfill: WebAnalyticsBackfillStatus }>(
      '/api/webAnalytics.backfillStart',
      { workspace_id: workspaceId }
    )
    return response.backfill
  },

  async backfillStatus(workspaceId: string): Promise<WebAnalyticsBackfillStatus | null> {
    const response = await api.post<{ backfill: WebAnalyticsBackfillStatus | null }>(
      '/api/webAnalytics.backfillStatus',
      { workspace_id: workspaceId }
    )
    return response.backfill
  },

  async backfillCancel(workspaceId: string): Promise<void> {
    await api.post('/api/webAnalytics.backfillCancel', { workspace_id: workspaceId })
  }
}

/** Install snippet shown on the settings page. */
/**
 * Origin the tracking snippet must beat to.
 *
 * A workspace serving analytics from its own domain wins; otherwise the API
 * host this console talks to, and finally the page's own origin for a
 * single-domain install.
 *
 * Typed structurally rather than as `Workspace`: the workspace module already
 * imports this one for its settings type.
 */
export function resolveTrackingEndpoint(
  workspace?: { settings?: { custom_endpoint_url?: string } } | null
): string {
  return (
    workspace?.settings?.custom_endpoint_url ||
    window.API_ENDPOINT?.trim().replace(/\/+$/, '') ||
    window.location.origin
  )
}

export function buildInstallSnippet(endpoint: string, workspaceId: string): string {
  const origin = endpoint.replace(/\/$/, '')
  return [
    '<script>',
    `  window.NotifuseAnalyticsConfig = { workspace_id: ${JSON.stringify(workspaceId)}, endpoint: ${JSON.stringify(origin)} };`,
    '</script>',
    `<script async src="${origin}/na.js"></script>`
  ].join('\n')
}
