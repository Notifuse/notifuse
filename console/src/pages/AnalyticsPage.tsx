import { useState, useEffect } from 'react'
import { useParams } from '@tanstack/react-router'
import { Segmented, Select } from 'antd'
import dayjs from 'dayjs'
import { useAuth } from '../contexts/AuthContext'
import { AnalyticsDashboard } from '../components/analytics/AnalyticsDashboard'
import { TIMEZONE_OPTIONS } from '../lib/timezones'
import { getBrowserTimezone } from '../lib/timezoneNormalizer'
import { isMailStudioAdmin } from '../lib/hidden-mail-studio-nav'
import { useLingui } from '@lingui/react/macro'

type TimePeriod = '7D' | '14D' | '30D' | '90D'

export function AnalyticsPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { workspaces, user } = useAuth()
  const isAdmin = isMailStudioAdmin(user?.email)

  const [selectedPeriod, setSelectedPeriod] = useState<TimePeriod>('14D')
  const [selectedTimezone, setSelectedTimezone] = useState<string>('')

  const workspace = workspaces.find((w) => w.id === workspaceId)

  // Get browser timezone on component mount (normalized to canonical IANA name)
  useEffect(() => {
    const browserTimezone = getBrowserTimezone()
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSelectedTimezone(browserTimezone)
  }, [])

  // Calculate time range based on selected period
  const getTimeRangeFromPeriod = (period: TimePeriod): [string, string] => {
    const endDate = dayjs().add(1, 'day') // Use tomorrow instead of today
    let startDate: dayjs.Dayjs

    switch (period) {
      case '7D':
        startDate = endDate.subtract(7, 'days')
        break
      case '14D':
        startDate = endDate.subtract(14, 'days')
        break
      case '30D':
        startDate = endDate.subtract(30, 'days')
        break
      case '90D':
        startDate = endDate.subtract(90, 'days')
        break
      default:
        startDate = endDate.subtract(30, 'days')
    }

    return [startDate.format('YYYY-MM-DD'), endDate.format('YYYY-MM-DD')]
  }

  const timeRange = getTimeRangeFromPeriod(selectedPeriod)

  const handlePeriodChange = (value: TimePeriod) => {
    setSelectedPeriod(value)
  }

  const handleTimezoneChange = (value: string) => {
    setSelectedTimezone(value)
  }

  if (!workspace) {
    return (
      <div style={{ padding: '24px', textAlign: 'center' }}>
        <h2>{t`Workspace not found`}</h2>
        <p>{t`The requested workspace could not be found.`}</p>
      </div>
    )
  }

  return (
    <div className="p-2 sm:p-4 md:p-6 space-y-4 sm:space-y-6">
      {/* Mail Studio light dashboard hero */}
      <div className="mailstudio-glass-hero p-4 sm:p-6 rounded-2xl border border-[#e6ecf8] shadow-[0_8px_24px_rgba(11,20,51,0.08)] flex flex-col md:flex-row justify-between items-start md:items-center gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-xl sm:text-2xl font-bold text-[#0b1433] tracking-tight m-0">{t`Dashboard`}</h1>
            <span className="bg-[#e8efff] text-[#2457d6] text-[11px] sm:text-xs px-2.5 py-0.5 rounded-full font-semibold border border-[#c7d2e8]">
              Live Monitor
            </span>
          </div>
          <p className="text-xs sm:text-sm text-[#5e6b8c] mt-1 mb-0">
            {isAdmin
              ? t`Real-time dispatch performance, audience metrics, and email provider health.`
              : t`Real-time email send performance for this workspace.`}
          </p>
        </div>

        <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2.5 w-full md:w-auto">
          {isAdmin ? (
            <div className="flex items-center justify-between sm:justify-start gap-2 bg-white backdrop-blur-sm p-1 rounded-xl border border-[#e6ecf8] shadow-sm">
              <span className="text-xs text-[#5e6b8c] font-medium pl-2.5">{t`TZ:`}</span>
              <Select
                value={selectedTimezone}
                onChange={handleTimezoneChange}
                options={TIMEZONE_OPTIONS}
                optionFilterProp="label"
                variant="borderless"
                style={{ width: 145 }}
                placeholder={t`Select timezone`}
                className="text-xs font-medium text-[#0b1433]"
                showSearch
                filterOption={(input, option) =>
                  (option?.label ?? '').toString().toLowerCase().includes(input.toLowerCase())
                }
              />
            </div>
          ) : null}

          <div className="bg-white backdrop-blur-sm p-1 rounded-xl border border-[#e6ecf8] shadow-sm flex justify-center">
            <Segmented
              value={selectedPeriod}
              onChange={handlePeriodChange}
              options={[
                { label: '7D', value: '7D' },
                { label: '14D', value: '14D' },
                { label: '30D', value: '30D' },
                { label: '90D', value: '90D' }
              ]}
            />
          </div>
        </div>
      </div>

      <AnalyticsDashboard workspace={workspace} timeRange={timeRange} timezone={selectedTimezone} />
    </div>
  )
}
