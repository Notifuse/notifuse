import { useEffect } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { KpiStrip, useNumberFormat, type KpiItem } from '../layout'
import { TrackedTooltipTitle } from '../common/TrackedTooltipTitle'
import type { StoredPermissions } from '../../services/api/permissions'
import type { Workspace } from '../../services/api/types'
import type { AnalyticsQuery } from '../../services/api/analytics'
import { buildWebQuery } from '../web_analytics/lib/query'
import type { ResolvedRange } from '../web_analytics/lib/types'
import { DASHBOARD_REFRESH_MS, dashboardAnalytics } from './dashboardClient'
import {
  computeDashboardKpis,
  emptyTotals,
  readTotals,
  toNumber,
  type MessageOrigin,
  type MessageTotals
} from './dashboardKpis'
import type { DashboardPeriods } from './dashboardPeriods'

interface DashboardKpisProps {
  workspace: Workspace
  permissions: StoredPermissions
  periods: DashboardPeriods
  timezone: string
  /** Totals of the period by origin, from the chart's series; undefined while they load. */
  totals?: Record<MessageOrigin, MessageTotals>
  seriesError: boolean
  onRetrySeries: () => void
}

const HOUR = 3_600_000

/** One count of a schema, as the engine returns it in the first row. */
function useCount(key: unknown[], query: AnalyticsQuery, workspaceId: string, enabled: boolean, stable = false) {
  return useQuery({
    queryKey: key,
    queryFn: async () => {
      const response = await dashboardAnalytics.query(query, workspaceId)
      return (response.data?.[0] ?? {}) as Record<string, unknown>
    },
    enabled,
    refetchInterval: stable ? false : DASHBOARD_REFRESH_MS,
    staleTime: stable ? HOUR : 0,
    placeholderData: keepPreviousData
  })
}

const messagesInRange = (range: ResolvedRange, timezone: string): AnalyticsQuery => ({
  schema: 'message_history',
  measures: ['count_opened', 'count_clicked', 'count_opens_tracked', 'count_clicks_tracked'],
  dimensions: [],
  timezone,
  filters: [
    {
      member: 'created_at',
      operator: 'inDateRange',
      values: [range.startUtc, range.endUtc]
    },
    { member: 'channel', operator: 'equals', values: ['email'] }
  ]
})

