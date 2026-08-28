import { useCallback, useEffect, useState } from 'react'
import { Button, Form, App, Descriptions, Input, Divider, Select, Row, Col } from 'antd'
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined
} from '@ant-design/icons'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { SEOSettingsForm } from '../seo/SEOSettingsForm'
import { SettingsSaveBar } from './SettingsSaveBar'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { RecentThemesTable } from '../blog/RecentThemesTable'
import { blogThemesApi } from '../../services/api/blog'
import { THEME_PRESETS } from '../blog/themePresets'
import { ImageURLInput } from '../common/ImageURLInput'

interface BlogSettingsProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  canManage: boolean
}

interface BlogSettingsFormValues {
  blog_enabled?: boolean
  blog_settings?: {
    title?: string
    logo_url?: string
    icon_url?: string
    home_page_size?: number
    category_page_size?: number
    feed_max_items?: number
    feed_summary_only?: boolean
    seo?: {
      meta_title?: string
      meta_description?: string
      og_title?: string
      og_description?: string
      og_image?: string
      canonical_url?: string
      keywords?: string[]
      meta_robots?: string
    }
  }
}

/**
 * The stored blog settings as the form holds them. Both the initial load and
 * Discard read from here, so "restore what was saved" can never drift from
 * "what was loaded".
 *
 * Every field SEOSettingsForm renders has to be listed: the save replaces
 * blog_settings wholesale, so a field the form shows but this builder forgets
 * reaches the backend empty and wipes whatever was stored.
 */
function toFormValues(workspace: Workspace | null): BlogSettingsFormValues {
  const stored = workspace?.settings.blog_settings
  return {
    blog_enabled: workspace?.settings.blog_enabled || false,
    blog_settings: {
      title: stored?.title || '',
      logo_url: stored?.logo_url || '',
      icon_url: stored?.icon_url || '',
      home_page_size: stored?.home_page_size || 20,
      category_page_size: stored?.category_page_size || 20,
      feed_max_items: stored?.feed_max_items || 20,
      feed_summary_only: stored?.feed_summary_only || false,
      seo: {
        meta_title: stored?.seo?.meta_title || '',
        meta_description: stored?.seo?.meta_description || '',
        og_title: stored?.seo?.og_title || '',
        og_description: stored?.seo?.og_description || '',
        og_image: stored?.seo?.og_image || '',
        canonical_url: stored?.seo?.canonical_url || '',
        keywords: stored?.seo?.keywords || [],
        meta_robots: (stored?.seo?.meta_robots ?? 'index,follow') as string
      }
    }
  }
}

