import React from 'react'
import { Form, Input } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { WebhookNodeConfig } from '../../../services/api/automation'
import { refusedWebhookHost } from '../utils/webhookTarget'

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

  // The scheme message comes first: an address is only worth judging once the URL
  // is a web URL at all.
  const host = config.url ? refusedWebhookHost(config.url) : null
  let urlError: string | undefined
  if (config.url && !isValidUrl(config.url)) {
    urlError = t`URL must start with http:// or https://`
  } else if (host) {
    urlError = t`The webhook URL points at a private or internal address (${host}), which Notifuse does not connect to. Use a public URL, or set WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS=true on a self-hosted install that delivers to its own network.`
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label={t`Webhook URL`}
        required
        validateStatus={urlError ? 'error' : undefined}
        help={urlError}
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
