import { FormInstance } from 'antd'
export interface MapOfStrings {
  [key: string]: string
}

export interface MapOfInterfaces {
  [key: string]: any
}

export interface Segment {
  id: string
  name: string
  color: string
  parent_segment_id?: string
  tree: TreeNode
  timezone: string
  version: number
  status: 'active' | 'deleted' | 'building'
  generated_sql?: string
  generated_args?: any[]
  db_created_at: string
  db_updated_at: string
  // joined server-side
  users_count: number
}
export interface TreeNode {
  kind: 'branch' | 'leaf'
  branch?: TreeNodeBranch
  leaf?: TreeNodeLeaf
}

export interface TreeNodeBranch {
  operator: 'and' | 'or'
  leaves: TreeNode[]
}

export interface TreeNodeLeaf {
  table: string // contacts, contact_lists, contact_timeline
  contact?: ContactCondition // Used by contacts table
  contact_list?: ContactListCondition // Used by contact_lists table
  contact_timeline?: ContactTimelineCondition // Used by contact_timeline table
}

export interface DimensionFilter {
  field_name: string
  field_type: FieldTypeValue
  operator: Operator
  string_values?: string[]
  number_values?: number[]
}

export interface ContactCondition {
  filters: DimensionFilter[]
}

export interface ContactListCondition {
  operator: 'in' | 'not_in'
  list_id: string
  status?: string
}

export interface ContactTimelineCondition {
  kind: string
  count_operator: 'at_least' | 'at_most' | 'exactly'
  count_value: number
  timeframe_operator?:
    | 'anytime'
    | 'in_date_range'
    | 'before_date'
    | 'after_date'
    | 'in_the_last_days'
  timeframe_values?: string[]
  filters?: DimensionFilter[]
}

// current editing condition
export interface EditingNodeLeaf extends TreeNode {
  is_new?: boolean // flag to remove node from tree if cancel a new condition without confirm
  path: string
  key: number
}

export interface FieldDefinition {
  table: string
  field_name: string
  definition: FieldSchema
}

export type FieldTypeValue = 'string' | 'number' | 'time'

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

export type Operator =
  | 'is_set'
  | 'is_not_set'
  | 'equals'
  | 'not_equals'
  | 'contains'
  | 'not_contains'
  // | 'starts_with'
  // | 'not_starts_with'
  // | 'ends_with'
  // | 'not_ends_with'
  | 'gt'
  | 'gte'
  | 'lt'
  | 'lte'
  | 'in_date_range'
  | 'not_in_date_range'
  | 'before_date'
  | 'after_date'

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
