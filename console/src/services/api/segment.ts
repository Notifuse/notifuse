import { api } from './client'
import { FormInstance } from 'antd'

// Utility types
export interface MapOfStrings {
  [key: string]: string
}

export interface MapOfInterfaces {
  [key: string]: any
}

// Segment types
export type SegmentStatus = 'active' | 'deleted' | 'building'

// Tree structure types
export type TreeNodeKind = 'branch' | 'leaf'
export type BooleanOperator = 'and' | 'or'
export type TableType = 'contacts' | 'contact_lists' | 'contact_timeline'

// Dimension filter types
export type FieldType = 'string' | 'number' | 'time'
export type FilterOperator =
  | 'is_set'
  | 'is_not_set'
  | 'equals'
  | 'not_equals'
  | 'contains'
  | 'not_contains'
  | 'starts_with'
  | 'ends_with'
  | 'gt'
  | 'gte'
  | 'lt'
  | 'lte'
  | 'in_date_range'
  | 'not_in_date_range'
  | 'before_date'
  | 'after_date'
  | 'in_the_last_days'

// Contact list operators
export type ContactListOperator = 'in' | 'not_in'

// Timeline operators
export type CountOperator = 'at_least' | 'at_most' | 'exactly'
export type TimeframeOperator =
  | 'anytime'
  | 'in_date_range'
  | 'before_date'
  | 'after_date'
  | 'in_the_last_days'

export interface DimensionFilter {
  field_name: string
  field_type: FieldType
  operator: FilterOperator
  string_values?: string[]
  number_values?: number[]
}

export interface ContactCondition {
  filters: DimensionFilter[]
}

export interface ContactListCondition {
  operator: ContactListOperator
  list_id: string
  status?: string
}

export interface ContactTimelineCondition {
  kind: string
  count_operator: CountOperator
  count_value: number
  timeframe_operator?: TimeframeOperator
  timeframe_values?: string[]
  filters?: DimensionFilter[]
}

export interface TreeNodeLeaf {
  table: TableType
  contact?: ContactCondition
  contact_list?: ContactListCondition
  contact_timeline?: ContactTimelineCondition
}

export interface TreeNodeBranch {
  operator: BooleanOperator
  leaves: TreeNode[]
}

export interface TreeNode {
  kind: TreeNodeKind
  branch?: TreeNodeBranch
  leaf?: TreeNodeLeaf
}

export interface Segment {
  id: string
  name: string
  color: string
  parent_segment_id?: string
  tree: TreeNode
  timezone: string
  version: number
  status: SegmentStatus
  generated_sql?: string
  generated_args?: Record<string, any>
  db_created_at: string
  db_updated_at: string
  users_count?: number
}

// Editing state type
export interface EditingNodeLeaf extends TreeNode {
  is_new?: boolean // flag to remove node from tree if cancel a new condition without confirm
  path: string
  key: number
}

// Field definition type
export interface FieldDefinition {
  table: string
  field_name: string
  definition: FieldSchema
}

// Type alias for backwards compatibility
export type FieldTypeValue = FieldType

// Operator type (used by UI components)
export type Operator =
  | 'is_set'
  | 'is_not_set'
  | 'equals'
  | 'not_equals'
  | 'contains'
  | 'not_contains'
  | 'gt'
  | 'gte'
  | 'lt'
  | 'lte'
  | 'in_date_range'
  | 'not_in_date_range'
  | 'before_date'
  | 'after_date'
  | 'in_the_last_days'

// Field type renderer interfaces
export interface FieldTypeRenderer {
  operators: IOperator[]
  render: (filter: DimensionFilter, schema: FieldSchema) => JSX.Element
  renderFormItems: (fieldType: FieldTypeValue, fieldName: string, form: FormInstance) => JSX.Element
}

export interface FieldTypeRendererDictionary {
  [key: string]: FieldTypeRenderer
}

export interface IOperator {
  type: Operator
  label: string
  render: (filter: DimensionFilter) => JSX.Element
  renderFormItems: (fieldType: FieldTypeValue, fieldName: string, form: FormInstance) => JSX.Element
}

// Schema interfaces for segmentation
export interface TableSchema {
  name: string
  title: string
  description?: string
  icon?: any // FontAwesome icon definition
  fields: { [key: string]: FieldSchema }
}

