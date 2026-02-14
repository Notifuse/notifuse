import { api } from './client'
import { analyticsService } from './analytics'
import { TreeNode } from './segment'

// Automation status types
export type AutomationStatus = 'draft' | 'live' | 'paused'

// Trigger frequency types
export type TriggerFrequency = 'once' | 'every_time'

// Node types
export type NodeType =
  | 'trigger'
  | 'delay'
  | 'email'
  | 'branch'
  | 'filter'
  | 'add_to_list'
  | 'remove_from_list'
  | 'ab_test'
  | 'webhook'
  | 'list_status_branch'

// Contact automation status
export type ContactAutomationStatus = 'active' | 'completed' | 'exited' | 'failed'

// Node action types
export type NodeAction = 'entered' | 'processing' | 'completed' | 'failed' | 'skipped'

// Valid event kinds for automation triggers
export const VALID_EVENT_KINDS = [
  // Contact events
  'contact.created',
  'contact.updated',
  'contact.deleted',
  // List events (require list_id)
  'list.subscribed',
  'list.unsubscribed',
  'list.confirmed',
  'list.resubscribed',
  'list.bounced',
  'list.complained',
  'list.pending',
  'list.removed',
  // Segment events (require segment_id)
  'segment.joined',
  'segment.left',
  // Email events
  'email.sent',
  'email.delivered',
  'email.opened',
  'email.clicked',
  'email.bounced',
  'email.complained',
  'email.unsubscribed',
  // Custom events (require custom_event_name)
  'custom_event'
] as const

export type EventKind = (typeof VALID_EVENT_KINDS)[number]

// Trigger configuration
export interface TimelineTriggerConfig {
  event_kind: string
  list_id?: string // Required for list.* events
  segment_id?: string // Required for segment.* events
  custom_event_name?: string // Required for custom_event
  updated_fields?: string[] // For contact.updated: only trigger on these field changes
  conditions?: TreeNode
  frequency: TriggerFrequency
}

// Automation statistics
export interface AutomationStats {
  enrolled: number
  completed: number
  exited: number
  failed: number
}

// Node position for visual editor
export interface NodePosition {
  x: number
  y: number
}

// Node configuration types
export interface DelayNodeConfig {
  duration: number
  unit: 'minutes' | 'hours' | 'days'
}

export interface EmailNodeConfig {
  template_id: string
  integration_id?: string
  subject_override?: string
  from_override?: string
}

export interface BranchPath {
  id: string
  name: string
  conditions?: TreeNode
  next_node_id: string
}

export interface BranchNodeConfig {
  paths: BranchPath[]
  default_path_id: string
}

export interface FilterNodeConfig {
  description?: string
  conditions?: TreeNode
  continue_node_id: string
  exit_node_id: string
}

export interface AddToListNodeConfig {
  list_id: string
  status: 'active' | 'pending'
  metadata?: Record<string, unknown>
}

export interface RemoveFromListNodeConfig {
  list_id: string
}

export interface ListStatusBranchNodeConfig {
  list_id: string
  not_in_list_node_id: string
  active_node_id: string
  non_active_node_id: string
}

export interface ABTestVariant {
  id: string
  name: string
  weight: number
  next_node_id: string
}

export interface ABTestNodeConfig {
  variants: ABTestVariant[]
}

export interface WebhookNodeConfig {
  url: string
  secret?: string // Optional Authorization Bearer token
}

// Union type for node configs
export type NodeConfig =
  | DelayNodeConfig
  | EmailNodeConfig
  | BranchNodeConfig
  | FilterNodeConfig
  | AddToListNodeConfig
  | RemoveFromListNodeConfig
  | ListStatusBranchNodeConfig
  | ABTestNodeConfig
  | WebhookNodeConfig
  | Record<string, unknown> // For trigger nodes with no config

// Automation node
export interface AutomationNode {
  id: string
  automation_id: string
  type: NodeType
  config: Record<string, unknown>
  next_node_id?: string
  position: NodePosition
  created_at: string
}

// Main automation interface
export interface Automation {
  id: string
  workspace_id: string
  name: string
  status: AutomationStatus
  list_id: string
  trigger?: TimelineTriggerConfig
  trigger_sql?: string
  root_node_id: string
  nodes: AutomationNode[]
  stats?: AutomationStats
  created_at: string
  updated_at: string
  deleted_at?: string
}

// Contact automation tracking
export interface ContactAutomation {
  id: string
  automation_id: string
  contact_email: string
  current_node_id?: string
  status: ContactAutomationStatus
  exit_reason?: string
  entered_at: string
  scheduled_at?: string
  context?: Record<string, unknown>
  retry_count: number
  last_error?: string
  last_retry_at?: string
  max_retries: number
}

