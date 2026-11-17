import { useState } from 'react'
import { DatePicker } from 'antd'
import type { MenuProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { dayBound } from './auditSearch'
import { FilterChip } from './FilterChip'
import { LOGS_PERIODS, type LogsPeriod, type LogsSearch } from './logsSearch'
import { customRangeValue, usePeriodLabels } from './periodLabels'

/** The rolling choices of the period menu, all time included; a custom range is picked below it. */
const MENU_PERIODS = LOGS_PERIODS.filter((period) => period !== 'custom')

export interface PeriodChipProps {
  period: LogsPeriod
  /** The bounds of a custom range, from the URL. */
  search: Pick<LogsSearch, 'from' | 'to'>
  timezone?: string
  /** Writes the period chosen, with the bounds of a custom range (cleared otherwise). */
  onChange: (patch: Pick<LogsSearch, 'period' | 'from' | 'to'>) => void
}

/**
 * The Period filter of the Logs tabs: the same rolling periods and all time on each, and a custom
 * range of whole days in the workspace's timezone.
 */
export function PeriodChip({ period, search, timezone, onChange }: PeriodChipProps) {
  const { t } = useLingui()
  const labels = usePeriodLabels()
  const [open, setOpen] = useState(false)
  const custom = period === 'custom' ? customRangeValue(search, timezone) : null

  const menu: MenuProps = {
    items: MENU_PERIODS.map((key) => ({ key, label: labels[key] })),
    selectable: true,
    selectedKeys: [period],
    onClick: ({ key }) => {
      setOpen(false)
      onChange({ period: key as LogsPeriod, from: undefined, to: undefined })
    }
  }

  return (
    <FilterChip
      label={t`Period`}
      value={custom ? `${custom[0].format('ll')} → ${custom[1].format('ll')}` : labels[period]}
      menu={menu}
      open={open}
      onOpenChange={setOpen}
      popupRender={(items) => (
        <div className="rounded-lg bg-surface p-1 shadow-popup">
          {items}
          <div className="flex flex-col gap-1 border-t border-line-soft px-2 pt-2 pb-1">
            <span className="text-xs text-ink-3">{labels.custom}</span>
            <DatePicker.RangePicker
              size="small"
              aria-label={labels.custom}
              value={custom}
              onChange={(range) => {
                if (!range || !range[0] || !range[1]) return
                setOpen(false)
                onChange({
                  period: 'custom',
                  from: dayBound(range[0], 'start', timezone),
                  to: dayBound(range[1], 'end', timezone)
                })
              }}
            />
          </div>
        </div>
      )}
    />
  )
}
