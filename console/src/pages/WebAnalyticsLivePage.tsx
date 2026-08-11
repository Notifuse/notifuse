import { Link, useParams } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { ArrowLeft } from 'lucide-react'
import { Dayjs } from 'dayjs'
import { WebAnalyticsProvider, useWebAnalytics } from '../components/web_analytics/context'
import { WebAnalyticsGate } from '../components/web_analytics/InstallOverlay'
import {
  ColumnConfig,
  DimensionTableWidget,
  LiveOverride
} from '../components/web_analytics/DimensionTableWidget'
import {
  LiveSessionLocation,
  LiveSessionMap
} from '../components/web_analytics/LiveSessionMap'
import {
  buildWebQuery,
  readTotals,
  useWebQuery,
  webAnalyticsLiveClient
} from '../components/web_analytics/lib/query'
import { useMinuteTick } from '../components/web_analytics/lib/useMinuteTick'
import { ResolvedRange } from '../components/web_analytics/lib/types'

const REFRESH_MS = 10_000
const WINDOW_MINUTES = 30

/** Last activity, not session start — see liveRange. */
const LIVE_TIME_DIMENSION = 'updated_at'

/** Enough marks to fill a world map without asking for every session on it. */
const MAP_LIMIT = 500

/**
 * Sessions still active, measured on last activity rather than start time: a
 * visitor who landed an hour ago and is still reading is live, and one who
 * arrived two minutes ago and left is not.
 */
function liveRange(anchor: Dayjs): ResolvedRange {
  const start = anchor.subtract(WINDOW_MINUTES, 'minute')
  const end = anchor.add(5, 'minute')
  return {
    startDay: start.format('YYYY-MM-DD'),
    endDay: end.format('YYYY-MM-DD'),
    startUtc: start.toISOString(),
    endUtc: end.toISOString()
  }
}

export function WebAnalyticsLivePage() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })

  // The breakdown widgets are the dashboard's, so they read the section
  // context for the workspace, timezone and active filters.
  return (
    <WebAnalyticsProvider workspaceId={workspaceId}>
      <LiveView workspaceId={workspaceId} />
    </WebAnalyticsProvider>
  )
}

function LiveView(props: { workspaceId: string }) {
  const { t } = useLingui()
  const context = useWebAnalytics()

  // The window advances once a minute rather than on every render, which would
  // give each render a different query key and refetch endlessly. Inside the
  // minute the poll below is what keeps it moving.
  const range = liveRange(useMinuteTick())
  const live: LiveOverride = {
    range,
    timeDimension: LIVE_TIME_DIMENSION,
    refetchInterval: REFRESH_MS
  }

  const totals = useWebQuery(
    props.workspaceId,
    buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      range,
      timeDimension: LIVE_TIME_DIMENSION,
      timezone: context.timezone
    }),
    { refetchInterval: REFRESH_MS, client: webAnalyticsLiveClient }
  )
  const liveSessions = readTotals(totals.data, ['sessions']).sessions

  const mapResult = useWebQuery(
    props.workspaceId,
    buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['latitude', 'longitude', 'city', 'country'],
      range,
      timeDimension: LIVE_TIME_DIMENSION,
      limit: MAP_LIMIT,
      timezone: context.timezone
    }),
    { refetchInterval: REFRESH_MS, client: webAnalyticsLiveClient }
  )

  const locations: LiveSessionLocation[] = (mapResult.data?.data ?? []).map((row) => ({
    latitude: row.latitude == null ? null : Number(row.latitude),
    longitude: row.longitude == null ? null : Number(row.longitude),
    city: (row.city as string) || null,
    country: (row.country as string) || null,
    sessions: Number(row.sessions ?? 0)
  }))

  // One measure only: a thirty-minute window is too short for the averages the
  // dashboard columns carry to mean anything.
  const columns: ColumnConfig[] = [
    { key: 'sessions', label: t`Sessions`, format: 'number', heatMap: true }
  ]

  return (
    <div className="p-4 md:p-6">
      <div className="mb-6 flex items-center justify-between">
        <Link
          to="/console/workspace/$workspaceId/web-analytics/$tab"
          params={{ workspaceId: props.workspaceId, tab: 'dashboard' }}
          className="flex items-center gap-1 text-gray-500 transition-colors hover:text-gray-700"
        >
          <ArrowLeft size={16} />
          <span className="text-sm">{t`Dashboard`}</span>
        </Link>
        <div className="flex items-center gap-3">
          <span className="relative flex h-2 w-2">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
            <span className="relative inline-flex h-2 w-2 rounded-full bg-green-500" />
          </span>
          <span className="text-lg font-medium text-gray-800">
            {t`${liveSessions} live now`}
          </span>
        </div>
      </div>

      <WebAnalyticsGate>
        <LiveSessionMap data={locations} loading={mapResult.isLoading} />

        <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-3">
          <DimensionTableWidget
            title={t`Top Pages`}
            columns={columns}
            live={live}
            emptyText={t`No pages`}
            tabs={[
              {
                key: 'landing',
                label: t`Landing pages`,
                dimensionLabel: t`Page`,
                dimension: 'landing_path'
              }
            ]}
          />
          <DimensionTableWidget
            title={t`Top Cities`}
            columns={columns}
            live={live}
            emptyText={t`No city data`}
            tabs={[
              { key: 'cities', label: t`Cities`, dimensionLabel: t`City`, dimension: 'city' }
            ]}
          />
          <DimensionTableWidget
            title={t`Top Referrers`}
            columns={columns}
            live={live}
            emptyText={t`No referrers`}
            tabs={[
              {
                key: 'referrers',
                label: t`Referrers`,
                dimensionLabel: t`Referrer domain`,
                dimension: 'referrer_domain'
              }
            ]}
          />
        </div>

        <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-3">
          <DimensionTableWidget
            title={t`Devices`}
            columns={columns}
            live={live}
            emptyText={t`No device data`}
            tabs={[
              { key: 'devices', label: t`Devices`, dimensionLabel: t`Device`, dimension: 'device' }
            ]}
          />
          <DimensionTableWidget
            title={t`Campaigns`}
            columns={columns}
            live={live}
            emptyText={t`No campaign data`}
            tabs={[
              {
                key: 'campaigns',
                label: t`Campaigns`,
                dimensionLabel: t`Campaign`,
                dimension: 'utm_campaign',
                // Most sessions carry no campaign at all; without this the table
                // would be one giant "(empty)" row.
                filters: [{ dimension: 'utm_campaign', operator: 'isNotEmpty', values: [] }]
              }
            ]}
          />
          <DimensionTableWidget
            title={t`Channels`}
            columns={columns}
            live={live}
            emptyText={t`No channel data`}
            tabs={[
              {
                key: 'channels',
                label: t`Channels`,
                dimensionLabel: t`Channel group`,
                dimension: 'channel_group'
              }
            ]}
          />
        </div>
      </WebAnalyticsGate>
    </div>
  )
}