/** The six KPIs of the dashboard; a member sees only those their rights let the page ask for. */
export function DashboardKpis({
  workspace,
  permissions,
  periods,
  timezone,
  totals,
  seriesError,
  onRetrySeries
}: DashboardKpisProps) {
  const { t } = useLingui()
  const { int, percent, points, compact } = useNumberFormat()
  const ws = workspace.id
  const { current, previous, days } = periods

  const canContacts = Boolean(permissions.contacts?.read)
  const canWeb = Boolean(permissions.web_analytics?.read)
  const webConfigured = Boolean(workspace.settings?.web_analytics?.enabled)
  const canWriteWorkspace = Boolean(permissions.workspace?.write)

  const previousRates = useCount(
    ['dashboard', 'previous-rates', ws, timezone, previous.startUtc, previous.endUtc],
    messagesInRange(previous, timezone),
    ws,
    true,
    true
  )
  const contactsTotal = useCount(
    // Under ['contacts', ws], which the CSV import invalidates when it finishes.
    ['contacts', ws, 'dashboard-total'],
    { schema: 'contacts', measures: ['count'], dimensions: [], timezone },
    ws,
    canContacts
  )
  const contactsNew = useCount(
    ['contacts', ws, 'dashboard-new', timezone, current.startUtc, current.endUtc],
    {
      schema: 'contacts',
      measures: ['count'],
      dimensions: [],
      timezone,
      filters: [
        {
          member: 'created_at',
          operator: 'inDateRange',
          values: [current.startUtc, current.endUtc]
        }
      ]
    },
    ws,
    canContacts
  )
  const visitsQuery = (range: ResolvedRange) =>
    buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      range,
      timezone
    })
  const visits = useCount(
    ['dashboard', 'visits', ws, timezone, current.startUtc, current.endUtc],
    visitsQuery(current),
    ws,
    canWeb && webConfigured
  )
  const previousVisits = useCount(
    ['dashboard', 'visits', ws, timezone, previous.startUtc, previous.endUtc],
    visitsQuery(previous),
    ws,
    canWeb && webConfigured,
    true
  )

  // A failed request is shown only when nothing was loaded before: a failed refresh keeps the figures.
  const failedNow = (query: { isError: boolean; data?: unknown }) => query.isError && query.data === undefined
  const queries = [previousRates, contactsTotal, contactsNew, visits, previousVisits]
  const errors = queries.map((query) => (failedNow(query) ? query.error : null))
  useEffect(() => {
    // A warning, not an error: the strip already says what failed, and the page goes on.
    for (const error of errors) if (error) console.warn('Dashboard KPI failed to load', error)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- logged once per error object
  }, errors)

  const kpis = computeDashboardKpis({
    series: totals ?? {
      broadcast: emptyTotals(),
      transactional: emptyTotals(),
      automation: emptyTotals()
    },
    // The previous period of the period shown: after a period switch, the placeholder is the old one.
    previous: previousRates.data && !previousRates.isPlaceholderData ? readTotals(previousRates.data) : undefined,
    contactsTotal: toNumber(contactsTotal.data?.count),
    contactsNew: toNumber(contactsNew.data?.count),
    visits: visits.data ? toNumber(visits.data.sessions) : undefined,
    previousVisits: previousVisits.data ? toNumber(previousVisits.data.sessions) : undefined
  })

  const dash = '—'
  // The strip truncates a sub-line to its cell: the title keeps it readable on narrow screens.
  const titled = (text: string | undefined) => (text === undefined ? undefined : <span title={text}>{text}</span>)
  const rate = (value: number | null) => (value === null ? dash : percent(value))
  const failed = (retry: () => void): Pick<KpiItem, 'value' | 'sub' | 'tone'> => ({
    value: dash,
    sub: (
      <span>
        {t`Could not load`} ·{' '}
        <button type="button" className="cursor-pointer border-0 bg-transparent p-0 text-primary-600" onClick={retry}>
          {t`Retry`}
        </button>
      </span>
    ),
    tone: 'neutral'
  })
  const vsPrevious = (shown: string) =>
    t`${plural(days, { one: `${shown} vs prev. # d`, other: `${shown} vs prev. # d` })}`

  const rateItem = (key: 'open' | 'click', label: string, kpi: typeof kpis.openRate, pending: string): KpiItem => {
    if (seriesError) return { key, label, ...failed(onRetrySeries) }
    return {
      key,
      label,
      value: rate(kpi.rate),
      sub: titled(kpi.deltaPoints === null ? undefined : vsPrevious(points(kpi.deltaPoints))),
      tone: kpi.tone,
      hint: <TrackedTooltipTitle title={pending} tracked={kpi.partial ? kpi.tracked : undefined} />
    }
  }

  const items: KpiItem[] = []

  if (canContacts) {
    const contactsValue = (
      <Link
        to="/console/workspace/$workspaceId/contacts"
        params={{ workspaceId: ws }}
        className="text-inherit hover:text-primary-600"
      >
        {int(kpis.contacts.total)}
      </Link>
    )
    items.push(
      failedNow(contactsTotal) || failedNow(contactsNew)
        ? {
            key: 'contacts',
            label: t`Contacts`,
            ...failed(() => {
              void contactsTotal.refetch()
              void contactsNew.refetch()
            })
          }
        : {
            key: 'contacts',
            label: t`Contacts`,
            value: contactsValue,
            // Nothing until the count of new contacts arrives, rather than a false « +0 ».
            sub:
              contactsNew.data === undefined
                ? undefined
                : t`${plural(days, {
                    one: `+${int(kpis.contacts.added)} over # d`,
                    other: `+${int(kpis.contacts.added)} over # d`
                  })}`,
            tone: kpis.contacts.tone
          }
    )
  }

  items.push(
    seriesError
      ? { key: 'sent', label: t`Emails sent`, ...failed(onRetrySeries) }
      : {
          key: 'sent',
          label: t`Emails sent`,
          value: int(kpis.sent.total),
          sub: kpis.sent.broadcastShare === null ? undefined : t`${percent(kpis.sent.broadcastShare, 0)} broadcasts`
        }
  )

  const delivered = kpis.delivered
  items.push(
    seriesError
      ? { key: 'delivered', label: t`Delivered`, ...failed(onRetrySeries) }
      : {
          key: 'delivered',
          label: t`Delivered`,
          value: rate(delivered.rate),
          sub: titled(
            delivered.noDeliveryEvents
              ? t`No delivery events`
              : delivered.bounceRate === null
                ? undefined
                : t`Bounced ${percent(delivered.bounceRate)} · Spam ${percent(delivered.complaintRate ?? 0, 2)}`
          ),
          tone: delivered.tone,
          hint: t`Counted from your providers' delivery webhooks`
        }
  )

  items.push(rateItem('open', t`Open rate`, kpis.openRate, t`Recent emails may still be opened`))
  items.push(rateItem('click', t`Click rate`, kpis.clickRate, t`Recent emails may still be clicked`))

  if (canWeb && webConfigured) {
    items.push(
      failedNow(visits) || failedNow(previousVisits)
        ? {
            key: 'visits',
            label: t`Website visits`,
            ...failed(() => {
              void visits.refetch()
              void previousVisits.refetch()
            })
          }
        : {
            key: 'visits',
            label: t`Website visits`,
            value: (
              <Link
                to="/console/workspace/$workspaceId/web-analytics/$tab"
                params={{ workspaceId: ws, tab: 'dashboard' }}
                search={{
                  period: periods.preset,
                  comparison: 'previous_period'
                }}
                className="text-inherit hover:text-primary-600"
              >
                {kpis.visits.value === null ? dash : compact(kpis.visits.value)}
              </Link>
            ),
            sub: titled(
              kpis.visits.deltaRatio === null
                ? undefined
                : vsPrevious(formatSignedPercent(kpis.visits.deltaRatio, percent))
            ),
            tone: kpis.visits.tone
          }
    )
  } else if (canWeb && canWriteWorkspace) {
    items.push({
      key: 'visits',
      label: t`Website visits`,
      value: dash,
      sub: (
        <Link
          to="/console/workspace/$workspaceId/settings/$section"
          params={{ workspaceId: ws, section: 'web-analytics' }}
          className="text-primary-600"
        >
          {t`Set up web analytics`}
        </Link>
      )
    })
  }

  const loading =
    (!totals && !seriesError) ||
    (canContacts && (contactsTotal.isLoading || contactsNew.isLoading)) ||
    (canWeb && webConfigured && visits.isLoading)
  return <KpiStrip items={items} loading={loading} />
}

/** "+17.5 %" with a typographic minus, as the points of the rates. */
function formatSignedPercent(ratio: number, percent: (ratio: number, digits?: number) => string): string {
  const rounded = Math.round(ratio * 1000) / 1000
  const text = percent(Math.abs(rounded))
  if (rounded > 0) return `+${text}`
  if (rounded < 0) return `−${text}`
  return text
}
