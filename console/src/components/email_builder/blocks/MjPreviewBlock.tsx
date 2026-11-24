import React from 'react'
import { i18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { MJMLComponentType } from '../types'
import { BaseEmailBlock } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { faEye } from '@fortawesome/free-regular-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'

/**
 * Implementation for mj-preview blocks (email preview text)
 */
export class MjPreviewBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faEye} className="opacity-70" />
  }

  getLabel(): string {
    return i18n._(msg({ message: 'Subject preview', context: 'email block' }))
  }

  getDescription(): React.ReactNode {
    return 'Sets the subject preview text that appears in email clients'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-preview'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  getEdit(): React.ReactNode {
    // Preview blocks don't render in preview (they're metadata)
    return null
  }

  /**
   * No panel: the server writes this block from the template's settings on every save
   * (subject and preview text), so an edit made here would not last. The editors that
   * show the email tree hide this block.
   */
  renderSettingsPanel(): React.ReactNode {
    return null
  }
}
