import { useCallback, useEffect, useState } from 'react'
import { Button, Form, App, Descriptions, Input, Segmented, Select } from 'antd'
import { Link } from '@tanstack/react-router'
import { ExternalLink } from 'lucide-react'
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined
} from '@ant-design/icons'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { SeoPreview } from '../seo/SeoPreview'
import { blogBaseUrl } from '../blog/category/categorySeo'
import { SettingsRow, SettingsRows } from './SettingsRow'
import { SettingsSaveBar } from './SettingsSaveBar'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { Panel, Pill } from '../layout'
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
 * Every field the form renders has to be listed: the save replaces
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
  // For the home page preview; watched here, as hooks come before the read-only return.
  const watchedSeo = (Form.useWatch(['blog_settings', 'seo'], { form, preserve: true }) ?? {}) as NonNullable<
    NonNullable<BlogSettingsFormValues['blog_settings']>['seo']
  >
  const watchedIcon = Form.useWatch(['blog_settings', 'icon_url'], { form, preserve: true }) as string | undefined

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
      title: t`Turn off the blog?`,
      icon: <ExclamationCircleOutlined />,
      // The server only clears the flag (SetBlogSettings): the title, SEO, pages and feeds stay
      // stored, and turning the blog back on brings them back.
      content: t`The blog's pages stop being served. Its posts and settings are kept, and come back when you turn it on again.`,
      okText: t`Turn off the blog`,
      okType: 'danger',
      cancelText: t`Cancel`,
      onOk: async () => {
        // Set blog_enabled to false and submit
        form.setFieldValue('blog_enabled', false)
        await handleSaveSettings({ ...(form.getFieldsValue(true) as BlogSettingsFormValues), blog_enabled: false })
      }
    })
  }

  if (!canManage) {
    return (
      <>
        <SettingsSectionHeader title={t`Blog`} description={t`Blog styling and SEO settings`} />

        <Panel>
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
        </Panel>
      </>
    )
  }

  const domain = workspace?.settings.custom_endpoint_url || ''
  // 10. The same address rule as the blog's other screens (blogBaseUrl).
  const blogAddress = domain ? blogBaseUrl(domain, undefined) : ''
  const blogHost = blogAddress.replace(/^https?:\/\//, '')
  const workspaceName = workspace?.name || ''
  const pageSizes = [5, 10, 15, 20].map((value) => ({ value, label: String(value) }))

  // The home page as the default theme writes it: the title falls back to the workspace name,
  // the share title too; the descriptions to nothing.
  const homeSeo = {
    title: watchedSeo.meta_title?.trim() || workspaceName,
    description: watchedSeo.meta_description?.trim() || '',
    ogTitle: watchedSeo.og_title?.trim() || workspaceName,
    ogDescription: watchedSeo.og_description?.trim() || '',
    ogImage: watchedSeo.og_image?.trim() || ''
  }

  const imageInput = (placeholder: string) => (
    <ImageURLInput
      placeholder={placeholder}
      acceptFileType="image/*"
      acceptItem={(item) => !item.is_folder && item.file_info?.content_type?.startsWith('image/')}
      buttonText={t`Select`}
      size="middle"
    />
  )

  return (
    <>
      <SettingsSectionHeader
        title={t`Blog`}
        description={t`The blog served on your custom domain: its identity, pages, feeds and search tags`}
      />

      <Form
        form={form}
        layout="vertical"
        onFinish={handleSaveSettings}
        onValuesChange={handleFormChange}
        // The save control floats over the page, so a rejected field can sit
        // anywhere off screen when it is pressed.
        scrollToFirstError
        component={false}
      >
        <Panel title={t`Blog`}>
          <SettingsRows>
            {workspace?.settings.blog_enabled ? (
              <SettingsRow label={t`Status`} help={t`Visitors can read the published posts`}>
                <div className="flex items-center justify-between gap-3">
                  <Pill tone="success">{t`On`}</Pill>
                  {blogAddress ? (
                    <a
                      href={blogAddress}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex min-w-0 items-center gap-1 text-sm text-primary-600 hover:text-primary-700"
                    >
                      <span className="truncate">{blogHost}</span>
                      <ExternalLink size={12} aria-hidden="true" />
                    </a>
                  ) : null}
                </div>
              </SettingsRow>
            ) : (
              <SettingsRow label={t`Status`} help={t`Turn it on to publish posts at the root of your custom domain`}>
                <div className="flex items-center justify-between gap-3">
                  <Pill tone="neutral">{t`Off`}</Pill>
                  <Button
                    type="primary"
                    disabled={!domain || themesLoading}
                    loading={savingSettings || themesLoading}
                    onClick={async () => {
                      form.setFieldValue('blog_enabled', true)
                      // Initialize blog_settings with title if not set
                      // The whole store: every blog_settings field is unmounted while the blog is off,
                      // and the save replaces blog_settings wholesale, so the mounted fields alone
                      // would erase the logo, SEO, pages and feeds the off dialog promised to keep.
                      const currentValues = form.getFieldsValue(true) as BlogSettingsFormValues
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
                    {t`Turn on the blog`}
                  </Button>
                </div>
              </SettingsRow>
            )}
            {!domain ? (
              <SettingsRow
                label={t`Custom domain`}
                help={<span className="text-warning-fg">{t`Needed first: the blog is served at its root`}</span>}
              >
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className="text-ink-3">{t`Not set`}</span>
                  {workspace ? (
                    <Link
                      to="/console/workspace/$workspaceId/settings/$section"
                      params={{ workspaceId: workspace.id, section: 'general' }}
                      className="text-primary-600 hover:text-primary-700"
                    >
                      {t`Set it in General`}
                    </Link>
                  ) : null}
                </div>
              </SettingsRow>
            ) : null}
          </SettingsRows>
        </Panel>

        {workspace?.settings.blog_enabled && domain ? (
          <>
            <RecentThemesTable workspaceId={workspace.id} workspace={workspace} />

            <Panel title={t`Identity`}>
              <SettingsRows>
                <SettingsRow label={t`Title`} help={t`The home page headline, and the name in the footer`} htmlFor="blog_settings_title">
                  <Form.Item name={['blog_settings', 'title']} rules={[{ max: 255, message: t`255 characters at most` }]}>
                    <Input placeholder={workspaceName || t`My Amazing Blog`} />
                  </Form.Item>
                </SettingsRow>
                <SettingsRow label={t`Logo`} help={t`In the header. Without one, the workspace logo`}>
                  <Form.Item name={['blog_settings', 'logo_url']}>{imageInput(t`Enter logo URL or select image`)}</Form.Item>
                </SettingsRow>
                <SettingsRow label={t`Favicon`} help={t`The icon in the browser tab; 32 or 192 px square`}>
                  <Form.Item name={['blog_settings', 'icon_url']}>{imageInput(t`Enter icon URL or select image`)}</Form.Item>
                </SettingsRow>
              </SettingsRows>
            </Panel>

            <Panel title={t`Pages and feeds`}>
              <SettingsRows>
                <SettingsRow label={t`Posts on the home page`} help={t`Per page`}>
                  <Form.Item
                    name={['blog_settings', 'home_page_size']}
                    rules={[{ required: true, message: t`Please select a page size` }]}
                  >
                    <Select options={pageSizes} aria-label={t`Posts on the home page`} />
                  </Form.Item>
                </SettingsRow>
                <SettingsRow label={t`Posts on a category page`} help={t`Per page`}>
                  <Form.Item
                    name={['blog_settings', 'category_page_size']}
                    rules={[{ required: true, message: t`Please select a page size` }]}
                  >
                    <Select options={pageSizes} aria-label={t`Posts on a category page`} />
                  </Form.Item>
                </SettingsRow>
                <SettingsRow label={t`Posts in the feeds`} help={t`RSS and JSON feeds, newest first`}>
                  <Form.Item name={['blog_settings', 'feed_max_items']}>
                    <Select options={pageSizes} aria-label={t`Posts in the feeds`} />
                  </Form.Item>
                </SettingsRow>
                <SettingsRow label={t`Feed content`} help={t`What a feed reader shows of each post`}>
                  <Form.Item
                    name={['blog_settings', 'feed_summary_only']}
                    // Stored as a boolean; the switch speaks in content kinds.
                    getValueProps={(summaryOnly: boolean) => ({ value: summaryOnly ? 'excerpt' : 'full' })}
                    normalize={(kind: string) => kind === 'excerpt'}
                  >
                    <Segmented
                      aria-label={t`Feed content`}
                      options={[
                        { value: 'full', label: t`Full post` },
                        { value: 'excerpt', label: t`Excerpt only` }
                      ]}
                    />
                  </Form.Item>
                </SettingsRow>
              </SettingsRows>
            </Panel>

            <Panel
              title={t`Search engines`}
              extra={<span className="text-xs text-ink-3">{t`The home page, and every page without its own tags`}</span>}
            >
              <SettingsRows>
                <SettingsRow label={t`Meta title`} help={t`Defaults to the workspace name`} htmlFor="blog_settings_seo_meta_title">
                  <Form.Item name={['blog_settings', 'seo', 'meta_title']}>
                    <Input maxLength={60} showCount placeholder={workspaceName} />
                  </Form.Item>
                </SettingsRow>
                <SettingsRow label={t`Meta description`} help={t`Under the title in search results`} htmlFor="blog_settings_seo_meta_description">
                  {/* Room for the count antd sets under a text area: it sat on the row's divider. */}
                  <div data-count-room className="pb-3">
                    <Form.Item name={['blog_settings', 'seo', 'meta_description']}>
                      <Input.TextArea rows={2} maxLength={160} showCount placeholder={t`Brief description for search results`} />
                    </Form.Item>
                  </div>
                </SettingsRow>
                <SettingsRow label={t`Keywords`}>
                  <Form.Item name={['blog_settings', 'seo', 'keywords']}>
                    <Select mode="tags" placeholder={t`Add keywords...`} aria-label={t`Keywords`} />
                  </Form.Item>
                </SettingsRow>
                <SettingsRow label={t`Indexing`} help={t`For the whole blog: the theme writes this robots tag on every page`}>
                  <Form.Item name={['blog_settings', 'seo', 'meta_robots']}>
                    <Select
                      aria-label={t`Indexing`}
                      options={[
                        { value: 'index,follow', label: t`Index and follow links` },
                        { value: 'noindex,follow', label: t`Don't index, but follow links` },
                        { value: 'index,nofollow', label: t`Index, but don't follow links` },
                        { value: 'noindex,nofollow', label: t`Don't index and don't follow links` }
                      ]}
                    />
                  </Form.Item>
                </SettingsRow>
              </SettingsRows>
            </Panel>

            <Panel title={t`Social share`} extra={<span className="text-xs text-ink-3">{t`When a link to the blog is shared`}</span>}>
              <SettingsRows>
                <SettingsRow label={t`Share title`} help={t`Defaults to the workspace name`} htmlFor="blog_settings_seo_og_title">
                  <Form.Item name={['blog_settings', 'seo', 'og_title']}>
                    <Input maxLength={60} showCount placeholder={workspaceName} />
                  </Form.Item>
                </SettingsRow>
                <SettingsRow label={t`Share description`} htmlFor="blog_settings_seo_og_description">
                  {/* Room for the count antd sets under a text area: it sat on the row's divider. */}
                  <div data-count-room className="pb-3">
                    <Form.Item name={['blog_settings', 'seo', 'og_description']}>
                      <Input.TextArea rows={2} maxLength={160} showCount />
                    </Form.Item>
                  </div>
                </SettingsRow>
                <SettingsRow
                  label={t`Share image`}
                  help={t`Also the image of a post or category without one; 1200 × 630 px`}
                >
                  <Form.Item name={['blog_settings', 'seo', 'og_image']}>{imageInput('https://example.com/image.jpg')}</Form.Item>
                </SettingsRow>
              </SettingsRows>
            </Panel>

            <SeoPreview effective={homeSeo} domain={blogHost} path={[]} iconUrl={watchedIcon || undefined} />

            <Panel title={t`Turn off the blog`} tone="danger">
              <SettingsRows>
                <SettingsRow label={t`Turn off`} help={t`The pages stop being served; posts and settings are kept`}>
                  <Button danger onClick={handleDisableBlog}>
                    {t`Turn off the blog`}
                  </Button>
                </SettingsRow>
              </SettingsRows>
            </Panel>
          </>
        ) : null}
      </Form>

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
