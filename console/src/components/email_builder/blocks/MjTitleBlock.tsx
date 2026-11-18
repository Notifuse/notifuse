import React from 'react'
import type { MJMLComponentType } from '../types'
import { BaseEmailBlock } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { faHeading } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'

/**
 * Implementation for mj-title blocks (email title/subject)
 */
export class MjTitleBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faHeading} className="opacity-70" />
  }

  getLabel(): string {
    return 'Email Title'
  }

  getDescription(): React.ReactNode {
    return 'Sets the email title that appears in the browser tab and some email clients'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-title'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  getEdit(): React.ReactNode {
    // Title blocks don't render in preview (they're metadata)
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
