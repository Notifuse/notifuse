import { useEffect, useState } from 'react'
import { Button, Form, App, Switch, Descriptions } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { Section } from './Section'
import { SEOSettingsForm } from '../seo/SEOSettingsForm'

interface WebPublicationSettingsProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  isOwner: boolean
}

export function WebPublicationSettings({
  workspace,
  onWorkspaceUpdate,
  isOwner
}: WebPublicationSettingsProps) {
  const [savingSettings, setSavingSettings] = useState(false)
  const [formTouched, setFormTouched] = useState(false)
  const [form] = Form.useForm()
  const { message } = App.useApp()

  useEffect(() => {
    // Set form values from workspace data whenever workspace changes
    form.setFieldsValue({
      web_publications_enabled: workspace?.settings.web_publications_enabled || false,
      web_publication_settings: {
        meta_title: workspace?.settings.web_publication_settings?.meta_title || '',
        meta_description: workspace?.settings.web_publication_settings?.meta_description || '',
        og_title: workspace?.settings.web_publication_settings?.og_title || '',
        og_description: workspace?.settings.web_publication_settings?.og_description || '',
        og_image: workspace?.settings.web_publication_settings?.og_image || '',
        keywords: workspace?.settings.web_publication_settings?.keywords || []
      }
    })
    setFormTouched(false)
  }, [workspace, form])

  const handleSaveSettings = async (values: any) => {
    if (!workspace) return

    setSavingSettings(true)
    try {
      await workspaceService.update({
        ...workspace,
        settings: {
          ...workspace.settings,
          web_publications_enabled: values.web_publications_enabled || false,
          web_publication_settings: values.web_publication_settings || null
        }
      })

      // Refresh the workspace data
      const response = await workspaceService.get(workspace.id)

      // Update the parent component with the new workspace data
      onWorkspaceUpdate(response.workspace)

      setFormTouched(false)
      message.success('Web publication settings updated successfully')
    } catch (error: any) {
      console.error('Failed to update web publication settings', error)
      // Extract the actual error message from the API response
      const errorMessage = error?.message || 'Failed to update web publication settings'
      message.error(errorMessage)
    } finally {
      setSavingSettings(false)
    }
  }

  const handleFormChange = () => {
    setFormTouched(true)
  }

  if (!isOwner) {
    return (
      <Section
        title="Web Publication Settings"
        description="Default SEO settings for web publications"
      >
        <Descriptions
          bordered
          column={1}
          size="small"
          labelStyle={{ width: '200px', fontWeight: '500' }}
        >
          <Descriptions.Item label="Web Publications">
            {workspace?.settings.web_publications_enabled ? (
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

          {workspace?.settings.web_publications_enabled && (
            <>
              <Descriptions.Item label="Default Meta Title">
                {workspace?.settings.web_publication_settings?.meta_title || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="Default Meta Description">
                {workspace?.settings.web_publication_settings?.meta_description || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="Default OG Title">
                {workspace?.settings.web_publication_settings?.og_title || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="Default OG Description">
                {workspace?.settings.web_publication_settings?.og_description || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="Default OG Image">
                {workspace?.settings.web_publication_settings?.og_image ? (
                  <img
                    src={workspace.settings.web_publication_settings.og_image}
                    alt="OG preview"
                    style={{ height: '48px', width: 'auto', objectFit: 'contain' }}
                  />
                ) : (
                  'Not set'
                )}
              </Descriptions.Item>

              <Descriptions.Item label="Default Keywords">
                {workspace?.settings.web_publication_settings?.keywords?.join(', ') || 'Not set'}
              </Descriptions.Item>
            </>
          )}
        </Descriptions>
      </Section>
    )
  }

  return (
    <Section
      title="Web Publication Settings"
      description="Configure default SEO settings for web publications. These will be used as defaults for blog posts published to the web."
    >
      <Form
        form={form}
        layout="vertical"
        onFinish={handleSaveSettings}
        onValuesChange={handleFormChange}
      >
        {!workspace?.settings.custom_endpoint_url && (
          <div
            style={{
              marginBottom: 16,
              padding: '12px 16px',
              background: '#fff7e6',
              border: '1px solid #ffd591',
              borderRadius: '4px'
            }}
          >
            ⚠️ You must configure a Custom Endpoint URL in General Settings above before enabling
            web publications.
          </div>
        )}

        <Form.Item
          name="web_publications_enabled"
          label="Enable Web Publications"
          tooltip="Allow broadcasting content to a public blog on your custom domain"
        >
          <Switch disabled={!workspace?.settings.custom_endpoint_url} />
        </Form.Item>

        <Form.Item
          noStyle
          shouldUpdate={(prevValues, currentValues) =>
            prevValues.web_publications_enabled !== currentValues.web_publications_enabled
          }
        >
          {({ getFieldValue }) => {
            const webEnabled = getFieldValue('web_publications_enabled')
            const customEndpoint = workspace?.settings.custom_endpoint_url

            if (!webEnabled || !customEndpoint) {
              return null
            }

            return (
              <>
                <div style={{ marginBottom: 16, color: '#666' }}>
                  These settings will be used as defaults for all web publications. Individual
                  posts can override these values.
                </div>

                <SEOSettingsForm
                  namePrefix={['web_publication_settings']}
                  showCanonical={false}
                  titlePlaceholder="My Amazing Blog"
                  descriptionPlaceholder="Welcome to my blog where I share insights about..."
                />
              </>
            )
          }}
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
  )
}

