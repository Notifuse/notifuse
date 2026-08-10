import { Card, Col, Row, Statistic, Table, Tag } from 'antd'
import { Link, useParams } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { Dayjs } from 'dayjs'
import { buildWebQuery, readTotals, useWebQuery } from '../components/web_analytics/lib/query'
import { useMinuteTick } from '../components/web_analytics/lib/useMinuteTick'
import { ResolvedRange } from '../components/web_analytics/lib/types'

const REFRESH_MS = 10_000
const WINDOW_MINUTES = 30

/** Last activity, not session start — see liveRange. */
const LIVE_TIME_DIMENSION = 'updated_at'

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
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })

  // The window advances once a minute rather than on every render, which would
  // give each render a different query key and refetch endlessly.
  const range = liveRange(useMinuteTick())
  const cacheKey = workspaceId

  const totalsQuery = buildWebQuery({
    schema: 'web_sessions',
    measures: ['sessions', 'pageviews'],
    range,
    timeDimension: LIVE_TIME_DIMENSION,
    timezone: 'UTC'
  })
  const { data: totals, isLoading } = useWebQuery(cacheKey, totalsQuery, {
    refetchInterval: REFRESH_MS
  })
  const values = readTotals(totals, ['sessions', 'pageviews'])

  return (
    <div className="p-4 md:p-6">
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3 text-2xl font-medium">
          {t`Live`}
          <Tag color="green">{t`Last ${WINDOW_MINUTES} minutes`}</Tag>
        </div>
        <Link
          to="/console/workspace/$workspaceId/web-analytics/$tab"
          params={{ workspaceId, tab: 'dashboard' }}
          className="text-sm text-[var(--primary)]"
        >
          {t`Back to the dashboard`}
        </Link>
      </div>

      <Row gutter={[16, 16]} className="mb-4">
        <Col xs={12} md={6}>
          <Card size="small">
            <Statistic title={t`Active sessions`} value={values.sessions} loading={isLoading} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card size="small">
            <Statistic title={t`Pageviews`} value={values.pageviews} loading={isLoading} />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <LiveBreakdown
            cacheKey={cacheKey}
            range={range}
            title={t`Landing pages`}
            dimension="landing_path"
          />
        </Col>
        <Col xs={24} lg={12}>
          <LiveBreakdown
            cacheKey={cacheKey}
            range={range}
            title={t`Channels`}
            dimension="channel_group"
          />
        </Col>
        <Col xs={24} lg={12}>
          <LiveBreakdown
            cacheKey={cacheKey}
            range={range}
            title={t`Countries`}
            dimension="country"
          />
        </Col>
        <Col xs={24} lg={12}>
          <LiveBreakdown cacheKey={cacheKey} range={range} title={t`Devices`} dimension="device" />
        </Col>
      </Row>
    </div>
  )
}

function LiveBreakdown(props: {
  cacheKey: string
  range: ResolvedRange
  title: string
  dimension: string
}) {
  const { t } = useLingui()
  const query = buildWebQuery({
    schema: 'web_sessions',
    measures: ['sessions'],
    dimensions: [props.dimension],
    range: props.range,
    timeDimension: LIVE_TIME_DIMENSION,
    limit: 8,
    timezone: 'UTC'
  })
  const { data, isLoading } = useWebQuery(props.cacheKey, query, { refetchInterval: REFRESH_MS })

  return (
    <Card size="small" title={props.title}>
      <Table
        size="small"
        rowKey={(row) => String(row[props.dimension] ?? '')}
        loading={isLoading}
        dataSource={data?.data ?? []}
        pagination={false}
        columns={[
          {
            title: props.title,
            dataIndex: props.dimension,
            ellipsis: true,
            render: (value: string) => value || t`(empty)`
          },
          { title: t`Sessions`, dataIndex: 'sessions', width: 110, align: 'right' as const }
        ]}
      />
    </Card>
  )
}
