import { useQuery } from '@tanstack/react-query'
import { listsApi } from '../../services/api/list'
import { listSegments, type Segment } from '../../services/api/segment'

/** While a segment is being computed, segments.list is read again this often. */
export const SEGMENT_BUILD_REFETCH_MS = 15000

export function segmentsRefetchInterval(segments: Pick<Segment, 'status'>[] | undefined): number | false {
  return segments?.some((segment) => segment.status === 'building') ? SEGMENT_BUILD_REFETCH_MS : false
}

/**
 * The workspace's segments with their users_count (a join and a count per segment): kept 60 s,
 * read again every 15 s only while one is being computed.
 */
export function useSegmentsQuery(workspaceId: string, enabled = true) {
  return useQuery({
    queryKey: ['segments', workspaceId, { with_count: true }],
    queryFn: () => listSegments({ workspace_id: workspaceId, with_count: true }),
    enabled,
    staleTime: 60000,
    refetchInterval: (query) => segmentsRefetchInterval(query.state.data?.segments)
  })
}

/** The workspace's lists, for names and actions. */
export function useListsQuery(workspaceId: string, enabled = true) {
  return useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled
  })
}

/**
 * The lists with their active subscribers (one GROUP BY over contact_lists). Its own key: the
 * many invalidations of 'lists' (bulk actions, the sheet) would recount every list. Invalidated
 * only by a list created, edited or deleted, an import to a list, and the page's Refresh.
 */
export function useListsWithCountsQuery(workspaceId: string, enabled = true) {
  return useQuery({
    queryKey: ['lists-with-counts', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId, with_counts: true }),
    enabled,
    staleTime: 60000
  })
}
