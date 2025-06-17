import React from 'react'
import { Input } from 'antd'
import type { MJMLComponentType, EmailBlock } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { faEye } from '@fortawesome/free-regular-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import PanelLayout from '../panels/PanelLayout'
import InputLayout from '../ui/InputLayout'

const { TextArea } = Input

/**
 * Implementation for mj-preview blocks (email preview text)
 */
export class MjPreviewBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faEye} className="opacity-70" />
  }

  getLabel(): string {
    return 'Subject preview'
  }

  getDescription(): React.ReactNode {
    return 'Sets the subject preview text that appears in email clients'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, any> {
    return MJML_COMPONENT_DEFAULTS['mj-preview'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  getEdit(_props: PreviewProps): React.ReactNode {
    // Preview blocks don't render in preview (they're metadata)
    return null
  }

  /**
   * Render the settings panel for the preview block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    _blockDefaults: Record<string, any>,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentContent = (this.block as any).content || ''

    const handleContentChange = (value: string) => {
      onUpdate({ content: value })
    }

    return (
      <PanelLayout title="Preview Text Settings">
        <InputLayout
          label="Subject preview"
          help="This text appears in the email client's preview pane, typically after the subject line. Keep it concise and compelling (100-140 characters recommended)."
          layout="vertical"
        >
          <TextArea
            value={currentContent}
            onChange={(e) => handleContentChange(e.target.value)}
            placeholder="Enter preview text that appears in email clients..."
            rows={3}
            maxLength={200}
            showCount
          />
        </InputLayout>

        <div className="text-xs text-gray-500 mt-2">
          <div className="mb-1">
            <strong>Tips for effective preview text:</strong>
          </div>
          <ul className="list-disc list-inside space-y-1">
            <li>Keep it between 35-140 characters</li>
            <li>Complement, don't repeat the subject line</li>
            <li>Create urgency or curiosity</li>
            <li>Include a clear call-to-action</li>
          </ul>
        </div>
      </PanelLayout>
    )
  }
}
