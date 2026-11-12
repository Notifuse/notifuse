import { useMemo } from 'react'
import { useQueries, useQuery } from '@tanstack/react-query'
import dayjs from '../../lib/dayjs'
import type { Broadcast } from '../../services/api/broadcast'
import { getBroadcastStats } from '../../services/api/messages_history'
import { taskApi, type Task } from '../../services/api/task'
import type { Workspace } from '../../services/api/types'
import { campaignDayRows, campaignVisitsQuery, pageStatsQuery, pageVisitsStart, statsByBroadcast, type BroadcastRowStats } from './broadcastAnalytics'
import { runBroadcastQuery } from './broadcastQueryClient'
import { broadcastVisits, campaignsOf } from './attribution'
import {
  deliveryVerdict,
  isFollowedRow,
  isLiveRow,
  progressStatsFrom,
  type DeliveryVerdict,
  type ProgressStats
} from './broadcastStatus'
import { LIVE_REFETCH_MS } from './useBroadcastsList'
import { workspaceTimezone } from './access'

const TEN_MINUTES = 10 * 60_000
const sortedIds = (rows: Broadcast[]) => rows.map((b) => b.id).sort()

export interface PageStatsOptions {
  workspace: Workspace | undefined
  rows: Broadcast[]
  canReadHistory: boolean
  canReadWeb: boolean
}

export interface PageStats {
  /** L2 by broadcast id. */
  stats: Map<string, BroadcastRowStats>
  statsLoading: boolean
  statsError: boolean
  /** Followed rows whose queue is loaded. */
  verdicts: Map<string, DeliveryVerdict>
  progress: Map<string, ProgressStats>
  tasks: Map<string, Task>
  /** L3 by broadcast id: null for "—". */
  visits: Map<string, number | null>
  visitsLoading: boolean
  visitsError: boolean
  /** A followed row sends by its verdict. */
  verdictLive: boolean
}

/**
 * The numbers of the loaded rows: L2a (live rows, every 30 s), L2b (finished rows, 10 min), L2c
 * (queue and task of followed rows) and L3 (website visits).
 */
export function useBroadcastPageStats({ workspace, rows, canReadHistory, canReadWeb }: PageStatsOptions): PageStats {
  const workspaceId = workspace?.id ?? ''
  const timezone = workspaceTimezone(workspace)
  const now = dayjs()
  const followed = rows.filter((b) => isFollowedRow(b, now))

  // L2c: one shared read of the last 100 tasks, each followed row picks its own.
  const tasksQuery = useQuery({
    queryKey: ['broadcast-tasks', workspaceId],
    queryFn: () => taskApi.list({ workspace_id: workspaceId, limit: 100 }),
    enabled: Boolean(workspaceId) && followed.length > 0,
    refetchInterval: followed.some((b) => isLiveRow(b)) ? LIVE_REFETCH_MS : false
  })
  const tasks = useMemo(() => {
    const map = new Map<string, Task>()
    for (const task of tasksQuery.data?.tasks ?? []) {
      if (task.broadcast_id && !map.has(task.broadcast_id)) map.set(task.broadcast_id, task)
    }
    return map
  }, [tasksQuery.data])

  const statsResults = useQueries({
    queries: followed.map((b) => ({
      queryKey: ['broadcast-stats', workspaceId, b.id],
      queryFn: () => getBroadcastStats(workspaceId, b.id),
      enabled: canReadHistory && Boolean(workspaceId),
      refetchInterval: isLiveRow(b) || b.status === 'processed' ? LIVE_REFETCH_MS : (false as const)
    }))
  })

  const { verdicts, progress } = useMemo(() => {
    const verdictMap = new Map<string, DeliveryVerdict>()
    const progressMap = new Map<string, ProgressStats>()
    followed.forEach((b, index) => {
      const data = statsResults[index]?.data
      if (!data) return
      const p = progressStatsFrom(data, tasks.get(b.id)?.state?.send_broadcast?.enqueued_count)
      progressMap.set(b.id, p)
      verdictMap.set(b.id, deliveryVerdict(p))
    })
    return { verdicts: verdictMap, progress: progressMap }
    // statsResults is a new array on each render; its data is what matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [followed.map((b) => b.id).join(), statsResults.map((r) => r.dataUpdatedAt).join(), tasks])

  const live = rows.filter((b) => isLiveRow(b, verdicts.get(b.id)))
  const done = rows.filter((b) => b.started_at && !live.includes(b))

  const liveIds = sortedIds(live)
  const doneIds = sortedIds(done)
  const liveStats = useQuery({
    queryKey: ['broadcast-page-stats', workspaceId, 'live', liveIds],
    queryFn: () => runBroadcastQuery(pageStatsQuery(liveIds, timezone), workspaceId),
    enabled: canReadHistory && liveIds.length > 0,
    refetchInterval: LIVE_REFETCH_MS
  })
  const doneStats = useQuery({
    queryKey: ['broadcast-page-stats', workspaceId, 'done', doneIds],
    queryFn: () => runBroadcastQuery(pageStatsQuery(doneIds, timezone), workspaceId),
    enabled: canReadHistory && doneIds.length > 0,
    staleTime: TEN_MINUTES
  })
  const stats = useMemo(
    () => new Map([...statsByBroadcast(doneStats.data?.data ?? []), ...statsByBroadcast(liveStats.data?.data ?? [])]),
    [doneStats.data, liveStats.data]
  )

  // L3: sessions by campaign and day since the oldest start of the page (90 days at most).
  const started = rows.filter((b) => b.started_at)
  const startedIds = sortedIds(started)
  const visitsStart = pageVisitsStart(started, timezone, now)
  const campaigns = campaignsOf(started)
  const visitsQuery = useQuery({
    queryKey: ['broadcast-page-visits', workspaceId, startedIds],
    queryFn: () =>
      runBroadcastQuery(
        campaignVisitsQuery({ campaigns, start: visitsStart!, end: dayjs(), timezone }),
        workspaceId
      ),
    enabled: canReadWeb && campaigns.length > 0 && visitsStart !== null,
    staleTime: TEN_MINUTES
  })
  const visits = useMemo(() => {
    const dayRows = campaignDayRows(visitsQuery.data?.data ?? [])
    const map = new Map<string, number | null>()
    for (const b of started) map.set(b.id, broadcastVisits(b, dayRows, timezone))
    return map
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visitsQuery.data, startedIds.join(), timezone])

  return {
    stats,
    statsLoading: liveStats.isLoading || doneStats.isLoading,
    statsError: liveStats.isError || doneStats.isError,
    verdicts,
    progress,
    tasks,
    visits,
    visitsLoading: visitsQuery.isLoading,
    visitsError: visitsQuery.isError,
    verdictLive: rows.some((b) => b.status === 'processed' && verdicts.get(b.id)?.kind === 'sending')
  }
}
