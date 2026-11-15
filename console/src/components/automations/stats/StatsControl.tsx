import { Segmented, Switch, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { StatsPeriod } from './nodeStatsState'

interface StatsControlProps {
  on: boolean
  onToggle: (on: boolean) => void
  period: StatsPeriod
  onPeriodChange: (period: StatsPeriod) => void
  /** Why the stats cannot be shown yet (unsaved, no journeys); the switch is then off and locked. */
  disabledReason?: string
}

/** Shows each step's results on the canvas, over the chosen period. */
export function StatsControl({ on, onToggle, period, onPeriodChange, disabledReason }: StatsControlProps) {
  const { t } = useLingui()
  return (
    <div
      className="flex items-center gap-2 rounded-md border border-line bg-surface px-2 py-1 shadow-sm"
      data-stats-control
    >
      <Tooltip title={disabledReason}>
        {/* A disabled switch gets no pointer events: the span carries the tooltip. */}
        <span className="flex items-center gap-1.5">
          <span className="text-sm text-ink-2">{t`Stats`}</span>
          <Switch
            size="small"
            aria-label={t`Show stats`}
            checked={on}
            disabled={!!disabledReason}
            onChange={onToggle}
          />
        </span>
      </Tooltip>
      {on ? (
        <Segmented<StatsPeriod>
          size="small"
          aria-label={t`Period`}
          value={period}
          onChange={onPeriodChange}
          options={[
            { value: 'last7', label: t`7 d` },
            { value: 'last30', label: t`30 d` },
            { value: 'all', label: t`All time` }
          ]}
        />
      ) : null}
    </div>
  )
}
