import { useMemo } from 'react'
import { keepPreviousData, useInfiniteQuery } from '@tanstack/react-query'
import {
  broadcastApi,
  getStatusesForGroup,
  type Broadcast,
  type BroadcastStatus
} from '../../services/api/broadcast'
import { isLiveRow } from './broadcastStatus'

export const PAGE_SIZE = 25
export const LIVE_REFETCH_MS = 30_000

export interface BroadcastsListOptions {
  workspaceId: string
  statusGroup: string
  q: string
  /** A followed row is sending by its verdict: the list refetches as for a live status. */
  verdictLive?: boolean
}

/**
 * L1: the broadcasts of a status group and search, 25 at a time. The key holds the group and the
 * search only, so opening or walking the detail never refetches the list. It refetches every
 * 30 s while a loaded row is live, so pills and test countdowns follow.
 */
export function useBroadcastsList({ workspaceId, statusGroup, q, verdictLive }: BroadcastsListOptions) {
  const query = useInfiniteQuery({
    queryKey: ['broadcasts', workspaceId, statusGroup, q],
    queryFn: ({ pageParam }) =>
      broadcastApi.list({
        workspace_id: workspaceId,
        statuses: getStatusesForGroup(statusGroup),
        search: q || undefined,
        limit: PAGE_SIZE,
        offset: pageParam,
        with_templates: true,
        // The counts follow the search, not the page: asked once.
        with_status_counts: pageParam === 0 ? true : undefined
      }),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.length * PAGE_SIZE
      return loaded < last.total_count ? loaded : undefined
    },
    placeholderData: keepPreviousData,
    refetchInterval: (q) => {
      const rows = q.state.data?.pages.flatMap((p) => p.broadcasts ?? []) ?? []
      return verdictLive || rows.some((b) => isLiveRow(b)) ? LIVE_REFETCH_MS : false
    }
  })

  const rows = useMemo(() => {
    // A broadcast created between two page loads shifts the offsets: the same row can come back
    // on two pages.
    const seen = new Set<string>()
    const result: Broadcast[] = []
    for (const page of query.data?.pages ?? []) {
      for (const broadcast of page.broadcasts ?? []) {
        if (seen.has(broadcast.id)) continue
        seen.add(broadcast.id)
        result.push(broadcast)
      }
    }
    return result
  }, [query.data])

  const firstPage = query.data?.pages[0]
  const statusCounts: Partial<Record<BroadcastStatus, number>> | undefined = firstPage?.status_counts
  const total = firstPage?.total_count ?? 0

  return { ...query, rows, statusCounts, total }
}

/** The count of a group from `status_counts`; a status absent counts 0. */
export function groupCount(
  counts: Partial<Record<BroadcastStatus, number>> | undefined,
  statuses: BroadcastStatus[] | undefined
): number | undefined {
  if (!counts) return undefined
  const keys = statuses ?? (Object.keys(counts) as BroadcastStatus[])
  return keys.reduce((sum, status) => sum + (counts[status] ?? 0), 0)
}
