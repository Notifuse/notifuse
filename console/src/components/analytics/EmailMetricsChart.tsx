import React, { useState, useEffect } from 'react'
import { Segmented, Alert, Row, Col, Statistic, Space, Tooltip, Spin, Card } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faPaperPlane,
  faCircleCheck,
  faEye,
  faCircleXmark,
  faFaceFrown
} from '@fortawesome/free-regular-svg-icons'
import { faArrowPointer, faTriangleExclamation, faBan } from '@fortawesome/free-solid-svg-icons'
import { ChartVisualization } from './ChartVisualization'
import { analyticsService, AnalyticsQuery, AnalyticsResponse } from '../../services/api/analytics'
import { Workspace } from '../../services/api/types'

interface EmailMetricsChartProps {
  workspace: Workspace
  timeRange?: [string, string]
  timezone?: string
}

type MessageTypeFilter = 'all' | 'broadcasts' | 'transactional'

export const EmailMetricsChart: React.FC<EmailMetricsChartProps> = ({
  workspace,
  timeRange = ['2024-01-01', '2024-12-31'],
  timezone
}) => {
  const [messageTypeFilter, setMessageTypeFilter] = useState<MessageTypeFilter>('all')
  const [data, setData] = useState<AnalyticsResponse | null>(null)
  const [statsData, setStatsData] = useState<AnalyticsResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [statsLoading, setStatsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // State to track which chart lines are visible
  const [visibleLines, setVisibleLines] = useState<Record<string, boolean>>({
    count_sent: true,
    count_delivered: true,
    count_opened: true,
    count_clicked: true,
    count_bounced: true,
    count_complained: true,
    count_unsubscribed: true,
    count_failed: true
  })

  // Function to toggle line visibility
  const toggleLineVisibility = (measure: string) => {
    setVisibleLines((prev) => ({
      ...prev,
      [measure]: !prev[measure]
    }))
  }

  const buildQuery = (filter: MessageTypeFilter): AnalyticsQuery => {
    // Only include measures that are visible
    const visibleMeasures = [
      'count_sent',
      'count_delivered',
      'count_bounced',
      'count_complained',
      'count_opened',
      'count_clicked',
      'count_unsubscribed',
      'count_failed'
    ].filter((measure) => visibleLines[measure])

    const baseQuery: AnalyticsQuery = {
      schema: 'message_history',
      measures: visibleMeasures,
      dimensions: [],
      timezone: timezone || workspace.settings.timezone || 'UTC',
      timeDimensions: [
        {
          dimension: 'created_at',
          granularity: 'day',
          dateRange: timeRange
        }
      ],
      filters: []
    }

    // Add broadcast_id filter if not 'all'
    if (filter === 'broadcasts') {
      baseQuery.filters?.push({
        member: 'broadcast_id',
        operator: 'set',
        values: []
      })
    } else if (filter === 'transactional') {
      baseQuery.filters?.push({
        member: 'broadcast_id',
        operator: 'notSet',
        values: []
      })
    }

    return baseQuery
  }

  const buildStatsQuery = (filter: MessageTypeFilter): AnalyticsQuery => {
    // Stats query should always include all measures regardless of visibility
    const baseQuery: AnalyticsQuery = {
      schema: 'message_history',
      measures: [
        'count_sent',
        'count_delivered',
        'count_bounced',
        'count_complained',
        'count_opened',
        'count_clicked',
        'count_unsubscribed',
        'count_failed'
      ],
      dimensions: [],
      timezone: timezone || workspace.settings.timezone || 'UTC',
      timeDimensions: [
        {
          dimension: 'created_at',
          granularity: 'day', // We need granularity, but we'll aggregate the results
          dateRange: timeRange
        }
      ],
      filters: []
    }

    // Add broadcast_id filter if not 'all'
    if (filter === 'broadcasts') {
      baseQuery.filters?.push({
        member: 'broadcast_id',
        operator: 'set',
        values: []
      })
    } else if (filter === 'transactional') {
      baseQuery.filters?.push({
        member: 'broadcast_id',
        operator: 'notSet',
        values: []
      })
    }

    return baseQuery
  }

  const fetchData = async (filter: MessageTypeFilter) => {
    try {
      setLoading(true)
      setStatsLoading(true)
      setError(null)

      // Fetch both chart data and stats data in parallel
      const [chartResponse, statsResponse] = await Promise.all([
        analyticsService.query(buildQuery(filter), workspace.id),
        analyticsService.query(buildStatsQuery(filter), workspace.id)
      ])

      setData(chartResponse)
      setStatsData(statsResponse)
    } catch (err) {
      console.error('Failed to fetch email metrics:', err)
      setError(err instanceof Error ? err.message : 'Failed to fetch email metrics')
    } finally {
      setLoading(false)
      setStatsLoading(false)
    }
  }

  useEffect(() => {
    fetchData(messageTypeFilter)
  }, [workspace.id, messageTypeFilter, timeRange, visibleLines])

  const handleFilterChange = (value: MessageTypeFilter) => {
    setMessageTypeFilter(value)
  }

  // Extract and aggregate stats from the stats response (sum up all daily values)
  const stats = statsData?.data?.reduce(
    (acc, row) => ({
      count_sent: acc.count_sent + (row.count_sent || 0),
      count_delivered: acc.count_delivered + (row.count_delivered || 0),
      count_opened: acc.count_opened + (row.count_opened || 0),
      count_clicked: acc.count_clicked + (row.count_clicked || 0),
      count_bounced: acc.count_bounced + (row.count_bounced || 0),
      count_complained: acc.count_complained + (row.count_complained || 0),
      count_unsubscribed: acc.count_unsubscribed + (row.count_unsubscribed || 0),
      count_failed: acc.count_failed + (row.count_failed || 0)
    }),
    {
      count_sent: 0,
      count_delivered: 0,
      count_opened: 0,
      count_clicked: 0,
      count_bounced: 0,
      count_complained: 0,
      count_unsubscribed: 0,
      count_failed: 0
    }
  ) || {
    count_sent: 0,
    count_delivered: 0,
    count_opened: 0,
    count_clicked: 0,
    count_bounced: 0,
    count_complained: 0,
    count_unsubscribed: 0,
    count_failed: 0
  }

  const getRate = (numerator: number, denominator: number) => {
    if (denominator === 0) return '-'
    const percentage = (numerator / denominator) * 100
    if (percentage === 0 || percentage >= 10) {
      return `${Math.round(percentage)}%`
    }
    return `${percentage.toFixed(1)}%`
  }

  // Formatter function for statistics that handles loading state
  const formatStat = (value: number | string) => {
    if (statsLoading) {
      return <Spin size="small" />
    }
    return value
  }

  // Define colors that match the icon colors in the statistics cards
  const chartColors = {
    count_sent: '#3b82f6', // blue-500
    count_delivered: '#10b981', // green-500
    count_opened: '#8b5cf6', // purple-500
    count_clicked: '#06b6d4', // cyan-500
    count_bounced: '#f97316', // orange-500
    count_complained: '#f97316', // orange-500
    count_unsubscribed: '#f97316', // orange-500
    count_failed: '#ef4444' // red-500
  }

  // Define measure titles for tooltip display
  const measureTitles = {
    count_sent: 'Sent',
    count_delivered: 'Delivered',
    count_opened: 'Opens',
    count_clicked: 'Clicks',
    count_bounced: 'Bounced',
    count_complained: 'Complaints',
    count_unsubscribed: 'Unsubscribes',
    count_failed: 'Failed'
  }

  return (
    <Card
      title="Email Metrics"
      extra={
        <Segmented
          value={messageTypeFilter}
          onChange={handleFilterChange}
          options={[
            { label: 'All', value: 'all' },
            { label: 'Broadcasts', value: 'broadcasts' },
            { label: 'Transactional', value: 'transactional' }
          ]}
        />
      }
    >
      {/* Error Alert */}
      {error && (
        <Alert
          message="Error"
          description={error}
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      {/* Stats Row */}
      <Row gutter={[16, 16]} wrap className="flex-nowrap overflow-x-auto">
        <Col span={3}>
          <Tooltip
            title={`${stats.count_sent} total emails sent${!visibleLines.count_sent ? ' (hidden from chart)' : ''}`}
          >
            <div
              className="p-2 cursor-pointer hover:bg-gray-50 rounded transition-colors"
              style={{ opacity: visibleLines.count_sent ? 1 : 0.5 }}
              onClick={() => toggleLineVisibility('count_sent')}
            >
              <Statistic
                title={
                  <Space className="font-medium">
                    <FontAwesomeIcon
                      icon={faPaperPlane}
                      style={{ opacity: 0.7 }}
                      className="text-blue-500"
                    />{' '}
                    Sent
                  </Space>
                }
                value={stats.count_sent}
                valueStyle={{ fontSize: '16px' }}
                formatter={formatStat}
              />
            </div>
          </Tooltip>
        </Col>
        <Col span={3}>
          <Tooltip
            title={`${stats.count_delivered} emails successfully delivered${!visibleLines.count_delivered ? ' (hidden from chart)' : ''}`}
          >
            <div
              className="p-2 cursor-pointer hover:bg-gray-50 rounded transition-colors"
              style={{ opacity: visibleLines.count_delivered ? 1 : 0.5 }}
              onClick={() => toggleLineVisibility('count_delivered')}
            >
              <Statistic
                title={
                  <Space className="font-medium">
                    <FontAwesomeIcon
                      icon={faCircleCheck}
                      style={{ opacity: 0.7 }}
                      className="text-green-500"
                    />{' '}
                    Delivered
                  </Space>
                }
                value={getRate(stats.count_delivered, stats.count_sent)}
                valueStyle={{ fontSize: '16px' }}
                formatter={formatStat}
              />
            </div>
          </Tooltip>
        </Col>
        <Col span={3}>
          <Tooltip
            title={`${stats.count_opened} total opens${!visibleLines.count_opened ? ' (hidden from chart)' : ''}`}
          >
            <div
              className="p-2 cursor-pointer hover:bg-gray-50 rounded transition-colors"
              style={{ opacity: visibleLines.count_opened ? 1 : 0.5 }}
              onClick={() => toggleLineVisibility('count_opened')}
            >
              <Statistic
                title={
                  <Space className="font-medium">
                    <FontAwesomeIcon
                      icon={faEye}
                      style={{ opacity: 0.7 }}
                      className="text-purple-500"
                    />{' '}
                    Opens
                  </Space>
                }
                value={getRate(stats.count_opened, stats.count_sent)}
                valueStyle={{ fontSize: '16px' }}
                formatter={formatStat}
              />
            </div>
          </Tooltip>
        </Col>
        <Col span={3}>
          <Tooltip
            title={`${stats.count_clicked} total clicks${!visibleLines.count_clicked ? ' (hidden from chart)' : ''}`}
          >
            <div
              className="p-2 cursor-pointer hover:bg-gray-50 rounded transition-colors"
              style={{ opacity: visibleLines.count_clicked ? 1 : 0.5 }}
              onClick={() => toggleLineVisibility('count_clicked')}
            >
              <Statistic
                title={
                  <Space className="font-medium">
                    <FontAwesomeIcon
                      icon={faArrowPointer}
                      style={{ opacity: 0.7 }}
                      className="text-cyan-500 mr-1"
                    />{' '}
                    Clicks
                  </Space>
                }
                value={getRate(stats.count_clicked, stats.count_sent)}
                valueStyle={{ fontSize: '16px' }}
                formatter={formatStat}
              />
            </div>
          </Tooltip>
        </Col>
        <Col span={3}>
          <Tooltip
            title={`${stats.count_bounced} emails bounced back${!visibleLines.count_bounced ? ' (hidden from chart)' : ''}`}
          >
            <div
              className="p-2 cursor-pointer hover:bg-gray-50 rounded transition-colors"
              style={{ opacity: visibleLines.count_bounced ? 1 : 0.5 }}
              onClick={() => toggleLineVisibility('count_bounced')}
            >
              <Statistic
                title={
                  <Space className="font-medium">
                    <FontAwesomeIcon
                      icon={faTriangleExclamation}
                      style={{ opacity: 0.7 }}
                      className="text-orange-500"
                    />{' '}
                    Bounced
                  </Space>
                }
                value={getRate(stats.count_bounced, stats.count_sent)}
                valueStyle={{ fontSize: '16px' }}
                formatter={formatStat}
              />
            </div>
          </Tooltip>
        </Col>
        <Col span={3}>
          <Tooltip
            title={`${stats.count_complained} total complaints${!visibleLines.count_complained ? ' (hidden from chart)' : ''}`}
          >
            <div
              className="p-2 cursor-pointer hover:bg-gray-50 rounded transition-colors"
              style={{ opacity: visibleLines.count_complained ? 1 : 0.5 }}
              onClick={() => toggleLineVisibility('count_complained')}
            >
              <Statistic
                title={
                  <Space className="font-medium">
                    <FontAwesomeIcon
                      icon={faFaceFrown}
                      style={{ opacity: 0.7 }}
                      className="text-orange-500"
                    />{' '}
                    Complaints
                  </Space>
                }
                value={getRate(stats.count_complained, stats.count_sent)}
                valueStyle={{ fontSize: '16px' }}
                formatter={formatStat}
              />
            </div>
          </Tooltip>
        </Col>
        <Col span={3}>
          <Tooltip
            title={`${stats.count_unsubscribed} total unsubscribes${!visibleLines.count_unsubscribed ? ' (hidden from chart)' : ''}`}
          >
            <div
              className="p-2 cursor-pointer hover:bg-gray-50 rounded transition-colors"
              style={{ opacity: visibleLines.count_unsubscribed ? 1 : 0.5 }}
              onClick={() => toggleLineVisibility('count_unsubscribed')}
            >
              <Statistic
                title={
                  <Space className="font-medium">
                    <FontAwesomeIcon
                      icon={faBan}
                      style={{ opacity: 0.7 }}
                      className="text-orange-500"
                    />{' '}
                    Unsub.
                  </Space>
                }
                value={getRate(stats.count_unsubscribed, stats.count_sent)}
                valueStyle={{ fontSize: '16px' }}
                formatter={formatStat}
              />
            </div>
          </Tooltip>
        </Col>
        <Col span={3}>
          <Tooltip
            title={`${stats.count_failed} emails failed to send${!visibleLines.count_failed ? ' (hidden from chart)' : ''}`}
          >
            <div
              className="p-2 cursor-pointer hover:bg-gray-50 rounded transition-colors"
              style={{ opacity: visibleLines.count_failed ? 1 : 0.5 }}
              onClick={() => toggleLineVisibility('count_failed')}
            >
              <Statistic
                title={
                  <Space className="font-medium">
                    <FontAwesomeIcon
                      icon={faCircleXmark}
                      style={{ opacity: 0.7 }}
                      className="text-red-500"
                    />{' '}
                    Failed
                  </Space>
                }
                value={getRate(stats.count_failed, stats.count_sent)}
                valueStyle={{ fontSize: '16px' }}
                formatter={formatStat}
              />
            </div>
          </Tooltip>
        </Col>
      </Row>

      {/* Chart */}
      <ChartVisualization
        data={data}
        chartType="line"
        query={buildQuery(messageTypeFilter)}
        loading={loading}
        error={error}
        height={220}
        showLegend={false}
        colors={chartColors}
        measureTitles={measureTitles}
      />
    </Card>
  )
}
