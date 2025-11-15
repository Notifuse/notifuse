import { useEffect, useState } from 'react'
import { Button, Form, App, Descriptions, Input, Divider } from 'antd'
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined
} from '@ant-design/icons'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { SEOSettingsForm } from '../seo/SEOSettingsForm'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { RecentThemesTable } from '../blog/RecentThemesTable'
import Subtitle from '../common/subtitle'
import { blogThemesApi } from '../../services/api/blog'
import { DEFAULT_BLOG_TEMPLATES } from '../../utils/defaultBlogTemplates'
import { DEFAULT_BLOG_STYLES } from '../../utils/defaultBlogStyles'

interface BlogSettingsProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  isOwner: boolean
}

export function BlogSettings({ workspace, onWorkspaceUpdate, isOwner }: BlogSettingsProps) {
  const [savingSettings, setSavingSettings] = useState(false)
  const [formTouched, setFormTouched] = useState(false)
  const [form] = Form.useForm()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()

  // Fetch themes unconditionally (even if blog is disabled)
  const { data: themesData, isLoading: themesLoading } = useQuery({
    queryKey: ['blog-themes', workspace?.id],
    queryFn: () =>
      workspace?.id ? blogThemesApi.list(workspace.id, { limit: 3, offset: 0 }) : null,
    enabled: !!workspace?.id && isOwner
  })

  useEffect(() => {
    // Only set form values if user is owner (form exists)
    if (!isOwner) return

    // Set form values from workspace data whenever workspace changes
    form.setFieldsValue({
      blog_enabled: workspace?.settings.blog_enabled || false,
      blog_settings: {
        title: workspace?.settings.blog_settings?.title || '',
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
      // Check if enabling blog and no themes exist
      const isEnablingBlog = values.blog_enabled === true && !workspace.settings.blog_enabled
      const hasNoThemes = !themesData?.themes || themesData.themes.length === 0

      console.log('handleSaveSettings', {
        isEnablingBlog,
        hasNoThemes,
        themesCount: themesData?.themes?.length || 0,
        themesLoading
      })

      if (isEnablingBlog && hasNoThemes) {
        try {
          console.log('Creating default theme...')
          // Create default theme
          const createdTheme = await blogThemesApi.create(workspace.id, {
            files: DEFAULT_BLOG_TEMPLATES,
            styling: DEFAULT_BLOG_STYLES,
            notes: 'Default theme'
          })

          console.log('Theme created:', createdTheme.theme.version)

          // Publish the default theme
          await blogThemesApi.publish(workspace.id, { version: createdTheme.theme.version })

          console.log('Theme published successfully')

          // Invalidate theme query to refetch
          await queryClient.invalidateQueries({ queryKey: ['blog-themes', workspace.id] })

          message.success('Default theme created and published')
        } catch (themeError: any) {
          console.error('Failed to create default theme', themeError)
          message.warning('Blog enabled but theme creation failed. Please create a theme manually.')
        }
      }

      const blogSettings = values.blog_settings || null

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

  const handleFormChange = (changedValues: any) => {
    setFormTouched(true)

    // If blog was just enabled and title is empty, set it to workspace name
    if (changedValues.blog_enabled === true) {
      const currentTitle = form.getFieldValue(['blog_settings', 'title'])
      if (!currentTitle && workspace?.name) {
        form.setFieldValue(['blog_settings', 'title'], workspace.name)
      }
    }
  }

  const handleDisableBlog = () => {
    modal.confirm({
      title: 'Disable Blog?',
      icon: <ExclamationCircleOutlined />,
      content:
        'Are you sure you want to disable the blog? All SEO settings and blog visibility will be lost. This action cannot be undone.',
      okText: 'Disable Blog',
      okType: 'danger',
      cancelText: 'Cancel',
      onOk: async () => {
        // Set blog_enabled to false and submit
        form.setFieldValue('blog_enabled', false)
        await handleSaveSettings({ ...form.getFieldsValue(), blog_enabled: false })
      }
    })
  }

  if (!isOwner) {
    return (
      <>
        <SettingsSectionHeader title="Blog" description="Blog styling and SEO settings" />

        <Descriptions
          bordered
          column={1}
          size="small"
          styles={{ label: { width: '200px', fontWeight: '500' } }}
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
          ⚠️ You must configure a Custom Endpoint URL in General Settings above before enabling the
          blog.
        </div>
      )}

      {workspace?.settings.blog_enabled && workspace?.settings.custom_endpoint_url && (
        <>
          <RecentThemesTable workspaceId={workspace.id} workspace={workspace} />
          <Divider />
        </>
      )}

      <Form
        form={form}
        layout="vertical"
        onFinish={handleSaveSettings}
        onValuesChange={handleFormChange}
      >
        {/* Show enable button only when blog is disabled */}
        {!workspace?.settings.blog_enabled && (
          <div
            style={{
              padding: '24px',
              border: '1px solid #d9d9d9',
              borderRadius: '8px',
              backgroundColor: '#fafafa',
              marginBottom: 24
            }}
          >
            <h3 style={{ marginBottom: 8, fontSize: '16px', fontWeight: 600 }}>Enable Blog</h3>
            <p style={{ marginBottom: 16, color: '#595959', lineHeight: '1.6' }}>
              Enable the blog feature to publish articles and content on your custom domain
              homepage. Your blog will be accessible at{' '}
              <strong>
                {workspace?.settings.custom_endpoint_url || 'your-custom-domain.com'}/
              </strong>
            </p>
            <Button
              type="primary"
              size="large"
              disabled={!workspace?.settings.custom_endpoint_url || themesLoading}
              loading={savingSettings || themesLoading}
              onClick={async () => {
                form.setFieldValue('blog_enabled', true)
                // Initialize blog_settings with title if not set
                const currentValues = form.getFieldsValue()
                const blogSettings = currentValues.blog_settings || {}
                if (!blogSettings.title && workspace?.name) {
                  blogSettings.title = workspace.name
                }
                await handleSaveSettings({
                  ...currentValues,
                  blog_enabled: true,
                  blog_settings: blogSettings
                })
              }}
            >
              Enable Blog
            </Button>
          </div>
        )}

        {/* Show blog settings when enabled */}
        {workspace?.settings.blog_enabled && workspace?.settings.custom_endpoint_url && (
          <>
            <Subtitle borderBottom={true} primary>
              Homepage Settings
            </Subtitle>

            <Form.Item
              name={['blog_settings', 'title']}
              label="Blog Title"
              tooltip="The main title for your blog"
            >
              <Input placeholder={workspace?.name || 'My Amazing Blog'} />
            </Form.Item>

            <SEOSettingsForm
              namePrefix={['blog_settings', 'seo']}
              titlePlaceholder="My Amazing Blog"
              descriptionPlaceholder="Welcome to my blog where I share insights about..."
            />

            {formTouched && (
              <div style={{ textAlign: 'right', marginBottom: 24 }}>
                <Button type="primary" htmlType="submit" loading={savingSettings}>
                  Save Changes
                </Button>
              </div>
            )}
          </>
        )}
      </Form>

      {/* Danger Zone - Show when blog is enabled */}
      {workspace?.settings.blog_enabled && (
        <>
          <Divider />
          <div
            style={{
              marginTop: 32,
              padding: '24px',
              border: '1px solid #ff4d4f',
              borderRadius: '4px',
              backgroundColor: '#fff1f0'
            }}
          >
            <h3 style={{ color: '#cf1322', marginBottom: 8 }}>Danger Zone</h3>
            <p style={{ marginBottom: 16, color: '#595959' }}>
              Disabling the blog will remove all SEO settings and make your blog inaccessible to
              visitors. This action will affect your blog's visibility and search engine rankings.
            </p>
            <Button danger onClick={handleDisableBlog}>
              Disable Blog
            </Button>
          </div>
        </>
      )}
    </>
  )
}