export function BlogSettings({ workspace, onWorkspaceUpdate, canManage }: BlogSettingsProps) {
  const { t } = useLingui()
  const [savingSettings, setSavingSettings] = useState(false)
  const [formTouched, setFormTouched] = useState(false)
  const [form] = Form.useForm<BlogSettingsFormValues>()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()

  // Fetch themes unconditionally (even if blog is disabled)
  const { data: themesData, isLoading: themesLoading } = useQuery({
    queryKey: ['blog-themes', workspace?.id],
    queryFn: () =>
      workspace?.id ? blogThemesApi.list(workspace.id, { limit: 3, offset: 0 }) : null,
    enabled: !!workspace?.id && canManage
  })

  useEffect(() => {
    // Only set form values if the user can manage the blog (form exists)
    if (!canManage) return

    // Set form values from workspace data whenever workspace changes
    form.setFieldsValue(toFormValues(workspace))
    setFormTouched(false)
  }, [workspace, form, canManage])

  // resetFields() would empty the form: the values arrive through
  // setFieldsValue above, so the form has no initialValues to reset to.
  const handleDiscard = useCallback(() => {
    form.setFieldsValue(toFormValues(workspace))
    setFormTouched(false)
  }, [form, workspace])

  const handleSaveSettings = async (values: BlogSettingsFormValues) => {
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
            files: THEME_PRESETS[0].files,
            notes: THEME_PRESETS[0].description
          })

          console.log('Theme created:', createdTheme.theme.version)

          // Publish the default theme
          await blogThemesApi.publish(workspace.id, { version: createdTheme.theme.version })

          console.log('Theme published successfully')

          // Invalidate theme query to refetch
          await queryClient.invalidateQueries({ queryKey: ['blog-themes', workspace.id] })

          message.success(t`Default theme created and published`)
        } catch (themeError: unknown) {
          console.error('Failed to create default theme', themeError)
          message.warning(t`Blog enabled but theme creation failed. Please create a theme manually.`)
        }
      }

      // Blog settings are saved via a dedicated, blog:write-gated endpoint (not the
      // owner-only workspaces.update). That endpoint leaves the stored flag alone when
      // the body names no blog_enabled, so an ordinary save omits the key instead of
      // re-asserting a state nobody touched. The Enable button and the disable
      // confirmation each pass their boolean explicitly.
      await workspaceService.setBlogSettings({
        workspace_id: workspace.id,
        ...(values.blog_enabled !== undefined ? { blog_enabled: values.blog_enabled } : {}),
        blog_settings: values.blog_settings
      })

      // Refresh the workspace data
      const response = await workspaceService.get(workspace.id)

      // Update the parent component with the new workspace data
      onWorkspaceUpdate(response.workspace)

      setFormTouched(false)
      message.success(t`Blog settings updated successfully`)
    } catch (error: unknown) {
      console.error('Failed to update blog settings', error)
      // Extract the actual error message from the API response
      const errorMessage = (error as Error)?.message || t`Failed to update blog settings`
      message.error(errorMessage)
    } finally {
      setSavingSettings(false)
    }
  }

  const handleFormChange = (changedValues: { blog_enabled?: boolean }) => {
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
      title: t`Disable Blog?`,
      icon: <ExclamationCircleOutlined />,
      content: t`Are you sure you want to disable the blog? All SEO settings and blog visibility will be lost. This action cannot be undone.`,
      okText: t`Disable Blog`,
      okType: 'danger',
      cancelText: t`Cancel`,
      onOk: async () => {
        // Set blog_enabled to false and submit
        form.setFieldValue('blog_enabled', false)
        await handleSaveSettings({ ...form.getFieldsValue(), blog_enabled: false })
      }
    })
  }

  if (!canManage) {
    return (
      <>
        <SettingsSectionHeader title={t`Blog`} description={t`Blog styling and SEO settings`} />

        <Descriptions
          bordered
          column={1}
          size="small"
          styles={{ label: { width: '200px', fontWeight: '500' } }}
        >
          <Descriptions.Item label={t`Blog`}>
            {workspace?.settings.blog_enabled ? (
              <span style={{ color: '#52c41a' }}>
                <CheckCircleOutlined style={{ marginRight: '8px' }} />
                {t`Enabled`}
              </span>
            ) : (
              <span style={{ color: '#ff4d4f' }}>
                <CloseCircleOutlined style={{ marginRight: '8px' }} />
                {t`Disabled`}
              </span>
            )}
          </Descriptions.Item>

          {workspace?.settings.blog_enabled && workspace?.settings.blog_settings && (
            <>
              <Descriptions.Item label={t`Title`}>
                {workspace.settings.blog_settings.title || t`Not set`}
              </Descriptions.Item>

              <Descriptions.Item label={t`Meta Title`}>
                {workspace.settings.blog_settings.seo?.meta_title || t`Not set`}
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
        title={t`Blog`}
        description={t`Configure styling and SEO settings for your blog. These settings will be applied to all blog pages.`}
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
          {t`You must configure a Custom Endpoint URL in General Settings above before enabling the blog.`}
        </div>
      )}

      {workspace?.settings.blog_enabled && workspace?.settings.custom_endpoint_url && (
        <>
          <RecentThemesTable workspaceId={workspace.id} workspace={workspace} />
          <Divider className="!my-12" />
        </>
      )}

      <Form
        form={form}
        layout="vertical"
        onFinish={handleSaveSettings}
        onValuesChange={handleFormChange}
        // The save control floats over the page, so a rejected field can sit
        // anywhere off screen when it is pressed.
        scrollToFirstError
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
            <h3 style={{ marginBottom: 8, fontSize: '16px', fontWeight: 600 }}>{t`Enable Blog`}</h3>
            <p style={{ marginBottom: 16, color: '#595959', lineHeight: '1.6' }}>
              {t`Enable the blog feature to publish articles and content on your custom domain homepage. Your blog will be accessible at`}{' '}
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
              {t`Enable Blog`}
            </Button>
          </div>
        )}

        {/* Show blog settings when enabled */}
        {workspace?.settings.blog_enabled && workspace?.settings.custom_endpoint_url && (
          <>
            <div className="text-xl font-medium mb-8">{t`General Settings`}</div>

            <Form.Item
              name={['blog_settings', 'title']}
              label={t`Blog Title`}
              tooltip={t`The main title for your blog`}
            >
              <Input placeholder={workspace?.name || t`My Amazing Blog`} />
            </Form.Item>

            <Form.Item
              name={['blog_settings', 'logo_url']}
              label={t`Logo URL`}
              tooltip={t`Main logo for your blog. Recommended size: 200x50px or similar aspect ratio`}
            >
              <ImageURLInput
                placeholder={t`Enter logo URL or select image`}
                acceptFileType="image/*"
                acceptItem={(item) =>
                  !item.is_folder && item.file_info?.content_type?.startsWith('image/')
                }
                buttonText={t`Select`}
                size="middle"
              />
            </Form.Item>

            <Form.Item
              name={['blog_settings', 'icon_url']}
              label={t`Icon/Favicon URL`}
              tooltip={t`Favicon for your blog. Recommended: 32x32px or 192x192px PNG`}
            >
              <ImageURLInput
                placeholder={t`Enter icon URL or select image`}
                acceptFileType="image/*"
                acceptItem={(item) =>
                  !item.is_folder && item.file_info?.content_type?.startsWith('image/')
                }
                buttonText={t`Select`}
                size="middle"
              />
            </Form.Item>

            <Row gutter={24}>
              <Col span={12}>
                <Form.Item
                  name={['blog_settings', 'home_page_size']}
                  label={t`Posts on homepage`}
                  tooltip={t`Number of posts to display per page on the homepage`}
                  rules={[{ required: true, message: t`Please select a page size` }]}
                >
                  <Select placeholder={t`Select page size`}>
                    <Select.Option value={5}>5</Select.Option>
                    <Select.Option value={10}>10</Select.Option>
                    <Select.Option value={15}>15</Select.Option>
                    <Select.Option value={20}>20</Select.Option>
                  </Select>
                </Form.Item>
              </Col>

              <Col span={12}>
                <Form.Item
                  name={['blog_settings', 'category_page_size']}
                  label={t`Posts on category pages`}
                  tooltip={t`Number of posts to display per page on category pages`}
                  rules={[{ required: true, message: t`Please select a page size` }]}
                >
                  <Select placeholder={t`Select page size`}>
                    <Select.Option value={5}>5</Select.Option>
                    <Select.Option value={10}>10</Select.Option>
                    <Select.Option value={15}>15</Select.Option>
                    <Select.Option value={20}>20</Select.Option>
                  </Select>
                </Form.Item>
              </Col>
            </Row>

            <Divider className="!my-8" />

            <SettingsSectionHeader
              title={t`RSS / Feeds`}
              description={t`Configure how your blog's RSS and JSON feeds are generated.`}
              className="!mb-4"
            />

            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name={['blog_settings', 'feed_max_items']}
                  label={t`Items per feed`}
                  tooltip={t`Maximum number of posts to include in the RSS/JSON feed (1–20)`}
                >
                  <Select placeholder={t`Select feed size`}>
                    <Select.Option value={5}>5</Select.Option>
                    <Select.Option value={10}>10</Select.Option>
                    <Select.Option value={15}>15</Select.Option>
                    <Select.Option value={20}>20</Select.Option>
                  </Select>
                </Form.Item>
              </Col>

              <Col span={12}>
                <Form.Item
                  name={['blog_settings', 'feed_summary_only']}
                  label={t`Summary-only feed`}
                  tooltip={t`When enabled, the feed includes only excerpts instead of full article content`}
                >
                  <Select placeholder={t`Select content mode`}>
                    <Select.Option value={false}>{t`Full content`}</Select.Option>
                    <Select.Option value={true}>{t`Excerpt only`}</Select.Option>
                  </Select>
                </Form.Item>
              </Col>
            </Row>

            <Divider className="!my-8" />

            <SEOSettingsForm
              namePrefix={['blog_settings', 'seo']}
              titlePlaceholder={t`My Amazing Blog`}
              descriptionPlaceholder={t`Welcome to my blog where I share insights about...`}
            />
          </>
        )}
      </Form>

      {/* Danger Zone - Show when blog is enabled */}
      {workspace?.settings.blog_enabled && (
        <>
          <Divider className="!my-12" />
          <div
            style={{
              marginTop: 32,
              padding: '24px',
              border: '1px solid #ff4d4f',
              borderRadius: '4px',
              backgroundColor: '#fff1f0'
            }}
          >
            <h3 style={{ color: '#cf1322', marginBottom: 8 }}>{t`Danger Zone`}</h3>
            <p style={{ marginBottom: 16, color: '#595959' }}>
              {t`Disabling the blog will remove all SEO settings and make your blog inaccessible to visitors. This action will affect your blog's visibility and search engine rankings.`}
            </p>
            <Button danger onClick={handleDisableBlog}>
              {t`Disable Blog`}
            </Button>
          </div>
        </>
      )}

      {/* Last child on purpose — see SettingsSaveBar for why. */}
      <SettingsSaveBar
        dirty={canManage && formTouched}
        saving={savingSettings}
        onSave={() => form.submit()}
        onDiscard={handleDiscard}
        leaveWarning={t`Your blog settings have not been saved. Leaving this page discards them.`}
      />
    </>
  )
}
