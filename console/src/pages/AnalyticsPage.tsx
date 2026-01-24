import { useState, useEffect } from 'react'
import { useParams } from '@tanstack/react-router'
import { Segmented, Select, Space } from 'antd'
import dayjs from 'dayjs'
import { useAuth } from '../contexts/AuthContext'
import { AnalyticsDashboard } from '../components/analytics/AnalyticsDashboard'
import { TIMEZONE_OPTIONS } from '../lib/timezones'
import { getBrowserTimezone } from '../lib/timezoneNormalizer'
import { useLingui } from '@lingui/react/macro'

type TimePeriod = '7D' | '14D' | '30D' | '90D'

export function AnalyticsPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { workspaces } = useAuth()

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
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div className="text-2xl font-medium">{t`Dashboard`}</div>
        <Space>
          <Select
            value={selectedTimezone}
            onChange={handleTimezoneChange}
            options={TIMEZONE_OPTIONS}
            optionFilterProp="label"
            variant="filled"
            style={{ width: 170 }}
            placeholder={t`Select timezone`}
            showSearch
            filterOption={(input, option) =>
              (option?.label ?? '').toString().toLowerCase().includes(input.toLowerCase())
            }
          />
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
        </Space>
      </div>
      <AnalyticsDashboard workspace={workspace} timeRange={timeRange} timezone={selectedTimezone} />
    </div>
  )
}
