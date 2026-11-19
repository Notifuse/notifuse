import React from 'react'
import { Row, Col, Button, Spin } from 'antd'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import numbro from 'numbro'
import { EmailMetricsChart } from './EmailMetricsChart'
import { Workspace, Integration } from '../../services/api/types'
import { FailedMessagesTable } from './FailedMessagesTable'
import { NewContactsTable } from './NewContactsTable'
import { emailProviders } from '../integrations/EmailProviders'
import { analyticsService } from '../../services/api/analytics'
import { useAuth } from '../../contexts/AuthContext'
import { isMailStudioAdmin } from '../../lib/hidden-mail-studio-nav'

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
  const { user } = useAuth()
  const isAdmin = isMailStudioAdmin(user?.email)

  // Use timeRange and timezone as refresh key to update components when they change
  const refreshKey = `${timeRange[0]}-${timeRange[1]}-${timezone || ''}`

  // Query for total contacts count (super/root only — brands don't see these cards)
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
    enabled: isAdmin,
    refetchInterval: 60000
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
    enabled: isAdmin,
    refetchInterval: 60000
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
    <div className="space-y-6">
      {/* Super/root only — brands keep a clean email-metrics dashboard */}
      {isAdmin ? (
        <Row gutter={[16, 16]}>
          {/* Total Contacts */}
          <Col xs={24} sm={12} lg={6}>
            <div className="mailstudio-card p-5 h-full flex flex-col justify-between">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">{t`Total Contacts`}</span>
                <div className="w-9 h-9 rounded-xl bg-[#e8efff] text-[#2457d6] flex items-center justify-center border border-[#c7d2e8]">
                  <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/>
                    <circle cx="9" cy="7" r="4"/>
                    <path d="M22 21v-2a4 4 0 0 0-3-3.87"/>
                    <path d="M16 3.13a4 4 0 0 1 0 7.75"/>
                  </svg>
                </div>
              </div>
              <div className="mt-3">
                <div className="text-2xl font-bold text-[#0b1433] tracking-tight">
                  {formatStat(totalContacts as number, totalContactsLoading)}
                </div>
                <div className="flex items-center gap-1.5 mt-1 text-xs text-slate-400">
                  <span className="text-[#1d9a6c] font-semibold bg-[#e3f6ee] px-1.5 py-0.5 rounded-full text-[11px] border border-[#b7e5d0]">+100%</span>
                  <span>audience database</span>
                </div>
              </div>
            </div>
          </Col>

          {/* New Contacts */}
          <Col xs={24} sm={12} lg={6}>
            <div className="mailstudio-card p-5 h-full flex flex-col justify-between">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">{t`New Contacts`}</span>
                <div className="w-9 h-9 rounded-xl bg-[#e3f6ee] text-[#1d9a6c] flex items-center justify-center border border-[#b7e5d0]">
                  <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/>
                    <circle cx="9" cy="7" r="4"/>
                    <line x1="19" x2="19" y1="8" y2="14"/>
                    <line x1="16" x2="22" y1="11" y2="11"/>
                  </svg>
                </div>
              </div>
              <div className="mt-3">
                <div className="text-2xl font-bold text-[#0b1433] tracking-tight">
                  {formatStat(newContactsCount as number, newContactsLoading)}
                </div>
                <div className="flex items-center gap-1.5 mt-1 text-xs text-slate-400">
                  <span className="text-[#2457d6] font-semibold bg-[#e8efff] px-1.5 py-0.5 rounded text-[11px] border border-[#c7d2e8]">Range</span>
                  <span>in selected period</span>
                </div>
              </div>
            </div>
          </Col>

          {/* Transactional Email Provider */}
          <Col xs={24} sm={12} lg={6}>
            <div className="mailstudio-card p-5 h-full flex flex-col justify-between">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">{t`Transactional Provider`}</span>
                <div className="w-9 h-9 rounded-xl bg-[#f0ebff] text-[#7a5bff] flex items-center justify-center border border-[#c9b8ff]">
                  <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="m22 2-7 20-4-9-9-4Z"/>
                    <path d="M22 2 11 13"/>
                  </svg>
                </div>
              </div>
              <div className="mt-3">
                {transactionalProvider ? (
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="font-bold text-[#0b1433] text-sm">{transactionalProviderInfo?.name}</span>
                      <span className="bg-[#e3f6ee] text-[#1d9a6c] text-[10px] px-1.5 py-0.5 rounded-full font-semibold border border-[#b7e5d0]">
                        Active
                      </span>
                    </div>
                    {transactionalSender && (
                      <div className="text-xs text-slate-400 mt-1 truncate max-w-[200px]" title={transactionalSender.email}>
                        {transactionalSender.email}
                      </div>
                    )}
                  </div>
                ) : (
                  <div>
                    <div className="text-xs text-slate-500 mb-2">{t`Not configured`}</div>
                    <Button size="small" type="primary" onClick={handleNavigateToSettings} className="text-xs">
                      {t`Configure`}
                    </Button>
                  </div>
                )}
              </div>
            </div>
          </Col>

          {/* Marketing Email Provider */}
          <Col xs={24} sm={12} lg={6}>
            <div className="mailstudio-card p-5 h-full flex flex-col justify-between">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">{t`Marketing Provider`}</span>
                <div className="w-9 h-9 rounded-xl bg-[#fff1e5] text-[#e2701f] flex items-center justify-center border border-[#f0c49a]">
                  <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="m3 11 18-5v12L3 14v-3z"/>
                    <path d="M11.6 16.8 a3 3 0 1 1-5.8-1.6"/>
                  </svg>
                </div>
              </div>
              <div className="mt-3">
                {marketingProvider ? (
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="font-bold text-[#0b1433] text-sm">{marketingProviderInfo?.name}</span>
                      <span className="bg-[#e3f6ee] text-[#1d9a6c] text-[10px] px-1.5 py-0.5 rounded-full font-semibold border border-[#b7e5d0]">
                        Active
                      </span>
                    </div>
                    {marketingSender && (
                      <div className="text-xs text-slate-400 mt-1 truncate max-w-[200px]" title={marketingSender.email}>
                        {marketingSender.email}
                      </div>
                    )}
                  </div>
                ) : (
                  <div>
                    <div className="text-xs text-slate-500 mb-2">{t`Not configured`}</div>
                    <Button size="small" type="primary" onClick={handleNavigateToSettings} className="text-xs">
                      {t`Configure`}
                    </Button>
                  </div>
                )}
              </div>
            </div>
          </Col>
        </Row>
      ) : null}

      {/* Email Metrics Chart - Full Width */}
      <EmailMetricsChart
        key={`email-metrics-${refreshKey}`}
        workspace={workspace}
        timeRange={timeRange}
        timezone={timezone}
      />

      {/* Brands already have Contacts nav hidden — hide this contact list too */}
      {isAdmin && (
        <div className="mt-8">
          <NewContactsTable key={`new-contacts-${refreshKey}`} workspace={workspace} />
        </div>
      )}

      <div className="mt-8">
        <FailedMessagesTable key={`failed-messages-${refreshKey}`} workspace={workspace} />
      </div>
    </div>
  )
}
