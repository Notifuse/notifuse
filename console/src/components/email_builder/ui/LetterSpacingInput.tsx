import React, { useState, useEffect } from 'react'
import { InputNumber, Segmented } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface LetterSpacingInputProps {
  value?: string
  onChange: (value: string | undefined) => void
  placeholder?: string
}

const LetterSpacingInput: React.FC<LetterSpacingInputProps> = ({
  value,
  onChange,
  placeholder
}) => {
  const { t } = useLingui()
  const [numericValue, setNumericValue] = useState<number | undefined>()
  const resolvedPlaceholder = placeholder || t`Enter spacing`
  const [unit, setUnit] = useState<'px' | 'em'>('px')

  // Parse incoming value to extract number and unit
  useEffect(() => {
    /* eslint-disable react-hooks/set-state-in-effect */
    if (value && value !== 'none' && value !== 'normal') {
      const match = value.match(/^(-?\d+(?:\.\d+)?)(px|em)$/)
      if (match) {
        setNumericValue(parseFloat(match[1]))
        setUnit(match[2] as 'px' | 'em')
      } else {
        // Try to parse as just a number (assume px)
        const numValue = parseFloat(value)
        if (!isNaN(numValue)) {
          setNumericValue(numValue)
          setUnit('px')
        } else {
          setNumericValue(undefined)
          setUnit('px')
        }
      }
    } else {
      setNumericValue(undefined)
      setUnit('px')
    }
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [value])

  const handleNumberChange = (newValue: number | null) => {
    setNumericValue(newValue || undefined)
    if (newValue !== null && newValue !== undefined) {
      onChange(`${newValue}${unit}`)
    } else {
      onChange(undefined)
    }
  }

  const handleUnitChange = (newUnit: string) => {
    const unitValue = newUnit as 'px' | 'em'
    setUnit(unitValue)
    if (numericValue !== undefined && numericValue !== null) {
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
        step={unit === 'em' ? 0.1 : 1}
        className="flex-1"
        style={{ width: 80 }}
        precision={unit === 'em' ? 2 : 0}
      />
      <Segmented
        size="small"
        value={unit}
        onChange={handleUnitChange}
        options={[
          { label: 'px', value: 'px' },
          { label: 'em', value: 'em' }
        ]}
        style={{ width: 70 }}
      />
    </div>
  )
}

export default LetterSpacingInput
