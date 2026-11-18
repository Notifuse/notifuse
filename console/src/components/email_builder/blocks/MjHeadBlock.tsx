import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType } from '../types'
import {
  BaseEmailBlock
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'

// The head has no settings of its own: a message says where they are
const MjHeadSettingsPanel: React.FC = () => {
  const { t } = useLingui()

  return (
    <div className="px-6 py-6 text-center text-xs text-ink-3">
      <p className="m-0">{t`No settings available for the head element.`}</p>
      <p className="m-0 mt-1">{t`Add child elements like mj-font, mj-style, or mj-preview to configure email metadata.`}</p>
    </div>
  )
}

/**
 * Implementation for mj-head blocks
 */
export class MjHeadBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return null
  }

  getLabel(): string {
    return 'Head'
  }

  getDescription(): React.ReactNode {
    return 'Contains metadata and configuration for the email'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-head'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return [
      'mj-attributes',
      'mj-breakpoint',
      'mj-font',
      'mj-html-attributes',
      'mj-preview',
      'mj-style',
      'mj-title'
    ]
  }

  /**
   * Render the settings panel for the head block
   */
  renderSettingsPanel(): React.ReactNode {
    return <MjHeadSettingsPanel />
  }

  getEdit(): React.ReactNode {
    // Head blocks don't render in preview (they contain metadata)
    return null
  }
}
