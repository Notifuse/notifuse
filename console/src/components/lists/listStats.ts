import { useQuery } from '@tanstack/react-query'
import { listsApi } from '../../services/api/list'
import type { ListStats } from '../../services/api/types'

export type ListStatus = 'active' | 'pending' | 'unsubscribed' | 'bounced' | 'complained'

export const LIST_STATUSES: ListStatus[] = [
  'active',
  'pending',
  'unsubscribed',
  'bounced',
  'complained'
]

export const LIST_STAT_KEYS: Record<ListStatus, keyof ListStats> = {
  active: 'total_active',
  pending: 'total_pending',
  unsubscribed: 'total_unsubscribed',
  bounced: 'total_bounced',
  complained: 'total_complained'
}

/** lists.stats of one list, read again every minute. */
export function useListStats(workspaceId: string, listId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: ['list-stats', workspaceId, listId],
    queryFn: () => listsApi.stats({ workspace_id: workspaceId, list_id: listId! }),
    enabled: enabled && !!listId,
    refetchInterval: 60000
  })
}
