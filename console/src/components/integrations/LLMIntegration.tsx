import React, { useEffect } from 'react'
import { AutoComplete, Form, Input, message, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Integration, LLMProviderKind, Workspace } from '../../services/api/types'
import { llmProviders } from './LLMProviders'

// Available Anthropic models with pricing (input/output per million tokens)
// Model IDs from: https://docs.anthropic.com/en/docs/about-claude/models/overview
const anthropicModels = [
  { value: 'claude-opus-4-6', label: 'Claude Opus 4.6 ($5/$25 per MTok)' },
  { value: 'claude-sonnet-4-6', label: 'Claude Sonnet 4.6 ($3/$15 per MTok)' },
  { value: 'claude-haiku-4-5-20251001', label: 'Claude Haiku 4.5 ($1/$5 per MTok)' }
]

// Suggested Gemini models with pricing (input/output per million tokens).
// Free text is allowed, so any current/future model ID can be entered.
// Pricing from: https://ai.google.dev/gemini-api/docs/pricing
const geminiModels = [
  { value: 'gemini-3.1-pro-preview', label: 'gemini-3.1-pro-preview ($2/$12 per MTok)' }
]

interface LLMIntegrationProps {
  integration?: Integration
  workspace: Workspace
  providerKind: LLMProviderKind
  onSave: (integration: Integration) => Promise<void>
  isOwner: boolean
  formRef?: React.RefObject<{ submit: () => void } | null>
}

export const LLMIntegration: React.FC<LLMIntegrationProps> = ({
  integration,
  providerKind,
  onSave,
  isOwner,
  formRef
}) => {
  const { t } = useLingui()
  const [form] = Form.useForm()

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
          model: provider.model || providerInfo?.defaultModel || 'gpt-4.1',
          base_url: provider.base_url || '',
          // Always read it back so handleSave can resend it (UpdateIntegration
          // overwrites the whole settings object; an omitted field would be wiped).
          reasoning_effort: provider.reasoning_effort || ''
        })
      } else {
        form.setFieldsValue({
          name: providerInfo?.name || 'OpenAI',
          model: providerInfo?.defaultModel || 'gpt-4.1',
          base_url: '',
          reasoning_effort: ''
        })
      }
    } else if (providerKind === 'gemini') {
      if (integration?.llm_provider?.gemini) {
        const provider = integration.llm_provider.gemini
        form.setFieldsValue({
          name: integration.name,
          model: provider.model || providerInfo?.defaultModel || 'gemini-3.1-pro-preview'
        })
      } else {
        form.setFieldsValue({
          name: providerInfo?.name || 'Google Gemini',
          model: providerInfo?.defaultModel || 'gemini-3.1-pro-preview'
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
          model: providerInfo?.defaultModel || 'claude-sonnet-4-6'
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
              model: isString(values.model) ? values.model : 'gpt-4o',
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
                : providerInfo?.defaultModel || 'gemini-3.1-pro-preview'
            }
          })
        },
        created_at: integration?.created_at || new Date().toISOString(),
        updated_at: new Date().toISOString()
      }

      await onSave(integrationData)
    } catch (error) {
      console.error('Failed to save LLM integration:', error)
      message.error(t`Failed to save integration`)
    }
  }

  return (
    <Form form={form} layout="vertical" onFinish={handleSave} disabled={!isOwner}>
      <Form.Item
        label={t`Integration Name`}
        name="name"
        rules={[{ required: true, message: t`Please enter integration name` }]}
      >
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

      <Form.Item
        label={t`API Key`}
        name="api_key"
        extra={integration ? t`Leave blank to keep the existing API key` : undefined}
        rules={
          integration ? [] : [{ required: true, message: t`Please enter your API key` }]
        }
      >
        <Input.Password
          placeholder={
            providerKind === 'openai'
              ? 'sk-proj-...'
              : providerKind === 'gemini'
                ? 'AIza...'
                : 'sk-ant-api03-...'
          }
        />
      </Form.Item>

      {providerKind === 'anthropic' && (
        <Form.Item
          label={t`Model`}
          name="model"
          rules={[{ required: true, message: t`Please select a model` }]}
        >
          <Select placeholder={t`Select a model`} options={anthropicModels} />
        </Form.Item>
      )}

      {providerKind === 'openai' && (
        <>
          <Form.Item
            label={t`Model`}
            name="model"
            rules={[{ required: true, message: t`Please enter a model name` }]}
          >
            <Input placeholder={t`gpt-4.1, gpt-5.4, o4-mini, or custom model name`} />
          </Form.Item>

          <Form.Item
            label={t`Base URL`}
            name="base_url"
            extra={t`Optional. For OpenAI-compatible providers (Ollama, vLLM, LiteLLM, Azure, etc.). Leave empty for the default OpenAI endpoint.`}
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>

          <Form.Item
            label={t`Reasoning effort`}
            name="reasoning_effort"
            extra={t`Optional. Controls how much reasoning models think. Leave as Default for non-reasoning models; some providers ignore it.`}
          >
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
        </>
      )}

      {providerKind === 'gemini' && (
        <Form.Item
          label={t`Model`}
          name="model"
          rules={[{ required: true, message: t`Please select or enter a model` }]}
        >
          <AutoComplete
            options={geminiModels}
            placeholder={t`gemini-3.1-pro-preview or a custom model ID`}
            filterOption={(inputValue, option) =>
              ((option?.value as string) ?? '').toLowerCase().includes(inputValue.toLowerCase())
            }
          />
        </Form.Item>
      )}
    </Form>
  )
}
