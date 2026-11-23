import { api } from './client'

// The DKIM keys of a workspace's domains. Notifuse signs the emails sent through SMTP
// integrations with the key of the sender's domain once its DNS record is published.

export type DKIMKeyStatus = 'pending' | 'published' | 'mismatch'
export type DKIMCheckResult = 'ok' | 'not_found' | 'mismatch' | 'dns_error'

export interface DKIMDNSRecord {
  type: string
  name: string
  value: string
}

export interface DKIMDomainUser {
  integration_id: string
  integration_name: string
}

export interface DKIMDomain {
  domain: string
  selector: string
  status: DKIMKeyStatus
  dns_record: DKIMDNSRecord
  created_at: string
  verified_at?: string
  last_checked_at?: string
  last_check_error?: string
  /** Failed checks counted against a published key; it stops signing at 3. */
  consecutive_failures: number
  downgraded_at?: string
  downgrade_reason?: string
  used_by: DKIMDomainUser[]
}

export interface DKIMVerifyResult {
  domain: DKIMDomain
  check: { result: DKIMCheckResult; detail?: string }
  /** The key changed during the check: the stored state was left as it was. */
  stale_conflict?: boolean
}

/** A published key stops signing after this many failed checks, an hour apart. */
export const DKIM_DOWNGRADE_THRESHOLD = 3

export const dkimApi = {
  list: async (workspaceId: string): Promise<{ domains: DKIMDomain[] }> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', workspaceId)
    return api.get<{ domains: DKIMDomain[] }>(`/api/workspaces.listDKIMDomains?${searchParams.toString()}`)
  },
  create: async (workspaceId: string, domain: string): Promise<{ domain: DKIMDomain }> =>
    api.post<{ domain: DKIMDomain }>('/api/workspaces.createDKIMDomain', { workspace_id: workspaceId, domain }),
  regenerate: async (workspaceId: string, domain: string): Promise<{ domain: DKIMDomain }> =>
    api.post<{ domain: DKIMDomain }>('/api/workspaces.regenerateDKIMKey', { workspace_id: workspaceId, domain }),
  verify: async (workspaceId: string, domain: string): Promise<DKIMVerifyResult> =>
    api.post<DKIMVerifyResult>('/api/workspaces.verifyDKIMDomain', { workspace_id: workspaceId, domain }),
  remove: async (workspaceId: string, domain: string): Promise<{ success: boolean }> =>
    api.post<{ success: boolean }>('/api/workspaces.deleteDKIMDomain', { workspace_id: workspaceId, domain })
}

export const dkimQueryKeys = {
  list: (workspaceId: string) => ['dkim-domains', workspaceId] as const
}

/**
 * The ASCII, lower-case domain of an email address, as the server matches it: an
 * internationalized domain in its punycode form (bücher.de → xn--bcher-kva.de), which is how
 * the keys are stored. "" when the address has no valid domain.
 */
export function emailDomain(address: string): string {
  const at = address.lastIndexOf('@')
  if (at < 0) return ''
  const domain = address
    .slice(at + 1)
    .trim()
    .replace(/>$/, '')
    .replace(/\.$/, '')
  if (!domain) return ''
  try {
    return new URL(`http://${domain}`).hostname
  } catch {
    return ''
  }
}
