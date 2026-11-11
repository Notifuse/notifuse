import React, { useState, useEffect, memo } from 'react'
import { Select, Row, Col, InputNumber } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faFont, faDesktop } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import LetterSpacingInput from './LetterSpacingInput'
import LineHeightInput from './LineHeightInput'

interface FontStyleValues {
  fontFamily: string | undefined
  fontSize: string | undefined
  fontWeight: string | undefined
  fontStyle: string | undefined
  textTransform: string | undefined
  textDecoration: string | undefined
  lineHeight: string | undefined
  letterSpacing: string | undefined
  textAlign: string | undefined
}

interface FontStyleInputProps {
  value?: FontStyleValues
  defaultValue: FontStyleValues
  onChange?: (value: FontStyleValues) => void
  importedFonts?: Array<{ name: string; href: string }>
}

const FontStyleInput: React.FC<FontStyleInputProps> = memo(
  ({ value, defaultValue, onChange, importedFonts = [] }) => {
    const { t } = useLingui()
    const [localValue, setLocalValue] = useState<FontStyleValues>(value || defaultValue)

    useEffect(() => {
      if (value) {
        // eslint-disable-next-line react-hooks/set-state-in-effect
        setLocalValue(value)
      }
    }, [value])

    const handleValueChange = (
      property: keyof FontStyleValues,
      newValue: string | number | undefined
    ) => {
      let formattedValue: string | undefined

      if (property === 'fontSize') {
        // Handle fontSize as number and format with px
        formattedValue = newValue ? `${newValue}px` : undefined
      } else {
        // Handle other properties as strings
        formattedValue = newValue as string | undefined
      }

      const updatedValue = { ...localValue, [property]: formattedValue }
      setLocalValue(updatedValue)
      onChange?.(updatedValue)
    }

    // Parse fontSize to get numeric value
    const getFontSizeNumber = (fontSize?: string): number | undefined => {
      if (!fontSize) return undefined
      const match = fontSize.match(/^(\d+(?:\.\d+)?)px?$/)
      return match ? parseFloat(match[1]) : undefined
    }

    // Build font family options with imported fonts grouped separately
    const systemFonts = [
      { value: 'Arial, sans-serif', label: 'Arial, sans-serif' },
      { value: 'Helvetica, sans-serif', label: 'Helvetica, sans-serif' },
      {
        value: 'Ubuntu, Helvetica, Arial, sans-serif',
        label: 'Ubuntu, Helvetica, Arial, sans-serif'
      },
      { value: 'Georgia, serif', label: 'Georgia, serif' },
      { value: 'Times New Roman, serif', label: 'Times New Roman, serif' },
      { value: 'Courier New, monospace', label: 'Courier New, monospace' }
    ]

    const fontFamilyOptions = []

    // Add imported fonts group if there are any
    if (importedFonts.length > 0) {
      fontFamilyOptions.push({
        label: (
          <span>
            <FontAwesomeIcon icon={faFont} className="mr-2" />
            {t`Imported Fonts`}
          </span>
        ),
        options: importedFonts.map((font) => ({
          value: font.name,
          label: font.name
        }))
      })
    }

    // Add system fonts group
    fontFamilyOptions.push({
      label: (
        <span>
          <FontAwesomeIcon icon={faDesktop} className="mr-2" />
          {t`System Fonts`}
        </span>
      ),
      options: systemFonts
    })

    const fontWeightOptions = [
      { value: 'normal', label: <span className="font-normal">{t`Normal`}</span> },
      { value: 'bold', label: <span className="font-bold">{t`Bold`}</span> },
      { value: '100', label: <span className="font-thin">{t`100 - Thin`}</span> },
      { value: '200', label: <span className="font-extralight">{t`200 - Extra Light`}</span> },
      { value: '300', label: <span className="font-light">{t`300 - Light`}</span> },
      { value: '400', label: <span className="font-normal">{t`400 - Normal`}</span> },
      { value: '500', label: <span className="font-medium">{t`500 - Medium`}</span> },
      { value: '600', label: <span className="font-semibold">{t`600 - Semi Bold`}</span> },
      { value: '700', label: <span className="font-bold">{t`700 - Bold`}</span> },
      { value: '800', label: <span className="font-extrabold">{t`800 - Extra Bold`}</span> },
      { value: '900', label: <span className="font-black">{t`900 - Black`}</span> }
    ]

    const fontStyleOptions = [
      { value: 'normal', label: <span className="not-italic">{t`Normal`}</span> },
      { value: 'italic', label: <span className="italic">{t`Italic`}</span> },
      { value: 'oblique', label: <span className="italic">{t`Oblique`}</span> }
    ]

    const textTransformOptions = [
      { value: 'none', label: t`None` },
      { value: 'uppercase', label: t`UPPERCASE` },
      { value: 'lowercase', label: t`lowercase` },
      { value: 'capitalize', label: t`Capitalize` }
    ]

    const textDecorationOptions = [
      { value: 'none', label: <span className="no-underline">{t`None`}</span> },
      { value: 'underline', label: <span className="underline">{t`Underline`}</span> },
      { value: 'line-through', label: <span className="line-through">{t`Line Through`}</span> },
      { value: 'overline', label: <span style={{ textDecoration: 'overline' }}>{t`Overline`}</span> }
    ]

    const textAlignOptions = [
      {
        value: 'left',
        label: t`Left`
      },
      {
        value: 'center',
        label: t`Center`
      },
      {
        value: 'right',
        label: t`Right`
      },
      {
        value: 'justify',
        label: t`Justify`
      }
    ]

    return (
      <div>
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Font Family`}</span>
              <Select
                size="small"
                value={localValue.fontFamily || defaultValue.fontFamily}
                onChange={(value) => handleValueChange('fontFamily', value)}
                options={fontFamilyOptions}
                style={{ width: '100%', marginTop: '4px' }}
                popupMatchSelectWidth={false}
              />
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Size`}</span>
              <InputNumber
                size="small"
                value={getFontSizeNumber(localValue.fontSize)}
                onChange={(value) => handleValueChange('fontSize', value || undefined)}
                placeholder={getFontSizeNumber(defaultValue.fontSize)?.toString() || '13'}
                min={1}
                max={200}
                step={1}
                suffix="px"
                style={{ width: '100%', marginTop: '4px' }}
              />
            </div>
          </Col>
        </Row>

        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Weight`}</span>
              <Select
                size="small"
                value={localValue.fontWeight || defaultValue.fontWeight}
                onChange={(value) => handleValueChange('fontWeight', value)}
                options={fontWeightOptions}
                style={{ width: '100%', marginTop: '4px' }}
              />
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Style`}</span>
              <Select
                size="small"
                value={localValue.fontStyle || defaultValue.fontStyle || 'normal'}
                onChange={(value) => handleValueChange('fontStyle', value)}
                options={fontStyleOptions}
                style={{ width: '100%', marginTop: '4px' }}
              />
            </div>
          </Col>
        </Row>

        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Text Align`}</span>
              <Select
                size="small"
                value={localValue.textAlign || defaultValue.textAlign}
                onChange={(value) => handleValueChange('textAlign', value)}
                options={textAlignOptions}
                style={{ width: '100%', marginTop: '4px' }}
                placeholder={t`Default`}
              />
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Text Transform`}</span>
              <Select
                size="small"
                value={localValue.textTransform || 'none'}
                onChange={(value) => handleValueChange('textTransform', value)}
                options={textTransformOptions}
                style={{ width: '100%', marginTop: '4px' }}
              />
            </div>
          </Col>
        </Row>

        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Line Height`}</span>
              <div style={{ marginTop: '4px' }}>
                <LineHeightInput
                  value={localValue.lineHeight || ''}
                  onChange={(value) => handleValueChange('lineHeight', value)}
                  placeholder={defaultValue.lineHeight || '120%'}
                />
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Letter Spacing`}</span>
              <div style={{ marginTop: '4px' }}>
                <LetterSpacingInput
                  value={localValue.letterSpacing || ''}
                  onChange={(value) => handleValueChange('letterSpacing', value)}
                  placeholder={defaultValue.letterSpacing || 'none'}
                />
              </div>
            </div>
          </Col>
        </Row>

        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Text Decoration`}</span>
              <Select
                size="small"
                value={localValue.textDecoration || defaultValue.textDecoration || 'none'}
                onChange={(value) => handleValueChange('textDecoration', value)}
                options={textDecorationOptions}
                style={{ width: '100%', marginTop: '4px' }}
              />
            </div>
          </Col>
          <Col span={12}>{/* Empty column for layout balance */}</Col>
        </Row>
      </div>
    )
  }
)

export default FontStyleInput
