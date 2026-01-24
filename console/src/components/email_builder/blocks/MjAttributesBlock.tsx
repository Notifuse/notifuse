import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType } from '../types'
import { BaseEmailBlock } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import PanelLayout from '../panels/PanelLayout'

// Functional component for settings panel with i18n support
const MjAttributesSettingsPanel: React.FC = () => {
  const { t } = useLingui()

  return (
    <PanelLayout title={t`Default Attributes`}>
      <div className="text-sm text-gray-500 text-center py-8">
        {t`No settings available for the attributes container.`}
        <br />
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
    return ['mj-text', 'mj-button', 'mj-image', 'mj-section', 'mj-column', 'mj-wrapper', 'mj-body']
  }

  /**
   * Render the settings panel for the attributes block
   */
  renderSettingsPanel(): React.ReactNode {
    return <MjAttributesSettingsPanel />
  }

  getEdit(): React.ReactNode {
    // Attributes blocks don't render in preview (they contain configuration)
    return null
  }
}
