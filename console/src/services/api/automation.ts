import { api } from './client'
import { TreeNode } from './segment'
import type { EmailProviderKind } from './workspace'

// Email provider kinds that can ingest inbound replies (used to gate the automation
// "Exit on reply" feature). Keep in sync with the backend: a provider belongs here once
// it has a reply parser + a matchable stored Message-ID (and route registration).
// More ESPs will be added as their inbound support ships.
export const INBOUND_REPLY_PROVIDER_KINDS: EmailProviderKind[] = ['mailgun', 'ses']

// AWS regions where Amazon SES supports inbound email RECEIVING (receipt rules). SES inbound
// only works in these; in a sending-only region stop-on-reply can't be provisioned. Mirror of
// the backend sesReceivingRegions allowlist (internal/service/ses_service.go) — keep in sync.
export const SES_RECEIVING_REGIONS: ReadonlySet<string> = new Set([
  'us-east-1',
  'us-east-2',
  'us-west-1',
  'us-west-2',
  'af-south-1',
  'ap-southeast-3',
  'ap-south-1',
  'ap-northeast-3',
  'ap-northeast-2',
  'ap-southeast-1',
  'ap-southeast-2',
  'ap-northeast-1',
  'ca-central-1',
  'eu-central-1',
  'eu-west-1',
  'eu-west-2',
  'eu-south-1',
  'eu-west-3',
  'eu-north-1',
  'il-central-1',
  'me-south-1',
  'sa-east-1'
])

// supportsInboundReplies reports whether an integration can ingest inbound replies (gating the
// automation "Exit on reply" feature). SES additionally requires a receiving-capable region.
export function supportsInboundReplies(provider: {
  kind: EmailProviderKind
  ses?: { region?: string }
}): boolean {
  if (!INBOUND_REPLY_PROVIDER_KINDS.includes(provider.kind)) return false
  if (provider.kind === 'ses') return SES_RECEIVING_REGIONS.has(provider.ses?.region || '')
  return true
}

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
  // Email events (sent/delivered omitted: no matching timeline kind, triggers never fire)
  'email.opened',
  'email.clicked',
  'email.bounced',
  'email.complained',
  'email.unsubscribed',
  // A link clicked for the first time in an email (optional link_url, broadcast_id, template_id)
  'email.link_clicked',
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
  link_url?: string // For email.link_clicked: the clicked URL contains this (case-insensitive)
  broadcast_id?: string // For email.link_clicked: the email was sent by this broadcast
  template_id?: string // For email.link_clicked: the email was sent with this template
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

// Every node config carries an optional author-facing description, shown under the node's title on
// the canvas so a flow can be read without opening each node. It lives in the config bag because the
// backend stores that verbatim (AutomationNode.Config is an untyped map), so no schema change.
export interface NodeConfigBase {
  description?: string
}

export interface DelayNodeConfig extends NodeConfigBase {
  duration: number
  unit: 'minutes' | 'hours' | 'days'
}

export interface EmailNodeConfig extends NodeConfigBase {
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

export interface BranchNodeConfig extends NodeConfigBase {
  paths: BranchPath[]
  default_path_id: string
}

export interface FilterNodeConfig extends NodeConfigBase {
  conditions?: TreeNode
  continue_node_id: string
  exit_node_id: string
}

export interface AddToListNodeConfig extends NodeConfigBase {
  list_id: string
  status: 'active' | 'pending'
  metadata?: Record<string, unknown>
}

export interface RemoveFromListNodeConfig extends NodeConfigBase {
  list_id: string
}

export interface ListStatusBranchNodeConfig extends NodeConfigBase {
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

export interface ABTestNodeConfig extends NodeConfigBase {
  variants: ABTestVariant[]
}

export interface WebhookNodeConfig extends NodeConfigBase {
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
  exit_on_reply?: boolean // Stop the journey when the contact replies (requires inbound reply setup at the ESP)
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
  /** Adds the journey counts of each listed automation, exact, since its creation. */
  with_journey_counts?: boolean
}

/** The journeys of an automation by status, since its creation. */
export interface JourneyCounts {
  active: number
  completed: number
  exited: number
  failed: number
  /** Every journey, one in a status not listed above included. */
  total: number
}

export interface ListAutomationsResponse {
  automations: Automation[]
  total: number
  /** By automation id, when asked with with_journey_counts. */
  journey_counts?: Record<string, JourneyCounts>
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

/** The email counters of an email step, from its executions joined to the messages they sent. */
export interface NodeEmailStats {
  /** Completed executions that produced a message to send. */
  executions: number
  /** Emails still in the queue, without a message yet. */
  queued: number
  /** Executions with neither a message nor a queued email (a send cancelled by an exit on reply). */
  not_sent: number
  sent: number
  delivered: number
  opened: number
  opens_tracked: number
  clicked: number
  clicks_tracked: number
  failed: number
  bounced: number
  complained: number
  unsubscribed: number
}

/** The results of one step of an automation (automations.nodeStats). */
export interface NodeStats {
  node_type: NodeType
  /** Distinct journeys that reached the step: a retry is not a new visit. */
  visits: number
  completed: number
  /** Each failed attempt, retries included. */
  failed_attempts: number
  skipped: number
  skip_reasons: Record<string, number>
  /** Completed executions that recorded an error (a list step whose list was missing). */
  errors: number
  /** Journeys that ended failed on this step; the period bounds their entry in the automation. */
  failed_journeys: number
  /** Journeys on the step now, whatever the period. */
  in_progress: number
  waiting: number
  retrying: number
  /** Completed executions by path: yes/no, a list status key, a variant id, a path id, default, none. */
  branches: Record<string, number>
  /** Only when the member may read the message history. */
  email?: NodeEmailStats
}

export interface NodeStatsRequest {
  workspace_id: string
  automation_id: string
  /** RFC 3339 instants, together or absent (all time). */
  since?: string
  until?: string
}

export interface NodeStatsResponse {
  automation_id: string
  since?: string
  until?: string
  nodes: Record<string, NodeStats>
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
    if (params.with_journey_counts) searchParams.append('with_journey_counts', 'true')

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

  // Both answer { success: true }, without the automation.
  activate: async (params: ActivateAutomationRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/automations.activate', params)
  },

  pause: async (params: PauseAutomationRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/automations.pause', params)
  },

  getNodeExecutions: async (params: GetNodeExecutionsRequest): Promise<GetNodeExecutionsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('automation_id', params.automation_id)
    searchParams.append('email', params.email)

    return api.get<GetNodeExecutionsResponse>(`/api/automations.nodeExecutions?${searchParams.toString()}`)
  },

  nodeStats: async (params: NodeStatsRequest): Promise<NodeStatsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('automation_id', params.automation_id)
    if (params.since && params.until) {
      searchParams.append('since', params.since)
      searchParams.append('until', params.until)
    }
    return api.get<NodeStatsResponse>(`/api/automations.nodeStats?${searchParams.toString()}`)
  }
}
