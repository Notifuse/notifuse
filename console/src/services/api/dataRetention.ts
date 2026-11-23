import { api } from './client'
import { taskApi, type Task } from './task'

// Data retention: how long each kind of personal data is kept, and the daily run that applies it.

export type RetentionCategory =
  | 'inactive_contacts'
  | 'message_history'
  | 'contact_timeline'
  | 'custom_events'
  | 'automation_history'
  | 'web_analytics'
  | 'inbound_webhook_events'
  | 'webhook_deliveries'
  | 'failed_emails'

export interface RetentionCategorySpec {
  category: RetentionCategory
  min_days: number
  suggested_days: number
  default_days: number
}

export type RetentionDays = Partial<Record<RetentionCategory, number>>

export interface DataRetentionPolicy {
  /** Period in days per category; absent follows the deployment default, 0 is off. */
  categories: RetentionDays
  exempt_list_ids?: string[]
  exempt_segment_ids?: string[]
  engagement_event_names?: string[]
}

export interface PendingRetentionChange {
  policy: DataRetentionPolicy
  effective_at: string
  requested_by?: string
  requested_at: string
}

export interface DataRetentionSettings {
  policy?: DataRetentionPolicy
  pending?: PendingRetentionChange
  revision?: number
}

export type RetentionRunStatus = 'running' | 'completed' | 'partial' | 'failed'

export interface RetentionCategoryResult {
  days: number
  affected: number
  opt_outs_kept?: number
  remaining?: boolean
  skipped?: string
  error?: string
}

export interface RetentionRun {
  id: string
  started_at: string
  finished_at?: string
  status: RetentionRunStatus
  policy: RetentionDays
  results: Partial<Record<RetentionCategory, RetentionCategoryResult>>
  error?: string
}

export interface DataRetentionView {
  policy?: DataRetentionPolicy
  pending?: PendingRetentionChange
  in_force: Record<RetentionCategory, number>
  pending_days?: Record<RetentionCategory, number>
  pending_at?: string
  defaults: Record<RetentionCategory, number>
  categories: RetentionCategorySpec[]
  backfill_ready: boolean
  runs: RetentionRun[]
  event_names: string[]
}

export interface DataRetentionPreview {
  categories: Partial<Record<RetentionCategory, RetentionCategoryResult>>
  pending: boolean
  pending_at?: string
}

export interface RetentionDefaults {
  categories: RetentionDays
  pending?: { categories: RetentionDays; effective_at: string }
}

export interface RetentionDefaultsView {
  defaults: Record<RetentionCategory, number>
  stored: RetentionDefaults | null
  categories: RetentionCategorySpec[]
}

export const dataRetentionQueryKey = (workspaceId: string) => ['data-retention', workspaceId] as const

/** The task that reads the engagement of existing contacts back from their history. */
export const DATA_RETENTION_BACKFILL_TASK = 'data_retention_backfill'

export const dataRetentionApi = {
  get: (workspaceId: string) =>
    api.get<DataRetentionView>(`/api/dataRetention.get?workspace_id=${encodeURIComponent(workspaceId)}`),

  preview: (workspaceId: string, policy: DataRetentionPolicy) =>
    api.post<DataRetentionPreview>('/api/dataRetention.preview', { workspace_id: workspaceId, policy }),

  set: (workspaceId: string, policy: DataRetentionPolicy) =>
    api.post<{ data_retention: DataRetentionSettings }>('/api/dataRetention.set', {
      workspace_id: workspaceId,
      policy
    }),

  cancelPending: (workspaceId: string) =>
    api.post<{ data_retention: DataRetentionSettings }>('/api/dataRetention.cancelPending', {
      workspace_id: workspaceId
    }),

  exportRuns: (workspaceId: string) => api.download('/api/dataRetention.exportRuns', { workspace_id: workspaceId }),

  /** The workspace's backfill task, if one was queued (workspaces upgraded to 45). */
  backfillTask: async (workspaceId: string): Promise<Task | null> => {
    const result = await taskApi.list({ workspace_id: workspaceId, type: DATA_RETENTION_BACKFILL_TASK, limit: 1 })
    return result.tasks[0] ?? null
  },

  /** Runs a failed backfill again (tasks.reset only restarts recurring tasks). */
  retryBackfill: (workspaceId: string) =>
    api.post<{ success: boolean }>('/api/dataRetention.retryBackfill', { workspace_id: workspaceId }),

  getDefaults: () => api.get<RetentionDefaultsView>('/api/dataRetention.getDefaults'),

  setDefaults: (categories: RetentionDays) =>
    api.post<{ stored: RetentionDefaults; defaults: Record<RetentionCategory, number> }>(
      '/api/dataRetention.setDefaults',
      { categories }
    )
}
