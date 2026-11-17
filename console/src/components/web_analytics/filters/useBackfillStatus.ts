import { useQuery } from '@tanstack/react-query'
import { webAnalyticsService } from '../../../services/api/web_analytics'

export const BACKFILL_QUERY_KEY = 'web-analytics-backfill'

/**
 * The workspace's last backfill. Polled while a run is under way; the rules tab and the rule
 * editor share the key, so the editor reads the tab's cache.
 */
export function useBackfillStatus(workspaceId: string) {
  return useQuery({
    queryKey: [BACKFILL_QUERY_KEY, workspaceId],
    queryFn: () => webAnalyticsService.backfillStatus(workspaceId),
    refetchInterval: (query) => {
      const current = query.state.data?.status
      // A long backfill parks in "paused" between runtime slices, so it has to
      // keep polling or the progress bar freezes mid-run.
      return current === 'pending' || current === 'running' || current === 'paused' ? 2000 : false
    }
  })
}
