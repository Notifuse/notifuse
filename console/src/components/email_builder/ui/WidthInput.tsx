import React, { useState, useEffect } from 'react'
import { InputNumber, Segmented } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface WidthInputProps {
  value?: string
  onChange: (value: string | undefined) => void
  placeholder?: string
}

const WidthInput: React.FC<WidthInputProps> = ({
  value,
  onChange,
  placeholder
}) => {
  const { t } = useLingui()
  const [numericValue, setNumericValue] = useState<number | undefined>()
  const resolvedPlaceholder = placeholder || t`Enter width`
  const [unit, setUnit] = useState<'px' | '%'>('%')

  // Parse incoming value to extract number and unit
  useEffect(() => {
    /* eslint-disable react-hooks/set-state-in-effect */
    if (value) {
      const match = value.match(/^(\d+(?:\.\d+)?)(px|%)$/)
      if (match) {
        setNumericValue(parseFloat(match[1]))
        setUnit(match[2] as 'px' | '%')
      } else {
        // Try to parse as just a number (assume %)
        const numValue = parseFloat(value)
        if (!isNaN(numValue)) {
          setNumericValue(numValue)
          setUnit('%')
        } else {
          setNumericValue(undefined)
          setUnit('%')
        }
      }
    } else {
      setNumericValue(undefined)
      setUnit('%')
    }
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [value])

  const handleNumberChange = (newValue: number | null) => {
    setNumericValue(newValue || undefined)
    if (newValue) {
      onChange(`${newValue}${unit}`)
    } else {
      onChange(undefined)
    }
  }

  const handleUnitChange = (newUnit: string) => {
    const unitValue = newUnit as 'px' | '%'
    setUnit(unitValue)
    if (numericValue) {
      onChange(`${numericValue}${unitValue}`)
    }
  }

  return (
    <div className="flex items-center gap-1">
      <InputNumber
        size="small"
        value={numericValue}
        onChange={handleNumberChange}
        placeholder={resolvedPlaceholder}
        min={0}
        step={unit === '%' ? 1 : 10}
        className="flex-1"
        style={{ width: 80 }}
      />
      <Segmented
        size="small"
        value={unit}
        onChange={handleUnitChange}
        options={[
          { label: '%', value: '%' },
          { label: 'px', value: 'px' }
        ]}
        style={{ width: 70 }}
      />
    </div>
  )
}

export default WidthInput
