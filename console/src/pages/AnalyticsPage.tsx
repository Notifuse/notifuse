import { useEffect, useMemo, useState } from 'react'
import { Navigate, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { Segmented, Skeleton } from 'antd'
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
import { resolveDashboardPeriods } from '../components/analytics/dashboardPeriods'
import {
  dashboardMetric,
  dashboardPeriod,
  validateDashboardSearch,
  type DashboardMetric,
  type DashboardPeriod
} from '../components/analytics/dashboardSearch'
import { firstAccessiblePage, type HasAccess } from '../layouts/app-shell/menuItems'
import type { UserPermissions } from '../services/api/types'

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
  const { t } = useLingui()
  // Mounted on the workspace index and on /analytics: both validate the same search.
  const search = validateDashboardSearch(useSearch({ strict: false }))
  const navigate = useNavigate()
  const { workspaces } = useAuth()
  const { permissions, loading: permissionsLoading } = useWorkspacePermissions(workspaceId)
  const workspace = workspaces.find((candidate) => candidate.id === workspaceId)
  const timezone = workspace?.settings?.timezone || 'UTC'

  const period = dashboardPeriod(search)
  const metric = dashboardMetric(search)
  const tick = useMinuteTick()
  // eslint-disable-next-line react-hooks/exhaustive-deps -- tick only recomputes the bounds
  const periods = useMemo(() => resolveDashboardPeriods(period, timezone), [period, timezone, tick])

  // Read strictly: message_history write is granted to every member the console saves, so the
  // sidebar's read-or-write check shows the dashboard to members who cannot read messages.
  const canRead = Boolean(permissions?.message_history?.read)
  const series = useOriginSeries({
    workspaceId,
    timezone,
    range: periods.current,
    enabled: canRead
  })

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
    const strictAccess: HasAccess = (resource: keyof UserPermissions) => {
      if (resource === 'message_history') return false
      const granted = permissions?.[resource]
      return Boolean(granted?.read || granted?.write)
    }
    return <Navigate {...firstAccessiblePage(strictAccess, workspaceId).target} replace />
  }

  const header = (
    <PageHeader
      title={t`Dashboard`}
      actions={
        <Segmented<DashboardPeriod>
          value={period}
          onChange={(value) => setSearch({ period: value })}
          options={[
            { label: t`7 d`, value: '7d' },
            { label: t`30 d`, value: '30d' },
            { label: t`90 d`, value: '90d' }
          ]}
        />
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

  return (
    <div className="flex flex-col gap-3">
      {header}
      <DashboardKpis
        workspace={workspace}
        permissions={permissions}
        periods={periods}
        timezone={timezone}
        totals={series.totals}
        seriesError={series.isError}
        seriesPlaceholder={series.isPlaceholderData}
        onRetrySeries={series.refetch}
      />
      <div className="grid grid-cols-1 items-start gap-3 min-[1200px]:grid-cols-[minmax(0,1fr)_var(--spacing-aside)]">
        <div className="flex min-w-0 flex-col gap-3">
          <OriginVolumeChart data={series} metric={metric} onMetricChange={(value) => setSearch({ metric: value })} />
          <div className="grid grid-cols-1 items-start gap-3 min-[1000px]:grid-cols-2">
            <RecentFailures workspaceId={workspaceId} timezone={timezone} />
            {can('contacts') ? (
              <NewContactsTable workspaceId={workspaceId} canImport={can('contacts', 'write')} />
            ) : null}
          </div>
        </div>
        <div className="flex min-w-0 flex-col gap-3">
          <SendingCard workspace={workspace} canConfigure={can('workspace', 'write')} />
          {can('broadcasts') ? (
            <UpcomingSends
              workspaceId={workspaceId}
              timezone={timezone}
              canReadLists={can('lists')}
              canCreate={can('broadcasts', 'write')}
            />
          ) : null}
        </div>
      </div>
    </div>
  )
}
