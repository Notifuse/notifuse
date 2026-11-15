import { useQuery } from '@tanstack/react-query'
import { automationApi } from '../../services/api/automation'
import { lastDays } from './lastDays'
import type { StatsPeriod } from './stats/nodeStatsState'

const ONE_MINUTE = 60_000

export interface UseNodeStatsOptions {
  workspaceId: string
  automationId: string
  timezone: string
  period: StatsPeriod
  /** Saved, and some contact ever entered: otherwise there is nothing to count. */
  available: boolean
  /** The results are on screen (stats on, or a step's panel open). */
  watched: boolean
  live: boolean
}

/** E3: each step's results over the last 7 or 30 days or all time; refreshed every minute while live and watched. */
export function useNodeStats({ workspaceId, automationId, timezone, period, available, watched, live }: UseNodeStatsOptions) {
  const range = period === 'all' ? null : lastDays(timezone, period === 'last7' ? 7 : 30)
  return useQuery({
    queryKey: ['automation-node-stats', workspaceId, automationId, range ? range.startUtc : 'all'],
    queryFn: () =>
      automationApi.nodeStats({
        workspace_id: workspaceId,
        automation_id: automationId,
        ...(range ? { since: range.startUtc, until: range.endUtc } : {})
      }),
    enabled: !!workspaceId && available && watched,
    staleTime: ONE_MINUTE,
    refetchInterval: live && watched ? ONE_MINUTE : false
  })
}
