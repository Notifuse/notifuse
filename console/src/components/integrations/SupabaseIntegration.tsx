import React, { useState, useEffect } from 'react'
import { App, Form, Input, Select, Switch } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Integration, SupabaseIntegrationSettings, Workspace } from '../../services/api/types'
import { listsApi, List } from '../../services/api/list'
import { Mono, Panel } from '../layout'
import { SettingsRow, SettingsRows } from '../settings/SettingsRow'
import { useSecretPlaceholder } from '../settings/integrations/secrets'
import { OptionalLabel, RequiredLabel } from '../settings/integrations/FieldLabel'
import { supabaseWebhookURL } from './supabaseWebhookURL'

interface SupabaseIntegrationProps {
  integration?: Integration
  workspace: Workspace
  onSave: (integration: Integration) => Promise<void>
  isOwner: boolean
  formRef?: React.RefObject<{ submit: () => void } | null>
}

/** The rows of a Supabase integration: one panel per Supabase hook, with its address and secret. */
export const SupabaseIntegration: React.FC<SupabaseIntegrationProps> = ({
  integration,
  workspace,
  onSave,
  isOwner,
  formRef
}) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const secretPlaceholder = useSecretPlaceholder(integration)

  // Expose form instance to parent via ref
  React.useEffect(() => {
    if (formRef) {
      ;(formRef as React.MutableRefObject<{ submit: () => void } | null>).current = form
    }
  }, [form, formRef])

  const [lists, setLists] = useState<List[]>([])
  const [loading, setLoading] = useState(true)

  // Fetch lists on mount
  useEffect(() => {
    const fetchData = async () => {
      try {
        const listsResponse = await listsApi.list({ workspace_id: workspace.id })
        setLists(listsResponse.lists || [])
      } catch (error) {
        console.error('Failed to fetch lists:', error)
        message.error(t`Failed to load contact lists`)
        // Ensure we have empty array on error
        setLists([])
      } finally {
        setLoading(false)
      }
    }
    fetchData()
  }, [workspace.id, t, message])

  useEffect(() => {
    if (integration?.supabase_settings) {
      const settings = integration.supabase_settings

      form.setFieldsValue({
        name: integration.name,
        auth_email_signature_key: settings.auth_email_hook?.signature_key || '',
        user_created_signature_key: settings.before_user_created_hook?.signature_key || '',
        add_user_created_to_lists: settings.before_user_created_hook?.add_user_to_lists || [],
        user_created_custom_json_field:
          settings.before_user_created_hook?.custom_json_field || undefined,
        reject_disposable_email: settings.before_user_created_hook?.reject_disposable_email || false
      })
    } else {
      // Default values for new integration
      form.setFieldsValue({
        name: 'Supabase'
      })
    }
  }, [integration, form])

  const handleSave = async (values: Record<string, unknown>) => {
    if (!isOwner) {
      message.error(t`Only workspace owners can modify integrations`)
      return
    }

    try {
      // Type guard helpers
      const isString = (value: unknown): value is string => typeof value === 'string'
      const isStringArray = (value: unknown): value is string[] =>
        Array.isArray(value) && value.every((item) => typeof item === 'string')
      const isBoolean = (value: unknown): value is boolean => typeof value === 'boolean'

      const supabaseSettings: SupabaseIntegrationSettings = {
        auth_email_hook: {
          signature_key: isString(values.auth_email_signature_key)
            ? values.auth_email_signature_key
            : undefined
        },
        before_user_created_hook: {
          signature_key: isString(values.user_created_signature_key)
            ? values.user_created_signature_key
            : undefined,
          add_user_to_lists: isStringArray(values.add_user_created_to_lists)
            ? values.add_user_created_to_lists
            : [],
          custom_json_field: isString(values.user_created_custom_json_field)
            ? values.user_created_custom_json_field
            : undefined,
          reject_disposable_email: isBoolean(values.reject_disposable_email)
            ? values.reject_disposable_email
            : false
        }
      }

      const integrationData: Integration = {
        id: integration?.id || `int_${Date.now()}`,
        name: isString(values.name) ? values.name : 'Supabase',
        type: 'supabase',
        supabase_settings: supabaseSettings,
        created_at: integration?.created_at || new Date().toISOString(),
        updated_at: new Date().toISOString()
      }

      await onSave(integrationData)
      // Success message is shown by parent component (Integrations.tsx)
    } catch (error) {
      console.error('Failed to save Supabase integration:', error)
      message.error(t`Failed to save integration`)
    }
  }

  const withOptional = (label: string) => <OptionalLabel>{label}</OptionalLabel>

  // The address exists once the integration is saved: it carries the integration's id.
  const hookURL = (hook: 'auth-email' | 'before-user-created') =>
    integration ? (
      <Mono copy>{supabaseWebhookURL(hook, workspace.id, integration.id)}</Mono>
    ) : (
      <span className="text-sm text-ink-3">{t`Available after saving`}</span>
    )

  const secretRow = (
    name: string,
    hintKey: string,
    stored: boolean,
    help: string
  ) => (
    <SettingsRow label={withOptional(t`Hook secret`)} help={help} htmlFor={name}>
      <Form.Item
        name={name}
        extra={integration && stored ? t`Blank keeps the current one.` : undefined}
      >
        <Input.Password autoComplete="new-password" placeholder={secretPlaceholder(hintKey, stored, 'v1,whsec_...')} />
      </Form.Item>
    </SettingsRow>
  )

  const settings = integration?.supabase_settings
  const authHookStored = Boolean(
    integration?.credential_hints?.['supabase.auth_email_hook.signature_key'] ||
      settings?.auth_email_hook?.encrypted_signature_key
  )
  const userHookStored = Boolean(
    integration?.credential_hints?.['supabase.before_user_created_hook.signature_key'] ||
      settings?.before_user_created_hook?.encrypted_signature_key
  )

  return (
    <Form form={form} layout="vertical" component={false} onFinish={handleSave} disabled={!isOwner}>
      <Panel title={t`Integration`}>
        <SettingsRows>
          <SettingsRow
            label={<RequiredLabel>{t`Name`}</RequiredLabel>}
            help={t`On the card, and on the notifications it creates`}
            htmlFor="name"
          >
            <Form.Item name="name" rules={[{ required: true, message: t`Please enter integration name` }]}>
              <Input placeholder={t`e.g., My Supabase Integration`} />
            </Form.Item>
          </SettingsRow>
        </SettingsRows>
      </Panel>

      <Panel
        title={t`Send Email hook`}
        extra={<span className="text-xs text-ink-3">{t`Supabase sends its auth emails through Notifuse`}</span>}
      >
        <SettingsRows>
          <SettingsRow label={t`Hook URL`} help={t`Paste it in Supabase › Authentication › Hooks`}>
            <div data-hook-url="auth-email" className="flex min-h-8 items-center">
              {hookURL('auth-email')}
            </div>
          </SettingsRow>
          {secretRow(
            'auth_email_signature_key',
            'supabase.auth_email_hook.signature_key',
            authHookStored,
            t`Generate this key in Supabase Auth Hooks settings`
          )}
        </SettingsRows>
      </Panel>

      <Panel
        title={t`Before User Created hook`}
        extra={<span className="text-xs text-ink-3">{t`New Supabase users become contacts`}</span>}
      >
        <SettingsRows>
          <SettingsRow label={t`Hook URL`} help={t`Paste it in Supabase › Authentication › Hooks`}>
            <div data-hook-url="before-user-created" className="flex min-h-8 items-center">
              {hookURL('before-user-created')}
            </div>
          </SettingsRow>
          {secretRow(
            'user_created_signature_key',
            'supabase.before_user_created_hook.signature_key',
            userHookStored,
            t`Generate this key in Supabase Auth Hooks settings`
          )}
          <SettingsRow
            label={withOptional(t`Subscribe to lists`)}
            help={t`Automatically add new users to the selected lists`}
            htmlFor="add_user_created_to_lists"
          >
            <Form.Item name="add_user_created_to_lists">
              <Select
                placeholder={t`Select lists (optional)`}
                mode="multiple"
                allowClear
                showSearch
                optionFilterProp="label"
                loading={loading}
                options={lists.map((list) => ({ value: list.id, label: list.name }))}
              />
            </Form.Item>
          </SettingsRow>
          <SettingsRow
            label={withOptional(t`Save user metadata to`)}
            help={t`The user_metadata field in Supabase will be saved to this custom JSON field`}
            htmlFor="user_created_custom_json_field"
          >
            <Form.Item name="user_created_custom_json_field">
              <Select
                placeholder={t`Select custom JSON field (optional)`}
                allowClear
                options={[1, 2, 3, 4, 5].map((num) => {
                  const fieldName = `custom_json_${num}`
                  return {
                    value: fieldName,
                    label: workspace.settings?.custom_field_labels?.[fieldName] || t`Custom JSON ${num}`
                  }
                })}
              />
            </Form.Item>
          </SettingsRow>
          <SettingsRow
            label={t`Reject disposable emails`}
            help={t`Supabase refuses the sign-up of a disposable address. Errors never block a sign-up`}
            htmlFor="reject_disposable_email"
          >
            <Form.Item name="reject_disposable_email" valuePropName="checked">
              <Switch />
            </Form.Item>
          </SettingsRow>
        </SettingsRows>
      </Panel>
    </Form>
  )
}
