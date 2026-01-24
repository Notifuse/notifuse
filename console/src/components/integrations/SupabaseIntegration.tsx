import React, { useState, useEffect } from 'react'
import { Form, Input, Select, Space, Divider, Alert, message, Tooltip, Switch } from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { Integration, SupabaseIntegrationSettings, Workspace } from '../../services/api/types'
import { listsApi, List } from '../../services/api/list'

interface SupabaseIntegrationProps {
  integration?: Integration
  workspace: Workspace
  onSave: (integration: Integration) => Promise<void>
  isOwner: boolean
  formRef?: React.RefObject<{ submit: () => void } | null>
}

export const SupabaseIntegration: React.FC<SupabaseIntegrationProps> = ({
  integration,
  workspace,
  onSave,
  isOwner,
  formRef
}) => {
  const { t } = useLingui()
  const [form] = Form.useForm()

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
  }, [workspace.id, t])

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

  return (
    <Form form={form} layout="vertical" onFinish={handleSave} disabled={!isOwner}>
      <Form.Item
        label={t`Integration Name`}
        name="name"
        rules={[{ required: true, message: t`Please enter integration name` }]}
      >
        <Input placeholder={t`e.g., My Supabase Integration`} />
      </Form.Item>

      <div className="mt-12">
        <Divider orientation="center" plain>
          {t`Auth Email Hook`}
        </Divider>
      </div>

      <Form.Item
        label={
          <Space>
            <span>{t`Auth Email Hook Secret`}</span>
            <Tooltip title={t`Generate this key in Supabase Auth Hooks settings`}>
              <InfoCircleOutlined />
            </Tooltip>
          </Space>
        }
        name="auth_email_signature_key"
      >
        <Input.Password placeholder="v1,whsec_..." />
      </Form.Item>

      <div className="mt-12">
        <Divider orientation="center" plain>
          {t`User Created Hook`}
        </Divider>
      </div>

      <Form.Item
        label={
          <Space>
            <span>{t`User Created Hook Secret`}</span>
            <Tooltip title={t`Generate this key in Supabase Auth Hooks settings`}>
              <InfoCircleOutlined />
            </Tooltip>
          </Space>
        }
        name="user_created_signature_key"
      >
        <Input.Password placeholder="v1,whsec_..." />
      </Form.Item>

      <Form.Item
        label={
          <Space>
            <span>{t`Subscribe users to these lists (Optional)`}</span>
            <Tooltip title={t`Automatically add new users to the selected lists`}>
              <InfoCircleOutlined />
            </Tooltip>
          </Space>
        }
        name="add_user_created_to_lists"
      >
        <Select
          placeholder={t`Select lists (optional)`}
          mode="multiple"
          allowClear
          showSearch
          optionFilterProp="children"
          loading={loading}
        >
          {lists.map((list) => (
            <Select.Option key={list.id} value={list.id}>
              {list.name}
            </Select.Option>
          ))}
        </Select>
      </Form.Item>

      <Form.Item
        label={
          <Space>
            <span>{t`Save user metadata to this custom JSON field (Optional)`}</span>
            <Tooltip title={t`The user_metadata field in Supabase will be saved to this custom JSON field`}>
              <InfoCircleOutlined />
            </Tooltip>
          </Space>
        }
        name="user_created_custom_json_field"
      >
        <Select placeholder={t`Select custom JSON field (optional)`} allowClear>
          {[1, 2, 3, 4, 5].map((num) => {
            const fieldName = `custom_json_${num}`
            const friendlyName =
              workspace.settings?.custom_field_labels?.[fieldName] || t`Custom JSON ${num}`
            return (
              <Select.Option key={fieldName} value={fieldName}>
                {friendlyName}
              </Select.Option>
            )
          })}
        </Select>
      </Form.Item>

      <Form.Item
        label={
          <Space>
            <span>{t`Reject Disposable Email Addresses`}</span>
            <Tooltip title={t`When enabled, user creation will be rejected if a disposable email address is detected`}>
              <InfoCircleOutlined />
            </Tooltip>
          </Space>
        }
        name="reject_disposable_email"
        valuePropName="checked"
      >
        <Switch />
      </Form.Item>

      <Alert
        description={t`This hook never blocks Supabase user creation for errors. However, if 'Reject Disposable Email Addresses' is enabled and a disposable email is detected, the user signup will be rejected by Supabase.`}
        type="info"
      />
    </Form>
  )
}
