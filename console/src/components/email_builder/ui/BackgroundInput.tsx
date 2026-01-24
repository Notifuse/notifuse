import React, { useState, useEffect, memo } from 'react'
import { Radio, Select, Row, Col, InputNumber } from 'antd'
import { useLingui } from '@lingui/react/macro'
import ColorPickerWithPresets from './ColorPickerWithPresets'
import InputLayout from './InputLayout'
import FileSrc from './FileSrc'

interface BackgroundInputProps {
  value?: {
    backgroundColor?: string
    backgroundUrl?: string
    backgroundSize?: string
    backgroundRepeat?: string
    backgroundPosition?: string
    backgroundPositionX?: string
    backgroundPositionY?: string
  }
  onChange?: (value: {
    backgroundColor?: string
    backgroundUrl?: string
    backgroundSize?: string
    backgroundRepeat?: string
    backgroundPosition?: string
    backgroundPositionX?: string
    backgroundPositionY?: string
  }) => void
  showBackgroundColor?: boolean
  showBackgroundImage?: boolean
}

const BackgroundInput: React.FC<BackgroundInputProps> = memo(
  ({ value = {}, onChange, showBackgroundColor = true, showBackgroundImage = true }) => {
    const { t } = useLingui()
    // Determine background type based on whether backgroundUrl is set
    const [backgroundType, setBackgroundType] = useState<'color' | 'image'>(
      value.backgroundUrl ? 'image' : 'color'
    )

    useEffect(() => {
      // Update background type when value changes
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setBackgroundType(value.backgroundUrl ? 'image' : 'color')
    }, [value.backgroundUrl])

    // Helper function to parse position values
    const parsePositionValue = (position: string, index: 0 | 1): number => {
      const parts = position.trim().split(/\s+/)
      const value = parts[index] || '50%'
      const numValue = parseInt(value.replace(/[%px]/, ''))
      return isNaN(numValue) ? 50 : numValue
    }

    const handleBackgroundTypeChange = (type: 'color' | 'image') => {
      setBackgroundType(type)

      if (type === 'color') {
        // Clear image-related properties when switching to color
        onChange?.({
          ...value,
          backgroundUrl: undefined,
          backgroundSize: undefined,
          backgroundRepeat: undefined,
          backgroundPosition: undefined,
          backgroundPositionX: undefined,
          backgroundPositionY: undefined
        })
      } else {
        // Clear color when switching to image
        onChange?.({
          ...value,
          backgroundColor: undefined
        })
      }
    }

    const handlePropertyChange = (property: string, newValue: string | undefined) => {
      onChange?.({
        ...value,
        [property]: newValue
      })
    }

    // Background size options
    const backgroundSizeOptions = [
      { value: 'auto', label: t`Auto` },
      { value: 'cover', label: t`Cover` },
      { value: 'contain', label: t`Contain` },
      { value: 'custom-px', label: t`Custom (px)` },
      { value: 'custom-percent', label: t`Custom (%)` }
    ]

    // Background repeat options
    const backgroundRepeatOptions = [
      { value: 'repeat', label: t`Repeat` },
      { value: 'repeat-x', label: t`Repeat X` },
      { value: 'repeat-y', label: t`Repeat Y` },
      { value: 'no-repeat', label: t`No Repeat` },
      { value: 'space', label: t`Space` },
      { value: 'round', label: t`Round` }
    ]

    // Background position options
    const backgroundPositionOptions = [
      { value: 'top left', label: t`Top Left` },
      { value: 'top center', label: t`Top Center` },
      { value: 'top right', label: t`Top Right` },
      { value: 'center left', label: t`Center Left` },
      { value: 'center center', label: t`Center Center` },
      { value: 'center right', label: t`Center Right` },
      { value: 'bottom left', label: t`Bottom Left` },
      { value: 'bottom center', label: t`Bottom Center` },
      { value: 'bottom right', label: t`Bottom Right` },
      { value: 'custom', label: t`Custom` }
    ]

    const renderBackgroundSizeInput = () => {
      const currentSize = value.backgroundSize || 'auto'
      const isCustomPx =
        currentSize.includes('px') && !['cover', 'contain', 'auto'].includes(currentSize)
      const isCustomPercent =
        currentSize.includes('%') && !['cover', 'contain', 'auto'].includes(currentSize)

      let selectedValue = currentSize
      if (isCustomPx) selectedValue = 'custom-px'
      if (isCustomPercent) selectedValue = 'custom-percent'

      return (
        <div>
          <Select
            size="small"
            value={selectedValue}
            onChange={(val) => {
              if (['auto', 'cover', 'contain'].includes(val)) {
                handlePropertyChange('backgroundSize', val)
              } else {
                // For custom values, keep current value or set default
                if (val === 'custom-px' && !isCustomPx) {
                  handlePropertyChange('backgroundSize', '100px')
                } else if (val === 'custom-percent' && !isCustomPercent) {
                  handlePropertyChange('backgroundSize', '100%')
                }
              }
            }}
            options={backgroundSizeOptions}
            style={{ width: '100%', marginBottom: 8 }}
          />

          {(selectedValue === 'custom-px' || selectedValue === 'custom-percent') && (
            <InputNumber
              size="small"
              value={
                selectedValue === 'custom-px'
                  ? parseInt(currentSize.replace('px', '')) || 100
                  : parseInt(currentSize.replace('%', '')) || 100
              }
              onChange={(newValue) => {
                const suffix = selectedValue === 'custom-px' ? 'px' : '%'
                handlePropertyChange(
                  'backgroundSize',
                  newValue ? `${newValue}${suffix}` : undefined
                )
              }}
              placeholder={selectedValue === 'custom-px' ? '100' : '100'}
              suffix={selectedValue === 'custom-px' ? 'px' : '%'}
              style={{ width: '100%', marginTop: '4px' }}
              min={1}
              max={selectedValue === 'custom-px' ? 2000 : 500}
            />
          )}
        </div>
      )
    }

    const renderBackgroundPositionInputs = () => {
      const currentPosition = value.backgroundPosition || 'top center'
      const isCustom = !backgroundPositionOptions.some(
        (opt) => opt.value === currentPosition && opt.value !== 'custom'
      )

      return (
        <div>
          <Select
            size="small"
            value={isCustom ? 'custom' : currentPosition}
            onChange={(val) => {
              if (val !== 'custom') {
                handlePropertyChange('backgroundPosition', val)
              } else {
                handlePropertyChange('backgroundPosition', '50% 50%')
              }
            }}
            options={backgroundPositionOptions}
            style={{ width: '100%', marginBottom: 8 }}
          />

          {(isCustom || currentPosition === 'custom') && (
            <Row gutter={8}>
              <Col span={12}>
                <InputNumber
                  size="small"
                  value={parsePositionValue(
                    currentPosition === 'custom' ? '50% 50%' : currentPosition,
                    0
                  )}
                  onChange={(newValue) => {
                    const yValue = parsePositionValue(
                      currentPosition === 'custom' ? '50% 50%' : currentPosition,
                      1
                    )
                    const newPosition = `${newValue || 50}% ${yValue || 50}%`
                    handlePropertyChange('backgroundPosition', newPosition)
                  }}
                  placeholder="50"
                  suffix="%"
                  style={{ width: '100%' }}
                />
              </Col>
              <Col span={12}>
                <InputNumber
                  size="small"
                  value={parsePositionValue(
                    currentPosition === 'custom' ? '50% 50%' : currentPosition,
                    1
                  )}
                  onChange={(newValue) => {
                    const xValue = parsePositionValue(
                      currentPosition === 'custom' ? '50% 50%' : currentPosition,
                      0
                    )
                    const newPosition = `${xValue || 50}% ${newValue || 50}%`
                    handlePropertyChange('backgroundPosition', newPosition)
                  }}
                  placeholder="50"
                  suffix="%"
                  style={{ width: '100%' }}
                />
              </Col>
            </Row>
          )}
        </div>
      )
    }

    // Only show type selector if both options are available
    const showTypeSelector = showBackgroundColor && showBackgroundImage

    return (
      <div>
        {showTypeSelector && (
          <InputLayout label={t`Background Type`}>
            <Radio.Group
              size="small"
              value={backgroundType}
              onChange={(e) => handleBackgroundTypeChange(e.target.value)}
              style={{ width: '100%' }}
            >
              <Radio.Button value="color">{t`Color`}</Radio.Button>
              <Radio.Button value="image">{t`Image`}</Radio.Button>
            </Radio.Group>
          </InputLayout>
        )}

        {backgroundType === 'color' && showBackgroundColor && (
          <InputLayout label={t`Background Color`}>
            <ColorPickerWithPresets
              value={value.backgroundColor}
              onChange={(color) => handlePropertyChange('backgroundColor', color)}
            />
          </InputLayout>
        )}

        {backgroundType === 'image' && showBackgroundImage && (
          <>
            <InputLayout label={t`Background Image`} layout="vertical">
              <FileSrc
                size="small"
                value={value.backgroundUrl || ''}
                onChange={(newValue) => handlePropertyChange('backgroundUrl', newValue)}
                placeholder={t`Enter background image URL`}
                acceptFileType="image/*"
                acceptItem={(item) =>
                  !item.is_folder && (item.file_info?.content_type?.startsWith('image/') ?? false)
                }
                buttonText={t`Browse Images`}
              />
            </InputLayout>

            <InputLayout label={t`Background Size`}>{renderBackgroundSizeInput()}</InputLayout>

            <InputLayout label={t`Background Repeat`}>
              <Select
                size="small"
                value={value.backgroundRepeat || 'no-repeat'}
                onChange={(val) => handlePropertyChange('backgroundRepeat', val)}
                options={backgroundRepeatOptions}
                style={{ width: '100%' }}
              />
            </InputLayout>

            <InputLayout label={t`Background Position`}>
              {renderBackgroundPositionInputs()}
            </InputLayout>
          </>
        )}
      </div>
    )
  }
)

export default BackgroundInput
