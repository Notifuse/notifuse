import React, { useEffect } from 'react'
import { Form, Input, Alert, message } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Integration, Workspace } from '../../services/api/types'

interface FirecrawlIntegrationProps {
  integration?: Integration
  workspace: Workspace
  onSave: (integration: Integration) => Promise<void>
  isOwner: boolean
  formRef?: React.RefObject<{ submit: () => void } | null>
}

export const FirecrawlIntegration: React.FC<FirecrawlIntegrationProps> = ({
  integration,
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

  useEffect(() => {
    if (integration?.firecrawl_settings) {
      form.setFieldsValue({
        name: integration.name,
        base_url: integration.firecrawl_settings.base_url || ''
      })
    } else {
      form.setFieldsValue({
        name: 'Firecrawl',
        base_url: ''
      })
    }
  }, [integration, form])

  const handleSave = async (values: Record<string, unknown>) => {
    if (!isOwner) {
      message.error(t`Only workspace owners can modify integrations`)
      return
    }

    try {
      const isString = (value: unknown): value is string => typeof value === 'string'

      const integrationData: Integration = {
        id: integration?.id || `int_${Date.now()}`,
        name: isString(values.name) ? values.name : 'Firecrawl',
        type: 'firecrawl',
        firecrawl_settings: {
          api_key: isString(values.api_key) && values.api_key !== '' ? values.api_key : undefined,
          base_url: isString(values.base_url) && values.base_url !== '' ? values.base_url : undefined
        },
        created_at: integration?.created_at || new Date().toISOString(),
        updated_at: new Date().toISOString()
      }

      await onSave(integrationData)
    } catch (error) {
      console.error('Failed to save Firecrawl integration:', error)
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
        <Input placeholder={t`e.g., My Firecrawl Integration`} />
      </Form.Item>

      <Form.Item
        label={t`API Key`}
        name="api_key"
        extra={integration ? t`Leave blank to keep the existing API key` : undefined}
        rules={integration ? [] : [{ required: true, message: t`Please enter your API key` }]}
      >
        <Input.Password placeholder="fc-..." />
      </Form.Item>

      <Form.Item
        label={t`Base URL (Optional)`}
        name="base_url"
        extra={t`Leave blank to use the default Firecrawl API (api.firecrawl.dev)`}
      >
        <Input placeholder="https://api.firecrawl.dev" />
      </Form.Item>

      <Alert
        message={t`Available Tools`}
        description={
          <ul className="list-disc pl-4 mt-2">
            <li>
              <strong>scrape_url</strong> - {t`Scrapes a URL and returns its content as markdown`}
            </li>
            <li>
              <strong>search_web</strong> - {t`Searches the web and returns relevant URLs`}
            </li>
          </ul>
        }
        type="info"
        showIcon
        className="mt-4"
      />
    </Form>
  )
}
