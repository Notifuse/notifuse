import React, { useState, useEffect, useCallback } from 'react'
import { Switch, Select, InputNumber } from 'antd'
import { useLingui } from '@lingui/react/macro'
import ColorPickerWithPresets from './ColorPickerWithPresets'

interface BorderValue {
  color?: string
  width?: string
  style?: string
}

interface BorderInputValue {
  top?: BorderValue
  right?: BorderValue
  bottom?: BorderValue
  left?: BorderValue
}

interface MjmlBorderValue {
  borderTop?: string
  borderRight?: string
  borderBottom?: string
  borderLeft?: string
}

interface BorderInputProps {
  value?: MjmlBorderValue
  onChange: (value: MjmlBorderValue) => void
  placeholder?: {
    color?: string
    width?: string
    style?: string
  }
  className?: string
}

// Moved inside component to use translation

/**
 * BorderInput component for managing border properties on all four sides
 * Supports unified mode (all borders same) and separate mode (individual borders)
 * Automatically detects mode based on whether border values differ
 * Handles MJML border format conversion internally
 */
const BorderInput: React.FC<BorderInputProps> = ({
  value = {},
  onChange,
  placeholder = { color: 'transparent', width: '0px', style: 'none' },
  className
}) => {
  const { t } = useLingui()
  const [mode, setMode] = useState<'all' | 'separate'>('all')

  const borderStyles = [
    { label: t`None`, value: 'none' },
    { label: t`Solid`, value: 'solid' },
    { label: t`Dashed`, value: 'dashed' },
    { label: t`Dotted`, value: 'dotted' },
    { label: t`Double`, value: 'double' },
    { label: t`Groove`, value: 'groove' },
    { label: t`Ridge`, value: 'ridge' },
    { label: t`Inset`, value: 'inset' },
    { label: t`Outset`, value: 'outset' }
  ]
  const [manualModeSet, setManualModeSet] = useState(false)

  /**
   * Parse MJML border value to BorderValue format
   */
  const parseBorderValue = useCallback((borderStr?: string): BorderValue => {
    if (!borderStr || borderStr === 'none') {
      return { width: '0px', style: 'none', color: 'transparent' }
    }

    // Parse CSS border shorthand like "1px solid #000"
    const parts = borderStr.trim().split(' ')
    return {
      width: parts[0] || '0px',
      style: parts[1] || 'none',
      color: parts[2] || 'transparent'
    }
  }, [])

  /**
   * Format BorderValue to MJML border format
   */
  const formatBorderValue = useCallback((border?: BorderValue): string | undefined => {
    if (!border || border.style === 'none' || border.width === '0px') {
      return undefined
    }

    const width = border.width || '0px'
    const style = border.style || 'none'
    const color = border.color || 'transparent'

    if (style === 'none' || width === '0px') {
      return undefined
    }

    return `${width} ${style} ${color}`
  }, [])

  /**
   * Convert MJML border values to internal format
   */
  const getCurrentBorderValues = useCallback(
    (mjmlValue: MjmlBorderValue): BorderInputValue => {
      return {
        top: parseBorderValue(mjmlValue.borderTop),
        right: parseBorderValue(mjmlValue.borderRight),
        bottom: parseBorderValue(mjmlValue.borderBottom),
        left: parseBorderValue(mjmlValue.borderLeft)
      }
    },
    [parseBorderValue]
  )

  /**
   * Convert internal format back to MJML border values
   */
  const formatToMjmlValues = useCallback(
    (internalValue: BorderInputValue): MjmlBorderValue => {
      return {
        borderTop: formatBorderValue(internalValue.top),
        borderRight: formatBorderValue(internalValue.right),
        borderBottom: formatBorderValue(internalValue.bottom),
        borderLeft: formatBorderValue(internalValue.left)
      }
    },
    [formatBorderValue]
  )

  // Convert MJML values to internal format for processing
  const internalValue = getCurrentBorderValues(value)

  // Check if all borders have the same values
  const bordersAreEqual = useCallback(
    (borders: BorderInputValue): boolean => {
      const { top, right, bottom, left } = borders
      if (!top && !right && !bottom && !left) return true

      const allBorders = [top, right, bottom, left]
      const firstBorder = allBorders.find((b) => b) || {}

      return allBorders.every((border) => {
        if (!border && !firstBorder) return true
        if (!border || !firstBorder) return false
        return (
          (border.color || placeholder.color) === (firstBorder.color || placeholder.color) &&
          (border.width || placeholder.width) === (firstBorder.width || placeholder.width) &&
          (border.style || placeholder.style) === (firstBorder.style || placeholder.style)
        )
      })
    },
    [placeholder]
  )

  // Auto-detect mode based on current values (only if user hasn't manually set mode)
  useEffect(() => {
    /* eslint-disable react-hooks/set-state-in-effect */
    if (!manualModeSet && internalValue && Object.keys(internalValue).length > 0) {
      setMode(bordersAreEqual(internalValue) ? 'all' : 'separate')
    } else if (!manualModeSet) {
      setMode('all')
    }
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [internalValue, bordersAreEqual, manualModeSet])

  // Get unified border value (from first available border)
  const getUnifiedBorder = (): BorderValue => {
    const firstBorder =
      internalValue.top || internalValue.right || internalValue.bottom || internalValue.left
    return firstBorder || {}
  }

  // Update all borders with the same value
  const updateAllBorders = (borderValue: BorderValue) => {
    const newInternalValue = {
      top: borderValue,
      right: borderValue,
      bottom: borderValue,
      left: borderValue
    }
    onChange(formatToMjmlValues(newInternalValue))
  }

  // Update specific border
  const updateBorder = (side: keyof BorderInputValue, borderValue: BorderValue) => {
    const newInternalValue = {
      ...internalValue,
      [side]: borderValue
    }
    onChange(formatToMjmlValues(newInternalValue))
  }

  // Toggle between all/separate mode
  const handleModeToggle = (checked: boolean) => {
    const newMode = checked ? 'separate' : 'all'
    setMode(newMode)
    setManualModeSet(true) // Mark that user manually set the mode

    if (newMode === 'all' && internalValue) {
      // When switching to all mode, use the first available border value
      const unifiedBorder = getUnifiedBorder()
      updateAllBorders(unifiedBorder)
    }
  }

  // Render border controls for a specific side
  const renderBorderControls = (
    borderValue: BorderValue,
    updateFn: (border: BorderValue) => void,
    label?: string
  ) => {
    const currentColor = borderValue.color || placeholder.color
    const currentStyle = borderValue.style || placeholder.style
    const shouldDisableWidth = currentStyle === 'none' || currentColor === 'transparent'
    const shouldDisableStyle = currentColor === 'transparent'

    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: label ? 'space-between' : 'flex-end'
        }}
      >
        {label && <div className="text-xs text-gray-500">{label}</div>}
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <div style={{ width: 24 }}>
            <ColorPickerWithPresets
              value={currentColor}
              onChange={(color) => {
                const newColor = color || placeholder.color
                const newBorder = { ...borderValue, color: newColor }

                // If selecting a visible color and width is 0, set to 1px
                if (
                  newColor &&
                  newColor !== 'transparent' &&
                  (!borderValue.width || borderValue.width === '0px')
                ) {
                  newBorder.width = '1px'
                }

                // If selecting a visible color and style is none, set to solid
                if (
                  newColor &&
                  newColor !== 'transparent' &&
                  (!borderValue.style || borderValue.style === 'none')
                ) {
                  newBorder.style = 'solid'
                }

                // If setting transparent or clearing color, reset width and style
                if (!newColor || newColor === 'transparent') {
                  newBorder.width = '0px'
                  newBorder.style = 'none'
                }

                updateFn(newBorder)
              }}
              size="small"
              placeholder="None"
              showText={false}
            />
          </div>
          <div style={{ width: 80 }}>
            <InputNumber
              size="small"
              value={borderValue.width ? parseInt(borderValue.width) || 0 : 0}
              onChange={(width) => {
                const newWidth = width !== null && width !== undefined ? `${width}px` : '0px'
                const newBorder = { ...borderValue, width: newWidth }

                // If setting width > 0 and style is none, set to solid
                if (width && width > 0 && (!borderValue.style || borderValue.style === 'none')) {
                  newBorder.style = 'solid'
                }

                // If setting width > 0 and color is transparent, set to black
                if (
                  width &&
                  width > 0 &&
                  (!borderValue.color || borderValue.color === 'transparent')
                ) {
                  newBorder.color = '#000000'
                }

                updateFn(newBorder)
              }}
              placeholder="0"
              min={0}
              max={20}
              style={{ width: '100%' }}
              suffix="px"
              disabled={shouldDisableWidth}
            />
          </div>
          <div style={{ width: 120 }}>
            <Select
              size="small"
              value={currentStyle}
              onChange={(style) => {
                const newStyle = style || placeholder.style
                const newBorder = { ...borderValue, style: newStyle }

                // If setting style to none, reset width to 0
                if (newStyle === 'none') {
                  newBorder.width = '0px'
                }

                // If setting style to visible and width is 0, set to 1px
                if (newStyle !== 'none' && (!borderValue.width || borderValue.width === '0px')) {
                  newBorder.width = '1px'
                }

                // If setting style to visible and color is transparent, set to black
                if (
                  newStyle !== 'none' &&
                  (!borderValue.color || borderValue.color === 'transparent')
                ) {
                  newBorder.color = '#000000'
                }

                updateFn(newBorder)
              }}
              style={{ width: '100%' }}
              options={borderStyles}
              disabled={shouldDisableStyle}
            />
          </div>
        </div>
      </div>
    )
  }

  const unifiedBorder = getUnifiedBorder()

  return (
    <div className={className}>
      {/* Mode Toggle */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          gap: 8,
          marginBottom: 22
        }}
      >
        <span style={{ fontSize: '11px', color: '#666' }}>{t`Separate sides`}</span>
        <Switch size="small" checked={mode === 'separate'} onChange={handleModeToggle} />
      </div>

      {mode === 'all' ? (
        // Unified mode - all borders same
        renderBorderControls(unifiedBorder, updateAllBorders, t`All sides`)
      ) : (
        // Separate mode - individual borders
        <div>
          {renderBorderControls(
            internalValue.top || {},
            (border) => updateBorder('top', border),
            t`Top`
          )}
          {renderBorderControls(
            internalValue.right || {},
            (border) => updateBorder('right', border),
            t`Right`
          )}
          {renderBorderControls(
            internalValue.bottom || {},
            (border) => updateBorder('bottom', border),
            t`Bottom`
          )}
          {renderBorderControls(
            internalValue.left || {},
            (border) => updateBorder('left', border),
            t`Left`
          )}
        </div>
      )}
    </div>
  )
}

export default BorderInput
