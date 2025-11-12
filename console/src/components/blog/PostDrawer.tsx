import { useEffect, useState, useMemo } from 'react'
import { Button, Drawer, Form, Input, App, Select, InputNumber, Space, Tabs, Row, Col } from 'antd'
import { useMutation, useQueryClient, useQuery } from '@tanstack/react-query'
import { debounce } from 'lodash'
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
import { BlogContentEditor, jsonToHtml, extractTextContent } from '../blog_editor'
import { AuthorsTable } from './AuthorsTable'
import Subtitle from '../common/subtitle'

const { TextArea } = Input

interface PostDrawerProps {
  open: boolean
  onClose: () => void
  post?: BlogPost | null
  workspaceId: string
  initialCategoryId?: string | null
}

const HEADER_HEIGHT = 66

export function PostDrawer({
  open,
  onClose,
  post,
  workspaceId,
  initialCategoryId
}: PostDrawerProps) {
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const { message, modal } = App.useApp()
  const isEditMode = !!post
  const [tab, setTab] = useState<string>('settings')
  const [formTouched, setFormTouched] = useState(false)
  const [loading, setLoading] = useState(false)

  // Blog content state (Tiptap JSON)
  const [blogContent, setBlogContent] = useState<any>(null)

  // Auto-save state
  const [lastSaved, setLastSaved] = useState<Date | null>(null)
  const [isSaving, setIsSaving] = useState(false)

  // Template ID for new posts (generated on mount)
  const [newTemplateId] = useState<string>(() => crypto.randomUUID())

  // localStorage key for drafts
  const draftKey = `blog-post-draft-${post?.id || 'new'}-${workspaceId}`

  // Check if content is empty
  const isContentEmpty = (content: any): boolean => {
    if (!content) return true
    if (!content.content || content.content.length === 0) return true

    // Check if all content nodes are empty paragraphs
    return content.content.every((node: any) => {
      if (node.type === 'paragraph') {
        return !node.content || node.content.length === 0
      }
      return false
    })
  }

  // Debounced save to localStorage
  const debouncedLocalSave = useMemo(
    () =>
      debounce((content: any) => {
        // Don't save if content is empty
        if (isContentEmpty(content)) {
          // Remove any existing draft if content becomes empty
          localStorage.removeItem(draftKey)
          setLastSaved(null)
          setIsSaving(false)
          return
        }

        try {
          setIsSaving(true)
          localStorage.setItem(
            draftKey,
            JSON.stringify({
              content,
              savedAt: new Date().toISOString()
            })
          )
          setLastSaved(new Date())
          setIsSaving(false)
        } catch (e) {
          console.error('Failed to save draft:', e)
          setIsSaving(false)
        }
      }, 1000),
    [draftKey]
  )

  // Handle content change with auto-save
  const handleContentChange = (json: any) => {
    setBlogContent(json)

    // Only save if content is not empty
    if (!isContentEmpty(json)) {
      setIsSaving(true)
      debouncedLocalSave(json)
    }

    setFormTouched(true)
  }

  // Fetch categories for dropdown
  const { data: categoriesData } = useQuery({
    queryKey: ['blog-categories', workspaceId],
    queryFn: () => blogCategoriesApi.list(workspaceId),
    enabled: open
  })

  // Fetch template for editing existing posts
  const { data: templateData, isLoading: templateLoading } = useQuery({
    queryKey: ['template', workspaceId, post?.settings.template.template_id],
    queryFn: () =>
      templatesApi.get({
        workspace_id: workspaceId,
        id: post!.settings.template.template_id,
        version: post!.settings.template.template_version
      }),
    enabled: isEditMode && !!post && open
  })

  // Load form values and blog content
  useEffect(() => {
    if (open && post) {
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

      // Load template content
      if (templateData?.template?.web?.content) {
        setBlogContent(templateData.template.web.content)
      }
    } else if (open && !post) {
      // New post - try to load from localStorage
      const savedDraft = localStorage.getItem(draftKey)
      if (savedDraft) {
        try {
          const { content, savedAt } = JSON.parse(savedDraft)
          modal.confirm({
            title: 'Restore Draft?',
            content: `Found unsaved changes from ${new Date(savedAt).toLocaleString()}`,
            okText: 'Yes',
            cancelText: 'No',
            onOk: () => setBlogContent(content),
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
    }
  }, [open, post, form, templateData, draftKey, initialCategoryId, message, modal])

  const createMutation = useMutation({
    mutationFn: async (values: any) => {
      setIsSaving(true)

      // Get HTML and plain text from content
      const html = jsonToHtml(blogContent)
      const plainText = extractTextContent(blogContent)

      // First, create the template
      const templateCreateResponse = await templatesApi.create({
        workspace_id: workspaceId,
        id: newTemplateId,
        name: `Blog: ${values.title}`,
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

      return blogPostsApi.create(workspaceId, createRequest)
    },
    onSuccess: () => {
      message.success('Post created successfully')
      localStorage.removeItem(draftKey)
      setLastSaved(new Date())
      setIsSaving(false)
      queryClient.invalidateQueries({ queryKey: ['blog-posts', workspaceId] })
      handleClose()
    },
    onError: (error: any) => {
      message.error(`Failed to create post: ${error.message}`)
      setLoading(false)
      setIsSaving(false)
    }
  })

  const updateMutation = useMutation({
    mutationFn: async (values: any) => {
      setIsSaving(true)

      // Get HTML and plain text from content
      const html = jsonToHtml(blogContent)
      const plainText = extractTextContent(blogContent)

      // First, update the template (backend creates new version)
      await templatesApi.update({
        workspace_id: workspaceId,
        id: post!.settings.template.template_id,
        name: `Blog: ${values.title}`,
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
        workspace_id: workspaceId,
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

      return blogPostsApi.update(workspaceId, updateRequest)
    },
    onSuccess: () => {
      message.success('Post updated successfully')
      localStorage.removeItem(draftKey)
      setLastSaved(new Date())
      setIsSaving(false)
      queryClient.invalidateQueries({ queryKey: ['blog-posts', workspaceId] })
      handleClose()
    },
    onError: (error: any) => {
      message.error(`Failed to update post: ${error.message}`)
      setLoading(false)
      setIsSaving(false)
    }
  })

  const handleTitleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (isEditMode) return // Don't update slug in edit mode

    const title = e.target.value
    const slug = normalizeSlug(title)
    form.setFieldsValue({ slug })
  }

  const handleClose = () => {
    if (formTouched && !loading && !createMutation.isPending && !updateMutation.isPending) {
      modal.confirm({
        title: 'Unsaved changes',
        content: 'You have unsaved changes. Are you sure you want to close this drawer?',
        okText: 'Yes',
        cancelText: 'No',
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
    setTab('settings')
    setBlogContent(null)
  }

  const goNext = () => {
    setTab('content')
  }

  const onFinish = (values: any) => {
    console.log('values', values)

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
      title={isEditMode ? 'Edit Post' : 'Create New Post'}
      width={tab === 'content' ? '100%' : 1000}
      onClose={handleClose}
      open={open}
      keyboard={false}
      maskClosable={false}
      className={'drawer-no-transition drawer-body-no-padding'}
      extra={
        <div className="text-right">
          <Space>
            <Button type="link" loading={loading} onClick={handleClose}>
              Cancel
            </Button>

            {tab === 'settings' && (
              <Button type="primary" onClick={goNext}>
                Next
              </Button>
            )}

            {tab === 'content' && (
              <>
                <Button type="primary" ghost onClick={() => setTab('settings')}>
                  Previous
                </Button>
                <Button
                  loading={loading || createMutation.isPending || updateMutation.isPending}
                  onClick={() => {
                    form.submit()
                  }}
                  type="primary"
                >
                  {isEditMode ? 'Save' : 'Create'}
                </Button>
              </>
            )}
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
            const firstErrorField = info.errorFields[0].name[0]

            if (
              firstErrorField === 'title' ||
              firstErrorField === 'slug' ||
              firstErrorField === 'category_id' ||
              firstErrorField === 'authors' ||
              firstErrorField === 'reading_time_minutes' ||
              firstErrorField === 'excerpt' ||
              firstErrorField === 'featured_image_url' ||
              firstErrorField === 'seo'
            ) {
              setTab('settings')
            }

            message.error('Please check the form for errors.')
          }
          setLoading(false)
        }}
        onValuesChange={() => {
          setFormTouched(true)
        }}
        initialValues={{
          authors: [],
          reading_time_minutes: 5
        }}
      >
        {tab === 'settings' && (
          <>
            <div className="flex justify-center">
              <Tabs
                activeKey={tab}
                centered
                onChange={(k) => setTab(k)}
                style={{ display: 'inline-block' }}
                className="tabs-in-header"
                items={[
                  {
                    key: 'settings',
                    label: '1. Settings'
                  },
                  {
                    key: 'content',
                    label: '2. Content'
                  }
                ]}
              />
            </div>
            <div className="relative">
              <div className="p-8">
                <Row gutter={32}>
                  <Col span={12}>
                    <Subtitle className="mb-6" borderBottom primary>
                      Settings
                    </Subtitle>
                    <Form.Item
                      name="title"
                      label="Title"
                      rules={[
                        { required: true, message: 'Please enter a post title' },
                        { max: 500, message: 'Title must be less than 500 characters' }
                      ]}
                    >
                      <Input placeholder="Post title" onChange={handleTitleChange} />
                    </Form.Item>

                    <Form.Item
                      name="slug"
                      label="Slug"
                      rules={[
                        { required: true, message: 'Please enter a slug' },
                        {
                          pattern: /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
                          message: 'Slug must contain only lowercase letters, numbers, and hyphens'
                        },
                        { max: 100, message: 'Slug must be less than 100 characters' }
                      ]}
                      extra="URL-friendly identifier (lowercase, hyphens only)"
                    >
                      <Input placeholder="post-slug" disabled={isEditMode} />
                    </Form.Item>

                    <Row gutter={16}>
                      <Col span={16}>
                        <Form.Item
                          name="category_id"
                          label="Category"
                          rules={[{ required: true, message: 'Please select a category' }]}
                        >
                          <Select
                            placeholder="Select a category"
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
                          label="Reading Time"
                          rules={[{ required: true, message: 'Please enter reading time' }]}
                        >
                          <InputNumber style={{ width: '100%' }} min={1} max={120} suffix="min" />
                        </Form.Item>
                      </Col>
                    </Row>

                    <Form.Item
                      name="authors"
                      label="Authors"
                      required
                      rules={[
                        {
                          required: true,
                          message: 'Please add at least one author',
                          type: 'array',
                          min: 1
                        }
                      ]}
                    >
                      <AuthorsTable />
                    </Form.Item>

                    <Form.Item
                      name="excerpt"
                      label="Excerpt"
                      extra="Brief summary shown in post listings and previews"
                    >
                      <TextArea
                        rows={3}
                        placeholder="Brief summary of the post"
                        showCount
                        maxLength={500}
                      />
                    </Form.Item>

                    <Form.Item name="featured_image_url" label="Featured Image URL">
                      <ImageURLInput />
                    </Form.Item>
                  </Col>

                  <Col span={12}>
                    <Form.Item name="seo" noStyle>
                      <SEOSettingsForm />
                    </Form.Item>
                  </Col>
                </Row>
              </div>
            </div>
          </>
        )}

        {tab === 'content' && (
          <>
            <div className="flex justify-center">
              <Tabs
                activeKey={tab}
                centered
                onChange={(k) => setTab(k)}
                style={{ display: 'inline-block' }}
                className="tabs-in-header"
                items={[
                  {
                    key: 'settings',
                    label: '1. Settings'
                  },
                  {
                    key: 'content',
                    label: '2. Content'
                  }
                ]}
              />
            </div>
            <div className="relative">
              {templateLoading ? (
                <div className="flex items-center justify-center h-full">
                  <Space direction="vertical" align="center">
                    <div>Loading template...</div>
                  </Space>
                </div>
              ) : (
                <div style={{ height: `calc(100vh - ${HEADER_HEIGHT}px)`, overflow: 'auto' }}>
                  <BlogContentEditor
                    content={blogContent}
                    onChange={handleContentChange}
                    placeholder="Start writing your blog post..."
                    // minHeight={`calc(100vh - ${HEADER_HEIGHT + 120}px)`}
                    isSaving={isSaving}
                    lastSaved={lastSaved}
                  />
                </div>
              )}
            </div>
          </>
        )}
      </Form>
    </Drawer>
  )
}
