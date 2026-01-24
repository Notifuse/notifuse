import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Switch } from 'antd'
import type { MJMLComponentType, MJWrapperAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import InputLayout from '../ui/InputLayout'
import BackgroundInput from '../ui/BackgroundInput'
import BorderInput from '../ui/BorderInput'
import BorderRadiusInput from '../ui/BorderRadiusInput'
import PaddingInput from '../ui/PaddingInput'
import AlignSelector from '../ui/AlignSelector'
import StringPopoverInput from '../ui/StringPopoverInput'
import PanelLayout from '../panels/PanelLayout'

// Functional component for settings panel with i18n support
interface MjWrapperSettingsPanelProps {
  currentAttributes: MJWrapperAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
}

const MjWrapperSettingsPanel: React.FC<MjWrapperSettingsPanelProps> = ({
  currentAttributes,
  blockDefaults,
  onUpdate
}) => {
  const { t } = useLingui()

  const handleAttributeChange = (key: string, value: unknown) => {
    onUpdate({ [key]: value })
  }

  const handleBackgroundChange = (backgroundValues: Record<string, unknown>) => {
    onUpdate(backgroundValues)
  }

  return (
    <PanelLayout title={t`Wrapper Attributes`}>
      <div className="space-y-4">
        {/* Background Settings */}
        <BackgroundInput
          value={{
            backgroundColor: currentAttributes.backgroundColor,
            backgroundUrl: currentAttributes.backgroundUrl,
            backgroundSize: currentAttributes.backgroundSize,
            backgroundRepeat: currentAttributes.backgroundRepeat,
            backgroundPosition: currentAttributes.backgroundPosition
          }}
          onChange={handleBackgroundChange}
        />

        {/* Border Settings */}
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

        {/* Border Radius */}
        <InputLayout label={t`Border radius`}>
          <BorderRadiusInput
            value={currentAttributes.borderRadius}
            onChange={(value) => onUpdate({ borderRadius: value })}
            defaultValue={blockDefaults.borderRadius}
          />
        </InputLayout>

        {/* Full Width */}

        <InputLayout
          label={t`Full Width`}
          help={t`Makes the wrapper span the entire email viewport width, ignoring container constraints (typically 600px). Useful for full-bleed backgrounds and hero sections.`}
        >
          <Switch
            size="small"
            checked={currentAttributes.fullWidth === 'full-width'}
            onChange={(checked) =>
              handleAttributeChange('fullWidth', checked ? 'full-width' : '')
            }
          />
        </InputLayout>

        {/* Padding Settings */}
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

        {/* Text Alignment */}
        <InputLayout label={t`Text Alignment`}>
          <AlignSelector
            value={currentAttributes.textAlign || blockDefaults.textAlign || 'left'}
            onChange={(value) => handleAttributeChange('textAlign', value)}
          />
        </InputLayout>

        {/* CSS Class */}
        <InputLayout label={t`CSS Class`} help={t`Custom CSS class for styling`}>
          <StringPopoverInput
            value={currentAttributes.cssClass || ''}
            onChange={(value) => handleAttributeChange('cssClass', value)}
            placeholder={t`my-custom-class`}
            buttonText={t`Set Value`}
          />
        </InputLayout>
      </div>
    </PanelLayout>
  )
}

// Functional component for empty wrapper placeholder with i18n support
const MjWrapperEmptyPlaceholder: React.FC = () => {
  const { t } = useLingui()

  return (
    <div
      style={{
        padding: '20px',
        backgroundColor: '#f8f9fa',
        border: '2px dashed #dee2e6',
        borderRadius: '4px',
        color: '#6c757d',
        fontSize: '14px',
        textAlign: 'center',
        margin: '10px'
      }}
    >
      {t`This wrapper has no sections. Add a section to display content.`}
    </div>
  )
}

/**
 * Implementation for mj-wrapper blocks
 * A wrapper component that can contain sections and provides styling context
 */
export class MjWrapperBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return (
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="svg-inline--fa"
      >
        <path d="M5 3a2 2 0 0 0-2 2" />
        <path d="M19 3a2 2 0 0 1 2 2" />
        <path d="M21 19a2 2 0 0 1-2 2" />
        <path d="M5 21a2 2 0 0 1-2-2" />
        <path d="M9 3h1" />
        <path d="M9 21h1" />
        <path d="M14 3h1" />
        <path d="M14 21h1" />
        <path d="M3 9v1" />
        <path d="M21 9v1" />
        <path d="M3 14v1" />
        <path d="M21 14v1" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Wrapper'
  }

  getDescription(): React.ReactNode {
    return 'Container that wraps sections and provides styling context'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-wrapper'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-section', 'mj-raw']
  }

  /**
   * Render the settings panel for the wrapper block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJWrapperAttributes

    return (
      <MjWrapperSettingsPanel
        currentAttributes={currentAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
      />
    )
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const {
      selectedBlockId,
      onSelectBlock,
      attributeDefaults,
      emailTree,
      onUpdateBlock,
      onCloneBlock,
      onDeleteBlock,
      onSaveBlock: onSave,
      savedBlocks
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
      'mj-wrapper',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    const wrapperStyle: React.CSSProperties = {
      width: '100%',
      backgroundColor: attrs.fullWidthBackgroundColor,
      backgroundImage: attrs.backgroundUrl ? `url(${attrs.backgroundUrl})` : undefined,
      backgroundRepeat: attrs.backgroundRepeat,
      backgroundSize: attrs.backgroundSize,
      backgroundPosition: attrs.backgroundPosition,
      ...selectionStyle
    }

    const innerWrapperStyle: React.CSSProperties = {
      margin: '0 auto',
      backgroundColor: attrs.backgroundColor,
      paddingTop: attrs.paddingTop,
      paddingRight: attrs.paddingRight,
      paddingBottom: attrs.paddingBottom,
      paddingLeft: attrs.paddingLeft,
      textAlign: attrs.textAlign as React.CSSProperties['textAlign'],
      border: attrs.border !== 'none' ? attrs.border : undefined,
      borderTop: attrs.borderTop !== 'none' ? attrs.borderTop : undefined,
      borderRight: attrs.borderRight !== 'none' ? attrs.borderRight : undefined,
      borderBottom: attrs.borderBottom !== 'none' ? attrs.borderBottom : undefined,
      borderLeft: attrs.borderLeft !== 'none' ? attrs.borderLeft : undefined,
      borderRadius: attrs.borderRadius !== '0px' ? attrs.borderRadius : undefined
    }

    // Check if wrapper has no sections
    const hasSections =
      this.block.children && this.block.children.some((child) => child.type === 'mj-section')

    return (
      <div
        key={key}
        style={{ ...wrapperStyle, position: 'relative' }}
        className={`${attrs.cssClass} ${blockClasses}`.trim()}
        onClick={handleClick}
        data-block-id={this.block.id}
      >
        <div style={innerWrapperStyle}>
          {!hasSections ? (
            <MjWrapperEmptyPlaceholder />
          ) : (
            this.block.children?.map((child) => (
              <React.Fragment key={child.id}>
                {EmailBlockClass.renderEmailBlock(
                  child,
                  attributeDefaults,
                  selectedBlockId,
                  onSelectBlock,
                  emailTree,
                  onUpdateBlock,
                  onCloneBlock,
                  onDeleteBlock,
                  onSave,
                  savedBlocks
                )}
              </React.Fragment>
            ))
          )}
        </div>
      </div>
    )
  }
}
