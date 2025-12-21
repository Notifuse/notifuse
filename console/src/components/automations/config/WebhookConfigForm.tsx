import React from 'react'
import { Form, Input } from 'antd'
import type { WebhookNodeConfig } from '../../../services/api/automation'

interface WebhookConfigFormProps {
  config: WebhookNodeConfig
  onChange: (config: WebhookNodeConfig) => void
}

export const WebhookConfigForm: React.FC<WebhookConfigFormProps> = ({ config, onChange }) => {
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
        label="Webhook URL"
        required
        validateStatus={config.url && !isValidUrl(config.url) ? 'error' : undefined}
        help={
          config.url && !isValidUrl(config.url) ? 'URL must start with http:// or https://' : undefined
        }
        extra="The URL to send the POST request to"
      >
        <Input
          value={config.url || ''}
          onChange={handleUrlChange}
          placeholder="https://api.example.com/webhook"
        />
      </Form.Item>

      <Form.Item
        label="Authorization Secret"
        extra="Optional. If provided, sent as Authorization: Bearer <secret>"
      >
        <Input.Password
          value={config.secret || ''}
          onChange={handleSecretChange}
          placeholder="Optional bearer token"
        />
      </Form.Item>
    </Form>
  )
}
