import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { InputNumber, Select, Switch } from 'antd'
import { fromDays, inUnit, maxInUnit, toDays, type PeriodUnit } from './periods'

interface PeriodInputProps {
  /** Days; 0 is off. */
  days: number
  /** The period switching the category on gives. */
  onDays: number
  onChange: (days: number) => void
  label: string
  disabled?: boolean
}

// A switch, a whole number and a unit. The unit is the person's choice, kept here: deriving it
// from the days as they type would switch "30 days" to "1 month" in the middle of "300".
export function PeriodInput({ days, onDays, onChange, label, disabled }: PeriodInputProps) {
  const { t } = useLingui()
  const [unit, setUnit] = useState<PeriodUnit>(() => fromDays(days > 0 ? days : onDays).unit)
  const on = days > 0
  return (
    <div className="flex items-center gap-2">
      <Switch
        size="small"
        checked={on}
        disabled={disabled}
        aria-label={t`Limit how long ${label} is kept`}
        onChange={(checked) => {
          if (checked) setUnit(fromDays(onDays).unit)
          onChange(checked ? onDays : 0)
        }}
      />
      <InputNumber
        size="small"
        min={1}
        max={maxInUnit(unit)}
        precision={0}
        className="w-20"
        disabled={disabled || !on}
        aria-label={t`${label}: period`}
        value={on ? inUnit(days, unit) : null}
        onChange={(value) => {
          if (typeof value === 'number' && value > 0) onChange(toDays({ value: Math.round(value), unit }))
        }}
      />
      <Select<PeriodUnit>
        size="small"
        className="w-24"
        disabled={disabled || !on}
        aria-label={t`${label}: unit`}
        value={unit}
        options={[
          { value: 'days', label: t`days` },
          { value: 'months', label: t`months` },
          { value: 'years', label: t`years` }
        ]}
        onChange={(next) => {
          const value = Math.min(inUnit(days, unit), maxInUnit(next))
          setUnit(next)
          onChange(toDays({ value, unit: next }))
        }}
      />
    </div>
  )
}
