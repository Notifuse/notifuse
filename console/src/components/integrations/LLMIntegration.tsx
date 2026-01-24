import React, { useEffect } from 'react'
import { Form, Input, message, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Integration, LLMProviderKind, Workspace } from '../../services/api/types'
import { llmProviders } from './LLMProviders'

// Available Anthropic models with pricing (input/output per million tokens)
// Model IDs from: https://docs.anthropic.com/en/docs/about-claude/models/overview
const anthropicModels = [
  { value: 'claude-opus-4-5-20251101', label: 'Claude Opus 4.5 ($5/$25 per MTok)' },
  { value: 'claude-sonnet-4-5-20250929', label: 'Claude Sonnet 4.5 ($3/$15 per MTok)' },
  { value: 'claude-haiku-4-5-20251001', label: 'Claude Haiku 4.5 ($1/$5 per MTok)' }
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
    if (integration?.llm_provider) {
      const provider = integration.llm_provider
      form.setFieldsValue({
        name: integration.name,
        model: provider.anthropic?.model || providerInfo?.defaultModel || ''
      })
    } else {
      // Default values for new integration
      form.setFieldsValue({
        name: providerInfo?.name || 'Anthropic',
        model: providerInfo?.defaultModel || 'claude-sonnet-4-5-20250929'
      })
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
        name: isString(values.name) ? values.name : providerInfo?.name || 'Anthropic',
        type: 'llm',
        llm_provider: {
          kind: providerKind,
          anthropic: {
            api_key: isString(values.api_key) && values.api_key !== '' ? values.api_key : undefined,
            model: isString(values.model) ? values.model : providerInfo?.defaultModel || ''
          }
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
        <Input placeholder={t`e.g., My Anthropic Integration`} />
      </Form.Item>

      <Form.Item
        label={t`API Key`}
        name="api_key"
        extra={integration ? t`Leave blank to keep the existing API key` : undefined}
        rules={
          integration
            ? []
            : [{ required: true, message: t`Please enter your API key` }]
        }
      >
        <Input.Password placeholder="sk-ant-api03-..." />
      </Form.Item>

      <Form.Item
        label={t`Model`}
        name="model"
        rules={[{ required: true, message: t`Please select a model` }]}
      >
        <Select placeholder={t`Select a model`} options={anthropicModels} />
      </Form.Item>
    </Form>
  )
}
