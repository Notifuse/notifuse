import { useState } from 'react'
import { InputNumber, Segmented } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { FieldWrite } from './types'
import { useCoalesceKey } from './useCoalesceKey'

export interface NumberUnitFieldProps {
  /** The block's own value: '50%', '120px', 'auto', a bare number (read in the default unit), or undefined. */
  value?: string
  /** The value the block inherits, shown as the placeholder while `value` is undefined. */
  inherited?: string
  /** Shown when neither the block nor its inheritance holds a value ('Auto', 'None'). */
  placeholder?: string
  /** The units offered: one unit is shown as a suffix, several as a segmented control. Ignored in height mode. */
  units?: readonly string[]
  /** 'height': an Auto / px control instead of the units, with HeightInput's writes ('auto', then '100px'). */
  mode?: 'unit' | 'height'
  /** The unit used while the block and its inheritance hold no unit (WidthInput's `defaultUnit`). */
  defaultUnit?: string
  min?: number
  max?: number
  /** One step, or a step per unit (WidthInput: 1 for %, 10 for px). */
  step?: number | Readonly<Record<string, number>>
  onChange: FieldWrite<string | undefined>
  /** The block id and the field's keys: the base of the key that makes one focus session one undo step. */
  coalesceBase: string
  disabled?: boolean
  'aria-label': string
}

interface Parsed {
  number?: number
  unit?: string
}

// parseLength reads '50%', '120px' or '120' (no unit). Like WidthInput and HeightInput, a value with a
// unit the field does not offer still shows its number (parseFloat), in the field's unit.
function parseLength(value: string | undefined, units: readonly string[]): Parsed {
  if (value === undefined) return {}
  const match = /^\s*(-?\d+(?:\.\d+)?)\s*([a-z%]*)\s*$/i.exec(value)
  if (match) {
    const unit = match[2].toLowerCase()
    return { number: parseFloat(match[1]), unit: unit !== '' && units.includes(unit) ? unit : undefined }
  }
  const number = parseFloat(value)
  return isNaN(number) ? {} : { number }
}

const HEIGHT_UNITS = ['px'] as const

// A number with its unit: width in % or px, a size in px, or a height that is Auto or a number of px.
// Every typed write carries the focus session's coalesce key (one undo step per focus); a unit or mode
// switch is a click, written without one. min / max are antd's: a value typed out of bounds is not
// written while typed, and is clamped on blur (that write keeps the session's key).
export function NumberUnitField({
  value,
  inherited,
  placeholder,
  units = ['px'],
  mode = 'unit',
  defaultUnit,
  min = 0,
  max,
  step = 1,
  onChange,
  coalesceBase,
  disabled = false,
  'aria-label': ariaLabel
}: NumberUnitFieldProps) {
  const { t } = useLingui()
  const { coalesceKey, onFocus } = useCoalesceKey(coalesceBase)
  // A unit picked while the field holds no number: kept for the value it was picked on, so a new value
  // from outside (undo, another block) drops it.
  const [picked, setPicked] = useState<{ for: string | undefined; unit: string } | undefined>()

  const offered = mode === 'height' ? HEIGHT_UNITS : units
  const own = parseLength(value, offered)
  const inheritedParsed = parseLength(inherited, offered)
  const fallbackUnit = defaultUnit ?? offered[0]

  const unit =
    value !== undefined
      ? (own.unit ?? fallbackUnit)
      : picked && picked.for === value
        ? picked.unit
        : (inheritedParsed.unit ?? fallbackUnit)

  // Height mode: 'auto', or a value it cannot read, shows Auto (HeightInput); unset follows the inheritance.
  const heightMode: 'auto' | 'px' =
    value === undefined
      ? inherited !== undefined && inherited !== 'auto' && inheritedParsed.number !== undefined
        ? 'px'
        : 'auto'
      : value !== 'auto' && own.number !== undefined
        ? 'px'
        : 'auto'

  const number = mode === 'height' && heightMode === 'auto' ? undefined : own.number

  const placeholderText = (() => {
    if (value !== undefined) return undefined
    if (inherited === undefined || inherited === '') return placeholder
    if (inheritedParsed.number === undefined) return inherited
    // The inherited number in the unit shown, or the inherited text when its unit is another one.
    const inheritedUnit = inheritedParsed.unit ?? fallbackUnit
    return inheritedUnit === unit ? String(inheritedParsed.number) : inherited
  })()

  const stepFor = typeof step === 'number' ? step : (step[unit] ?? 1)

  const handleNumber = (next: number | string | null) => {
    const options = { coalesceKey: coalesceKey() }
    if (next === null || next === '' || next === undefined) {
      onChange(undefined, options)
      return
    }
    onChange(`${next}${unit}`, options)
  }

  const handleUnit = (nextUnit: string) => {
    if (own.number !== undefined) {
      onChange(`${own.number}${nextUnit}`)
      return
    }
    setPicked({ for: value, unit: nextUnit })
  }

  // Switching to px starts at 100px, as HeightInput did; Auto writes 'auto'.
  const handleHeightMode = (next: string) => {
    onChange(next === 'auto' ? 'auto' : '100px')
  }

  const input = (
    <InputNumber
      size="small"
      className="min-w-0 flex-1"
      value={number}
      onChange={handleNumber}
      onFocus={onFocus}
      placeholder={placeholderText}
      min={min}
      max={max}
      step={stepFor}
      suffix={mode === 'unit' && offered.length === 1 ? unit : undefined}
      disabled={disabled || (mode === 'height' && heightMode === 'auto')}
      aria-label={ariaLabel}
    />
  )

  if (mode === 'height') {
    return (
      <div className="flex min-w-0 items-center gap-1">
        <Segmented
          size="small"
          value={heightMode}
          onChange={(next) => handleHeightMode(String(next))}
          options={[
            { value: 'auto', label: t`Auto` },
            { value: 'px', label: 'px' }
          ]}
          disabled={disabled}
          aria-label={t`${ariaLabel} mode`}
        />
        {input}
      </div>
    )
  }

  if (offered.length === 1) return input

  return (
    <div className="flex min-w-0 items-center gap-1">
      {input}
      <Segmented
        size="small"
        value={unit}
        onChange={(next) => handleUnit(String(next))}
        options={offered.map((u) => ({ value: u, label: u }))}
        disabled={disabled}
        aria-label={t`${ariaLabel} unit`}
      />
    </div>
  )
}
