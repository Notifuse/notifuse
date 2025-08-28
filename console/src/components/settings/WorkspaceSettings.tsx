import { useEffect, useState } from 'react'
import { Button, Form, Input, Select, App, Switch } from 'antd'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { Section } from './Section'
import { TimezonesFormOptions } from '../utils/countries_timezones'
import { LogoInput } from './LogoInput'

interface WorkspaceSettingsProps {
  workspace: Workspace | null
  loading: boolean
  onWorkspaceUpdate: (workspace: Workspace) => void
  onWorkspaceDelete?: () => void
  isOwner: boolean
}

export function WorkspaceSettings({ workspace, onWorkspaceUpdate }: WorkspaceSettingsProps) {
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

  return (
    <>
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
            <Select options={TimezonesFormOptions} showSearch optionFilterProp="label" />
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
                  To use a custom domain, create a CNAME record pointing your domain to the API
                  endpoint and ensure SSL certificates are configured.
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
                  <strong>DNS Record:</strong>
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
                      return 'api.yourdomain.com'
                    } catch {
                      return 'api.yourdomain.com'
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
