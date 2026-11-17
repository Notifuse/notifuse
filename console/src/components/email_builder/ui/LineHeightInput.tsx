import React, { useState, useEffect } from 'react'
import { InputNumber, Segmented, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import {
  convertLineHeight,
  DEFAULT_LINE_HEIGHT_FONT_SIZE,
  parseLineHeight,
  roundLineHeight,
  type LineHeightUnit
} from './lineHeight'

interface LineHeightInputProps {
  value?: string
  onChange: (value: string | undefined) => void
  placeholder?: string
  /** The block's font size in pixels, to switch between pixels and the other units. */
  fontSize?: number
}

const suffix = (unit: LineHeightUnit) => (unit === 'x' ? '' : unit)

const LineHeightInput: React.FC<LineHeightInputProps> = ({
  value,
  onChange,
  placeholder,
  fontSize = DEFAULT_LINE_HEIGHT_FONT_SIZE
}) => {
  const { t } = useLingui()
  const [numericValue, setNumericValue] = useState<number | undefined>()
  const resolvedPlaceholder = placeholder || t`Enter height`
  const [unit, setUnit] = useState<LineHeightUnit>('x')

  // Parse incoming value to extract number and unit
  useEffect(() => {
    /* eslint-disable react-hooks/set-state-in-effect */
    const parsed = parseLineHeight(value)
    setNumericValue(parsed.number)
    // An empty value keeps the unit the author picked.
    if (parsed.number !== undefined) setUnit(parsed.unit)
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [value])

  // The field shows the number that is stored, rounded the same way in both places.
  const handleNumberChange = (newValue: number | null) => {
    if (newValue !== null && newValue !== undefined) {
      const rounded = roundLineHeight(newValue, unit)
      setNumericValue(rounded)
      onChange(`${rounded}${suffix(unit)}`)
    } else {
      setNumericValue(undefined)
      onChange(undefined)
    }
  }

  const handleUnitChange = (newUnit: string) => {
    const unitValue = newUnit as LineHeightUnit
    // Switching units keeps the rendered height: 1.5 is 150 %, and 24px at a 16px font size.
    const next =
      numericValue === undefined
        ? undefined
        : convertLineHeight(numericValue, unit, unitValue, fontSize)
    setUnit(unitValue)
    setNumericValue(next)
    if (next !== undefined) {
      onChange(`${next}${suffix(unitValue)}`)
    }
  }

  return (
    <div className="flex items-center gap-1">
      <InputNumber
        size="small"
        value={numericValue}
        onChange={handleNumberChange}
        placeholder={resolvedPlaceholder}
        aria-label={t`Line height`}
        min={0}
        step={unit === 'x' ? 0.1 : unit === '%' ? 10 : 1}
        style={{ width: 80 }}
      />
      <Segmented
        size="small"
        value={unit}
        onChange={handleUnitChange}
        options={[
          {
            label: (
              <Tooltip title={t`Times the font size`}>
                <span>×</span>
              </Tooltip>
            ),
            value: 'x'
          },
          { label: '%', value: '%' },
          { label: 'px', value: 'px' }
        ]}
      />
    </div>
  )
}

export default LineHeightInput
