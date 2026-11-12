import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { analyticsService } from '../../../services/api/analytics'
import { contactTimelineApi, type TimelineEntityType } from '../../../services/api/contact_timeline'
import { listMessages } from '../../../services/api/messages_history'
import { webAnalyticsService, type ContactVisitsFilter } from '../../../services/api/web_analytics'

export type ActivityFilter = 'all' | 'emails' | 'lists' | 'profile' | 'events' | 'web'

/** The entity_types each activity filter sends to timeline.list (none for All). */
export const ACTIVITY_ENTITY_TYPES: Record<ActivityFilter, TimelineEntityType[] | undefined> = {
  all: undefined,
  emails: ['message_history'],
  lists: ['contact_list', 'contact_segment'],
  profile: ['contact'],
  events: ['custom_event', 'inbound_webhook_event', 'webhook_event', 'automation'],
  web: ['web_session', 'web_page']
}

export const TIMELINE_PAGE_SIZE = 20
export const VISITS_PAGE_SIZE = 20
export const MESSAGES_PAGE_SIZE = 10

/**
 * One activity filter of a contact, page by page. The All filter is also what « Last activity »
 * reads: same key, same options, so both share one cache entry.
 */
export function useContactTimeline(workspaceId: string, email: string, filter: ActivityFilter) {
  return useInfiniteQuery({
    queryKey: ['contact_timeline', workspaceId, email, filter],
    queryFn: ({ pageParam }) =>
      contactTimelineApi.list({
        workspace_id: workspaceId,
        email,
        limit: TIMELINE_PAGE_SIZE,
        cursor: pageParam,
        entity_types: ACTIVITY_ENTITY_TYPES[filter]
      }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor || undefined
  })
}

/**
 * The contact's web sessions for one filter, 20 at a time. The summary comes with the first page
 * only and covers all sessions whatever the filter; the Site visits KPI and the tab's counts read
 * it from the All filter's first page.
 */
export function useContactVisits(
  workspaceId: string,
  email: string,
  filter: ContactVisitsFilter,
  enabled: boolean
) {
  return useInfiniteQuery({
    queryKey: ['contact-visits', workspaceId, email, filter],
    queryFn: ({ pageParam }) =>
      webAnalyticsService.contactVisits({
        workspace_id: workspaceId,
        email,
        filter,
        limit: VISITS_PAGE_SIZE,
        cursor: pageParam
      }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor || undefined,
    enabled
  })
}

/** The contact's messages, page by page, for the Messages tab. */
export function useContactMessages(workspaceId: string, email: string, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: ['message_history', workspaceId, email],
    queryFn: ({ pageParam }) =>
      listMessages(workspaceId, { contact_email: email, limit: MESSAGES_PAGE_SIZE, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor || undefined,
    enabled
  })
}

const toNumber = (value: unknown): number => {
  const parsed = typeof value === 'number' ? value : parseFloat(String(value ?? ''))
  return Number.isFinite(parsed) ? parsed : 0
}

export interface ContactMessageStats {
  sent: number
  sentLast30Days: number
  opened: number
  opensTracked: number
  clicked: number
  clicksTracked: number
  /** Every message of the contact, all channels and failures included: the Messages tab's count. */
  messages: number
}

const MEASURES = [
  'count_sent_emails',
  'count_opened',
  'count_opens_tracked',
  'count_clicked',
  'count_clicks_tracked',
  'count'
]

/** The 30 days before `now`, as the ISO instants an inDateRange filter takes. */
export function last30DaysRange(now: Date): [string, string] {
  const from = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000)
  return [from.toISOString(), now.toISOString()]
}

/** analytics.query on message_history for one contact: since always, and over the last 30 days. */
export function useContactMessageStats(workspaceId: string, email: string, enabled: boolean) {
  return useQuery({
    queryKey: ['contact-message-stats', workspaceId, email],
    queryFn: async (): Promise<ContactMessageStats> => {
      const byContact = { member: 'contact_email', operator: 'equals' as const, values: [email] }
      const [all, recent] = await Promise.all([
        analyticsService.query(
          { schema: 'message_history', measures: MEASURES, dimensions: [], filters: [byContact] },
          workspaceId
        ),
        analyticsService.query(
          {
            schema: 'message_history',
            measures: ['count_sent_emails'],
            dimensions: [],
            filters: [
              byContact,
              { member: 'sent_at', operator: 'inDateRange', values: last30DaysRange(new Date()) }
            ]
          },
          workspaceId
        )
      ])
      const row = all.data[0] ?? {}
      return {
        sent: toNumber(row.count_sent_emails),
        sentLast30Days: toNumber(recent.data[0]?.count_sent_emails),
        opened: toNumber(row.count_opened),
        opensTracked: toNumber(row.count_opens_tracked),
        clicked: toNumber(row.count_clicked),
        clicksTracked: toNumber(row.count_clicks_tracked),
        messages: toNumber(row.count)
      }
    },
    enabled
  })
}
