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

export const webAnalyticsService = {
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
