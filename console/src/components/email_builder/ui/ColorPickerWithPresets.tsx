import React, { useCallback } from 'react'
import { ColorPicker, Row, Col, Divider } from 'antd'
import type { ColorPickerProps } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface ColorPickerWithPresetsProps {
  value?: string
  onChange: (value: string | undefined) => void
  size?: 'small' | 'middle' | 'large'
  allowClear?: boolean
  disabled?: boolean
  placeholder?: string
  showText?: boolean
}

/**
 * ColorPicker component with Flowbite color presets
 * Uses onChangeComplete to only trigger updates when user finishes selecting a color
 * Features a compact horizontal layout with presets on the left and picker on the right
 */
const ColorPickerWithPresets: React.FC<ColorPickerWithPresetsProps> = ({
  value,
  onChange,
  size = 'small',
  allowClear = true,
  disabled = false,
  placeholder,
  showText = true
}) => {
  const { t } = useLingui()
  const resolvedPlaceholder = placeholder || t`None`
  const handleChangeComplete = useCallback(
    (color: unknown) => {
      const hexValue = (color as { toHexString?: () => string })?.toHexString?.()
      onChange(hexValue || undefined)
    },
    [onChange]
  )

  const handleClear = useCallback(() => {
    onChange('transparent')
  }, [onChange])

  // Treat "transparent" as empty value
  const normalizedValue = value === 'transparent' ? undefined : value

  const customPanelRender: ColorPickerProps['panelRender'] = (
    _,
    { components: { Picker, Presets } }
  ) => (
    <Row justify="space-between" wrap={false}>
      <Col span={8}>
        <div
          className="compact-color-presets"
          style={{
            maxHeight: '280px',
            overflow: 'auto'
          }}
        >
          <Presets />
        </div>
      </Col>
      <Divider type="vertical" style={{ height: 'auto' }} />
      <Col flex="auto">
        <Picker />
      </Col>
    </Row>
  )

  return (
    <div>
      <style>{`
        .compact-color-presets .ant-color-picker-presets-color {
          width: 14px !important;
          height: 14px !important;
          border-radius: 2px !important;
          margin: 1px !important;
        }
        .compact-color-presets .ant-color-picker-presets-label {
          display: none !important;
        }
        .compact-color-presets .ant-color-picker-presets {
          margin-bottom: 4px !important;
        }
      `}</style>
      <ColorPicker
        size={size}
        value={normalizedValue ? normalizedValue : undefined}
        onChangeComplete={handleChangeComplete}
        onClear={handleClear}
        allowClear={allowClear}
        disabled={disabled}
        styles={{
          popupOverlayInner: { width: 380 }
        }}
        panelRender={customPanelRender}
        showText={
          showText
            ? (color) => (
                <span style={{ fontSize: '12px' }}>
                  {color && value !== 'transparent' && value !== undefined
                    ? color.toHexString()
                    : resolvedPlaceholder}
                </span>
              )
            : false
        }
        presets={[
          {
            label: '',
            colors: [
              // Basic + Gray
              '#000000',
              '#ffffff',
              '#1f2937',
              '#f9fafb',
              '#f3f4f6',
              '#e5e7eb',
              '#d1d5db',
              '#9ca3af',
              '#6b7280',
              '#4b5563',
              '#374151',
              '#111827',
              // Red
              '#fdf2f2',
              '#fde8e8',
              '#fbd5d5',
              '#f8b4b4',
              '#f98080',
              '#f05252',
              '#e02424',
              '#c81e1e',
              '#9b1c1c',
              '#771d1d',
              // Yellow
              '#fdfdea',
              '#fdf6b2',
              '#fce96a',
              '#faca15',
              '#e3a008',
              '#c27803',
              '#9f580a',
              '#8e4b10',
              '#723b13',
              '#633112',
              // Green
              '#f3faf7',
              '#def7ec',
              '#bcf0da',
              '#84e1bc',
              '#31c48d',
              '#0e9f6e',
              '#057a55',
              '#046c4e',
              '#03543f',
              '#014737',
              // Blue
              '#ebf5ff',
              '#e1effe',
              '#c3ddfd',
              '#a4cafe',
              '#76a9fa',
              '#3f83f8',
              '#1c64f2',
              '#1a56db',
              '#1e429f',
              '#233876',
              // Indigo
              '#f0f5ff',
              '#e5edff',
              '#cddbfe',
              '#b4c6fc',
              '#8da2fb',
              '#6875f5',
              '#5850ec',
              '#5145cd',
              '#42389d',
              '#362f78',
              // Purple
              '#f6f5ff',
              '#edebfe',
              '#dcd7fe',
              '#cabffd',
              '#ac94fa',
              '#9061f9',
              '#7e3af2',
              '#6c2bd9',
              '#5521b5',
              '#4a1d96',
              // Pink
              '#fdf2f8',
              '#fce8f3',
              '#fad1e8',
              '#f8b4d9',
              '#f17eb8',
              '#e74694',
              '#d61f69',
              '#bf125d',
              '#99154b',
              '#751a3d'
            ]
          }
        ]}
      />
    </div>
  )
}

export default ColorPickerWithPresets
