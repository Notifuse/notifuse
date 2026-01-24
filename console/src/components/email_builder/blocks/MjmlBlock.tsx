import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faEnvelope } from '@fortawesome/free-solid-svg-icons'
import type { MJMLComponentType } from '../types'
import { BaseEmailBlock, type PreviewProps } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import PanelLayout from '../panels/PanelLayout'

// Functional component for settings panel with i18n support
const MjmlSettingsPanel: React.FC = () => {
  const { t } = useLingui()

  return <PanelLayout title={t`Email Attributes`}>{t`TODO`}</PanelLayout>
}

/**
 * Implementation for mjml root blocks
 */
export class MjmlBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faEnvelope} className="opacity-70" />
  }

  getLabel(): string {
    return 'Email'
  }

  getDescription(): React.ReactNode {
    return 'Root container for the entire email document'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mjml'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-head', 'mj-body']
  }

  /**
   * Render the settings panel for the mjml block
   */
  renderSettingsPanel(): React.ReactNode {
    return <MjmlSettingsPanel />
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
      'mjml',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    // Pass the current block as emailTree since this is the root
    const currentEmailTree = emailTree || this.block

    return (
      <div
        key={key}
        style={{
          fontFamily: 'Arial, sans-serif',
          direction: attrs.direction as 'ltr' | 'rtl' | undefined,
          ...selectionStyle
        }}
        className={blockClasses}
        onClick={handleClick}
        lang={attrs.lang}
      >
        {this.block.children?.map((child) => (
          <React.Fragment key={child.id}>
            {EmailBlockClass.renderEmailBlock(
              child,
              attributeDefaults,
              selectedBlockId,
              onSelectBlock,
              currentEmailTree,
              onUpdateBlock,
              onCloneBlock,
              onDeleteBlock,
              onSave,
              savedBlocks
            )}
          </React.Fragment>
        ))}
      </div>
    )
  }
}
