import { useQuery } from '@tanstack/react-query'
import { listMessages } from '../../services/api/messages_history'
import { DASHBOARD_REFRESH_MS } from './dashboardClient'

export interface LatestMessage {
  /** The latest message's creation; null when the workspace has none, undefined while it loads. */
  latestAt: string | null | undefined
  isError: boolean
}

/**
 * Whether the workspace ever sent anything, from its latest message: messages.list without a filter
 * is ordered by creation, so one row is one index probe. Any message counts, a template test or a
 * double opt-in confirmation included. Asked again each minute until the first one exists.
 */
export function useLatestMessage(workspaceId: string, enabled: boolean): LatestMessage {
  const query = useQuery({
    queryKey: ['dashboard', 'latest-message', workspaceId],
    queryFn: () => listMessages(workspaceId, { limit: 1 }),
    enabled,
    refetchInterval: (current) => (current.state.data?.messages?.length ? false : DASHBOARD_REFRESH_MS)
  })
  const latest = query.data?.messages?.[0]
  return {
    latestAt: query.data ? (latest?.created_at ?? null) : undefined,
    // A failed refresh keeps what was read.
    isError: query.isError && !query.data
  }
}