// Node execution log
export interface NodeExecution {
  id: string
  contact_automation_id: string
  node_id: string
  node_type: NodeType
  action: NodeAction
  entered_at: string
  completed_at?: string
  duration_ms?: number
  output?: Record<string, unknown>
  error?: string
}

// API Request types
export interface ListAutomationsRequest {
  workspace_id: string
  status?: AutomationStatus[]
  list_id?: string
  limit?: number
  offset?: number
}

export interface ListAutomationsResponse {
  automations: Automation[]
  total: number
}

export interface GetAutomationRequest {
  workspace_id: string
  automation_id: string
}

export interface GetAutomationResponse {
  automation: Automation
}

export interface CreateAutomationRequest {
  workspace_id: string
  automation: Automation
}

export interface UpdateAutomationRequest {
  workspace_id: string
  automation: Automation
}

export interface DeleteAutomationRequest {
  workspace_id: string
  automation_id: string
}

export interface ActivateAutomationRequest {
  workspace_id: string
  automation_id: string
}

export interface PauseAutomationRequest {
  workspace_id: string
  automation_id: string
}

export interface GetNodeExecutionsRequest {
  workspace_id: string
  automation_id: string
  email: string
}

export interface GetNodeExecutionsResponse {
  contact_automation: ContactAutomation
  node_executions: NodeExecution[]
}

// Node stats for flow viewer
export interface AutomationNodeStats {
  node_id: string
  node_type: NodeType
  entered: number
  completed: number
  failed: number
  skipped: number
}

export interface GetNodeStatsRequest {
  workspace_id: string
  automation_id: string
}

export interface GetNodeStatsResponse {
  node_stats: Record<string, AutomationNodeStats>
}

// API client
export const automationApi = {
  list: async (params: ListAutomationsRequest): Promise<ListAutomationsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    if (params.status && params.status.length > 0) {
      params.status.forEach((s) => searchParams.append('status', s))
    }
    if (params.list_id) searchParams.append('list_id', params.list_id)
    if (params.limit) searchParams.append('limit', params.limit.toString())
    if (params.offset) searchParams.append('offset', params.offset.toString())

    return api.get<ListAutomationsResponse>(`/api/automations.list?${searchParams.toString()}`)
  },

  get: async (params: GetAutomationRequest): Promise<GetAutomationResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('automation_id', params.automation_id)

    return api.get<GetAutomationResponse>(`/api/automations.get?${searchParams.toString()}`)
  },

  create: async (params: CreateAutomationRequest): Promise<GetAutomationResponse> => {
    return api.post<GetAutomationResponse>('/api/automations.create', params)
  },

  update: async (params: UpdateAutomationRequest): Promise<GetAutomationResponse> => {
    return api.post<GetAutomationResponse>('/api/automations.update', params)
  },

  delete: async (params: DeleteAutomationRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/automations.delete', params)
  },

  activate: async (params: ActivateAutomationRequest): Promise<GetAutomationResponse> => {
    return api.post<GetAutomationResponse>('/api/automations.activate', params)
  },

  pause: async (params: PauseAutomationRequest): Promise<GetAutomationResponse> => {
    return api.post<GetAutomationResponse>('/api/automations.pause', params)
  },

  getNodeExecutions: async (params: GetNodeExecutionsRequest): Promise<GetNodeExecutionsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('automation_id', params.automation_id)
    searchParams.append('email', params.email)

    return api.get<GetNodeExecutionsResponse>(`/api/automations.nodeExecutions?${searchParams.toString()}`)
  },

  getNodeStats: async (params: GetNodeStatsRequest): Promise<GetNodeStatsResponse> => {
    const response = await analyticsService.query(
      {
        schema: 'automation_node_executions',
        measures: ['count_entered', 'count_completed', 'count_failed', 'count_skipped'],
        dimensions: ['node_id', 'node_type'],
        filters: [
          {
            member: 'automation_id',
            operator: 'equals',
            values: [params.automation_id]
          }
        ]
      },
      params.workspace_id
    )

    // Transform analytics response (array) to map format expected by components
    const node_stats: Record<string, AutomationNodeStats> = {}
    for (const row of response.data) {
      const nodeId = row.node_id as string
      node_stats[nodeId] = {
        node_id: nodeId,
        node_type: row.node_type as NodeType,
        entered: (row.count_entered as number) || 0,
        completed: (row.count_completed as number) || 0,
        failed: (row.count_failed as number) || 0,
        skipped: (row.count_skipped as number) || 0
      }
    }
    return { node_stats }
  }
}
