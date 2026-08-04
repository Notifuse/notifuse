import { useEffect, useState } from 'react'
import {
  listSESTenants,
  listSESConfigurationSets,
  SESAccessDeniedError,
  type SESCredentialsRef
} from '../../services/api/ses'

export interface SESDiscoveryInput {
  /** Only fetch while the SES form is actually on screen. */
  active: boolean
  workspaceId?: string
  /** Set when editing a saved integration; its stored credentials are used server-side. */
  integrationId?: string | null
  /** Credentials typed into the create drawer, before anything has been saved. */
  region?: string
  accessKey?: string
  secretKey?: string
  /** Debounce before calling AWS. Tests set this to 0. */
  debounceMs?: number
}

export interface SESDiscoveryState {
  tenantOptions: { value: string }[]
  configurationSetOptions: { value: string }[]
  /**
   * True when the credentials lack the optional listing permissions. Not an error state: the
   * fields stay typeable and saving still works.
   */
  denied: boolean
  loading: boolean
}

interface DiscoveryResult {
  /** Which AWS account these names came from. Results are only shown for the current target. */
  target: string
  tenants: { value: string }[]
  configurationSets: { value: string }[]
  denied: boolean
}

const EMPTY: { value: string }[] = []

/**
 * Identifies the AWS account being inspected. Results are tagged with it and only surfaced when
 * the tag still matches, so one integration can never show another's tenant names — picking one
 * of those would write a tenant that does not exist in the target account, after which every
 * send from it fails. The secret key is deliberately not part of the identity: it does not
 * change which account is being looked at.
 */
function discoveryTarget({
  active,
  workspaceId,
  integrationId,
  region,
  accessKey
}: SESDiscoveryInput): string {
  if (!active || !workspaceId) return ''
  if (integrationId) return `integration:${workspaceId}:${integrationId}`
  if (region && accessKey) return `inline:${workspaceId}:${region}:${accessKey}`
  return ''
}

/**
 * Offers the names that actually exist in the integration's AWS account.
 *
 * The typed credentials are *inputs*, not values read once: reading them when the drawer opens
 * would mean the pickers never populate for a new integration, because nothing is filled in at
 * that moment.
 */
export function useSESDiscovery(input: SESDiscoveryInput): SESDiscoveryState {
  const { workspaceId, integrationId, region, accessKey, secretKey, debounceMs = 500 } = input

  const [result, setResult] = useState<DiscoveryResult | null>(null)
  const [loadingTarget, setLoadingTarget] = useState('')

  const target = discoveryTarget(input)

  useEffect(() => {
    if (!target || !workspaceId) return

    const ref: SESCredentialsRef | null = integrationId
      ? { workspace_id: workspaceId, integration_id: integrationId }
      : region && accessKey && secretKey
        ? { workspace_id: workspaceId, region, access_key: accessKey, secret_key: secretKey }
        : null

    if (!ref) return

    let cancelled = false

    // Every call is a real AWS request and the secret key is typed one character at a time.
    const timer = setTimeout(() => {
      setLoadingTarget(target)

      Promise.all([listSESTenants(ref), listSESConfigurationSets(ref)])
        .then(([tenantsResponse, setsResponse]) => {
          if (cancelled) return
          setResult({
            target,
            tenants: (tenantsResponse.tenants ?? []).map((tenant) => ({ value: tenant.name })),
            configurationSets: (setsResponse.configuration_sets ?? []).map((name) => ({
              value: name
            })),
            denied: false
          })
        })
        .catch((error) => {
          if (cancelled) return
          setResult({
            target,
            tenants: [],
            configurationSets: [],
            denied: error instanceof SESAccessDeniedError
          })
        })
        .finally(() => {
          if (cancelled) return
          setLoadingTarget('')
        })
    }, debounceMs)

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [target, workspaceId, integrationId, region, accessKey, secretKey, debounceMs])

  // Anything fetched for a different target is simply not this target's data. Deriving it here
  // rather than clearing it in an effect means there is no window in which the previous
  // account's names are on screen.
  const current = result && result.target === target ? result : null

  return {
    tenantOptions: current?.tenants ?? EMPTY,
    configurationSetOptions: current?.configurationSets ?? EMPTY,
    denied: current?.denied ?? false,
    loading: loadingTarget !== '' && loadingTarget === target
  }
}
