import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import {
  SESAccessDeniedError,
  verifySESTenant,
  type SESCredentialsRef,
  type SESTenantVerification
} from '../../../services/api/ses'

export interface SESTenantVerifyInput {
  workspaceId: string
  /** A saved integration: the server reads its stored credentials. */
  integrationId?: string
  /** Credentials typed into a new integration. */
  region?: string
  accessKey?: string
  secretKey?: string
  tenantName?: string
  configurationSetName?: string
}

export type SESTenantVerifyOutcome =
  | { tenant: string; verification: SESTenantVerification }
  | { tenant: string; denied: true }
  | { tenant: string; error: string }

const TENANT_PATTERN = /^[A-Za-z0-9_-]{1,64}$/

/**
 * Checks a tenant typed by hand before anything is sent through it (/api/ses.verifyTenant): an
 * unknown tenant, or one whose configuration set is not associated with it, makes SES reject every
 * send. The answer is kept for the tenant it was asked for, so editing the name drops it.
 */
export function useSESTenantVerify(input: SESTenantVerifyInput) {
  const { t } = useLingui()
  const [outcome, setOutcome] = useState<SESTenantVerifyOutcome | null>(null)
  const [verifying, setVerifying] = useState(false)

  const tenant = input.tenantName?.trim() ?? ''
  const credentials: SESCredentialsRef | null = input.integrationId
    ? { workspace_id: input.workspaceId, integration_id: input.integrationId }
    : input.region && input.accessKey && input.secretKey
      ? {
          workspace_id: input.workspaceId,
          region: input.region,
          access_key: input.accessKey,
          secret_key: input.secretKey
        }
      : null
  const canVerify = Boolean(credentials) && TENANT_PATTERN.test(tenant)

  const verify = async () => {
    if (!credentials || !canVerify) return
    setVerifying(true)
    try {
      const verification = await verifySESTenant({
        ...credentials,
        tenant_name: tenant,
        ...(input.configurationSetName ? { configuration_set_name: input.configurationSetName } : {})
      })
      setOutcome({ tenant, verification })
    } catch (error) {
      if (error instanceof SESAccessDeniedError) setOutcome({ tenant, denied: true })
      else setOutcome({ tenant, error: error instanceof Error && error.message ? error.message : t`Failed to verify the tenant` })
    } finally {
      setVerifying(false)
    }
  }

  return { verify, verifying, canVerify, outcome: outcome && outcome.tenant === tenant ? outcome : null }
}
