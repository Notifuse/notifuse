import { useEffect, useMemo, useState } from 'react'
import { Navigate, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { Segmented, Skeleton, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { KpiStrip, PageHeader, Panel } from '../components/layout'
import { DashboardKpis } from '../components/analytics/DashboardKpis'
import { OriginVolumeChart } from '../components/analytics/OriginVolumeChart'
import { RecentFailures } from '../components/analytics/RecentFailures'
import { NewContactsTable } from '../components/analytics/NewContactsTable'
import { SendingCard } from '../components/analytics/SendingCard'
import { UpcomingSends } from '../components/analytics/UpcomingSends'
import { useOriginSeries } from '../components/analytics/useOriginSeries'
import { useLatestMessage } from '../components/analytics/useLatestMessage'
import { GetStartedChecklist } from '../components/analytics/GetStartedChecklist'
import { ButtonLink } from '../components/analytics/ButtonLink'
import { sendStateOf } from '../components/analytics/dashboardSendState'
import { ownerEmails, roleReady } from '../components/analytics/getStartedModel'
import { MESSAGE_ORIGINS } from '../components/analytics/dashboardKpisModel'
import { resolveDashboardPeriods } from '../components/analytics/dashboardPeriods'
import {
  dashboardMetric,
  dashboardPeriod,
  validateDashboardSearch,
  type DashboardMetric,
  type DashboardPeriod
} from '../components/analytics/dashboardSearch'
import { canReadDashboard, dashboardFallback } from '../layouts/app-shell/menuItems'
import type { UserPermissions } from '../services/api/types'
import { parseRootEmails } from '../services/api/auth'

/** Changes once a minute, so the periods move to the next day at midnight on an open tab. */
function useMinuteTick(): number {
  const [tick, setTick] = useState(() => Math.floor(Date.now() / 60_000))
  useEffect(() => {
    const id = window.setInterval(() => setTick(Math.floor(Date.now() / 60_000)), 60_000)
    return () => window.clearInterval(id)
  }, [])
  return tick
}

/** The search of a period or metric choice: the defaults are left out of the URL. */
function searchWith(change: { period?: DashboardPeriod; metric?: DashboardMetric }) {
  return (previous: Record<string, unknown>): Record<string, unknown> => {
    const next = { ...previous }
    if (change.period) next.period = change.period === '30d' ? undefined : change.period
    if (change.metric) next.metric = change.metric === 'sent' ? undefined : change.metric
    return next
  }
}

/**
 * Keyed by workspace: the route keeps its component across a workspace switch, and the rights of
 * the previous workspace would otherwise send its requests to the new one until they reload.
 */
export function AnalyticsPage() {
  const { workspaceId } = useParams({
    from: '/console/workspace/$workspaceId'
  })
  return <Dashboard key={workspaceId} workspaceId={workspaceId} />
}

function Dashboard({ workspaceId }: { workspaceId: string }) {
  const { t, i18n } = useLingui()
  // Mounted on the workspace index and on /analytics: both validate the same search.
  const search = validateDashboardSearch(useSearch({ strict: false }))
  const navigate = useNavigate()
  const { workspaces, user } = useAuth()
  const { permissions, role, loading: permissionsLoading } = useWorkspacePermissions(workspaceId)
  const workspace = workspaces.find((candidate) => candidate.id === workspaceId)
  const timezone = workspace?.settings?.timezone || 'UTC'

  const period = dashboardPeriod(search)
  const metric = dashboardMetric(search)
  const tick = useMinuteTick()
  // eslint-disable-next-line react-hooks/exhaustive-deps -- tick only recomputes the bounds
  const periods = useMemo(() => resolveDashboardPeriods(period, timezone), [period, timezone, tick])

  // Read strictly, as the sidebar's logo reads it (menuItems.ts, canReadDashboard)
  const canRead = canReadDashboard(permissions)
  const series = useOriginSeries({
    workspaceId,
    timezone,
    range: periods.current,
    enabled: canRead
  })
  // Whether the workspace ever sent: the empty states and the "Get started" checklist follow it.
  const latest = useLatestMessage(workspaceId, canRead)
  const periodSent = series.totals
    ? MESSAGE_ORIGINS.reduce((sum, origin) => sum + (series.totals?.[origin]?.sent ?? 0), 0)
    : undefined
  const sendState = sendStateOf({
    latestAt: latest.latestAt,
    latestError: latest.isError,
    periodEndUtc: periods.current.endUtc,
    periodSent,
    seriesError: series.isError
  })
  // Before the first send there is no period to choose. Kept while that is unknown, so the header
  // does not flicker on a workspace that sends.
  const showPeriod = sendState !== 'never'

  const setSearch = (change: { period?: DashboardPeriod; metric?: DashboardMetric }) =>
    void navigate({ to: '.', search: searchWith(change) })

  if (!workspace) {
    return (
      <div className="text-center">
        <h2>{t`Workspace not found`}</h2>
        <p>{t`The requested workspace could not be found.`}</p>
      </div>
    )
  }

  if (!permissionsLoading && !canRead) {
    return <Navigate {...dashboardFallback(permissions, workspaceId).target} replace />
  }

  // The days the period covers, said beside it: complete days up to yesterday, which is not the
  // rolling "Last 7 days" of Logs, so the two counts differed with nothing to tell why.
  const day = (value: string) => new Date(`${value}T00:00:00Z`)
  const coveredDays = new Intl.DateTimeFormat(i18n.locale, {
    day: 'numeric',
    month: 'short',
    timeZone: 'UTC'
  }).formatRange(day(periods.current.startDay), day(periods.current.endDay))

  const header = (
    <PageHeader
      title={t`Dashboard`}
      actions={
        showPeriod ? (
        <div className="flex items-center gap-3">
          <Tooltip title={t`Complete days up to yesterday, in the workspace's time zone (${timezone})`}>
            <span data-dashboard-days className="text-xs text-ink-3 tabular-nums">
              {coveredDays}
            </span>
          </Tooltip>
          <Segmented<DashboardPeriod>
            value={period}
            onChange={(value) => setSearch({ period: value })}
            options={[
              { label: t`7 d`, value: '7d' },
              { label: t`30 d`, value: '30d' },
              { label: t`90 d`, value: '90d' }
            ]}
          />
        </div>
        ) : undefined
      }
    />
  )

  if (permissionsLoading || !permissions) {
    return (
      <div className="flex flex-col gap-3" data-testid="dashboard-loading">
        {header}
        <KpiStrip
          loading
          items={[
            { key: 'a', label: '', value: '' },
            { key: 'b', label: '', value: '' },
            { key: 'c', label: '', value: '' }
          ]}
        />
        <Panel>
          <Skeleton active paragraph={{ rows: 6 }} title={false} />
        </Panel>
      </div>
    )
  }

  const can = (resource: keyof UserPermissions, verb: 'read' | 'write' = 'read') =>
    Boolean(permissions[resource]?.[verb])
  // Providers and their uses are the owners' to set (integrations and workspace settings).
  const isOwner = role === 'owner' || role === 'root'
  const marketingReady = roleReady(workspace, 'marketing')

  const chartGhost =
    sendState === 'never'
      ? {
          message: t`Your emails will show here, day by day, from your first send.`,
          action:
            marketingReady && can('broadcasts', 'write') ? (
              <ButtonLink
                to="/console/workspace/$workspaceId/broadcasts"
                params={{ workspaceId }}
                search={{ compose: 'new' }}
              >
                {t`New broadcast`}
              </ButtonLink>
            ) : !marketingReady && isOwner ? (
              <ButtonLink
                to="/console/workspace/$workspaceId/settings/$section"
                params={{ workspaceId, section: 'integrations' }}
              >
                {t`Connect an email provider`}
              </ButtonLink>
            ) : undefined
        }
      : sendState === 'today'
        ? { message: t`Today's emails show here tomorrow: the chart counts complete days.` }
        : undefined

  return (
    <div className="flex flex-col gap-3">
      {header}
      {user ? (
        <GetStartedChecklist
          workspace={workspace}
          permissions={permissions}
          role={role}
          user={{ id: user.id, email: user.email }}
          latestAt={latest.isError ? undefined : latest.latestAt}
        />
      ) : null}
      <DashboardKpis
        workspace={workspace}
        permissions={permissions}
        periods={periods}
        timezone={timezone}
        totals={series.totals}
        seriesError={series.isError}
        seriesPlaceholder={series.isPlaceholderData}
        onRetrySeries={series.refetch}
        sendState={sendState}
      />
      <div className="grid grid-cols-1 items-start gap-3 min-[1200px]:grid-cols-[minmax(0,1fr)_var(--spacing-aside)]">
        <div className="flex min-w-0 flex-col gap-3">
          <OriginVolumeChart
            data={series}
            metric={metric}
            onMetricChange={(value) => setSearch({ metric: value })}
            ghost={chartGhost}
          />
          <div className="grid grid-cols-1 items-start gap-3 min-[1000px]:grid-cols-2">
            <RecentFailures workspaceId={workspaceId} timezone={timezone} sendState={sendState} />
            {can('contacts') ? (
              <NewContactsTable
                workspaceId={workspaceId}
                canImport={can('contacts', 'write')}
                canReadLists={can('lists')}
                ownerEmails={ownerEmails(parseRootEmails(window.ROOT_EMAIL))}
              />
            ) : null}
          </div>
        </div>
        <div className="flex min-w-0 flex-col gap-3">
          <SendingCard workspace={workspace} canConfigure={isOwner} />
          {can('broadcasts') ? (
            <UpcomingSends
              workspaceId={workspaceId}
              timezone={timezone}
              canReadLists={can('lists')}
              canCreate={can('broadcasts', 'write')}
              marketingReady={marketingReady}
              canConfigure={isOwner}
            />
          ) : null}
        </div>
      </div>
    </div>
  )
}
