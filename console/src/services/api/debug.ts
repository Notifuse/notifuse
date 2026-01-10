import { api } from './client'

// Verification Types
export interface VerificationSummary {
  total_databases: number
  passed_databases: number
  failed_databases: number
  total_issues: number
}

export interface FunctionVerification {
  name: string
  exists: boolean
}

export interface TriggerVerification {
  name: string
  table_name: string
  exists: boolean
  function?: string
}

export interface ColumnVerification {
  name: string
  expected_type: string
  actual_type: string
  matches: boolean
}

export interface IndexVerification {
  name: string
  exists: boolean
}

export interface TableVerification {
  name: string
  exists: boolean
  columns?: ColumnVerification[]
  indexes?: IndexVerification[]
  missing_columns?: string[]
}

export interface DatabaseVerification {
  status: 'passed' | 'failed' | 'error'
  error?: string
  tables: TableVerification[]
  trigger_functions: FunctionVerification[]
  triggers: TriggerVerification[]
  missing_tables?: string[]
}

export interface WorkspaceVerification extends DatabaseVerification {
  workspace_id: string
  workspace_name: string
}

export interface SchemaVerificationResult {
  verified_at: string
  system_db: DatabaseVerification
  workspace_dbs: WorkspaceVerification[]
  summary: VerificationSummary
}

// Repair Types
export interface SchemaRepairRequest {
  workspace_ids?: string[]
  repair_triggers: boolean
  repair_functions: boolean
}

export interface RepairSummary {
  total_workspaces: number
  successful_repairs: number
  failed_repairs: number
  functions_recreated: number
  triggers_recreated: number
}

export interface WorkspaceRepairResult {
  workspace_id: string
  workspace_name: string
  status: 'success' | 'partial' | 'failed'
  error?: string
  functions_recreated: string[]
  triggers_recreated: string[]
  functions_failed?: string[]
  triggers_failed?: string[]
}

export interface SchemaRepairResult {
  repaired_at: string
  workspace_dbs: WorkspaceRepairResult[]
  summary: RepairSummary
}

export const debugApi = {
  /**
   * Verify database schemas, triggers, and functions
   * Root user only
   */
  async verifySchema(): Promise<SchemaVerificationResult> {
    return api.get<SchemaVerificationResult>('/api/debug.verifySchema')
  },

  /**
   * Repair database triggers and functions
   * Root user only
   */
  async repairSchema(request: SchemaRepairRequest): Promise<SchemaRepairResult> {
    return api.post<SchemaRepairResult>('/api/debug.repairSchema', request)
  }
}
