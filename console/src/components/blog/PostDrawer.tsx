import { useEffect, useState, useMemo, useRef } from 'react'
import { useLingui } from '@lingui/react/macro'
import {
  Button,
  Drawer,
  Form,
  Input,
  App,
  Select,
  InputNumber,
  Space,
  Row,
  Col,
  Typography
} from 'antd'
import { useMutation, useQueryClient, useQuery } from '@tanstack/react-query'
import { debounce } from 'lodash'
import { Undo, Redo } from 'lucide-react'
import {
  blogPostsApi,
  blogCategoriesApi,
  normalizeSlug,
  BlogPost,
  BlogAuthor
} from '../../services/api/blog'
import type { CreateBlogPostRequest, UpdateBlogPostRequest } from '../../services/api/blog'
import { SEOSettingsForm } from '../seo/SEOSettingsForm'
import { ImageURLInput } from '../common/ImageURLInput'
import { templatesApi } from '../../services/api/template'
import { AuthorsTable } from './AuthorsTable'
import {
  NotifuseEditor,
  type NotifuseEditorRef,
  type TOCAnchor,
  DEFAULT_INITIAL_CONTENT
} from '../blog_editor'
import { jsonToHtml, extractTextContent } from './utils'
import type { Workspace } from '../../services/api/types'
import { BlogAIAssistant } from './BlogAIAssistant'

// TiptapNode interface matching the structure expected by utils
interface TiptapNode {
  type: string
  text?: string
  content?: TiptapNode[]
  marks?: Array<{
    type: string
    attrs?: Record<string, unknown>
  }>
  attrs?: Record<string, unknown>
}

const { TextArea } = Input

interface PostFormValues {
  title: string
  slug: string
  category_id: string
  excerpt?: string
  featured_image_url?: string
  authors: BlogAuthor[]
  reading_time_minutes?: number
  seo?: Record<string, unknown>
}

interface PostDrawerProps {
  open: boolean
  onClose: () => void
  post?: BlogPost | null
  workspace: Workspace
  initialCategoryId?: string | null
}

const HEADER_HEIGHT = 66

