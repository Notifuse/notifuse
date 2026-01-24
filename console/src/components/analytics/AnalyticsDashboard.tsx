import React from 'react'
import { Row, Col, Statistic, Button, Spin } from 'antd'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import numbro from 'numbro'
import { EmailMetricsChart } from './EmailMetricsChart'
// import { NewContactsTable } from './NewContactsTable'
import { Workspace, Integration } from '../../services/api/types'
import { FailedMessagesTable } from './FailedMessagesTable'
import { NewContactsTable } from './NewContactsTable'
import { emailProviders } from '../integrations/EmailProviders'
import { analyticsService } from '../../services/api/analytics'

interface AnalyticsDashboardProps {
  workspace: Workspace
  timeRange: [string, string]
  timezone?: string
}

export const AnalyticsDashboard: React.FC<AnalyticsDashboardProps> = ({
  workspace,
  timeRange,
  timezone
}) => {
  const { t } = useLingui()
  const navigate = useNavigate()

  // Use timeRange and timezone as refresh key to update components when they change
  const refreshKey = `${timeRange[0]}-${timeRange[1]}-${timezone || ''}`

  // Query for total contacts count
  const { data: totalContactsData, isLoading: totalContactsLoading } = useQuery({
    queryKey: ['analytics', 'total-contacts', workspace.id],
    queryFn: async () => {
      return analyticsService.query(
        {
          schema: 'contacts',
          measures: ['count'],
          dimensions: [],
          filters: []
        },
        workspace.id
      )
    },
    refetchInterval: 60000 // Refetch every minute
  })

  // Query for new contacts in the given date range
  const { data: newContactsData, isLoading: newContactsLoading } = useQuery({
    queryKey: ['analytics', 'new-contacts', workspace.id, timeRange[0], timeRange[1]],
    queryFn: async () => {
      return analyticsService.query(
        {
          schema: 'contacts',
          measures: ['count'],
          dimensions: [],
          filters: [
            {
              member: 'created_at',
              operator: 'inDateRange',
              values: timeRange
            }
          ]
        },
        workspace.id
      )
    },
    refetchInterval: 60000 // Refetch every minute
  })

  // Get provider information
  const transactionalProvider = workspace.settings.transactional_email_provider_id
    ? workspace.integrations?.find(
        (i) => i.id === workspace.settings.transactional_email_provider_id
      )
    : null

  const marketingProvider = workspace.settings.marketing_email_provider_id
    ? workspace.integrations?.find((i) => i.id === workspace.settings.marketing_email_provider_id)
    : null

  const getProviderInfo = (provider: Integration | null | undefined) => {
    if (!provider) return null
    return emailProviders.find((p) => p.kind === provider.email_provider?.kind)
  }

  const transactionalProviderInfo = getProviderInfo(transactionalProvider)
  const marketingProviderInfo = getProviderInfo(marketingProvider)

  const getDefaultSender = (provider: Integration | null | undefined) => {
    if (!provider?.email_provider?.senders) return null
    return (
      provider.email_provider.senders.find((s) => s.is_default) ||
      provider.email_provider.senders[0]
    )
  }

  const transactionalSender = getDefaultSender(transactionalProvider)
  const marketingSender = getDefaultSender(marketingProvider)

  // Calculate totals
  const totalContacts = totalContactsData?.data?.[0]?.['count'] || 0
  const newContactsCount = newContactsData?.data?.[0]?.['count'] || 0

  // Formatter function for statistics that handles loading state
  const formatStat = (value: number | string, isLoading: boolean) => {
    if (isLoading) {
      return <Spin size="small" />
    }
    return numbro(value).format({ thousandSeparated: true })
  }

  const handleNavigateToSettings = () => {
    navigate({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId: workspace.id, section: 'integrations' }
    })
  }

  return (
    <div>
      {/* Statistics Row - 4 columns */}
      <Row gutter={[16, 16]} className="mb-8">
        {/* Total Contacts */}
        <Col xs={24} sm={12} md={6}>
          <div className="p-4 rounded-lg bg-gray-100" style={{ height: '110px' }}>
            <Statistic
              title={t`Total Contacts`}
              value={totalContacts as number}
              valueStyle={{ fontSize: '24px', fontWeight: 'bold' }}
              formatter={(value) => formatStat(value as number, totalContactsLoading)}
            />
          </div>
        </Col>

        {/* New Contacts */}
        <Col xs={24} sm={12} md={6}>
          <div className="bg-gray-100 p-4 rounded-lg" style={{ height: '110px' }}>
            <Statistic
              title={t`New Contacts`}
              value={newContactsCount as number}
              valueStyle={{ fontSize: '24px', fontWeight: 'bold' }}
              formatter={(value) => formatStat(value as number, newContactsLoading)}
            />
          </div>
        </Col>

        {/* Transactional Email Provider */}
        <Col xs={24} sm={12} md={6}>
          <div className="bg-gray-100 p-4 rounded-lg" style={{ height: '110px' }}>
            <div className="text-gray-500 text-sm mb-2">{t`Transactional Provider`}</div>
            {transactionalProvider ? (
              <div>
                <div className="mb-1">
                  <span className="font-medium">{transactionalProviderInfo?.name}</span>
                </div>
                {transactionalSender && (
                  <div className="text-sm text-gray-600">{transactionalSender.email}</div>
                )}
              </div>
            ) : (
              <div>
                <div className="text-gray-400 mb-2">{t`Not configured`}</div>
                <Button size="small" type="primary" onClick={handleNavigateToSettings}>
                  {t`Configure`}
                </Button>
              </div>
            )}
          </div>
        </Col>

        {/* Marketing Email Provider */}
        <Col xs={24} sm={12} md={6}>
          <div className="bg-gray-100 p-4 rounded-lg" style={{ height: '110px' }}>
            <div className="text-gray-500 text-sm mb-2">{t`Marketing Provider`}</div>
            {marketingProvider ? (
              <div>
                <div className="mb-1">
                  <span className="font-medium">{marketingProviderInfo?.name}</span>
                </div>
                {marketingSender && (
                  <div className="text-sm text-gray-600">{marketingSender.email}</div>
                )}
              </div>
            ) : (
              <div>
                <div className="text-gray-400 mb-2">{t`Not configured`}</div>
                <Button size="small" type="primary" onClick={handleNavigateToSettings}>
                  {t`Configure`}
                </Button>
              </div>
            )}
          </div>
        </Col>
      </Row>

      {/* Email Metrics Chart - Full Width */}
      <EmailMetricsChart
        key={`email-metrics-${refreshKey}`}
        workspace={workspace}
        timeRange={timeRange}
        timezone={timezone}
      />

      <div className="mt-8">
        <NewContactsTable key={`new-contacts-${refreshKey}`} workspace={workspace} />
      </div>

      <div className="mt-8">
        <FailedMessagesTable key={`failed-messages-${refreshKey}`} workspace={workspace} />
      </div>
    </div>
  )
}
