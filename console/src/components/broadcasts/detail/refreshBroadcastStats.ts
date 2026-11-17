import type { QueryClient } from '@tanstack/react-query'

/** The queries of a broadcast's detail: the broadcast, its stats, task, test results, variations and links. */
export const BROADCAST_DETAIL_QUERY_KEYS = [
  'broadcast',
  'broadcast-stats',
  'task',
  'testResults',
  'broadcast-variation-stats',
  'broadcast-link-stats'
] as const

/** Reads every figure of a broadcast's detail again, for its drawer's refresh button. */
export function refreshBroadcastStats(queryClient: QueryClient, workspaceId: string, broadcastId: string) {
  for (const key of BROADCAST_DETAIL_QUERY_KEYS) {
    queryClient.invalidateQueries({ queryKey: [key, workspaceId, broadcastId] })
  }
}