export function PostDrawer({ open, onClose, post, workspace, initialCategoryId }: PostDrawerProps) {
  const { t } = useLingui()
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const { message, modal } = App.useApp()
  const isEditMode = !!post
  const [formTouched, setFormTouched] = useState(false)
  const [loading, setLoading] = useState(false)
  const [currentSlug, setCurrentSlug] = useState<string>('')

  // Blog content state (Tiptap JSON)
  const [blogContent, setBlogContent] = useState<TiptapNode | null>(null)

  // Template ID for new posts (generated on mount)
  // Generate 32-character ID by removing hyphens from UUID (UUIDs are 36 chars with hyphens, 32 without)
  const [newTemplateId] = useState<string>(() =>
    crypto.randomUUID().replace(/-/g, '').substring(0, 32)
  )

  // Editor ref for undo/redo
  const editorRef = useRef<NotifuseEditorRef>(null)
  const [canUndo, setCanUndo] = useState(false)
  const [canRedo, setCanRedo] = useState(false)

  // Table of Contents state
  const [tableOfContents, setTableOfContents] = useState<TOCAnchor[]>([])

  // Editor key counter - increment to force editor remount when restoring draft
  const [editorKeyCounter, setEditorKeyCounter] = useState(0)

  // Window width for responsive TOC display
  const [windowWidth, setWindowWidth] = useState<number>(
    typeof window !== 'undefined' ? window.innerWidth : 1920
  )

  // localStorage key for drafts
  const draftKey = `blog-post-draft-${post?.id || 'new'}-${workspace.id}`

  // Check if content is empty
  const isContentEmpty = (content: TiptapNode | null): boolean => {
    if (!content) return true
    if (!content.content || content.content.length === 0) return true

    // Check if all content nodes are empty paragraphs
    return content.content.every((node) => {
      if (node.type === 'paragraph') {
        return !node.content || node.content.length === 0
      }
      return false
    })
  }

  // Debounced save to localStorage - saves both content and form values
  const debouncedLocalSave = useMemo(
    () =>
      debounce((content: TiptapNode | null, formValues?: Record<string, unknown>) => {
        // Don't save if content is empty and no form values
        if (isContentEmpty(content) && !formValues) {
          // Remove any existing draft if content becomes empty
          localStorage.removeItem(draftKey)
          return
        }

        try {
          // Get current form values if not provided
          const valuesToSave = formValues || form.getFieldsValue()
          localStorage.setItem(
            draftKey,
            JSON.stringify({
              content,
              formValues: valuesToSave,
              savedAt: new Date().toISOString()
            })
          )
        } catch (e) {
          console.error('Failed to save draft:', e)
        }
      }, 1000),
    [draftKey, form]
  )

  // Handle content change with auto-save
  const handleContentChange = (json: Record<string, unknown>) => {
    const tiptapNode = json as unknown as TiptapNode
    setBlogContent(tiptapNode)

    // Update undo/redo state
    if (editorRef.current) {
      setCanUndo(editorRef.current.canUndo())
      setCanRedo(editorRef.current.canRedo())
    }

    // Only save if content is not empty
    if (!isContentEmpty(tiptapNode)) {
      debouncedLocalSave(tiptapNode)
    }

    setFormTouched(true)
  }

  // Handle Table of Contents updates
  const handleTOCUpdate = (anchors: TOCAnchor[]) => {
    setTableOfContents(anchors)
  }

  // Handle TOC item click
  const handleTOCClick = (anchor: TOCAnchor) => {
    // Scroll to the heading within the editor container
    if (anchor.dom) {
      const editorContainer = document.querySelector('[data-editor-container]')
      if (editorContainer) {
        // Calculate position relative to container
        const containerRect = editorContainer.getBoundingClientRect()
        const elementRect = anchor.dom.getBoundingClientRect()
        const scrollTop = editorContainer.scrollTop + (elementRect.top - containerRect.top) - 20

        editorContainer.scrollTo({
          top: scrollTop,
          behavior: 'smooth'
        })
      } else {
        // Fallback to regular scroll
        anchor.dom.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }
    }
  }

  // Handle window resize for responsive TOC
  useEffect(() => {
    const handleResize = () => {
      setWindowWidth(window.innerWidth)
    }

    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  // Fetch categories for dropdown
  const { data: categoriesData } = useQuery({
    queryKey: ['blog-categories', workspace.id],
    queryFn: () => blogCategoriesApi.list(workspace.id),
    enabled: open
  })

  // Fetch template for editing existing posts
  const { data: templateData, isLoading: templateLoading } = useQuery({
    queryKey: [
      'template',
      workspace.id,
      post?.settings.template.template_id,
      post?.settings.template.template_version
    ],
    queryFn: () =>
      templatesApi.get({
        workspace_id: workspace.id,
        id: post!.settings.template.template_id,
        version: post!.settings.template.template_version
      }),
    enabled: isEditMode && !!post && open,
    staleTime: 0, // Always fetch fresh data
    refetchOnMount: true // Refetch when drawer opens
  })

  // Reset form and content when drawer opens or post changes
  useEffect(() => {
    if (open) {
      if (post) {
        // Clear any localStorage draft for this post - we want to load from DB, not stale drafts
        localStorage.removeItem(draftKey)

        // Populate form with existing post data
        form.setFieldsValue({
          title: post.settings.title,
          slug: post.slug,
          category_id: post.category_id,
          excerpt: post.settings.excerpt,
          featured_image_url: post.settings.featured_image_url,
          authors: post.settings.authors,
          reading_time_minutes: post.settings.reading_time_minutes,
          seo: post.settings.seo
        })

        // Set current slug
        setCurrentSlug(post.slug)

        // Reset blog content to avoid showing stale data from previous post
        setBlogContent(null)
      } else {
        // New post - try to load from localStorage
        const savedDraft = localStorage.getItem(draftKey)
        if (savedDraft) {
          try {
            const { content, formValues, savedAt } = JSON.parse(savedDraft)
            modal.confirm({
              title: t`Restore Draft?`,
              content: t`Found unsaved changes from ${new Date(savedAt).toLocaleString()}`,
              okText: t`Yes`,
              cancelText: t`No`,
              onOk: () => {
                // Restore editor content
                setBlogContent(content)
                // Force editor remount with restored content
                setEditorKeyCounter((prev) => prev + 1)
                // Restore form values (title, excerpt, SEO, etc.)
                if (formValues) {
                  form.setFieldsValue(formValues)
                  if (formValues.slug) {
                    setCurrentSlug(formValues.slug as string)
                  }
                }
              },
              onCancel: () => localStorage.removeItem(draftKey)
            })
          } catch (error) {
            console.error('Error loading draft from localStorage:', error)
          }
        } else {
          setBlogContent(null) // Empty editor
        }

        form.resetFields()
        form.setFieldsValue({
          authors: [],
          reading_time_minutes: 5,
          category_id: initialCategoryId || undefined
        })
        setCurrentSlug('')
      }
      // Reset loading state when drawer opens
      setLoading(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, post?.id, form, draftKey, initialCategoryId, modal])

  // Load template content when template data is ready
  useEffect(() => {
    if (!open || !post) {
      return
    }

    // Only set content when template data matches the current post exactly
    if (
      !templateLoading &&
      templateData?.template?.web?.content &&
      templateData.template.id === post.settings.template.template_id &&
      templateData.template.version === post.settings.template.template_version
    ) {
      setBlogContent(templateData.template.web.content as TiptapNode)
    } else if (!templateLoading && post && !templateData?.template?.web?.content) {
      // Template failed to load or doesn't exist - ensure content is cleared
      setBlogContent(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    open,
    post?.id,
    templateLoading,
    templateData,
    post?.settings.template.template_id,
    post?.settings.template.template_version
  ])

  const createMutation = useMutation({
    mutationFn: async (values: PostFormValues) => {
      // Get HTML and plain text from content
      const html = editorRef.current?.getHTML() || jsonToHtml(blogContent)
      const plainText = extractTextContent(blogContent)

      // First, create the template
      // Template name is limited to 32 characters, so truncate if needed
      const templateName = `Blog: ${values.title}`.substring(0, 32)
      const templateCreateResponse = await templatesApi.create({
        workspace_id: workspace.id,
        id: newTemplateId,
        name: templateName,
        channel: 'web',
        category: 'blog',
        web: {
          content: blogContent, // Tiptap JSON
          html: html, // Pre-rendered HTML
          plain_text: plainText // Plain text for search
        }
      })

      // Then create the blog post with the template reference
      const createRequest: CreateBlogPostRequest = {
        category_id: values.category_id,
        slug: values.slug,
        title: values.title,
        template_id: newTemplateId,
        template_version: templateCreateResponse.template.version,
        excerpt: values.excerpt,
        featured_image_url: values.featured_image_url,
        authors: values.authors.filter((author: BlogAuthor) => author.name.trim() !== ''),
        reading_time_minutes: values.reading_time_minutes || 5,
        seo: values.seo
      }

      return blogPostsApi.create(workspace.id, createRequest)
    },
    onSuccess: () => {
      message.success(t`Post created successfully`)
      localStorage.removeItem(draftKey)
      queryClient.invalidateQueries({ queryKey: ['blog-posts', workspace.id] })
      handleClose()
    },
    onError: (error: Error) => {
      message.error(t`Failed to create post: ${error.message}`)
      setLoading(false)
    }
  })

  const updateMutation = useMutation({
    mutationFn: async (values: PostFormValues) => {
      // Get HTML and plain text from content
      const html = editorRef.current?.getHTML() || jsonToHtml(blogContent)
      const plainText = extractTextContent(blogContent)

      // First, update the template (backend creates new version)
      // Template name is limited to 32 characters, so truncate if needed
      const templateName = `Blog: ${values.title}`.substring(0, 32)
      await templatesApi.update({
        workspace_id: workspace.id,
        id: post!.settings.template.template_id,
        name: templateName,
        channel: 'web',
        category: 'blog',
        web: {
          content: blogContent, // Tiptap JSON
          html: html, // Pre-rendered HTML
          plain_text: plainText // Plain text for search
        }
      })

      // Fetch the updated template to get the new version
      const updatedTemplate = await templatesApi.get({
        workspace_id: workspace.id,
        id: post!.settings.template.template_id
      })

      // Then update the blog post
      const updateRequest: UpdateBlogPostRequest = {
        id: post!.id,
        category_id: values.category_id,
        slug: values.slug,
        title: values.title,
        template_id: post!.settings.template.template_id,
        template_version: updatedTemplate.template.version,
        excerpt: values.excerpt,
        featured_image_url: values.featured_image_url,
        authors: values.authors.filter((author: BlogAuthor) => author.name.trim() !== ''),
        reading_time_minutes: values.reading_time_minutes || 5,
        seo: values.seo
      }

      return blogPostsApi.update(workspace.id, updateRequest)
    },
    onSuccess: () => {
      message.success(t`Post updated successfully`)
      localStorage.removeItem(draftKey)
      queryClient.invalidateQueries({ queryKey: ['blog-posts', workspace.id] })
      handleClose()
    },
    onError: (error: Error) => {
      message.error(t`Failed to update post: ${error.message}`)
      setLoading(false)
    }
  })

  // Get the selected category
  const categoryId = Form.useWatch('category_id', form)
  const selectedCategory = (categoriesData?.categories ?? []).find((cat) => cat.id === categoryId)

  // Build full URL for preview
  const getFullPostUrl = () => {
    const baseUrl =
      workspace?.settings?.custom_endpoint_url || window.API_ENDPOINT || 'https://example.com'
    const categorySlug = selectedCategory?.slug || 'category'
    const postSlug = currentSlug || 'post-slug'

    return `${baseUrl}/${categorySlug}/${postSlug}`
  }

  const handleTitleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (isEditMode) return // Don't update slug in edit mode

    const title = e.target.value
    const slug = normalizeSlug(title)
    form.setFieldsValue({ slug })
    setCurrentSlug(slug)
  }

  const handleSlugChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    // Normalize spaces and non-allowed characters to hyphens
    const normalized = value
      .toLowerCase()
      .replace(/\s+/g, '-') // Replace spaces with hyphens
      .replace(/[^a-z0-9-]/g, '-') // Replace non-allowed characters with hyphens
      .replace(/-+/g, '-') // Replace multiple consecutive hyphens with a single hyphen
      .replace(/^-+|-+$/g, '') // Remove leading/trailing hyphens

    setCurrentSlug(normalized)
    form.setFieldsValue({ slug: normalized })
  }

  const handleClose = () => {
    if (formTouched && !loading && !createMutation.isPending && !updateMutation.isPending) {
      modal.confirm({
        title: t`Unsaved changes`,
        content: t`You have unsaved changes. Are you sure you want to close this drawer?`,
        okText: t`Yes`,
        cancelText: t`No`,
        onOk: () => {
          closeDrawer()
        }
      })
    } else {
      closeDrawer()
    }
  }

  const closeDrawer = () => {
    onClose()
    form.resetFields()
    setFormTouched(false)
    setBlogContent(null)
    setLoading(false)
    setEditorKeyCounter(0)
  }

  // Handler for AI assistant content updates
  const handleAIUpdateContent = (json: Record<string, unknown>) => {
    if (editorRef.current && json.type === 'doc') {
      editorRef.current.setContent(json)
      setBlogContent(json as TiptapNode)
      setFormTouched(true)
    }
  }

  // Handler for AI assistant metadata updates
  const handleAIUpdateMetadata = (metadata: {
    title?: string
    excerpt?: string
    meta_title?: string
    meta_description?: string
    keywords?: string[]
    og_title?: string
    og_description?: string
  }) => {
    const updates: Record<string, unknown> = {}

    if (metadata.title !== undefined) {
      updates.title = metadata.title
      // Also update slug for new posts
      if (!isEditMode) {
        const newSlug = normalizeSlug(metadata.title)
        updates.slug = newSlug
        setCurrentSlug(newSlug)
      }
    }
    if (metadata.excerpt !== undefined) updates.excerpt = metadata.excerpt
    if (metadata.meta_title !== undefined) updates['seo'] = { ...form.getFieldValue('seo'), meta_title: metadata.meta_title }
    if (metadata.meta_description !== undefined) updates['seo'] = { ...form.getFieldValue('seo'), ...updates['seo'] as object, meta_description: metadata.meta_description }
    if (metadata.keywords !== undefined) updates['seo'] = { ...form.getFieldValue('seo'), ...updates['seo'] as object, keywords: metadata.keywords }
    if (metadata.og_title !== undefined) updates['seo'] = { ...form.getFieldValue('seo'), ...updates['seo'] as object, og_title: metadata.og_title }
    if (metadata.og_description !== undefined) updates['seo'] = { ...form.getFieldValue('seo'), ...updates['seo'] as object, og_description: metadata.og_description }

    form.setFieldsValue(updates)
    setFormTouched(true)
  }

  // Get current metadata for AI assistant
  const getCurrentMetadata = () => {
    const seo = form.getFieldValue('seo') || {}
    return {
      title: form.getFieldValue('title'),
      excerpt: form.getFieldValue('excerpt'),
      meta_title: seo.meta_title,
      meta_description: seo.meta_description,
      keywords: seo.keywords,
      og_title: seo.og_title,
      og_description: seo.og_description
    }
  }

  const onFinish = (values: PostFormValues) => {
    setLoading(true)

    // Filter out empty authors
    const authors: BlogAuthor[] = (values.authors || []).filter(
      (author: BlogAuthor) => author.name.trim() !== ''
    )

    values.authors = authors

    if (isEditMode) {
      updateMutation.mutate(values)
    } else {
      createMutation.mutate(values)
    }
  }

  return (
    <Drawer
      title={isEditMode ? t`Edit Post` : t`Create New Post`}
      width="100%"
      onClose={handleClose}
      open={open}
      keyboard={false}
      maskClosable={false}
      className={'drawer-no-transition drawer-body-no-padding'}
      extra={
        <div className="text-right">
          <Space>
            <Button
              type="text"
              icon={<Undo size={16} />}
              onClick={() => editorRef.current?.undo()}
              disabled={!canUndo}
              title={t`Undo`}
            />
            <Button
              type="text"
              icon={<Redo size={16} />}
              onClick={() => editorRef.current?.redo()}
              disabled={!canRedo}
              title={t`Redo`}
            />
            {/* add vertical separator */}
            <div className="h-4 w-px bg-gray-200" />
            <Button type="link" loading={loading} onClick={handleClose}>
              {t`Cancel`}
            </Button>
            <Button
              loading={loading || createMutation.isPending || updateMutation.isPending}
              onClick={() => {
                form.submit()
              }}
              type="primary"
            >
              {isEditMode ? t`Save` : t`Create`}
            </Button>
          </Space>
        </div>
      }
    >
      <Form
        form={form}
        layout="vertical"
        onFinish={onFinish}
        onFinishFailed={(info) => {
          if (info.errorFields && info.errorFields.length > 0) {
            message.error(t`Please check the form for errors.`)
          }
          setLoading(false)
        }}
        onValuesChange={(_, allValues) => {
          setFormTouched(true)
          // Auto-save form values along with content
          debouncedLocalSave(blogContent, allValues)
        }}
        initialValues={{
          authors: [],
          reading_time_minutes: 5
        }}
      >
        <div style={{ display: 'flex', height: `calc(100vh - ${HEADER_HEIGHT}px)` }}>
          {/* Left Column: TOC + Title + Editor */}
          <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
            {/* TOC Sidebar - space always reserved on wide screens, content shown when available */}
            {windowWidth >= 1400 && (
              <aside
                style={{
                  width: '240px',
                  overflow: 'auto',
                  padding: '24px 32px',
                  position: 'relative',
                  marginTop: '90px',
                  flexShrink: 0
                }}
              >
                {tableOfContents.length > 0 ? (
                  <>
                    <div
                      style={{
                        position: 'sticky',
                        top: 0,
                        paddingBottom: '6px',
                        marginBottom: '12px',
                        borderBottom: '1px solid #f0f0f0'
                      }}
                    >
                      <Typography.Text strong style={{ fontSize: '13px' }}>
                        {t`Table of Contents`}
                      </Typography.Text>
                    </div>
                    <nav>
                      <ul style={{ listStyle: 'none', padding: '0 0 0 8px', margin: 0 }}>
                        {tableOfContents.map((anchor) => (
                          <li
                            key={anchor.id}
                            style={{
                              marginBottom: '8px',
                              paddingLeft: `${(anchor.level - 2) * 12}px`
                            }}
                          >
                            <button
                              type="button"
                              onClick={() => handleTOCClick(anchor)}
                              style={{
                                all: 'unset',
                                cursor: 'pointer',
                                fontSize: anchor.level === 2 ? '13px' : '12px',
                                fontWeight: anchor.level === 2 ? 500 : 400,
                                color: anchor.isActive ? '#1677ff' : 'rgba(0, 0, 0, 0.65)',
                                display: 'block',
                                transition: 'color 0.15s ease-in-out',
                                width: '100%',
                                textAlign: 'left',
                                padding: '4px 0',
                                lineHeight: '1.4'
                              }}
                              onMouseEnter={(e) => {
                                e.currentTarget.style.color = '#1677ff'
                              }}
                              onMouseLeave={(e) => {
                                e.currentTarget.style.color = anchor.isActive
                                  ? '#1677ff'
                                  : 'rgba(0, 0, 0, 0.65)'
                              }}
                            >
                              {anchor.textContent}
                            </button>
                          </li>
                        ))}
                      </ul>
                    </nav>
                  </>
                ) : null}
              </aside>
            )}

            {/* Content Container - centered, max-width 768px */}
            <div
              style={{ flex: 1, overflow: 'auto', display: 'flex', justifyContent: 'center' }}
              data-editor-container
            >
              <div style={{ width: '100%', maxWidth: '768px', padding: '24px' }}>
                <Form.Item
                  name="title"
                  label={t`Title`}
                  rules={[
                    { required: true, message: t`Please enter a post title` },
                    { max: 500, message: t`Title must be less than 500 characters` }
                  ]}
                  style={{ marginBottom: '16px' }}
                >
                  <Input placeholder={t`Post title`} onChange={handleTitleChange} size="large" />
                </Form.Item>

                {templateLoading || (post && !blogContent) ? (
                  <div className="flex items-center justify-center h-full">
                    <Space direction="vertical" align="center">
                      <div>{t`Loading template...`}</div>
                    </Space>
                  </div>
                ) : (
                  <NotifuseEditor
                    key={`editor-${post?.id || 'new'}-${post?.settings.template.template_id || 'no-template'}-${post?.settings.template.template_version || 0}-${editorKeyCounter}`}
                    ref={editorRef}
                    placeholder={t`Start writing your blog post...`}
                    initialContent={
                      blogContent
                        ? jsonToHtml(blogContent)
                        : !post && import.meta.env.DEV
                          ? DEFAULT_INITIAL_CONTENT
                          : ''
                    }
                    disableH1={true}
                    showHeader={false}
                    onChange={handleContentChange}
                    onTableOfContentsUpdate={handleTOCUpdate}
                  />
                )}
              </div>
            </div>
          </div>

          {/* Right Column: Settings */}
          <div
            style={{
              width: '450px',
              maxWidth: '450px',
              borderLeft: '1px solid #f0f0f0',
              overflow: 'auto',
              padding: '24px',
              paddingBottom: '120px'
            }}
          >
            <Form.Item
              name="slug"
              label={t`Slug`}
              rules={[
                { required: true, message: t`Please enter a slug` },
                {
                  pattern: /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
                  message: t`Slug must contain only lowercase letters, numbers, and hyphens`
                },
                { max: 100, message: t`Slug must be less than 100 characters` }
              ]}
              extra={getFullPostUrl()}
            >
              <Input placeholder="post-slug" disabled={isEditMode} onChange={handleSlugChange} />
            </Form.Item>

            <Row gutter={16}>
              <Col span={16}>
                <Form.Item
                  name="category_id"
                  label={t`Category`}
                  rules={[{ required: true, message: t`Please select a category` }]}
                >
                  <Select
                    placeholder={t`Select a category`}
                    options={(categoriesData?.categories ?? []).map((cat) => ({
                      label: cat.settings.name,
                      value: cat.id
                    }))}
                  />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item
                  name="reading_time_minutes"
                  label={t`Reading Time`}
                  rules={[{ required: true, message: t`Please enter reading time` }]}
                >
                  <InputNumber style={{ width: '100%' }} min={1} max={120} suffix={t`min`} />
                </Form.Item>
              </Col>
            </Row>

            <Form.Item
              name="authors"
              label={t`Authors`}
              required
              rules={[
                {
                  required: true,
                  message: t`Please add at least one author`,
                  type: 'array',
                  min: 1
                }
              ]}
            >
              <AuthorsTable />
            </Form.Item>

            <Form.Item
              name="excerpt"
              label={t`Excerpt`}
              extra={t`Brief summary shown in post listings and previews`}
            >
              <TextArea
                rows={3}
                placeholder={t`Brief summary of the post`}
                showCount
                maxLength={500}
              />
            </Form.Item>

            <Form.Item name="featured_image_url" label={t`Featured Image URL`}>
              <ImageURLInput />
            </Form.Item>

            <SEOSettingsForm namePrefix={['seo']} />
          </div>
        </div>
      </Form>

      <BlogAIAssistant
        workspace={workspace}
        onUpdateContent={handleAIUpdateContent}
        onUpdateMetadata={handleAIUpdateMetadata}
        currentContent={blogContent}
        currentMetadata={getCurrentMetadata()}
      />
    </Drawer>
  )
}
