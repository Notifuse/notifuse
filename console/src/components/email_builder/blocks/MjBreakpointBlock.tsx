import React from 'react'
import { InputNumber } from 'antd'
import type { MJMLComponentType, MJBreakpointAttributes, MergedBlockAttributes } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { faMobileAlt } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import PanelLayout from '../panels/PanelLayout'
import InputLayout from '../ui/InputLayout'

/**
 * Implementation for mj-breakpoint blocks (responsive breakpoint configuration)
 */
export class MjBreakpointBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faMobileAlt} className="opacity-70" />
  }

  getLabel(): string {
    return 'Breakpoint'
  }

  getDescription(): React.ReactNode {
    return 'Configure the responsive breakpoint for mobile view'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-breakpoint'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  getEdit(): React.ReactNode {
    // Breakpoint blocks don't render in preview (they're configuration)
    return null
  }

  /**
   * Render the settings panel for the breakpoint block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJBreakpointAttributes
    return (
      <PanelLayout title="Breakpoint Attributes">
        <div className="mb-4 p-3 bg-blue-50 border border-blue-200 rounded-lg">
          <div className="text-xs text-blue-700 font-medium mb-1">📱 Mobile Breakpoint</div>
          <div className="text-xs text-blue-600">
            Sets the screen width below which the email will switch to mobile layout. Most email
            clients use 480px as the standard mobile breakpoint.
          </div>
        </div>

        <div className="mb-4 p-3 bg-amber-50 border border-amber-200 rounded-lg">
          <div className="text-xs text-amber-700 font-medium mb-1">🔍 Preview Mode Required</div>
          <div className="text-xs text-amber-600">
            Switch to <strong>Preview mode</strong> to see the breakpoint effects. The edit mode
            shows the raw structure, but only preview mode compiles the MJML template and renders
            the responsive behavior properly.
          </div>
        </div>

        <div className="mt-4 p-3 bg-gray-50 border border-gray-200 rounded-lg">
          <div className="text-xs text-gray-600">
            <div className="font-medium mb-1">💡 How it works:</div>
            <ul className="space-y-1 ml-3">
              <li>• Sets the responsive breakpoint for the entire email</li>
              <li>• Only one breakpoint per email template is allowed</li>
              <li>• Affects how columns stack on mobile devices</li>
              <li>• Standard value is 480px for most email clients</li>
            </ul>
          </div>
        </div>
        <InputLayout
          label="Breakpoint Width"
          help="Screen width below which mobile layout is activated. Common values: 480px, 600px, 768px"
        >
          <InputNumber
            size="small"
            value={
              currentAttributes.width
                ? parseInt(currentAttributes.width.replace('px', ''))
                : undefined
            }
            onChange={(value) => onUpdate({ width: value ? `${value}px` : undefined })}
            placeholder={parseInt((blockDefaults.width || '480px').replace('px', '')).toString()}
            min={240}
            max={1200}
            step={10}
            suffix="px"
            style={{ width: '120px' }}
          />
        </InputLayout>
      </PanelLayout>
    )
  }
}
