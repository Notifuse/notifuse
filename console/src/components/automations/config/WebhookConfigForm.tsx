import React from 'react'
import { Form, Input } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { WebhookNodeConfig } from '../../../services/api/automation'

interface WebhookConfigFormProps {
  config: WebhookNodeConfig
  onChange: (config: WebhookNodeConfig) => void
}

export const WebhookConfigForm: React.FC<WebhookConfigFormProps> = ({ config, onChange }) => {
  const { t } = useLingui()

  const handleUrlChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onChange({ ...config, url: e.target.value })
  }

  const handleSecretChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    onChange({ ...config, secret: value || undefined })
  }

  const isValidUrl = (url: string) => {
    if (!url) return true // Empty is valid (just not configured)
    return url.startsWith('http://') || url.startsWith('https://')
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label={t`Webhook URL`}
        required
        validateStatus={config.url && !isValidUrl(config.url) ? 'error' : undefined}
        help={
          config.url && !isValidUrl(config.url) ? t`URL must start with http:// or https://` : undefined
        }
        extra={t`The URL to send the POST request to`}
      >
        <Input
          value={config.url || ''}
          onChange={handleUrlChange}
          placeholder="https://api.example.com/webhook"
        />
      </Form.Item>

      <Form.Item
        label={t`Authorization Secret`}
        extra={t`Optional. If provided, sent as Authorization: Bearer <secret>`}
      >
        <Input.Password
          value={config.secret || ''}
          onChange={handleSecretChange}
          placeholder={t`Optional bearer token`}
        />
      </Form.Item>
    </Form>
  )
}
