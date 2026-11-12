import { useEffect, useState } from 'react'
import { useQueries, useQuery } from '@tanstack/react-query'
import { broadcastApi, type AudienceSettings, type Broadcast } from '../../services/api/broadcast'
import type { UserPermissions } from '../../services/api/workspace'

export const MAX_ESTIMATES_IN_FLIGHT = 4
const ESTIMATE_STALE_MS = 60_000

// At most four estimates at once across the screen: each one counts a list on the database.
let inFlight = 0
const waiting: (() => void)[] = []

async function limited<T>(task: () => Promise<T>): Promise<T> {
  if (inFlight >= MAX_ESTIMATES_IN_FLIGHT) await new Promise<void>((resolve) => waiting.push(resolve))
  inFlight++
  try {
    return await task()
  } finally {
    inFlight--
    waiting.shift()?.()
  }
}

/** The cache key of an estimate: the same audience is counted once. */
export const estimateKey = (workspaceId: string, audience: AudienceSettings) => [
  'broadcast-estimate',
  workspaceId,
  audience.list ?? '',
  [...(audience.segments ?? [])].sort(),
  audience.exclude_unsubscribed
]

/** Whether the member may count this audience (lists, and segments when it has some). */
export const canEstimate = (audience: AudienceSettings, permissions?: UserPermissions | null) =>
  Boolean(audience.list) &&
  Boolean(permissions?.lists?.read) &&
  (!(audience.segments?.length) || Boolean(permissions?.segments?.read))

export const fetchEstimate = (workspaceId: string, audience: AudienceSettings) =>
  limited(() => broadcastApi.estimateAudience({ workspace_id: workspaceId, audience })).then((r) => r.count)

/** L8: the recipients each loaded draft or scheduled broadcast would have now. */
export function useAudienceEstimates(
  workspaceId: string,
  broadcasts: Broadcast[],
  permissions?: UserPermissions | null
): Map<string, { count?: number; error: boolean }> {
  const pending = broadcasts.filter((b) => b.status === 'draft' || b.status === 'scheduled')
  const results = useQueries({
    queries: pending.map((b) => ({
      queryKey: estimateKey(workspaceId, b.audience),
      queryFn: () => fetchEstimate(workspaceId, b.audience),
      enabled: Boolean(workspaceId) && canEstimate(b.audience, permissions),
      staleTime: ESTIMATE_STALE_MS
    }))
  })
  const map = new Map<string, { count?: number; error: boolean }>()
  pending.forEach((b, index) => map.set(b.id, { count: results[index]?.data, error: Boolean(results[index]?.isError) }))
  return map
}

/** For tests: forget the requests in flight. */
export function resetEstimateLimiter() {
  inFlight = 0
  waiting.length = 0
}

/** Time after the last audience change before the composer counts it again. */
export const COMPOSE_ESTIMATE_DELAY_MS = 400

export interface ComposeEstimate {
  /** The member may count this audience; "—" otherwise. */
  available: boolean
  count?: number
  /** Counting the audience as it is now (the delay, then the request). */
  counting: boolean
  error: boolean
}

/** The composer's "≈ n recipients", counted 400 ms after the last change of the audience. */
export function useComposeEstimate(
  workspaceId: string,
  audience: AudienceSettings,
  permissions?: UserPermissions | null
): ComposeEstimate {
  const available = canEstimate(audience, permissions)
  const key = JSON.stringify(estimateKey(workspaceId, audience))
  const [settled, setSettled] = useState<{ key: string; audience: AudienceSettings } | null>(
    available ? { key, audience } : null
  )
  useEffect(() => {
    if (!available) return
    if (settled?.key === key) return
    const timer = setTimeout(() => setSettled({ key, audience }), COMPOSE_ESTIMATE_DELAY_MS)
    return () => clearTimeout(timer)
    // The key carries the audience.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, available])
  const query = useQuery({
    queryKey: settled ? estimateKey(workspaceId, settled.audience) : ['broadcast-estimate', workspaceId, 'none'],
    queryFn: () => fetchEstimate(workspaceId, settled!.audience),
    enabled: available && !!settled,
    staleTime: ESTIMATE_STALE_MS
  })
  const current = available && settled?.key === key
  return {
    available,
    count: current ? query.data : undefined,
    counting: available && (!current || query.isFetching),
    error: current && query.isError
  }
}
