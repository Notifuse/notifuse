import { useEffect, useState } from 'react'
import { Button, Form, App, Switch, Descriptions, Input, ColorPicker, Divider, Row, Col } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import type { Color } from 'antd/es/color-picker'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { SEOSettingsForm } from '../seo/SEOSettingsForm'
import { SettingsSectionHeader } from './SettingsSectionHeader'

interface BlogSettingsProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  isOwner: boolean
}

export function BlogSettings({
  workspace,
  onWorkspaceUpdate,
  isOwner
}: BlogSettingsProps) {
  const [savingSettings, setSavingSettings] = useState(false)
  const [formTouched, setFormTouched] = useState(false)
  const [form] = Form.useForm()
  const { message } = App.useApp()

  useEffect(() => {
    // Only set form values if user is owner (form exists)
    if (!isOwner) return

    // Set form values from workspace data whenever workspace changes
    form.setFieldsValue({
      blog_enabled: workspace?.settings.blog_enabled || false,
      blog_settings: {
        title: workspace?.settings.blog_settings?.title || '',
        h1_color: workspace?.settings.blog_settings?.h1_color || '',
        h2_color: workspace?.settings.blog_settings?.h2_color || '',
        h3_color: workspace?.settings.blog_settings?.h3_color || '',
        h4_color: workspace?.settings.blog_settings?.h4_color || '',
        font_family: workspace?.settings.blog_settings?.font_family || '',
        font_size: workspace?.settings.blog_settings?.font_size || '',
        text_color: workspace?.settings.blog_settings?.text_color || '',
        seo: {
          meta_title: workspace?.settings.blog_settings?.seo?.meta_title || '',
          meta_description: workspace?.settings.blog_settings?.seo?.meta_description || '',
          og_title: workspace?.settings.blog_settings?.seo?.og_title || '',
          og_description: workspace?.settings.blog_settings?.seo?.og_description || '',
          og_image: workspace?.settings.blog_settings?.seo?.og_image || '',
          keywords: workspace?.settings.blog_settings?.seo?.keywords || []
        }
      }
    })
    setFormTouched(false)
  }, [workspace, form, isOwner])

  const handleSaveSettings = async (values: any) => {
    if (!workspace) return

    setSavingSettings(true)
    try {
      // Convert color picker values to hex strings
      const blogSettings = values.blog_settings ? {
        ...values.blog_settings,
        h1_color: typeof values.blog_settings.h1_color === 'object' 
          ? values.blog_settings.h1_color?.toHexString?.() 
          : values.blog_settings.h1_color,
        h2_color: typeof values.blog_settings.h2_color === 'object' 
          ? values.blog_settings.h2_color?.toHexString?.() 
          : values.blog_settings.h2_color,
        h3_color: typeof values.blog_settings.h3_color === 'object' 
          ? values.blog_settings.h3_color?.toHexString?.() 
          : values.blog_settings.h3_color,
        h4_color: typeof values.blog_settings.h4_color === 'object' 
          ? values.blog_settings.h4_color?.toHexString?.() 
          : values.blog_settings.h4_color,
        text_color: typeof values.blog_settings.text_color === 'object' 
          ? values.blog_settings.text_color?.toHexString?.() 
          : values.blog_settings.text_color,
      } : null

      const updatedSettings = {
        ...workspace.settings,
        blog_enabled: values.blog_enabled === true,
        blog_settings: blogSettings
      }
      const payload = {
        ...workspace,
        settings: updatedSettings
      }

      await workspaceService.update(payload)

      // Refresh the workspace data
      const response = await workspaceService.get(workspace.id)

      // Update the parent component with the new workspace data
      onWorkspaceUpdate(response.workspace)

      setFormTouched(false)
      message.success('Blog settings updated successfully')
    } catch (error: any) {
      console.error('Failed to update blog settings', error)
      // Extract the actual error message from the API response
      const errorMessage = error?.message || 'Failed to update blog settings'
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
      <>
        <SettingsSectionHeader
          title="Blog"
          description="Blog styling and SEO settings"
        />

        <Descriptions
          bordered
          column={1}
          size="small"
          labelStyle={{ width: '200px', fontWeight: '500' }}
        >
          <Descriptions.Item label="Blog">
            {workspace?.settings.blog_enabled ? (
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

          {workspace?.settings.blog_enabled && workspace?.settings.blog_settings && (
            <>
              <Descriptions.Item label="Title">
                {workspace.settings.blog_settings.title || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="Font Family">
                {workspace.settings.blog_settings.font_family || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="Font Size">
                {workspace.settings.blog_settings.font_size || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="Text Color">
                {workspace.settings.blog_settings.text_color || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="H1 Color">
                {workspace.settings.blog_settings.h1_color || 'Not set'}
              </Descriptions.Item>

              <Descriptions.Item label="Meta Title">
                {workspace.settings.blog_settings.seo?.meta_title || 'Not set'}
              </Descriptions.Item>
            </>
          )}
        </Descriptions>
      </>
    )
  }

  return (
    <>
      <SettingsSectionHeader
        title="Blog"
        description="Configure styling and SEO settings for your blog. These settings will be applied to all blog pages."
      />

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
            the blog.
          </div>
        )}

        <Form.Item
          name="blog_enabled"
          label="Enable Blog"
          tooltip="Enable the blog feature on your custom domain"
          valuePropName="checked"
        >
          <Switch disabled={!workspace?.settings.custom_endpoint_url} />
        </Form.Item>

        <Form.Item
          noStyle
          shouldUpdate={(prevValues, currentValues) =>
            prevValues.blog_enabled !== currentValues.blog_enabled
          }
        >
          {({ getFieldValue }) => {
            const blogEnabled = getFieldValue('blog_enabled')
            const customEndpoint = workspace?.settings.custom_endpoint_url

            if (!blogEnabled || !customEndpoint) {
              return null
            }

            return (
              <>
                <Form.Item
                  name={['blog_settings', 'title']}
                  label="Blog Title"
                  tooltip="The main title for your blog"
                >
                  <Input placeholder="My Amazing Blog" />
                </Form.Item>

                <Divider orientation="left">Typography & Colors</Divider>

                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item
                      name={['blog_settings', 'font_family']}
                      label="Font Family"
                      tooltip="CSS font family for blog content (e.g., 'Inter, sans-serif')"
                    >
                      <Input placeholder="Inter, sans-serif" />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item
                      name={['blog_settings', 'font_size']}
                      label="Base Font Size"
                      tooltip="Base font size for blog content (e.g., '16px', '1rem')"
                    >
                      <Input placeholder="16px" />
                    </Form.Item>
                  </Col>
                </Row>

                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item
                      name={['blog_settings', 'text_color']}
                      label="Text Color"
                      tooltip="Main text color for blog content"
                    >
                      <ColorPicker showText format="hex" />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item
                      name={['blog_settings', 'h1_color']}
                      label="H1 Heading Color"
                    >
                      <ColorPicker showText format="hex" />
                    </Form.Item>
                  </Col>
                </Row>

                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item
                      name={['blog_settings', 'h2_color']}
                      label="H2 Heading Color"
                    >
                      <ColorPicker showText format="hex" />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item
                      name={['blog_settings', 'h3_color']}
                      label="H3 Heading Color"
                    >
                      <ColorPicker showText format="hex" />
                    </Form.Item>
                  </Col>
                </Row>

                <Row gutter={16}>
                  <Col span={12}>
                    <Form.Item
                      name={['blog_settings', 'h4_color']}
                      label="H4 Heading Color"
                    >
                      <ColorPicker showText format="hex" />
                    </Form.Item>
                  </Col>
                </Row>

                <Divider orientation="left">SEO Settings</Divider>

                <div style={{ marginBottom: 16, color: '#666' }}>
                  These SEO settings will be used as defaults for your blog homepage. Individual posts
                  can override these values.
                </div>

                <SEOSettingsForm
                  namePrefix={['blog_settings', 'seo']}
                  titlePlaceholder="My Amazing Blog"
                  descriptionPlaceholder="Welcome to my blog where I share insights about..."
                />
              </>
            )
          }}
        </Form.Item>

        <Form.Item>
          <Button type="primary" htmlType="submit" loading={savingSettings} disabled={!formTouched}>
            Save Changes
          </Button>
        </Form.Item>
      </Form>
    </>
  )
}

