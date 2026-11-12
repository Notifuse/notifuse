import { useQuery } from '@tanstack/react-query'
import type { AnalyticsQuery } from '../../../services/api/analytics'
import type { Automation } from '../../../services/api/automation'
import type { Workspace } from '../../../services/api/types'
import { toNumber } from '../../web_analytics/lib/format'
import { runBroadcastQuery } from '../../broadcasts/broadcastQueryClient'
import { workspaceTimezone } from '../../broadcasts/access'
import { hasEmailStep } from '../journeyRates'
import { last30Days } from '../last30Days'

const TEN_MINUTES = 10 * 60_000

/** A2: opens over tracked opens by automation, over the last 30 days. */
export function openRatesQuery(ids: string[], range: { startUtc: string; endUtc: string }, timezone: string): AnalyticsQuery {
  return {
    schema: 'message_history',
    measures: ['count_opened', 'count_opens_tracked'],
    dimensions: ['automation_id'],
    timezone,
    filters: [
      { member: 'automation_id', operator: 'in', values: ids },
      { member: 'channel', operator: 'equals', values: ['email'] },
      { member: 'created_at', operator: 'inDateRange', values: [range.startUtc, range.endUtc] }
    ]
  }
}

export interface OpenRates {
  /** By automation id; null when nothing tracked was sent over the period. */
  rates: Map<string, number | null>
  loading: boolean
  error: boolean
}

/** The open rate of each automation with an email step; no request without message history read. */
export function useAutomationOpenRates(workspace: Workspace | undefined, automations: Automation[], canReadHistory: boolean): OpenRates {
  const workspaceId = workspace?.id ?? ''
  const timezone = workspaceTimezone(workspace)
  const range = last30Days(timezone)
  const ids = automations.filter(hasEmailStep).map((a) => a.id).sort()

  const query = useQuery({
    queryKey: ['automation-open-rates', workspaceId, ids, range.startUtc],
    queryFn: () => runBroadcastQuery(openRatesQuery(ids, range, timezone), workspaceId),
    enabled: !!workspaceId && canReadHistory && ids.length > 0,
    staleTime: TEN_MINUTES
  })

  const rates = new Map<string, number | null>()
  for (const id of ids) rates.set(id, null)
  for (const row of query.data?.data ?? []) {
    const id = String(row.automation_id ?? '')
    const tracked = toNumber(row.count_opens_tracked)
    rates.set(id, tracked > 0 ? toNumber(row.count_opened) / tracked : null)
  }
  return { rates, loading: query.isLoading, error: query.isError }
}
