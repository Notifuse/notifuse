import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { faHeading } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { DraftTextField } from '../ui/fields/DraftTextField'

// The settings panel of the email title
interface MjTitleSettingsPanelProps {
  currentContent: string
  onUpdate: OnUpdateAttributesFunction
}

const MjTitleSettingsPanel: React.FC<MjTitleSettingsPanelProps> = ({ currentContent, onUpdate }) => {
  const { t } = useLingui()

  return (
    <SettingsSection sectionKey="mj-title:title" title={t`Title`} first>
      <SettingsRow
        label={t`Title`}
        vertical
        help={
          <>
            <p className="m-0 mb-1">
              {t`This appears in browser tabs when viewing the email in a web browser and in some email clients. It's typically the same as or related to your email subject line.`}
            </p>
            <div className="mb-1 font-semibold">{t`Best practices:`}</div>
            <ul className="m-0 list-inside list-disc p-0">
              <li>{t`Keep it concise and descriptive`}</li>
              <li>{t`Make it relevant to your email content`}</li>
              <li>{t`Consider using the same text as your subject line`}</li>
              <li>{t`Avoid special characters that might not display properly`}</li>
            </ul>
          </>
        }
      >
        <DraftTextField
          aria-label={t`Title`}
          value={currentContent === '' ? undefined : currentContent}
          // An emptied title is written as an empty text, as before
          onChange={(value) => onUpdate({ content: value ?? '' })}
          placeholder={t`Enter email title...`}
          maxLength={100}
          trim={false}
        />
      </SettingsRow>
    </SettingsSection>
  )
}

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
   * Render the settings panel for the title block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction
  ): React.ReactNode {
    const currentContent = 'content' in this.block ? (this.block.content as string) : ''

    return (
      <MjTitleSettingsPanel
        currentContent={currentContent}
        onUpdate={onUpdate}
      />
    )
  }
}
