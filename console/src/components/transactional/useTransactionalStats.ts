import { keepPreviousData, useQueries, useQuery } from '@tanstack/react-query'
import { ApiError } from '../../services/api/client'
import { templatesApi, type Template } from '../../services/api/template'
import type { TransactionalPeriod } from './transactionalSearch'
import type { TransactionalPeriods } from './period'
import { transactionalAnalytics } from './transactionalClient'
import {
  lastSentByNotification,
  STATS_MEASURES,
  statsByNotification,
  type NotificationStats
} from './transactionalStats'
import type { TemplateState } from './PreviewPanel'

/** How far back « Last sent » looks. */
export const LAST_SENT_DAYS = 60

interface StatsOptions {
  workspaceId: string
  /** Sorted, so a reload of the list in the same order starts nothing. */
  ids: string[]
  period: TransactionalPeriod
  periods: TransactionalPeriods
  timezone: string
  /** Rights known and message_history.read granted. */
  enabled: boolean
  /** SMTP knows no delivery: the previous period's delivery rate is not asked. */
  smtp: boolean
}

export interface StatsResult<T> {
  data?: T
  isLoading: boolean
  /** Failed with nothing loaded; a failed refresh keeps the last figures. */
  isError: boolean
  error?: unknown
  /** The figures shown are those of the previous key, kept while the new one loads. */
  isPlaceholderData: boolean
}

interface QueryLike<T> {
  data?: T
  isLoading: boolean
  isError: boolean
  error: unknown
  isPlaceholderData: boolean
}

const result = <T>(query: QueryLike<T>): StatsResult<T> => ({
  data: query.data,
  isLoading: query.isLoading,
  isError: query.isError && query.data === undefined,
  error: query.error ?? undefined,
  isPlaceholderData: query.isPlaceholderData
})

export interface TransactionalStats {
  /** T2: the window's statistics by notification. */
  current: StatsResult<Map<string, NotificationStats>> & {
    /** The period of the figures shown, the old one while a switch loads. */
    period?: TransactionalPeriod
  }
  /** T3: sent and delivered of the previous window. */
  previous: StatsResult<Map<string, NotificationStats>>
  /** T4: when each notification last sent successfully. */
  lastSent: StatsResult<Map<string, string | null>>
}

/** The statistics of the list (T2, T3 and T4 of spec 7), one row per notification. */
export function useTransactionalStats({
  workspaceId,
  ids,
  period,
  periods,
  timezone,
  enabled,
  smtp
}: StatsOptions): TransactionalStats {
  const active = enabled && ids.length > 0
  const byId = { member: 'transactional_notification_id', operator: 'in' as const, values: ids }

  const current = useCurrentStats({ workspaceId, ids, period, periods, timezone, enabled: active })

  const previous = useQuery({
    queryKey: ['transactional-stats', workspaceId, 'previous', period, periods.current.endDay, ids],
    queryFn: async () => {
      const response = await transactionalAnalytics.query(
        {
          schema: 'message_history',
          measures: [STATS_MEASURES.sent, STATS_MEASURES.delivered],
          dimensions: ['transactional_notification_id'],
          timezone,
          filters: [
            byId,
            {
              member: 'created_at',
              operator: 'inDateRange',
              values: [periods.previous.startUtc, periods.previous.endUtc]
            }
          ]
        },
        workspaceId
      )
      return statsByNotification(response.data)
    },
    enabled: active && !smtp,
    staleTime: 10 * 60_000
  })

  const lastSent = useQuery({
    queryKey: ['transactional-last-sent', workspaceId, ids],
    queryFn: async () => {
      const since = new Date(Date.now() - LAST_SENT_DAYS * 24 * 3600_000).toISOString()
      const response = await transactionalAnalytics.query(
        {
          schema: 'message_history',
          measures: ['last_sent_at'],
          dimensions: ['transactional_notification_id'],
          timezone,
          filters: [byId, { member: 'created_at', operator: 'gte', values: [since] }]
        },
        workspaceId
      )
      return lastSentByNotification(response.data)
    },
    enabled: active,
    staleTime: 60_000,
    refetchOnWindowFocus: true
  })

  return {
    current: { ...result({ ...current, data: current.data?.stats }), period: current.data?.period },
    previous: result(previous),
    lastSent: result(lastSent)
  }
}

type CurrentStatsOptions = Omit<StatsOptions, 'smtp'>

/**
 * T2: the window's statistics of `ids`, with the period they belong to. The list and the sheet of a
 * notification outside it share this key and this query, so an entry one of them made fits the other.
 */
export function useCurrentStats({ workspaceId, ids, period, periods, timezone, enabled }: CurrentStatsOptions) {
  // The last day enters the key: at midnight the window slides, without a request at each render.
  return useQuery({
    queryKey: ['transactional-stats', workspaceId, period, periods.current.endDay, ids],
    queryFn: async () => {
      const response = await transactionalAnalytics.query(
        {
          schema: 'message_history',
          measures: Object.values(STATS_MEASURES),
          dimensions: ['transactional_notification_id'],
          timezone,
          filters: [
            { member: 'transactional_notification_id', operator: 'in', values: ids },
            {
              member: 'created_at',
              operator: 'inDateRange',
              values: [periods.current.startUtc, periods.current.endUtc]
            }
          ]
        },
        workspaceId
      )
      return { period, stats: statsByNotification(response.data) }
    },
    enabled,
    staleTime: 60_000,
    // Focus refetch is off for the console: a monitoring screen reads its figures again on return.
    refetchOnWindowFocus: true,
    placeholderData: keepPreviousData
  })
}

interface TemplatesOptions {
  workspaceId: string
  templateIds: string[]
  /** Rights known; templates.read decides between `forbidden` and a request. */
  ready: boolean
  canRead: boolean
}

/** T5: each distinct template of the list, read once, as a state the cells and the sheet show. */
export function useTemplateStates({
  workspaceId,
  templateIds,
  ready,
  canRead
}: TemplatesOptions): Map<string, TemplateState> {
  const queries = useQueries({
    queries: templateIds.map((id) => ({
      queryKey: ['template', workspaceId, id],
      queryFn: () => templatesApi.get({ workspace_id: workspaceId, id }),
      enabled: ready && canRead,
      staleTime: 5 * 60_000,
      // A missing template answers 404: asking again finds nothing more.
      retry: (count: number, error: unknown) => !(error instanceof ApiError && error.status === 404) && count < 2
    }))
  })

  const states = new Map<string, TemplateState>()
  templateIds.forEach((id, index) => {
    const query = queries[index]
    let state: TemplateState
    if (!ready) state = { status: 'loading' }
    else if (!canRead) state = { status: 'forbidden' }
    else if (query.data?.template) state = { status: 'ok', template: query.data.template as Template }
    else if (query.error instanceof ApiError && query.error.status === 404) state = { status: 'notFound' }
    else if (query.isError) state = { status: 'error', error: query.error, retry: () => void query.refetch() }
    else state = { status: 'loading' }
    states.set(id, state)
  })
  return states
}
