import React, { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { faEye } from '@fortawesome/free-regular-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { DraftTextField } from '../ui/fields/DraftTextField'

const PREVIEW_MAX_LENGTH = 200
// Past this length most inboxes cut the preview text
const PREVIEW_WARN_LENGTH = 140

// The settings panel of the preview text
interface MjPreviewSettingsPanelProps {
  currentContent: string
  onUpdate: OnUpdateAttributesFunction
}

const MjPreviewSettingsPanel: React.FC<MjPreviewSettingsPanelProps> = ({ currentContent, onUpdate }) => {
  const { t } = useLingui()
  // The length of the text in the field, typed or saved: the field keeps its draft to itself, so
  // the counter follows the typing from the change events that bubble up from it
  const [typed, setTyped] = useState<{ for: string; length: number } | null>(null)
  const length = typed && typed.for === currentContent ? typed.length : currentContent.length
  const tooLong = length > PREVIEW_WARN_LENGTH

  return (
    <SettingsSection sectionKey="mj-preview:preview" title={t`Preview`} first>
      <SettingsRow
        label={t`Subject preview`}
        vertical
        help={
          <>
            <p className="m-0 mb-1">
              {t`This text appears in the email client's preview pane, typically after the subject line. Keep it concise and compelling (100-140 characters recommended).`}
            </p>
            <div className="mb-1 font-semibold">{t`Tips for effective preview text:`}</div>
            <ul className="m-0 list-inside list-disc p-0">
              <li>{t`Keep it between 35-140 characters`}</li>
              <li>{t`Complement, don't repeat the subject line`}</li>
              <li>{t`Create urgency or curiosity`}</li>
              <li>{t`Include a clear call-to-action`}</li>
            </ul>
          </>
        }
        alert={
          tooLong ? (
            <div className="text-2xs text-warning-fg">{t`Past 140 characters, most inboxes cut the text.`}</div>
          ) : undefined
        }
      >
        <div
          onChange={(event) => {
            const target = event.target as HTMLTextAreaElement
            setTyped({ for: currentContent, length: target.value.length })
          }}
          onBlur={() => setTyped(null)}
          // Escape reverts the draft (the field stops the event, hence the capture)
          onKeyDownCapture={(event) => {
            if (event.key === 'Escape') setTyped(null)
          }}
        >
          <DraftTextField
            aria-label={t`Subject preview`}
            value={currentContent === '' ? undefined : currentContent}
            // An emptied preview is written as an empty text, as before
            onChange={(value) => onUpdate({ content: value ?? '' })}
            placeholder={t`Enter preview text that appears in email clients...`}
            multiline
            rows={3}
            maxLength={PREVIEW_MAX_LENGTH}
            trim={false}
          />
          <div
            data-testid="preview-counter"
            className={`mt-1 text-right text-2xs ${tooLong ? 'font-semibold text-warning-fg' : 'text-ink-4'}`}
          >
            {length} / {PREVIEW_MAX_LENGTH}
          </div>
        </div>
      </SettingsRow>
    </SettingsSection>
  )
}

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
   * Render the settings panel for the preview block
   */
  renderSettingsPanel(onUpdate: OnUpdateAttributesFunction): React.ReactNode {
    const currentContent = ((this.block as unknown as Record<string, unknown>).content || '') as string

    return (
      <MjPreviewSettingsPanel
        currentContent={currentContent}
        onUpdate={onUpdate}
      />
    )
  }
}
