import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { KpiStrip, useNumberFormat, type KpiItem, type KpiTone } from '../layout'
import dayjs from '../../lib/dayjs'
import { broadcastApi, type Broadcast } from '../../services/api/broadcast'
import type { Workspace } from '../../services/api/types'
import type { UserPermissions } from '../../services/api/workspace'
import { computeComparisonRange, computeDateRange, resolveRange } from '../web_analytics/lib/dates'
import { formatValue } from '../web_analytics/lib/format'
import { BROADCAST_CONVERSION_WINDOW_DAYS } from '../web_analytics/lib/broadcastLinks'
import { TrackedTooltipTitle } from '../common/TrackedTooltipTitle'
import { campaignDayRows, campaignVisitsQuery, periodTotalsQuery, readPeriodTotals } from './broadcastAnalytics'
import { runBroadcastQuery } from './broadcastQueryClient'
import { campaignsOf, kpiTraffic } from './attribution'
import { canReadWebAnalytics, workspaceTimezone } from './access'
import { CouldntLoad } from './CouldntLoad'

const FIVE_MINUTES = 5 * 60_000
const TEN_MINUTES = 10 * 60_000
const ONE_HOUR = 60 * 60_000
const CAMPAIGNS_PAGE = 100

/** The rate of a numerator over a denominator, or null when the denominator is 0. */
const rate = (numerator: number, denominator: number) => (denominator > 0 ? numerator / denominator : null)

/** Every broadcast started since `startedAfter` (L6), 100 at a time. */
async function listStartedSince(workspaceId: string, startedAfter: string): Promise<Broadcast[]> {
  const all: Broadcast[] = []
  for (let offset = 0; ; offset += CAMPAIGNS_PAGE) {
    const page = await broadcastApi.list({
      workspace_id: workspaceId,
      started_after: startedAfter,
      limit: CAMPAIGNS_PAGE,
      offset
    })
    all.push(...(page.broadcasts ?? []))
    if (offset + CAMPAIGNS_PAGE >= page.total_count) return all
  }
}

export interface BroadcastKpisProps {
  workspace: Workspace
  permissions: UserPermissions | null
}

