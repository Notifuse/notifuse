import React, { useEffect } from 'react'
import { App, Form, Input } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Integration, Workspace } from '../../services/api/types'
import { Mono, Panel } from '../layout'
import { SettingsRow, SettingsRows } from '../settings/SettingsRow'
import { useSecretPlaceholder } from '../settings/integrations/secrets'
import { OptionalLabel, RequiredLabel } from '../settings/integrations/FieldLabel'

interface FirecrawlIntegrationProps {
  integration?: Integration
  workspace: Workspace
  onSave: (integration: Integration) => Promise<void>
  isOwner: boolean
  formRef?: React.RefObject<{ submit: () => void } | null>
}

/** The rows of a Firecrawl integration: its key, its endpoint and the tools it gives the assistant. */
export const FirecrawlIntegration: React.FC<FirecrawlIntegrationProps> = ({
  integration,
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
      // The page reports a failed save itself (saveOtherIntegration), and a field that does not
      // validate shows under the field: a toast here would be the second one.
      console.error('Failed to save Firecrawl integration:', error)
    }
  }

  const keyStored = Boolean(
    integration?.credential_hints?.['firecrawl.api_key'] || integration?.firecrawl_settings?.encrypted_api_key
  )

  return (
    <Form form={form} layout="vertical" component={false} onFinish={handleSave} disabled={!isOwner}>
      <Panel title={t`Integration`}>
        <SettingsRows>
          <SettingsRow label={<RequiredLabel>{t`Name`}</RequiredLabel>} help={t`On the card`} htmlFor="name">
            <Form.Item name="name" rules={[{ required: true, message: t`Please enter integration name` }]}>
              <Input placeholder={t`e.g., My Firecrawl Integration`} />
            </Form.Item>
          </SettingsRow>
        </SettingsRows>
      </Panel>

      <Panel title={t`Account`}>
        <SettingsRows>
          <SettingsRow
            label={integration ? t`API key` : <RequiredLabel>{t`API key`}</RequiredLabel>}
            help={t`From firecrawl.dev, or your own instance`}
            htmlFor="api_key"
          >
            <Form.Item
              name="api_key"
              extra={integration && keyStored ? t`Leave blank to keep the existing API key` : undefined}
              rules={integration ? [] : [{ required: true, message: t`Please enter your API key` }]}
            >
              <Input.Password autoComplete="new-password" placeholder={secretPlaceholder('firecrawl.api_key', keyStored, 'fc-...')} />
            </Form.Item>
          </SettingsRow>
          <SettingsRow
            label={<OptionalLabel>{t`Base URL`}</OptionalLabel>}
            help={t`Leave blank to use the default Firecrawl API (api.firecrawl.dev)`}
            htmlFor="base_url"
          >
            <Form.Item name="base_url">
              <Input placeholder="https://api.firecrawl.dev" />
            </Form.Item>
          </SettingsRow>
        </SettingsRows>
      </Panel>

      <Panel
        title={t`Tools`}
        extra={<span className="text-xs text-ink-3">{t`Added to every AI assistant request`}</span>}
      >
        <SettingsRows>
          <SettingsRow label={<Mono>scrape_url</Mono>}>
            <span className="text-sm text-ink-2">{t`Scrapes a URL and returns its content as markdown`}</span>
          </SettingsRow>
          <SettingsRow label={<Mono>search_web</Mono>}>
            <span className="text-sm text-ink-2">{t`Searches the web and returns relevant URLs`}</span>
          </SettingsRow>
        </SettingsRows>
      </Panel>
    </Form>
  )
}
