import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock, MJMLComponentType, MergedBlockAttributes } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import PanelLayout from '../panels/PanelLayout'
import HeadingStylesInput from '../ui/HeadingStylesInput'

// Functional component for settings panel with i18n support
const MjAttributesSettingsPanel: React.FC<{
  emailTree?: EmailBlock
  onUpdateBlock?: (blockId: string, block: EmailBlock) => void
}> = ({ emailTree, onUpdateBlock }) => {
  const { t } = useLingui()

  return (
    <PanelLayout title={t`Default attributes`}>
      <HeadingStylesInput emailTree={emailTree} onUpdateBlock={onUpdateBlock} />
      <div className="text-sm text-gray-500 text-center py-8">
        {t`Add child elements for specific components (mj-text, mj-button, etc.) to set their default values.`}
      </div>
    </PanelLayout>
  )
}

/**
 * Implementation for mj-attributes blocks
 */
export class MjAttributesBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return null
  }

  getLabel(): string {
    return 'Default attributes'
  }

  getDescription(): React.ReactNode {
    return 'Defines default attribute values for MJML components'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-attributes'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    // mj-attributes can contain attribute elements for any MJML component type
    return [
      'mj-all', 'mj-class',
      'mj-text', 'mj-button', 'mj-image', 'mj-section', 'mj-column',
      'mj-wrapper', 'mj-body', 'mj-divider', 'mj-spacer', 'mj-social',
      'mj-social-element', 'mj-group'
    ]
  }

  /**
   * Render the settings panel for the attributes block
   */
  renderSettingsPanel(
    _onUpdate: OnUpdateAttributesFunction,
    _defaults: MergedBlockAttributes,
    emailTree?: EmailBlock,
    onUpdateBlock?: (blockId: string, block: EmailBlock) => void
  ): React.ReactNode {
    return <MjAttributesSettingsPanel emailTree={emailTree} onUpdateBlock={onUpdateBlock} />
  }

  getEdit(): React.ReactNode {
    // Attributes blocks don't render in preview (they contain configuration)
    return null
  }
}
