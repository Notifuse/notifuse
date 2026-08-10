import { useState } from 'react'
import { Button, Checkbox, DatePicker, Divider, Modal, Popover, Select, Space, Typography } from 'antd'
import { CheckOutlined, DownOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../lib/dayjs'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'
import { useWebAnalytics } from './context'
import { formatDateRangeLabel, validateDateRange } from './lib/dates'
import { ComparisonMode, DatePreset, Granularity, PRESET_GROUPS } from './lib/types'

/** Human labels for every preset, translated at render time. */
export function usePeriodLabels(): Record<DatePreset, string> {
  const { t } = useLingui()
  return {
    today: t`Today`,
    yesterday: t`Yesterday`,
    previous_7_days: t`Previous 7 days`,
    previous_14_days: t`Previous 14 days`,
    previous_28_days: t`Previous 28 days`,
    previous_30_days: t`Previous 30 days`,
    previous_90_days: t`Previous 90 days`,
    previous_91_days: t`Previous 91 days`,
    this_week: t`This week`,
    previous_week: t`Previous week`,
    this_month: t`Month to date`,
    previous_month: t`Previous month`,
    this_quarter: t`This quarter`,
    previous_quarter: t`Previous quarter`,
    this_year: t`Year to date`,
    previous_year: t`Previous year`,
    previous_12_months: t`Previous 12 months`,
    all_time: t`All time`,
    custom: t`Custom range`
  }
}

export function DateRangePicker({ size = 'middle' }: { size?: 'small' | 'middle' }) {
  const { t } = useLingui()
  const labels = usePeriodLabels()
  const { period, range, setPeriod, setCustomRange, customStart, customEnd, timezone, setTimezone } =
    useWebAnalytics()
  const [open, setOpen] = useState(false)
  const [customOpen, setCustomOpen] = useState(false)
  const [draft, setDraft] = useState<[dayjs.Dayjs, dayjs.Dayjs] | null>(null)
  const [problem, setProblem] = useState<string | null>(null)

  const label = period === 'custom' ? formatDateRangeLabel(range) : labels[period]

  const choose = (next: DatePreset) => {
    setOpen(false)
    if (next === 'custom') {
      setDraft(
        customStart && customEnd ? [dayjs(customStart), dayjs(customEnd)] : [range.start, range.end]
      )
      setProblem(null)
      setCustomOpen(true)
      return
    }
    setPeriod(next)
  }

  const applyCustom = () => {
    if (!draft) return
    const issue = validateDateRange({ start: draft[0], end: draft[1] })
    if (issue === 'end_before_start') return setProblem(t`End date must be after the start date`)
    if (issue === 'too_long') return setProblem(t`Maximum range is 2 years`)
    if (issue === 'in_future') return setProblem(t`Start date cannot be in the future`)
    setCustomRange(draft[0].format('YYYY-MM-DD'), draft[1].format('YYYY-MM-DD'))
    setCustomOpen(false)
  }

  const panel = (
    <div className="min-w-52 -mx-2">
      {PRESET_GROUPS.map((group, index) => (
        <div key={index}>
          {index > 0 ? <Divider className="!my-1" /> : null}
          {group.map((preset) => (
            <button
              key={preset}
              type="button"
              onClick={() => choose(preset)}
              className="flex w-full items-center justify-between gap-6 px-3 py-1.5 text-left text-sm hover:bg-gray-50"
            >
              <span>{labels[preset]}</span>
              {period === preset ? <CheckOutlined className="text-[var(--primary)]" /> : null}
            </button>
          ))}
        </div>
      ))}
      <Divider className="!my-1" />
      <div className="px-3 py-2">
        <div className="mb-1 text-xs text-gray-500">{t`Timezone`}</div>
        <Select
          showSearch
          size="small"
          className="w-full"
          value={timezone}
          options={TIMEZONE_OPTIONS}
          onChange={setTimezone}
        />
      </div>
    </div>
  )

  return (
    <>
      <Popover content={panel} trigger="click" open={open} onOpenChange={setOpen} placement="bottomRight">
        <Button size={size} color="default" variant="filled">
          {label} <DownOutlined className="text-xs" />
        </Button>
      </Popover>

      <Modal
        open={customOpen}
        title={t`Select a custom date range`}
        onCancel={() => setCustomOpen(false)}
        onOk={applyCustom}
        okText={t`Apply`}
        cancelText={t`Cancel`}
      >
        <DatePicker.RangePicker
          format="D MMM YYYY"
          variant="filled"
          className="w-full"
          value={draft}
          disabledDate={(current) => current && current > dayjs().endOf('day')}
          onChange={(value) => {
            setProblem(null)
            setDraft(value && value[0] && value[1] ? [value[0], value[1]] : null)
          }}
        />
        <Typography.Paragraph type={problem ? 'danger' : 'secondary'} className="!mt-3 !mb-0 text-xs">
          {problem ?? t`Maximum range is 2 years. Dates are read in ${timezone}.`}
        </Typography.Paragraph>
      </Modal>
    </>
  )
}

export function ComparisonPicker({ size = 'middle' }: { size?: 'small' | 'middle' }) {
  const { t } = useLingui()
  const { comparison, setComparison } = useWebAnalytics()

  return (
    <Space size={4}>
      <span className="text-sm text-gray-500">{t`vs.`}</span>
      <Select<ComparisonMode>
        size={size}
        variant="filled"
        className="min-w-36"
        value={comparison}
        onChange={setComparison}
        options={[
          { value: 'previous_period', label: t`Previous period` },
          { value: 'previous_year', label: t`Previous year` },
          { value: 'none', label: t`No comparison` }
        ]}
      />
    </Space>
  )
}

export function GranularitySelector() {
  const { t } = useLingui()
  const { granularity, availableGranularities, setGranularity } = useWebAnalytics()
  if (availableGranularities.length <= 1) return null

  const labels: Record<Granularity, string> = {
    hour: t`Hourly`,
    day: t`Daily`,
    week: t`Weekly`,
    month: t`Monthly`,
    year: t`Yearly`
  }

  return (
    <Select<Granularity>
      size="small"
      className="w-24"
      popupMatchSelectWidth={false}
      value={granularity}
      onChange={setGranularity}
      options={availableGranularities.map((value) => ({ value, label: labels[value] }))}
    />
  )
}

/** Mobile-friendly toggle that swaps table values for their change rates. */
export function EvolutionToggle(props: { checked: boolean; onChange: (checked: boolean) => void }) {
  const { t } = useLingui()
  return (
    <Checkbox checked={props.checked} onChange={(event) => props.onChange(event.target.checked)}>
      <span className="text-xs text-gray-500">{t`Show evolution`}</span>
    </Checkbox>
  )
}
