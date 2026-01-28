import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faImage } from '@fortawesome/free-solid-svg-icons'
import { Switch } from 'antd'
import type { MJMLComponentType, MJImageAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import StringPopoverInput from '../ui/StringPopoverInput'
import AlignSelector from '../ui/AlignSelector'
import HeightInput from '../ui/HeightInput'
import PaddingInput from '../ui/PaddingInput'
import InputLayout from '../ui/InputLayout'
import ColorPickerWithPresets from '../ui/ColorPickerWithPresets'
import BorderInput from '../ui/BorderInput'
import BorderRadiusInput from '../ui/BorderRadiusInput'
import FileSrc from '../ui/FileSrc'
import PanelLayout from '../panels/PanelLayout'
import WidthPxInput from '../ui/WidthPxInput'

// Functional component for settings panel with i18n support
interface MjImageSettingsPanelProps {
  currentAttributes: MJImageAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
}

const MjImageSettingsPanel: React.FC<MjImageSettingsPanelProps> = ({
  currentAttributes,
  blockDefaults,
  onUpdate
}) => {
  const { t } = useLingui()

  return (
    <PanelLayout title={t`Image Attributes`}>
      <InputLayout label={t`Image`} className="mt-0" layout="vertical">
        <FileSrc
          size="small"
          value={currentAttributes.src || ''}
          onChange={(value) => onUpdate({ src: value || undefined })}
          placeholder={t`Enter image URL`}
          acceptFileType="image/*"
          acceptItem={(item) =>
            !item.is_folder && (item.file_info?.content_type?.startsWith('image/') ?? false)
          }
          buttonText={t`Browse Images`}
        />
      </InputLayout>

      <InputLayout label={t`Link`}>
        <StringPopoverInput
          value={currentAttributes.href || ''}
          onChange={(value) => onUpdate({ href: value || undefined })}
          placeholder={t`Enter link URL or {{ url }}`}
          buttonText={t`Set link`}
          validateUri={true}
        />
      </InputLayout>

      <InputLayout label={t`Alt text`}>
        <StringPopoverInput
          value={currentAttributes.alt || ''}
          onChange={(value) => onUpdate({ alt: value || undefined })}
          placeholder={t`Alternative text`}
        />
      </InputLayout>

      <InputLayout label={t`Align`}>
        <AlignSelector
          value={currentAttributes.align || 'center'}
          onChange={(value) => onUpdate({ align: value })}
        />
      </InputLayout>

      <InputLayout label={t`Width`} help={t`Leave empty for full container width`}>
        <WidthPxInput
          value={currentAttributes.width}
          onChange={(value) => onUpdate({ width: value })}
          placeholder={t`Auto`}
        />
      </InputLayout>

      <InputLayout
        label={t`Full width on mobile`}
        help={t`If "true", will be full width on mobile even if width is set`}
      >
        <Switch
          size="small"
          checked={currentAttributes['fluidOnMobile'] === 'true'}
          onChange={(checked) => onUpdate({ fluidOnMobile: checked ? 'true' : 'false' })}
        />
      </InputLayout>

      <InputLayout label={t`Height`}>
        <HeightInput
          value={currentAttributes.height}
          onChange={(value) => onUpdate({ height: value })}
          placeholder={blockDefaults.height || t`Auto`}
        />
      </InputLayout>

      <InputLayout label={t`Padding`} layout="vertical">
        <PaddingInput
          value={{
            top: currentAttributes.paddingTop,
            right: currentAttributes.paddingRight,
            bottom: currentAttributes.paddingBottom,
            left: currentAttributes.paddingLeft
          }}
          defaultValue={{
            top: blockDefaults.paddingTop,
            right: blockDefaults.paddingRight,
            bottom: blockDefaults.paddingBottom,
            left: blockDefaults.paddingLeft
          }}
          onChange={(values: {
            top: string | undefined
            right: string | undefined
            bottom: string | undefined
            left: string | undefined
          }) => {
            onUpdate({
              paddingTop: values.top,
              paddingRight: values.right,
              paddingBottom: values.bottom,
              paddingLeft: values.left
            })
          }}
        />
      </InputLayout>

      <InputLayout label={t`Container Background`}>
        <ColorPickerWithPresets
          value={currentAttributes.containerBackgroundColor || undefined}
          onChange={(color) => {
            onUpdate({ containerBackgroundColor: color || undefined })
          }}
        />
      </InputLayout>

      <InputLayout label={t`Border Radius`}>
        <BorderRadiusInput
          value={currentAttributes.borderRadius}
          onChange={(value) => onUpdate({ borderRadius: value })}
          defaultValue={blockDefaults.borderRadius}
        />
      </InputLayout>

      <InputLayout label={t`Border`} layout="vertical">
        <BorderInput
          className="-mt-6"
          value={{
            borderTop: currentAttributes.borderTop,
            borderRight: currentAttributes.borderRight,
            borderBottom: currentAttributes.borderBottom,
            borderLeft: currentAttributes.borderLeft
          }}
          onChange={(borderValues) => {
            onUpdate({
              borderTop: borderValues.borderTop,
              borderRight: borderValues.borderRight,
              borderBottom: borderValues.borderBottom,
              borderLeft: borderValues.borderLeft
            })
          }}
        />
      </InputLayout>

      <InputLayout label={t`CSS Class`}>
        <StringPopoverInput
          value={currentAttributes.cssClass || ''}
          onChange={(value) => onUpdate({ cssClass: value || undefined })}
          placeholder={t`Enter CSS class name`}
        />
      </InputLayout>
    </PanelLayout>
  )
}

/**
 * Implementation for mj-image blocks
 */
export class MjImageBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faImage} className="opacity-70" />
  }

  getLabel(): string {
    return 'Image'
  }

  getDescription(): React.ReactNode {
    return (
      <div>
        <div style={{ marginBottom: 8 }}>Display images, logos, and visual content</div>
        <div style={{ opacity: 0.7 }}>
          <div
            style={{
              width: 60,
              height: 30,
              border: '2px solid #13c2c2',
              borderRadius: 4,
              backgroundColor: '#e6fffb',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              position: 'relative'
            }}
          >
            <div
              style={{
                width: 40,
                height: 20,
                backgroundColor: '#87e8de',
                borderRadius: 2,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center'
              }}
            >
              <FontAwesomeIcon icon={faImage} style={{ fontSize: 10, color: '#13c2c2' }} />
            </div>
          </div>
        </div>
      </div>
    )
  }

  getCategory(): 'content' | 'layout' {
    return 'content'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-image'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const {
      selectedBlockId,
      onSelectBlock,
      attributeDefaults
    } = props

    const key = this.block.id
    const isSelected = selectedBlockId === this.block.id
    const blockClasses = `email-block-hover ${isSelected ? 'selected' : ''}`.trim()

    const selectionStyle: React.CSSProperties = isSelected
      ? { position: 'relative', zIndex: 10 }
      : {}

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onSelectBlock) {
        onSelectBlock(this.block.id)
      }
    }

    const attrs = EmailBlockClass.mergeWithAllDefaults(
      'mj-image',
      this.block.attributes as Record<string, unknown> | undefined,
      attributeDefaults
    )

    // MJML wraps images in a table structure - outer container gets padding and alignment
    const containerStyle: React.CSSProperties = {
      fontSize: '0px',
      padding: `${attrs.paddingTop || '10px'} ${attrs.paddingRight || '25px'} ${
        attrs.paddingBottom || '10px'
      } ${attrs.paddingLeft || '25px'}`,
      wordBreak: 'break-word',
      backgroundColor: attrs.containerBackgroundColor,
      textAlign: attrs.align as React.CSSProperties['textAlign'], // This handles the image alignment
      ...selectionStyle
    }

    // MJML table style for image presentation
    // Per MJML spec: "if no width is provided, the image will use the parent column width"
    // When explicit width is set, use inline-block so alignment (text-align) works
    // When no width, use block with 100% width to fill the column
    const hasExplicitWidth = attrs.width && attrs.width !== '100%'
    const tableStyle: React.CSSProperties = {
      borderCollapse: 'collapse',
      borderSpacing: '0px',
      display: hasExplicitWidth ? 'inline-block' : 'block',
      width: hasExplicitWidth ? undefined : '100%'
    }

    // MJML inner cell style - contains width constraint
    const cellStyle: React.CSSProperties = {
      width: hasExplicitWidth ? attrs.width : '100%'
    }

    // MJML image style - matches actual generated styles
    const imageStyle: React.CSSProperties = {
      border: attrs.border || 'none',
      borderTop: attrs.borderTop,
      borderRight: attrs.borderRight,
      borderBottom: attrs.borderBottom,
      borderLeft: attrs.borderLeft,
      borderRadius: attrs.borderRadius,
      display: 'block',
      outline: 'none',
      textDecoration: 'none',
      height: attrs.height || 'auto',
      width: '100%', // MJML always sets width to 100% on the img element
      fontSize: '13px' // MJML sets this font-size on images
    }

    const imageElement = (
      <img
        src={attrs.src || 'https://via.placeholder.com/300x200'}
        alt={attrs.alt || ''}
        title={attrs.title}
        useMap={attrs.usemap}
        sizes={attrs.sizes}
        srcSet={attrs.srcset}
        style={imageStyle}
        width={attrs.width ? parseInt(attrs.width) : undefined}
        height={attrs.height && attrs.height !== 'auto' ? parseInt(attrs.height) : undefined}
      />
    )

    const contentElement = attrs.href ? (
      <a
        href={attrs.href}
        target={attrs.target || '_blank'}
        rel={attrs.rel}
        style={{
          textDecoration: 'none',
          display: 'block',
          borderRadius: attrs.borderRadius,
          overflow: 'hidden'
        }}
        onClick={(e) => e.preventDefault()}
      >
        {imageElement}
      </a>
    ) : (
      imageElement
    )

    // Simulate MJML's exact structure: table > tbody > tr > td > img
    return (
      <div
        key={key}
        className={`${attrs.cssClass || ''} ${blockClasses}`.trim()}
        onClick={handleClick}
        style={{ ...containerStyle, position: 'relative' }}
        data-block-id={this.block.id}
      >
        <table border={0} cellPadding="0" cellSpacing="0" role="presentation" style={tableStyle}>
          <tbody>
            <tr>
              <td style={cellStyle}>{contentElement}</td>
            </tr>
          </tbody>
        </table>
      </div>
    )
  }

  /**
   * Render the settings panel for the image block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJImageAttributes
    return (
      <MjImageSettingsPanel
        currentAttributes={currentAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
      />
    )
  }
}
