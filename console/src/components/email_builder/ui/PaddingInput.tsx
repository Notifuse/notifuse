import React, { useState, useEffect, memo, useCallback } from 'react'
import { InputNumber, Row, Col } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface PaddingValues {
  top: string | undefined
  right: string | undefined
  bottom: string | undefined
  left: string | undefined
}

interface IndividualPaddingProps {
  value?: PaddingValues
  defaultValue?: PaddingValues
  onChange?: (value: PaddingValues) => void
}

interface MjmlPaddingProps {
  value?: string
  defaultValue?: string
  onChange?: (value: string | undefined) => void
}

type PaddingInputProps = IndividualPaddingProps | MjmlPaddingProps

const isIndividualPadding = (props: PaddingInputProps): props is IndividualPaddingProps => {
  return (
    typeof (props as IndividualPaddingProps).value === 'object' ||
    (props as IndividualPaddingProps).value === undefined
  )
}

/**
 * PaddingInput component that supports both individual padding properties and MJML shorthand format
 * Automatically detects format based on value prop type
 */
const PaddingInput: React.FC<PaddingInputProps> = memo((props) => {
  const { t } = useLingui()
  const isIndividual = isIndividualPadding(props)

  /**
   * Parse MJML padding shorthand to individual values
   */
  const parsePaddingShorthand = useCallback((padding?: string): PaddingValues => {
    if (!padding) {
      return { top: undefined, right: undefined, bottom: undefined, left: undefined }
    }

    const parts = padding.trim().split(/\s+/)

    switch (parts.length) {
      case 1:
        // "10px" - all sides
        return { top: parts[0], right: parts[0], bottom: parts[0], left: parts[0] }
      case 2:
        // "10px 25px" - vertical horizontal
        return { top: parts[0], right: parts[1], bottom: parts[0], left: parts[1] }
      case 3:
        // "10px 25px 5px" - top horizontal bottom
        return { top: parts[0], right: parts[1], bottom: parts[2], left: parts[1] }
      case 4:
        // "10px 25px 5px 15px" - top right bottom left
        return { top: parts[0], right: parts[1], bottom: parts[2], left: parts[3] }
      default:
        return { top: undefined, right: undefined, bottom: undefined, left: undefined }
    }
  }, [])

  /**
   * Format individual values back to MJML padding shorthand
   */
  const formatPaddingShorthand = useCallback((values: PaddingValues): string | undefined => {
    const { top, right, bottom, left } = values

    // If all are empty, return undefined
    if (!top && !right && !bottom && !left) {
      return undefined
    }

    // Use defaults for missing values
    const topVal = top || '0px'
    const rightVal = right || '0px'
    const bottomVal = bottom || '0px'
    const leftVal = left || '0px'

    // Check for shorthand opportunities
    if (topVal === rightVal && rightVal === bottomVal && bottomVal === leftVal) {
      // All sides the same: "10px"
      return topVal
    } else if (topVal === bottomVal && rightVal === leftVal) {
      // Vertical/horizontal: "10px 25px"
      return `${topVal} ${rightVal}`
    } else {
      // Full format: "10px 25px 5px 15px"
      return `${topVal} ${rightVal} ${bottomVal} ${leftVal}`
    }
  }, [])

  // Convert values to internal format
  const getInternalValue = useCallback((): PaddingValues => {
    if (isIndividual) {
      return (
        props.value ||
        props.defaultValue || {
          top: undefined,
          right: undefined,
          bottom: undefined,
          left: undefined
        }
      )
    } else {
      return (
        parsePaddingShorthand(props.value) ||
        parsePaddingShorthand(props.defaultValue) || {
          top: undefined,
          right: undefined,
          bottom: undefined,
          left: undefined
        }
      )
    }
  }, [isIndividual, props.value, props.defaultValue, parsePaddingShorthand])

  const [localValue, setLocalValue] = useState<PaddingValues>(getInternalValue())

  useEffect(() => {
    setLocalValue(getInternalValue())
  }, [getInternalValue])

  const handleValueChange = (
    position: keyof PaddingValues,
    newValue: number | null | undefined
  ) => {
    const formattedValue = newValue != null ? `${newValue}px` : undefined
    const updatedValue = { ...localValue, [position]: formattedValue }
    setLocalValue(updatedValue)

    // Call appropriate onChange based on prop type
    if (isIndividual) {
      props.onChange?.(updatedValue)
    } else {
      const shorthandValue = formatPaddingShorthand(updatedValue)
      props.onChange?.(shorthandValue)
    }
  }

  const parsePixelValue = (value: string | undefined): number => {
    if (!value) return 0
    const num = parseInt(value.replace('px', ''))
    return isNaN(num) ? 0 : num
  }

  return (
    <div>
      <Row gutter={16}>
        <Col span={5}>
          <span className="text-xs text-gray-500">{t`Top`}</span>
        </Col>
        <Col span={7}>
          <InputNumber
            value={parsePixelValue(localValue.top)}
            onChange={(val) => handleValueChange('top', val)}
            size="small"
            style={{ width: '100%' }}
            suffix="px"
          />
        </Col>
        <Col span={5}>
          <span className="text-xs text-gray-500">{t`Right`}</span>
        </Col>
        <Col span={7}>
          <InputNumber
            value={parsePixelValue(localValue.right)}
            onChange={(val) => handleValueChange('right', val)}
            size="small"
            style={{ width: '100%' }}
            suffix="px"
          />
        </Col>
      </Row>

      <Row gutter={16} className="mt-4">
        <Col span={5}>
          <span className="text-xs text-gray-500">{t`Bottom`}</span>
        </Col>
        <Col span={7}>
          <InputNumber
            value={parsePixelValue(localValue.bottom)}
            onChange={(val) => handleValueChange('bottom', val)}
            size="small"
            style={{ width: '100%' }}
            suffix="px"
          />
        </Col>
        <Col span={5}>
          <span className="text-xs text-gray-500">{t`Left`}</span>
        </Col>
        <Col span={7}>
          <InputNumber
            value={parsePixelValue(localValue.left)}
            onChange={(val) => handleValueChange('left', val)}
            size="small"
            style={{ width: '100%' }}
            suffix="px"
          />
        </Col>
      </Row>
    </div>
  )
})

export default PaddingInput
