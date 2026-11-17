import React, { useEffect } from 'react'
import { App, AutoComplete, Form, Input, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Integration, LLMProviderKind, Workspace } from '../../services/api/types'
import { llmProviders } from './LLMProviders'
import {
  ANTHROPIC_DEFAULT_MODEL,
  anthropicModels,
  GEMINI_DEFAULT_MODEL,
  geminiModels,
  OPENAI_DEFAULT_MODEL
} from './llmModels'
import { Panel } from '../layout'
import { SettingsRow, SettingsRows } from '../settings/SettingsRow'
import { useSecretPlaceholder } from '../settings/integrations/secrets'
import { OptionalLabel, RequiredLabel } from '../settings/integrations/FieldLabel'

interface LLMIntegrationProps {
  integration?: Integration
  workspace: Workspace
  providerKind: LLMProviderKind
  onSave: (integration: Integration) => Promise<void>
  isOwner: boolean
  formRef?: React.RefObject<{ submit: () => void } | null>
}

const filterByValue = (inputValue: string, option?: { value?: string }) =>
  (option?.value ?? '').toLowerCase().includes(inputValue.toLowerCase())

/** The rows of an AI assistant integration: its name, key and model. */
export const LLMIntegration: React.FC<LLMIntegrationProps> = ({
  integration,
  providerKind,
  onSave,
  isOwner,
  formRef
}) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const secretPlaceholder = useSecretPlaceholder(integration)

  // Expose form instance to parent via ref
  useEffect(() => {
    if (formRef) {
      // eslint-disable-next-line react-hooks/immutability -- Intentionally exposing form to parent via ref
      ;(formRef as React.MutableRefObject<{ submit: () => void } | null>).current = form
    }
  }, [form, formRef])

  // Get the provider info for default values
  const providerInfo = llmProviders.find((p) => p.kind === providerKind)

  useEffect(() => {
    if (providerKind === 'openai') {
      if (integration?.llm_provider?.openai) {
        const provider = integration.llm_provider.openai
        form.setFieldsValue({
          name: integration.name,
          model: provider.model || providerInfo?.defaultModel || OPENAI_DEFAULT_MODEL,
          base_url: provider.base_url || '',
          // Always read it back so handleSave can resend it (UpdateIntegration
          // overwrites the whole settings object; an omitted field would be wiped).
          reasoning_effort: provider.reasoning_effort || ''
        })
      } else {
        form.setFieldsValue({
          name: providerInfo?.name || 'OpenAI',
          model: providerInfo?.defaultModel || OPENAI_DEFAULT_MODEL,
          base_url: '',
          reasoning_effort: ''
        })
      }
    } else if (providerKind === 'gemini') {
      if (integration?.llm_provider?.gemini) {
        const provider = integration.llm_provider.gemini
        form.setFieldsValue({
          name: integration.name,
          model: provider.model || providerInfo?.defaultModel || GEMINI_DEFAULT_MODEL
        })
      } else {
        form.setFieldsValue({
          name: providerInfo?.name || 'Google Gemini',
          model: providerInfo?.defaultModel || GEMINI_DEFAULT_MODEL
        })
      }
    } else {
      if (integration?.llm_provider) {
        const provider = integration.llm_provider
        form.setFieldsValue({
          name: integration.name,
          model: provider.anthropic?.model || providerInfo?.defaultModel || ''
        })
      } else {
        form.setFieldsValue({
          name: providerInfo?.name || 'Anthropic',
          model: providerInfo?.defaultModel || ANTHROPIC_DEFAULT_MODEL
        })
      }
    }
  }, [integration, providerKind, form, providerInfo])

  const handleSave = async (values: Record<string, unknown>) => {
    if (!isOwner) {
      message.error(t`Only workspace owners can modify integrations`)
      return
    }

    try {
      const isString = (value: unknown): value is string => typeof value === 'string'

      const integrationData: Integration = {
        id: integration?.id || `int_${Date.now()}`,
        name: isString(values.name) ? values.name : providerInfo?.name || '',
        type: 'llm',
        llm_provider: {
          kind: providerKind,
          ...(providerKind === 'anthropic' && {
            anthropic: {
              api_key:
                isString(values.api_key) && values.api_key !== '' ? values.api_key : undefined,
              model: isString(values.model) ? values.model : providerInfo?.defaultModel || ''
            }
          }),
          ...(providerKind === 'openai' && {
            openai: {
              api_key:
                isString(values.api_key) && values.api_key !== '' ? values.api_key : undefined,
              model: isString(values.model) ? values.model : OPENAI_DEFAULT_MODEL,
              base_url:
                isString(values.base_url) && values.base_url !== '' ? values.base_url : undefined,
              // Always send the explicit value (empty = provider default) so editing the
              // integration never silently resets it.
              reasoning_effort: isString(values.reasoning_effort) ? values.reasoning_effort : ''
            }
          }),
          ...(providerKind === 'gemini' && {
            gemini: {
              api_key:
                isString(values.api_key) && values.api_key !== '' ? values.api_key : undefined,
              model: isString(values.model)
                ? values.model
                : providerInfo?.defaultModel || GEMINI_DEFAULT_MODEL
            }
          })
        },
        created_at: integration?.created_at || new Date().toISOString(),
        updated_at: new Date().toISOString()
      }

      await onSave(integrationData)
    } catch (error) {
      // The page reports a failed save itself (saveOtherIntegration), and a field that does not
      // validate shows under the field: a toast here would be the second one.
      console.error('Failed to save LLM integration:', error)
    }
  }

  const keyStored = Boolean(
    integration?.credential_hints?.[`${providerKind}.api_key`] ||
      integration?.llm_provider?.[providerKind]?.encrypted_api_key
  )
  const keyHelp: Record<LLMProviderKind, string | undefined> = {
    anthropic: t`From console.anthropic.com`,
    openai: t`From platform.openai.com, or your endpoint's own key`,
    gemini: t`From Google AI Studio`
  }
  const keyExample: Record<LLMProviderKind, string> = {
    anthropic: 'sk-ant-api03-...',
    openai: 'sk-proj-...',
    gemini: 'AIza...'
  }

  return (
    <Form form={form} layout="vertical" component={false} onFinish={handleSave} disabled={!isOwner}>
      <Panel title={t`Integration`}>
        <SettingsRows>
          <SettingsRow
            label={<RequiredLabel>{t`Name`}</RequiredLabel>}
            help={t`Shown in the assistant's model picker`}
            htmlFor="name"
          >
            <Form.Item name="name" rules={[{ required: true, message: t`Please enter integration name` }]}>
              <Input
                placeholder={
                  providerKind === 'openai'
                    ? t`e.g., My OpenAI Integration`
                    : providerKind === 'gemini'
                      ? t`e.g., My Google Gemini Integration`
                      : t`e.g., My Anthropic Integration`
                }
              />
            </Form.Item>
          </SettingsRow>
        </SettingsRows>
      </Panel>

      <Panel title={t`Model`}>
        <SettingsRows>
          <SettingsRow
            label={integration ? t`API key` : <RequiredLabel>{t`API key`}</RequiredLabel>}
            help={keyHelp[providerKind]}
            htmlFor="api_key"
          >
            <Form.Item
              name="api_key"
              extra={integration && keyStored ? t`Leave blank to keep the existing API key` : undefined}
              rules={integration ? [] : [{ required: true, message: t`Please enter your API key` }]}
            >
              <Input.Password
                autoComplete="new-password"
                placeholder={secretPlaceholder(`${providerKind}.api_key`, keyStored, keyExample[providerKind])}
              />
            </Form.Item>
          </SettingsRow>

          {providerKind === 'anthropic' && (
            <SettingsRow
              label={<RequiredLabel>{t`Model`}</RequiredLabel>}
              help={t`Pick one, or type any Anthropic model ID`}
              htmlFor="model"
            >
              <Form.Item name="model" rules={[{ required: true, message: t`Please select or enter a model` }]}>
                <AutoComplete options={anthropicModels} placeholder={t`Select a model`} filterOption={filterByValue} />
              </Form.Item>
            </SettingsRow>
          )}

          {providerKind === 'openai' && (
            <>
              <SettingsRow
                label={<RequiredLabel>{t`Model`}</RequiredLabel>}
                help={t`gpt-6.1-sol, gpt-6-astra, gpt-6.1-terra, or the name your endpoint uses`}
                htmlFor="model"
              >
                <Form.Item name="model" rules={[{ required: true, message: t`Please enter a model name` }]}>
                  <Input placeholder={t`gpt-6.1-sol, gpt-6-astra, gpt-6.1-terra, or custom model name`} />
                </Form.Item>
              </SettingsRow>

              <SettingsRow
                label={<OptionalLabel>{t`Base URL`}</OptionalLabel>}
                help={t`For an OpenAI-compatible service (Ollama, vLLM, LiteLLM, Azure…). Empty: the OpenAI API`}
                htmlFor="base_url"
              >
                <Form.Item name="base_url">
                  <Input placeholder="https://api.openai.com/v1" />
                </Form.Item>
              </SettingsRow>

              <SettingsRow
                label={t`Reasoning effort`}
                help={t`How much a reasoning model thinks. Default for other models; some providers ignore it`}
                htmlFor="reasoning_effort"
              >
                <Form.Item name="reasoning_effort">
                  <Select
                    options={[
                      { value: '', label: t`Default` },
                      { value: 'none', label: t`None` },
                      { value: 'minimal', label: t`Minimal` },
                      { value: 'low', label: t`Low` },
                      { value: 'medium', label: t`Medium` },
                      { value: 'high', label: t`High` },
                      { value: 'xhigh', label: t`Extra high` }
                    ]}
                  />
                </Form.Item>
              </SettingsRow>
            </>
          )}

          {providerKind === 'gemini' && (
            <SettingsRow
              label={<RequiredLabel>{t`Model`}</RequiredLabel>}
              help={t`Pick the suggestion, or type any Gemini model ID`}
              htmlFor="model"
            >
              <Form.Item name="model" rules={[{ required: true, message: t`Please select or enter a model` }]}>
                <AutoComplete
                  options={geminiModels}
                  placeholder={t`gemini-4-argon or a custom model ID`}
                  filterOption={filterByValue}
                />
              </Form.Item>
            </SettingsRow>
          )}
        </SettingsRows>
      </Panel>
    </Form>
  )
}