export interface FieldSchema {
  name: string
  title: string
  description?: string
  type: 'string' | 'number' | 'time' | 'boolean'
  shown?: boolean
  options?: FieldOption[] // For predefined values (e.g., countries, languages)
}

export interface FieldOption {
  value: string | number
  label: string
}

export interface List {
  id: string
  name: string
}

// API Request/Response types
export interface CreateSegmentRequest {
  workspace_id: string
  id: string
  name: string
  color: string
  tree: TreeNode
  timezone: string
}

export interface GetSegmentsRequest {
  workspace_id: string
  with_count?: boolean // Whether to include contact counts (can be expensive)
}

export interface GetSegmentRequest {
  workspace_id: string
  id: string
}

export interface UpdateSegmentRequest {
  workspace_id: string
  id: string
  name: string
  color: string
  tree: TreeNode
  timezone: string
}

export interface DeleteSegmentRequest {
  workspace_id: string
  id: string
}

export interface RebuildSegmentRequest {
  workspace_id: string
  segment_id: string
}

export interface PreviewSegmentRequest {
  workspace_id: string
  tree: TreeNode
  limit?: number
}

export interface GetSegmentContactsRequest {
  workspace_id: string
  segment_id: string
  limit?: number
  offset?: number
}

export interface GetSegmentsResponse {
  segments: Segment[]
}

export interface GetSegmentResponse {
  segment: Segment
}

export interface CreateSegmentResponse {
  segment: Segment
}

export interface UpdateSegmentResponse {
  segment: Segment
}

export interface DeleteSegmentResponse {
  success: boolean
}

export interface RebuildSegmentResponse {
  success: boolean
  message: string
}

export interface PreviewSegmentResponse {
  emails: string[] // Deprecated: Always empty for privacy/performance. Use getSegmentContacts for actual emails.
  total_count: number
  limit: number
  generated_sql: string
  sql_args: any[]
}

export interface GetSegmentContactsResponse {
  emails: string[]
  limit: number
  offset: number
}

/**
 * List all segments for a workspace
 */
export async function listSegments(req: GetSegmentsRequest): Promise<GetSegmentsResponse> {
  const params = new URLSearchParams({
    workspace_id: req.workspace_id
  })

  if (req.with_count !== undefined) {
    params.append('with_count', req.with_count ? 'true' : 'false')
  }

  return api.get<GetSegmentsResponse>(`/api/segments.list?${params.toString()}`)
}

/**
 * Get a single segment by ID
 */
export async function getSegment(req: GetSegmentRequest): Promise<GetSegmentResponse> {
  const params = new URLSearchParams({
    workspace_id: req.workspace_id,
    id: req.id
  })

  return api.get<GetSegmentResponse>(`/api/segments.get?${params.toString()}`)
}

/**
 * Create a new segment
 */
export async function createSegment(req: CreateSegmentRequest): Promise<CreateSegmentResponse> {
  return api.post<CreateSegmentResponse>('/api/segments.create', req)
}

/**
 * Update an existing segment
 */
export async function updateSegment(req: UpdateSegmentRequest): Promise<UpdateSegmentResponse> {
  return api.post<UpdateSegmentResponse>('/api/segments.update', req)
}

/**
 * Delete a segment
 */
export async function deleteSegment(req: DeleteSegmentRequest): Promise<DeleteSegmentResponse> {
  return api.post<DeleteSegmentResponse>('/api/segments.delete', req)
}

/**
 * Rebuild a segment (recalculate membership)
 */
export async function rebuildSegment(req: RebuildSegmentRequest): Promise<RebuildSegmentResponse> {
  return api.post<RebuildSegmentResponse>('/api/segments.rebuild', req)
}

/**
 * Preview contacts that would match a segment tree
 */
export async function previewSegment(req: PreviewSegmentRequest): Promise<PreviewSegmentResponse> {
  return api.post<PreviewSegmentResponse>('/api/segments.preview', req)
}

/**
 * Get contacts belonging to a segment
 */
export async function getSegmentContacts(
  req: GetSegmentContactsRequest
): Promise<GetSegmentContactsResponse> {
  const params = new URLSearchParams({
    workspace_id: req.workspace_id,
    segment_id: req.segment_id
  })

  if (req.limit !== undefined) {
    params.append('limit', req.limit.toString())
  }

  if (req.offset !== undefined) {
    params.append('offset', req.offset.toString())
  }

  return api.get<GetSegmentContactsResponse>(`/api/segments.contacts?${params.toString()}`)
}
