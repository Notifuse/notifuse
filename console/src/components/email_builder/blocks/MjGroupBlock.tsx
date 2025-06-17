import React from 'react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faLock, faArrowLeft, faArrowRight } from '@fortawesome/free-solid-svg-icons'
import { Select, Radio, Tooltip } from 'antd'
import type { MJMLComponentType, EmailBlock, MJGroupAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import InputLayout from '../ui/InputLayout'
import BackgroundInput from '../ui/BackgroundInput'
import StringPopoverInput from '../ui/StringPopoverInput'
import WidthInput from '../ui/WidthInput'
import PanelLayout from '../panels/PanelLayout'

/**
 * Implementation for mj-group blocks
 * mj-group allows you to prevent columns from stacking on mobile.
 * Columns inside a group will stay side by side on mobile.
 */
export class MjGroupBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faLock} className="opacity-70" />
  }

  getLabel(): string {
    return 'Group'
  }

  getDescription(): React.ReactNode {
    return (
      <div>
        <div style={{ marginBottom: 8 }}>Prevent columns from stacking on mobile devices</div>
        <div style={{ opacity: 0.7 }}>
          <div
            style={{
              width: 60,
              height: 30,
              border: '2px solid #722ed1',
              borderRadius: 4,
              backgroundColor: '#f9f0ff',
              padding: 4,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}
          >
            <div style={{ display: 'flex', gap: 2 }}>
              <div
                style={{
                  width: 8,
                  height: 16,
                  backgroundColor: '#722ed1',
                  borderRadius: 1
                }}
              />
              <div
                style={{
                  width: 8,
                  height: 16,
                  backgroundColor: '#d3adf7',
                  borderRadius: 1
                }}
              />
              <div
                style={{
                  width: 8,
                  height: 16,
                  backgroundColor: '#722ed1',
                  borderRadius: 1
                }}
              />
            </div>
            <FontAwesomeIcon
              icon={faLock}
              style={{
                position: 'absolute',
                marginTop: -20,
                marginLeft: 40,
                fontSize: 8,
                color: '#722ed1'
              }}
            />
          </div>
        </div>
      </div>
    )
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, any> {
    return MJML_COMPONENT_DEFAULTS['mj-group'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-column']
  }

  /**
   * Render the settings panel for the group block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: Record<string, any>,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJGroupAttributes

    const handleAttributeChange = (key: string, value: any) => {
      onUpdate({ [key]: value })
    }

    const handleBackgroundChange = (backgroundValues: any) => {
      onUpdate(backgroundValues)
    }

    return (
      <PanelLayout title="Group Attributes">
        <div className="space-y-4">
          {/* Layout Settings */}
          <InputLayout label="Width">
            <WidthInput
              value={currentAttributes.width}
              onChange={(value) => onUpdate({ width: value })}
              placeholder={blockDefaults.width || '100%'}
            />
          </InputLayout>

          <InputLayout label="Vertical Align">
            <Select
              size="small"
              value={currentAttributes.verticalAlign || blockDefaults.verticalAlign || 'top'}
              onChange={(value) => handleAttributeChange('verticalAlign', value)}
              options={[
                { value: 'top', label: 'Top' },
                { value: 'middle', label: 'Middle' },
                { value: 'bottom', label: 'Bottom' }
              ]}
              style={{ width: '100%' }}
            />
          </InputLayout>

          <InputLayout label="Height">
            <StringPopoverInput
              value={currentAttributes.height || ''}
              onChange={(value) => handleAttributeChange('height', value || undefined)}
              placeholder="auto"
              buttonText="Set height"
            />
          </InputLayout>

          {/* Background Settings */}
          <BackgroundInput
            value={{
              backgroundColor: currentAttributes.backgroundColor,
              backgroundUrl: currentAttributes.backgroundUrl,
              backgroundSize: currentAttributes.backgroundSize,
              backgroundRepeat: currentAttributes.backgroundRepeat,
              backgroundPosition: currentAttributes.backgroundPosition,
              backgroundPositionX: currentAttributes.backgroundPositionX,
              backgroundPositionY: currentAttributes.backgroundPositionY
            }}
            onChange={handleBackgroundChange}
          />

          {/* Direction Settings */}
          <InputLayout label="Text Direction">
            <Radio.Group
              size="small"
              value={currentAttributes.direction || blockDefaults.direction || 'ltr'}
              onChange={(e) => handleAttributeChange('direction', e.target.value)}
            >
              <Radio.Button value="ltr">
                <Tooltip title="Left to Right">
                  <FontAwesomeIcon icon={faArrowRight} style={{ marginRight: 4 }} />
                  LTR
                </Tooltip>
              </Radio.Button>
              <Radio.Button value="rtl">
                <Tooltip title="Right to Left">
                  <FontAwesomeIcon icon={faArrowLeft} style={{ marginRight: 4 }} />
                  RTL
                </Tooltip>
              </Radio.Button>
            </Radio.Group>
          </InputLayout>

          {/* Advanced Settings */}
          <InputLayout label="CSS Class" help="Custom CSS class for styling">
            <StringPopoverInput
              value={currentAttributes.cssClass}
              onChange={(value) => handleAttributeChange('cssClass', value)}
              placeholder="my-custom-class"
              buttonText="Set Value"
            />
          </InputLayout>
        </div>
      </PanelLayout>
    )
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const {
      selectedBlockId,
      onSelectBlock,
      onUpdateBlock,
      attributeDefaults,
      emailTree,
      onCloneBlock,
      onDeleteBlock,
      onSaveBlock: onSave,
      savedBlocks
    } = props

    const key = this.block.id
    const isSelected = selectedBlockId === this.block.id
    const blockClasses = `email-block-hover ${isSelected ? 'selected' : ''}`.trim()

    const selectionStyle: React.CSSProperties = isSelected ? { zIndex: 10 } : {}

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onSelectBlock) {
        onSelectBlock(this.block.id)
      }
    }

    const attrs = EmailBlockClass.mergeWithAllDefaults(
      'mj-group',
      this.block.attributes,
      attributeDefaults
    )

    const groupStyle: React.CSSProperties = {
      display: 'flex',
      flexDirection: attrs.direction === 'rtl' ? 'row-reverse' : 'row',
      width: attrs.width,
      verticalAlign: attrs.verticalAlign as any,
      backgroundColor: attrs.backgroundColor,
      alignItems:
        attrs.verticalAlign === 'middle'
          ? 'center'
          : attrs.verticalAlign === 'bottom'
          ? 'flex-end'
          : 'flex-start',
      ...selectionStyle
    }

    // Render child columns
    const children = this.block.children || []

    return (
      <div
        key={key}
        style={{ ...groupStyle, position: 'relative' }}
        className={`${attrs.cssClass || ''} ${blockClasses}`.trim()}
        onClick={handleClick}
        data-block-id={this.block.id}
      >
        {children.length === 0 ? (
          <div
            style={{
              width: '100%',
              height: '40px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#722ed1',
              fontSize: '12px',
              fontStyle: 'italic'
            }}
          >
            Group (drag columns here)
          </div>
        ) : (
          children.map((child) => (
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
    )
  }
}
