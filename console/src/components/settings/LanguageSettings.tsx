import { useEffect, useState } from 'react'
import { Button, Form, Select, App, Descriptions } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { SettingsSectionHeader } from './SettingsSectionHeader'

const LANGUAGE_OPTIONS = [
  { value: 'en', label: 'English' },
  { value: 'fr', label: 'French' },
  { value: 'de', label: 'German' },
  { value: 'es', label: 'Spanish' },
  { value: 'pt', label: 'Portuguese' },
  { value: 'pt-BR', label: 'Portuguese (Brazil)' },
  { value: 'it', label: 'Italian' },
  { value: 'nl', label: 'Dutch' },
  { value: 'ja', label: 'Japanese' },
  { value: 'ko', label: 'Korean' },
  { value: 'zh', label: 'Chinese' },
  { value: 'ru', label: 'Russian' },
  { value: 'ar', label: 'Arabic' },
  { value: 'hi', label: 'Hindi' },
  { value: 'tr', label: 'Turkish' },
  { value: 'pl', label: 'Polish' },
  { value: 'sv', label: 'Swedish' },
  { value: 'da', label: 'Danish' },
  { value: 'fi', label: 'Finnish' },
  { value: 'nb', label: 'Norwegian' }
]

interface LanguageSettingsProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  isOwner: boolean
}

export function LanguageSettings({ workspace, onWorkspaceUpdate, isOwner }: LanguageSettingsProps) {
  const { t } = useLingui()
  const [savingSettings, setSavingSettings] = useState(false)
  const [formTouched, setFormTouched] = useState(false)
  const [form] = Form.useForm()
  const { message } = App.useApp()

  useEffect(() => {
    if (!isOwner) return

    form.setFieldsValue({
      default_language: workspace?.settings.default_language || 'en',
      supported_languages: workspace?.settings.supported_languages || ['en']
    })
    setFormTouched(false)
  }, [workspace, form, isOwner])

  const handleSaveSettings = async (values: {
    default_language: string
    supported_languages: string[]
  }) => {
    if (!workspace) return

    // Ensure the default language is included in supported languages
    let supportedLanguages = values.supported_languages || []
    if (!supportedLanguages.includes(values.default_language)) {
      supportedLanguages = [values.default_language, ...supportedLanguages]
    }

    setSavingSettings(true)
    try {
      await workspaceService.update({
        ...workspace,
        settings: {
          ...workspace.settings,
          default_language: values.default_language,
          supported_languages: supportedLanguages
        }
      })

      // Refresh the workspace data
      const response = await workspaceService.get(workspace.id)

      // Update the parent component with the new workspace data
      onWorkspaceUpdate(response.workspace)

      setFormTouched(false)
      message.success(t`Language settings updated successfully`)
    } catch (error: unknown) {
      console.error('Failed to update language settings', error)
      const errorMessage = (error as Error)?.message || t`Failed to update language settings`
      message.error(errorMessage)
    } finally {
      setSavingSettings(false)
    }
  }

  const handleFormChange = () => {
    setFormTouched(true)
  }

  const getLabelForLanguage = (code: string) => {
    const option = LANGUAGE_OPTIONS.find((o) => o.value === code)
    return option ? option.label : code
  }

  if (!isOwner) {
    const defaultLang = workspace?.settings.default_language || 'en'
    const supportedLangs = workspace?.settings.supported_languages || ['en']

    return (
      <>
        <SettingsSectionHeader
          title={t`Languages`}
          description={t`Language configuration for templates and content`}
        />

        <Descriptions
          bordered
          column={1}
          size="small"
          styles={{ label: { width: '200px', fontWeight: '500' } }}
        >
          <Descriptions.Item label={t`Default Language`}>
            {getLabelForLanguage(defaultLang)}
          </Descriptions.Item>

          <Descriptions.Item label={t`Supported Languages`}>
            {supportedLangs.map((lang) => getLabelForLanguage(lang)).join(', ')}
          </Descriptions.Item>
        </Descriptions>
      </>
    )
  }

  return (
    <>
      <SettingsSectionHeader
        title={t`Languages`}
        description={t`Configure the default language and supported languages for your workspace templates and content.`}
      />

      <Form
        form={form}
        layout="vertical"
        onFinish={handleSaveSettings}
        onValuesChange={handleFormChange}
      >
        <Form.Item
          name="default_language"
          label={t`Default Language`}
          tooltip={t`The primary language used for templates when no specific language is specified.`}
          rules={[{ required: true, message: t`Please select a default language` }]}
        >
          <Select
            options={LANGUAGE_OPTIONS}
            showSearch
            optionFilterProp="label"
            placeholder={t`Select default language`}
          />
        </Form.Item>

        <Form.Item
          name="supported_languages"
          label={t`Supported Languages`}
          tooltip={t`Languages available for template translations. The default language is always included.`}
          rules={[{ required: true, message: t`Please select at least one supported language` }]}
        >
          <Select
            mode="multiple"
            options={LANGUAGE_OPTIONS}
            showSearch
            optionFilterProp="label"
            placeholder={t`Select supported languages`}
          />
        </Form.Item>

        <Form.Item>
          <Button type="primary" htmlType="submit" loading={savingSettings} disabled={!formTouched}>
            {t`Save Changes`}
          </Button>
        </Form.Item>
      </Form>
    </>
  )
}
