import { api } from './client'

/**
 * Which AWS credentials a call should use. Two modes on purpose: a saved integration (the edit
 * drawer, whose secret-key field is deliberately blank), or credentials typed into the create
 * drawer before anything has been saved.
 */
export interface SESCredentialsRef {
  workspace_id: string
  integration_id?: string
  region?: string
  access_key?: string
  secret_key?: string
}

export interface SESTenant {
  name: string
  id?: string
  arn?: string
  created_at?: string
}

export interface ListSESTenantsResponse {
  tenants: SESTenant[] | null
  has_more: boolean
}

export interface ListSESConfigurationSetsResponse {
  configuration_sets: string[] | null
  has_more: boolean
}

export interface SESTenantVerification {
  tenant_name: string
  exists: boolean
  configuration_set_associated: boolean
  configuration_set_name?: string
  /** 'TENANT' or 'ACCOUNT'. ACCOUNT means bounces still hit the shared suppression list. */
  suppression_scope?: string
  /** 'ENABLED' | 'REINSTATED' | 'DISABLED'. DISABLED means SES paused this tenant. */
  sending_status?: string
  fix_command?: string
}

export interface SESTenantProvisionResult {
  tenant_name: string
  created: boolean
  suppression_scoped: boolean
  /** SES rejects a send whose configuration set is not associated with the named tenant. */
  configuration_set_associated: boolean
  associated?: string[]
  unverified_senders?: string[]
  missing_permissions?: string[]
  fix_commands?: string[]
  /** AWS has the tenant (and is billing for it) but Notifuse could not record it. Retry is safe. */
  provisioned_but_unsaved?: boolean
  sending_status?: string
}

/**
 * Raised when the AWS credentials lack an optional IAM permission. Callers must treat this as
 * "offer a plain text input" rather than as a failure: every tenant feature is designed to
 * degrade without these permissions, and showing an error here would be misleading.
 */
export class SESAccessDeniedError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'SESAccessDeniedError'
  }
}

function isAccessDenied(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const candidate = error as { code?: string; status?: number; response?: { code?: string } }
  return candidate.code === 'ses_access_denied' || candidate.response?.code === 'ses_access_denied'
}

async function callSES<T>(path: string, request: object): Promise<T> {
  try {
    return await api.post<T>(path, request)
  } catch (error) {
    if (isAccessDenied(error)) {
      throw new SESAccessDeniedError(
        error instanceof Error ? error.message : 'Missing AWS permission'
      )
    }
    throw error
  }
}

/** List the SES tenants in the integration's region. */
export async function listSESTenants(
  request: SESCredentialsRef
): Promise<ListSESTenantsResponse> {
  return callSES<ListSESTenantsResponse>('/api/ses.listTenants', request)
}

/** List the SES configuration sets in the integration's region. */
export async function listSESConfigurationSets(
  request: SESCredentialsRef
): Promise<ListSESConfigurationSetsResponse> {
  return callSES<ListSESConfigurationSetsResponse>('/api/ses.listConfigurationSets', request)
}

/**
 * Check whether a tenant can actually be sent through. "The configuration set exists" is a
 * different question: an unassociated set makes every send fail.
 */
export async function verifySESTenant(
  request: SESCredentialsRef & { tenant_name: string; configuration_set_name?: string }
): Promise<SESTenantVerification> {
  return callSES<SESTenantVerification>('/api/ses.verifyTenant', request)
}

/**
 * Provision managed tenant isolation. Creates a billable AWS resource, so callers must confirm
 * with the operator first. Idempotent: safe to retry after a partial result.
 */
export async function enableSESTenantIsolation(request: {
  workspace_id: string
  integration_id: string
}): Promise<SESTenantProvisionResult> {
  return callSES<SESTenantProvisionResult>('/api/ses.enableTenantIsolation', request)
}
