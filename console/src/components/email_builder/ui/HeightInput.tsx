import React, { useState, useEffect } from 'react'
import { InputNumber, Radio } from 'antd'

interface HeightInputProps {
  value?: string
  onChange: (value: string | undefined) => void
  placeholder?: string
}

const HeightInput: React.FC<HeightInputProps> = ({
  value,
  onChange
}) => {
  const [mode, setMode] = useState<'auto' | 'custom'>('auto')
  const [numericValue, setNumericValue] = useState<number | undefined>()

  // Parse incoming value to determine mode and numeric value
  useEffect(() => {
    /* eslint-disable react-hooks/set-state-in-effect */
    if (value === 'auto' || !value) {
      setMode('auto')
      setNumericValue(undefined)
    } else {
      // Try to parse as a number with px suffix
      const match = value.match(/^(\d+(?:\.\d+)?)px?$/)
      if (match) {
        setMode('custom')
        setNumericValue(parseFloat(match[1]))
      } else {
        // Try to parse as just a number (assume px)
        const numValue = parseFloat(value)
        if (!isNaN(numValue)) {
          setMode('custom')
          setNumericValue(numValue)
        } else {
          setMode('auto')
          setNumericValue(undefined)
        }
      }
    }
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [value])

  const handleModeChange = (e: import('antd').RadioChangeEvent) => {
    const modeValue = e.target.value as 'auto' | 'custom'
    setMode(modeValue)

    if (modeValue === 'auto') {
      onChange('auto')
      setNumericValue(undefined)
    } else if (numericValue) {
      onChange(`${numericValue}px`)
    } else {
      onChange('100px')
    }
  }

  const handleNumberChange = (newValue: number | null) => {
    setNumericValue(newValue || undefined)
    if (newValue) {
      onChange(`${newValue}px`)
    } else if (mode === 'custom') {
      onChange(undefined)
    }
  }

  return (
    <div className="flex items-center gap-1">
      <Radio.Group
        size="small"
        value={mode}
        onChange={handleModeChange}
        optionType="button"
        buttonStyle="solid"
        style={{
          verticalAlign: 'baseline'
        }}
        className="!mr-2"
        options={[
          { label: 'auto', value: 'auto' },
          { label: 'px', value: 'custom' }
        ]}
      />
      {mode === 'custom' && (
        <InputNumber
          size="small"
          value={numericValue}
          onChange={handleNumberChange}
          placeholder="Height"
          min={0}
          step={10}
          style={{ width: 90 }}
          addonAfter="px"
        />
      )}
    </div>
  )
}

export default HeightInput
