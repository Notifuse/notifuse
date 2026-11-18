import { useLingui } from '@lingui/react/macro'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { DraftTextField } from '../ui/fields/DraftTextField'
import type { PanelFields } from './panelFields'

// The CSS class row, last section of most panels.
export function AdvancedSection({ type, fields }: { type: string; fields: PanelFields }) {
  const { t } = useLingui()
  return (
    <SettingsSection
      sectionKey={`${type}:advanced`}
      title={t`Advanced`}
      setCount={fields.setCount('cssClass')}
      summary={fields.value('cssClass')}
    >
      <SettingsRow
        label={t`CSS class`}
        help={t`Added to the block's element, to target it from a Style block.`}
        isSet={fields.isSet('cssClass')}
        onReset={fields.reset('cssClass')}
      >
        <DraftTextField
          aria-label={t`CSS class`}
          value={fields.value('cssClass')}
          onChange={fields.write('cssClass')}
          placeholder={t`e.g. hero`}
          mono
        />
      </SettingsRow>
    </SettingsSection>
  )
}
