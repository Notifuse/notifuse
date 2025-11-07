import { useEffect, useState } from 'react'
import { Button, Form, Input, Select, App, Switch, Descriptions } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { Section } from './Section'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'
import { LogoInput } from './LogoInput'

interface WorkspaceSettingsProps {
  workspace: Workspace | null
  loading: boolean
  onWorkspaceUpdate: (workspace: Workspace) => void
  onWorkspaceDelete?: () => void
  isOwner: boolean
}

export function WorkspaceSettings({
  workspace,
  onWorkspaceUpdate,
  isOwner
}: WorkspaceSettingsProps) {
  const [savingSettings, setSavingSettings] = useState(false)
  const [formTouched, setFormTouched] = useState(false)
  const [form] = Form.useForm()
  const { message } = App.useApp()

  useEffect(() => {
    // Set form values from workspace data whenever workspace changes
    form.setFieldsValue({
      name: workspace?.name || '',
      website_url: workspace?.settings.website_url || '',
      logo_url: workspace?.settings.logo_url || '',
      timezone: workspace?.settings.timezone || 'UTC',
      email_tracking_enabled: workspace?.settings.email_tracking_enabled || false,
      custom_endpoint_url: workspace?.settings.custom_endpoint_url || ''
    })
    setFormTouched(false)
  }, [workspace, form])

  const handleSaveSettings = async (values: any) => {
    if (!workspace) return

    setSavingSettings(true)
    try {
      await workspaceService.update({
        ...workspace,
        name: values.name,
        settings: {
          ...workspace.settings,
          website_url: values.website_url,
          logo_url: values.logo_url || null,
          cover_url: workspace?.settings.cover_url || null,
          timezone: values.timezone,
          email_tracking_enabled: values.email_tracking_enabled,
          custom_endpoint_url: values.custom_endpoint_url || null
        }
      })

      // Refresh the workspace data
      const response = await workspaceService.get(workspace.id)

      // Update the parent component with the new workspace data
      onWorkspaceUpdate(response.workspace)

      setFormTouched(false)
      message.success('Workspace settings updated successfully')
    } catch (error) {
      console.error('Failed to update workspace settings', error)
      message.error('Failed to update workspace settings')
    } finally {
      setSavingSettings(false)
    }
  }

  const handleFormChange = () => {
    setFormTouched(true)
  }

  if (!isOwner) {
    // Render read-only settings for non-owner users
    return (
      <>
        <Section
          title="Transactional SMTP Relay"
          description="SMTP relay server for forwarding transactional emails"
        >
          {window.SMTP_RELAY_ENABLED ? (
            <>
              <div style={{ marginBottom: '16px' }}>
                <a
                  href="https://docs.notifuse.com/concepts/transactional-api#smtp-relay"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  View SMTP Relay documentation and setup guide
                </a>
              </div>
              <Descriptions
                bordered
                column={1}
                size="small"
                labelStyle={{ width: '200px', fontWeight: '500' }}
              >
                <Descriptions.Item label="Domain">
                  {window.SMTP_RELAY_DOMAIN || 'Not set'}
                </Descriptions.Item>

                <Descriptions.Item label="Port">
                  {window.SMTP_RELAY_PORT || 'Not set'}
                </Descriptions.Item>

                <Descriptions.Item label="TLS">
                  {window.SMTP_RELAY_TLS_ENABLED ? (
                    <span style={{ color: '#52c41a' }}>
                      <CheckCircleOutlined style={{ marginRight: '8px' }} />
                      Enabled
                    </span>
                  ) : (
                    <span style={{ color: '#ff4d4f' }}>
                      <CloseCircleOutlined style={{ marginRight: '8px' }} />
                      Disabled
                    </span>
                  )}
                </Descriptions.Item>
              </Descriptions>
            </>
          ) : (
            <div style={{ color: '#8c8c8c', fontStyle: 'italic' }}>
              SMTP relay is not configured.{' '}
              <a
                href="https://docs.notifuse.com/installation#smtp-relay-configuration"
                target="_blank"
                rel="noopener noreferrer"
              >
                Learn how to enable SMTP relay
              </a>
            </div>
          )}
        </Section>

        <Section title="General Settings" description="General settings for your workspace">
          <Descriptions
            bordered
            column={1}
            size="small"
            labelStyle={{ width: '200px', fontWeight: '500' }}
          >
            <Descriptions.Item label="Workspace Name">
              {workspace?.name || 'Not set'}
            </Descriptions.Item>

            <Descriptions.Item label="Website URL">
              {workspace?.settings.website_url || 'Not set'}
            </Descriptions.Item>

            <Descriptions.Item label="Logo">
              {workspace?.settings.logo_url ? (
                <img
                  src={workspace.settings.logo_url}
                  alt="Workspace logo"
                  style={{ height: '24px', width: 'auto', objectFit: 'contain' }}
                />
              ) : (
                'Not set'
              )}
            </Descriptions.Item>

            <Descriptions.Item label="Timezone">
              {workspace?.settings.timezone || 'UTC'}
            </Descriptions.Item>

            <Descriptions.Item label="Email Opens and Clicks Tracking">
              {workspace?.settings.email_tracking_enabled ? (
                <span style={{ color: '#52c41a' }}>
                  <CheckCircleOutlined style={{ marginRight: '8px' }} />
                  Enabled
                </span>
              ) : (
                <span style={{ color: '#ff4d4f' }}>
                  <CloseCircleOutlined style={{ marginRight: '8px' }} />
                  Disabled
                </span>
              )}
            </Descriptions.Item>

            <Descriptions.Item label="Custom Endpoint URL">
              <div>{workspace?.settings.custom_endpoint_url || 'Default (API endpoint)'}</div>
            </Descriptions.Item>
          </Descriptions>
        </Section>
      </>
    )
  }

  return (
    <>
      <Section
        title="Transactional SMTP Relay"
        description="SMTP relay server for forwarding transactional emails"
      >
        {window.SMTP_RELAY_ENABLED ? (
          <>
            <div style={{ marginBottom: '16px' }}>
              <a
                href="https://docs.notifuse.com/concepts/transactional-api#smtp-relay"
                target="_blank"
                rel="noopener noreferrer"
              >
                View SMTP Relay documentation and setup guide
              </a>
            </div>
            <Descriptions
              bordered
              column={1}
              size="small"
              labelStyle={{ width: '200px', fontWeight: '500' }}
            >
              <Descriptions.Item label="SMTP domain">
                {window.SMTP_RELAY_DOMAIN || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="SMTP port">
                {window.SMTP_RELAY_PORT || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="TLS">
                {window.SMTP_RELAY_TLS_ENABLED ? (
                  <span style={{ color: '#52c41a' }}>
                    <CheckCircleOutlined style={{ marginRight: '8px' }} />
                    Enabled
                  </span>
                ) : (
                  <span style={{ color: '#ff4d4f' }}>
                    <CloseCircleOutlined style={{ marginRight: '8px' }} />
                    Disabled
                  </span>
                )}
              </Descriptions.Item>
            </Descriptions>
          </>
        ) : (
          <div style={{ color: '#8c8c8c', fontStyle: 'italic' }}>
            SMTP relay is not configured.{' '}
            <a
              href="https://docs.notifuse.com/installation#smtp-relay-configuration"
              target="_blank"
              rel="noopener noreferrer"
            >
              Learn how to enable SMTP relay
            </a>
          </div>
        )}
      </Section>

      <Section title="General Settings" description="General settings for your workspace">
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSaveSettings}
          onValuesChange={handleFormChange}
        >
          <Form.Item
            name="name"
            label="Workspace Name"
            rules={[{ required: true, message: 'Please enter workspace name' }]}
          >
            <Input placeholder="Enter workspace name" />
          </Form.Item>

          <Form.Item
            name="website_url"
            label="Website URL"
            rules={[{ type: 'url', message: 'Please enter a valid URL' }]}
          >
            <Input placeholder="https://example.com" />
          </Form.Item>

          <LogoInput />

          <Form.Item
            name="timezone"
            label="Timezone"
            rules={[{ required: true, message: 'Please select a timezone' }]}
          >
            <Select options={TIMEZONE_OPTIONS} showSearch optionFilterProp="label" />
          </Form.Item>

          <Form.Item
            name="email_tracking_enabled"
            label="Email Opens and Clicks Tracking"
            tooltip="When enabled, links in the email will be tracked for opens and clicks"
          >
            <Switch />
          </Form.Item>

          <Form.Item
            name="custom_endpoint_url"
            label="Custom Endpoint URL"
            tooltip="Custom domain for email links (unsubscribe, tracking, notification center). By default, the config API endpoint is used. Leave empty to use the default."
            rules={[{ type: 'url', message: 'Please enter a valid URL' }]}
            help={
              <div className="mb-4">
                <div>
                  Configure a custom domain for email links, notification center, and web publications. 
                  DNS verification will be performed before saving to ensure you control this domain.
                </div>
                <div
                  style={{
                    marginTop: 8,
                    fontFamily: 'monospace',
                    fontSize: '12px',
                    background: '#f5f5f5',
                    padding: '4px 8px',
                    borderRadius: '4px'
                  }}
                >
                  <strong>DNS Record Required:</strong>
                  <br />
                  Type: CNAME
                  <br />
                  Name:{' '}
                  {(() => {
                    try {
                      const customUrl = form.getFieldValue('custom_endpoint_url')
                      if (customUrl) {
                        return new URL(customUrl).hostname
                      }
                      return 'blog.yourdomain.com'
                    } catch {
                      return 'blog.yourdomain.com'
                    }
                  })()}
                  <br />
                  Value:{' '}
                  {(() => {
                    try {
                      const apiEndpoint = window.API_ENDPOINT || 'http://localhost:3000'
                      return new URL(apiEndpoint).hostname
                    } catch {
                      return 'your-api-endpoint.com'
                    }
                  })()}
                  <br />
                  <span style={{ color: '#999', fontSize: '11px' }}>
                    DNS verification prevents domain squatting
                  </span>
                </div>
              </div>
            }
          >
            <Input placeholder="https://api.yourdomain.com" />
          </Form.Item>

          <Form.Item>
            <Button
              type="primary"
              htmlType="submit"
              loading={savingSettings}
              disabled={!formTouched}
            >
              Save Changes
            </Button>
          </Form.Item>
        </Form>
      </Section>
    </>
  )
}
