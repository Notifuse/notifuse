import React, { memo, useState } from 'react'
import { Button, Input, InputNumber, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface BorderRadiusInputProps {
  value?: string
  onChange?: (value: string | undefined) => void
  placeholder?: string
  defaultValue?: string
  min?: number
  max?: number
  step?: number
  disabled?: boolean
  size?: 'small' | 'middle' | 'large'
  style?: React.CSSProperties
}

const BorderRadiusInput: React.FC<BorderRadiusInputProps> = memo(
  ({
    value,
    onChange,
    placeholder,
    defaultValue,
    min = 0,
    max = 100,
    step = 1,
    disabled = false,
    size = 'small',
    style = { width: '100px' }
  }) => {
    const { t } = useLingui()
    // The value the number field was asked for while it held one it cannot read
    const [numberFieldFor, setNumberFieldFor] = useState<string | undefined>()
    // Once text is typed, the text field stays until the px button: a value passing through a number while it is
    // typed ("50%" → "50" → "50px") must not swap the field under the cursor
    const [typingText, setTypingText] = useState(false)

    /**
     * Parse border radius to get numeric value
     */
    const parseBorderRadiusNumber = (borderRadius?: string): number | undefined => {
      if (!borderRadius) return undefined
      const match = borderRadius.match(/^(\d+(?:\.\d+)?)px?$/)
      return match ? parseFloat(match[1]) : undefined
    }

    const handleChange = (numValue: number | null) => {
      const formattedValue =
        numValue !== null && numValue !== undefined ? `${numValue}px` : undefined
      onChange?.(formattedValue)
    }

    const parsedValue = parseBorderRadiusNumber(value)

    // MJML 5 takes any border-radius ("50%", "10px 0", "4px / 8px"): one the number field cannot read is shown and
    // edited as text, so that it is neither hidden nor lost
    if (typingText || (value && parsedValue === undefined && numberFieldFor !== value)) {
      return (
        <div className="flex items-center gap-1" style={style}>
          <Input
            size={size}
            value={value ?? ''}
            onChange={(e) => {
              setTypingText(true)
              onChange?.(e.target.value.trim() === '' ? undefined : e.target.value)
            }}
            disabled={disabled}
            aria-label={t`Border radius`}
          />
          <Tooltip title={t`Enter a number of pixels instead`}>
            <Button
              size={size}
              onClick={() => {
                setTypingText(false)
                setNumberFieldFor(value)
              }}
              disabled={disabled}
            >
              px
            </Button>
          </Tooltip>
        </div>
      )
    }

    const parsedPlaceholder = placeholder || (parseBorderRadiusNumber(defaultValue) || 0).toString()

    return (
      <InputNumber
        size={size}
        value={parsedValue}
        onChange={handleChange}
        placeholder={parsedPlaceholder}
        min={min}
        max={max}
        step={step}
        suffix="px"
        disabled={disabled}
        style={style}
      />
    )
  }
)

export default BorderRadiusInput