/** L4 to L7: the broadcast emails of the last 30 days against the 30 before, and their traffic. */
export function BroadcastKpis({ workspace, permissions }: BroadcastKpisProps) {
  const { t } = useLingui()
  const { int, percent, points } = useNumberFormat()
  const timezone = workspaceTimezone(workspace)
  const canReadHistory = Boolean(permissions?.message_history?.read)
  const canReadWeb = canReadWebAnalytics(workspace, permissions)

  const { current, previous, campaignsSince } = useMemo(() => {
    const range = computeDateRange('previous_30_days', timezone)
    const comparison = computeComparisonRange(range, 'previous_period')!
    return {
      current: resolveRange(range, timezone),
      previous: resolveRange(comparison, timezone),
      campaignsSince: range.start.subtract(BROADCAST_CONVERSION_WINDOW_DAYS, 'day').toISOString()
    }
  }, [timezone])

  const currentQuery = useQuery({
    queryKey: ['broadcast-kpis', workspace.id, current.startUtc, current.endUtc],
    queryFn: () => runBroadcastQuery(periodTotalsQuery(current, timezone), workspace.id),
    enabled: canReadHistory,
    staleTime: FIVE_MINUTES
  })
  const previousQuery = useQuery({
    queryKey: ['broadcast-kpis', workspace.id, previous.startUtc, previous.endUtc],
    queryFn: () => runBroadcastQuery(periodTotalsQuery(previous, timezone), workspace.id),
    enabled: canReadHistory,
    staleTime: ONE_HOUR
  })
  const campaignsQuery = useQuery({
    queryKey: ['broadcast-campaigns', workspace.id, campaignsSince],
    queryFn: () => listStartedSince(workspace.id, campaignsSince),
    enabled: canReadHistory && canReadWeb,
    staleTime: TEN_MINUTES
  })
  const campaigns = campaignsOf(campaignsQuery.data ?? [])
  const trafficQuery = useQuery({
    queryKey: ['broadcast-traffic', workspace.id, current.startUtc, [...campaigns].sort()],
    queryFn: () =>
      runBroadcastQuery(
        campaignVisitsQuery({
          campaigns,
          start: dayjs(current.startUtc),
          end: dayjs(current.endUtc),
          timezone,
          withGoals: true
        }),
        workspace.id
      ),
    enabled: canReadHistory && canReadWeb && campaigns.length > 0,
    staleTime: TEN_MINUTES
  })

  if (!canReadHistory) return null

  const failed = <CouldntLoad />
  const now = readPeriodTotals(currentQuery.data?.data ?? [])
  const before = previousQuery.data ? readPeriodTotals(previousQuery.data.data ?? []) : null

  const rateCell = (
    key: string,
    label: string,
    numerator: number,
    tracked: number,
    beforeNumerator?: number,
    beforeTracked?: number
  ): KpiItem => {
    const value = rate(numerator, tracked)
    const prior = beforeNumerator !== undefined && beforeTracked !== undefined ? rate(beforeNumerator, beforeTracked) : null
    const delta = value !== null && prior !== null ? (value - prior) * 100 : null
    const tone: KpiTone = delta === null || Math.abs(delta) < 0.05 ? 'neutral' : delta > 0 ? 'positive' : 'negative'
    return {
      key,
      label,
      value: currentQuery.isError ? failed : value === null ? '—' : percent(value),
      sub: delta === null ? undefined : points(delta),
      tone,
      hint:
        tracked < now.sent && tracked > 0 ? <TrackedTooltipTitle title={label} tracked={tracked} /> : undefined
    }
  }

  const unsubscribed = now.unsubscribed
  const bounced = now.bounced
  const items: KpiItem[] = [
    {
      key: 'sent',
      label: t`Emails sent`,
      value: currentQuery.isError ? failed : int(now.sent),
      sub: t`Last 30 days`
    },
    rateCell('opens', t`Open rate`, now.opened, now.opensTracked, before?.opened, before?.opensTracked),
    rateCell('clicks', t`Click rate`, now.clicked, now.clicksTracked, before?.clicked, before?.clicksTracked),
    {
      key: 'unsubscribes',
      label: t`Unsubscribes`,
      value: currentQuery.isError ? failed : rate(unsubscribed, now.sent) === null ? '—' : percent(rate(unsubscribed, now.sent)!),
      sub: t`${plural(unsubscribed, { one: '# contact', other: '# contacts' })}`
    },
    {
      key: 'bounces',
      label: t`Bounces`,
      value: currentQuery.isError ? failed : rate(bounced, now.sent) === null ? '—' : percent(rate(bounced, now.sent)!),
      sub: t`${plural(bounced, { one: '# address', other: '# addresses' })}`
    }
  ]

  if (canReadWeb) {
    const traffic = kpiTraffic(
      campaignsQuery.data ?? [],
      campaignDayRows(trafficQuery.data?.data ?? []),
      { startDay: current.startDay, endDay: current.endDay },
      timezone
    )
    const conversions = traffic.conversions
    const valueText = formatValue(traffic.value, 'currency')
    const trafficFailed = campaignsQuery.isError || trafficQuery.isError
    items.push({
      key: 'traffic',
      label: t`Website traffic`,
      value: trafficFailed ? failed : int(traffic.sessions),
      sub: t`${plural(conversions, { one: '# conversion', other: '# conversions' })} · ${valueText}`,
      hint: t`Sessions from the UTM campaign of a broadcast, during the 7 days after it was sent. Broadcasts sharing a campaign count a session once here, and once each in the list.`
    })
  }

  return (
    <KpiStrip
      items={items}
      loading={currentQuery.isLoading || (canReadWeb && (campaignsQuery.isLoading || trafficQuery.isLoading) && campaigns.length > 0)}
    />
  )
}
