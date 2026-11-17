import { useEffect, useState, useMemo, useRef } from 'react'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import { Button, Form, Input, App, Select, InputNumber, Segmented, Space, Tooltip } from 'antd'
import { useMutation, useQueryClient, useQuery } from '@tanstack/react-query'
import { debounce } from 'lodash'
import { ExternalLink, FileText, ImageOff, Lock, Undo, Redo } from 'lucide-react'
import {
  blogPostsApi,
  blogCategoriesApi,
  normalizeSlug,
  BlogPost,
  BlogAuthor
} from '../../services/api/blog'
import type { CreateBlogPostRequest, UpdateBlogPostRequest } from '../../services/api/blog'
import { SearchEnginesCard, SocialShareCard } from '../seo/SeoCards'
import { SeoPreview } from '../seo/SeoPreview'
import { FullDrawer, Panel, Pill, useRelativeDate } from '../layout'
import { blogBaseUrl } from './category/categorySeo'
import { countWords, effectivePostSeo, type PostDraft } from './post/postSeo'
import { PostChecks } from './post/PostChecks'
import { usePostChecks } from './post/usePostChecks'
import { postState } from './postState'
import { docKey } from './post/docKey'
import { defaultEditorStyles } from '../blog_editor/config/defaultEditorStyles'
import { ImageURLInput } from '../common/ImageURLInput'
import { templatesApi } from '../../services/api/template'
import { ApiError } from '../../services/api/client'
import { useTemplateConflictModal } from '../templates/useTemplateConflictModal'
import { AuthorsField } from './AuthorsField'
import {
  NotifuseEditor,
  type NotifuseEditorRef,
  type TOCAnchor
} from '../blog_editor'
import { jsonToHtml, extractTextContent } from './utils'
import type { Workspace } from '../../services/api/types'
import { BlogAIAssistant } from './BlogAIAssistant'
import { blogTemplateName } from './blogTemplateName'
import dayjs from '../../lib/dayjs'

// TiptapNode shape matching the structure expected by utils.
// Declared as a type alias rather than an interface on purpose: the editor and the AI
// assistant both exchange documents as `Record<string, unknown>`, and only a type
// alias carries the implicit index signature that makes the two interchangeable.
type TiptapNode = {
  type: string
  text?: string
  content?: TiptapNode[]
  marks?: Array<{
    type: string
    attrs?: Record<string, unknown>
  }>
  attrs?: Record<string, unknown>
}

// Shallow check: a Tiptap document is identified by its string `type`, which is what
// every consumer here branches on.
function isTiptapNode(value: unknown): value is TiptapNode {
  return typeof value === 'object' && value !== null && 'type' in value && typeof value.type === 'string'
}

const { TextArea } = Input

// Type alias rather than an interface: antd hands these values to helpers typed as
// `Record<string, unknown>`, which only a type alias satisfies.
type PostFormValues = {
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

/** Details first (title, category, tags), then the text: any step opens at any time. */
type PostStep = 'details' | 'write'

export function PostDrawer({ open, onClose, post, workspace, initialCategoryId }: PostDrawerProps) {
  const { t } = useLingui()
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const { message, modal } = App.useApp()
  const { conflictModal, showConflict } = useTemplateConflictModal()
  const isEditMode = !!post
  const [formTouched, setFormTouched] = useState(false)
  const [loading, setLoading] = useState(false)

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

  // The template revision this post's editor content is based on. Sent as base_version
  // so the server rejects a stale-base overwrite (409) from a concurrent editor.
  const baseVersionRef = useRef<number>(post?.settings.template.template_version ?? 0)
  // Last submitted form values, so the conflict dialog can re-submit ("overwrite").
  const lastValuesRef = useRef<PostFormValues | null>(null)

  // The text as loaded, to tell an edit from the editor's own normalising.
  const loadedDocRef = useRef<string | null>(null)
  // Someone typed, clicked or pasted in the text since the editor (re)mounted.
  const interactedRef = useRef(false)
  // Read by the editor's change handler, which may hold an older render's closure.
  const touchedRef = useRef(false)

  // Table of Contents state
  const [tableOfContents, setTableOfContents] = useState<TOCAnchor[]>([])

  // Editor key counter - increment to force editor remount when restoring draft
  const [editorKeyCounter, setEditorKeyCounter] = useState(0)

  const [step, setStep] = useState<PostStep>('details')
  // A new post's slug follows its title until the slug is typed in.
  const [slugEdited, setSlugEdited] = useState(false)
  const relative = useRelativeDate()

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

    // The editor emits changes of its own as it loads a text (default attributes, the headings'
    // table-of-contents ids, whatever an older editor or the API wrote differently): until someone
    // types, clicks or pastes in it, what it emits is the text as loaded. After that, only a text
    // that differs from it counts, so an untouched post opens without « Unsaved changes ».
    if (touchedRef.current) return
    const key = docKey(tiptapNode)
    if (!interactedRef.current) loadedDocRef.current = key
    else if (key !== loadedDocRef.current) setFormTouched(true)
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

  // Fetch categories for dropdown
  const { data: categoriesData } = useQuery({
    queryKey: ['blog-categories', workspace.id],
    queryFn: () => blogCategoriesApi.list(workspace.id),
    enabled: open
  })

  // The authors of the latest posts, offered when one is added: a post's authors are typed per
  // post, so this is how the blog's regular authors come back with their photo. Under the posts'
  // key, so a saved post refreshes it.
  const { data: recentPosts } = useQuery({
    queryKey: ['blog-posts', workspace.id, 'authors'],
    queryFn: () => blogPostsApi.list(workspace.id, { status: 'all', limit: 100 }),
    enabled: open,
    staleTime: 60_000
  })
  const knownAuthors = useMemo(
    () => (recentPosts?.posts ?? []).flatMap((recent) => recent.settings.authors ?? []),
    [recentPosts]
  )

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
              content: t`Found unsaved changes from ${dayjs(savedAt).format('lll')}`,
              okText: t`Yes`,
              cancelText: t`No`,
              onOk: () => {
                // Restore editor content: unsaved, so closing asks first
                setBlogContent(content)
                setFormTouched(true)
                // Force editor remount with restored content
                setEditorKeyCounter((prev) => prev + 1)
                // Restore form values (title, excerpt, SEO, etc.)
                if (formValues) {
                  form.setFieldsValue(formValues)
                  // 3. A slug of its own (not the title's) keeps being its own after the restore.
                  const draftSlug = typeof formValues.slug === 'string' ? formValues.slug : ''
                  const draftTitle = typeof formValues.title === 'string' ? formValues.title : ''
                  if (draftSlug && draftSlug !== normalizeSlug(draftTitle)) setSlugEdited(true)
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
      }
      // Reset loading state when drawer opens
      setLoading(false)
      setStep('details')
      setSlugEdited(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, post?.id, form, draftKey, initialCategoryId, modal])

  // Load template content when template data is ready
  useEffect(() => {
    if (!open || !post) {
      return
    }

    // Only set content when template data matches the current post exactly. A post at version 0
    // (what the API stores when template_version is left out) renders the latest version, and
    // templates.get answers with that version's own number, never 0.
    const pinnedVersion = post.settings.template.template_version
    if (
      !templateLoading &&
      templateData?.template?.web?.content &&
      templateData.template.id === post.settings.template.template_id &&
      (pinnedVersion === 0 || templateData.template.version === pinnedVersion)
    ) {
      setBlogContent(templateData.template.web.content as TiptapNode)
      loadedDocRef.current = docKey(templateData.template.web.content)
      // The editor now holds this exact revision; that's the base for the next save.
      baseVersionRef.current = templateData.template.version
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

  // Discard local edits and load the latest template revision into the editor, so the
  // user can re-apply their changes on top of the newer version after a conflict.
  const reloadLatestTemplate = async () => {
    if (!post) return
    try {
      const { template: latest } = await templatesApi.get({
        workspace_id: workspace.id,
        id: post.settings.template.template_id
        // version omitted → latest
      })
      baseVersionRef.current = latest.version
      setBlogContent((latest.web?.content as TiptapNode) ?? null)
      loadedDocRef.current = docKey(latest.web?.content)
      setEditorKeyCounter((prev) => prev + 1)
    } catch {
      // Surface the failure and leave the editor untouched, so the user isn't misled into
      // thinking they're now on the latest revision when the reload didn't happen.
      message.error(t`Could not load the latest version. Please try again.`)
    }
  }

  const createMutation = useMutation({
    mutationFn: async (values: PostFormValues) => {
      // Get HTML and plain text from content
      const html = editorRef.current?.getHTML() || jsonToHtml(blogContent)
      const plainText = extractTextContent(blogContent)

      // First, create the template
      const templateName = blogTemplateName(values.title)
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
      // Remember the payload so the conflict dialog can re-submit it verbatim.
      lastValuesRef.current = values

      // Get HTML and plain text from content
      const html = editorRef.current?.getHTML() || jsonToHtml(blogContent)
      const plainText = extractTextContent(blogContent)

      // First, update the template (backend creates new version)
      const templateName = blogTemplateName(values.title)
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
        },
        // Base revision the content was loaded from; server rejects a stale-base save.
        base_version: baseVersionRef.current
      })

      // Fetch the updated template to get the new version
      const updatedTemplate = await templatesApi.get({
        workspace_id: workspace.id,
        id: post!.settings.template.template_id
      })
      // Advance the base to the just-saved revision so a subsequent save (e.g. an
      // "overwrite" retry, or reopening the drawer) isn't a false conflict on our own edit.
      baseVersionRef.current = updatedTemplate.template.version

      // Then update the blog post
      const updateRequest: UpdateBlogPostRequest = {
        id: post!.id,
        category_id: values.category_id,
        slug: values.slug,
        title: values.title,
        template_id: post!.settings.template.template_id,
        // A post following the latest version keeps doing so: pinning it here would leave the
        // template edits made later through the API off the page.
        template_version:
          post!.settings.template.template_version === 0 ? 0 : updatedTemplate.template.version,
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
      // 409: the template advanced past our base while editing. Let the user keep their
      // work (overwrite) or load the latest (discard theirs) rather than losing changes.
      if (error instanceof ApiError && error.status === 409) {
        const latest = (error.data as { latest_version?: number } | undefined)?.latest_version
        showConflict({
          latestVersion: latest,
          baseVersion: baseVersionRef.current,
          onOverwrite: () => {
            // Adopt the server's latest as the new base and re-submit. Fall back to 0
            // (skip the check, force the write) when the version is unknown, so
            // "overwrite" can never loop on a conflict it cannot resolve.
            baseVersionRef.current = latest ?? 0
            if (lastValuesRef.current) {
              updateMutation.mutate(lastValuesRef.current)
            }
          },
          onReload: () => {
            void reloadLatestTemplate()
          }
        })
        setLoading(false)
        return
      }
      message.error(t`Failed to update post: ${error.message}`)
      setLoading(false)
    }
  })

  useEffect(() => {
    touchedRef.current = formTouched
  }, [formTouched])

  // The values as typed, for the bar, the checks and the previews. Drafts are set with
  // setFieldsValue, which the watch follows; it answers undefined before the first render.
  const watched = (Form.useWatch([], { form, preserve: true }) ?? {}) as Partial<PostFormValues>
  const categories = categoriesData?.categories ?? []
  const selectedCategory = categories.find((cat) => cat.id === watched.category_id)
  // 2. The category the post is saved under: its public address until the change is saved.
  const savedCategory = post ? categories.find((cat) => cat.id === post.category_id) : undefined

  const handleTitleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (isEditMode || slugEdited) return // The slug no longer follows the title

    const title = e.target.value
    const slug = normalizeSlug(title)
    form.setFieldsValue({ slug })
  }

  const handleSlugChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    // Normalize spaces and non-allowed characters to hyphens
    const normalized = value
      .toLowerCase()
      .replace(/\s+/g, '-') // Replace spaces with hyphens
      .replace(/[^a-z0-9-]/g, '-') // Replace non-allowed characters with hyphens
      .replace(/-+/g, '-') // Replace multiple consecutive hyphens with a single hyphen
      .replace(/^-+/, '') // A slug starts with a letter or digit
    // The trailing hyphen stays while typing (« my- » before « my-post »); leaving the field trims it.

    setSlugEdited(true)
    form.setFieldsValue({ slug: normalized })
  }

  const handleSlugBlur = () => {
    const slug = form.getFieldValue('slug')
    if (typeof slug === 'string' && /-+$/.test(slug)) form.setFieldsValue({ slug: slug.replace(/-+$/, '') })
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
    if (editorRef.current && isTiptapNode(json) && json.type === 'doc') {
      editorRef.current.setContent(json)
      setBlogContent(json)
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
      // Also update slug for new posts, until it is typed in
      if (!isEditMode && !slugEdited) {
        const newSlug = normalizeSlug(metadata.title)
        updates.slug = newSlug
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
    // The form has no indexing field (the theme writes the blog's robots tag, never a post's),
    // and antd submits only the fields it shows: keep the one the post already has.
    const robots = post?.settings.seo?.meta_robots
    values.seo = { ...values.seo, ...(robots ? { meta_robots: robots } : {}) }

    if (isEditMode) {
      updateMutation.mutate(values)
    } else {
      createMutation.mutate(values)
    }
  }

  const editorKey = `editor-${post?.id || 'new'}-${post?.settings.template.template_id || 'no-template'}-${post?.settings.template.template_version || 0}-${editorKeyCounter}`
  // Each mount of the editor starts untouched: what it emits first is the text as loaded.
  useEffect(() => {
    interactedRef.current = false
  }, [editorKey])
  // The editor reads its initial content once, when it mounts: not rebuilt on every keystroke.
  const initialHtml = useMemo(
    () => (blogContent ? jsonToHtml(blogContent) : ''),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- on a mount, and when the text first arrives
    [editorKey, blogContent === null]
  )
  const markInteracted = () => {
    interactedRef.current = true
  }

  const draft: PostDraft = {
    title: watched.title,
    slug: watched.slug,
    category_id: watched.category_id,
    excerpt: watched.excerpt,
    featured_image_url: watched.featured_image_url,
    seo: watched.seo as PostDraft['seo']
  }
  const category = selectedCategory
    ? { name: selectedCategory.settings.name, description: selectedCategory.settings.description, seo: selectedCategory.settings.seo }
    : undefined
  const defaults = { workspaceName: workspace.name, seo: workspace.settings.blog_settings?.seo }
  const effective = effectivePostSeo(draft, category, defaults)
  // What an empty field falls back to, for its placeholder.
  const fallback = effectivePostSeo({ ...draft, seo: {} }, category, defaults)
  const fallbackTitle = fallback.title
  const fallbackShareTitle = fallback.ogTitle
  const hasExcerpt = !!draft.excerpt?.trim()
  const descriptionFallback = hasExcerpt
    ? t`Defaults to the excerpt`
    : fallback.description
      ? t`Defaults to the category's or the blog's description`
      : undefined
  const imagePlaceholder = draft.featured_image_url?.trim()
    ? t`Defaults to the featured image`
    : fallback.ogImage
      ? t`Defaults to the category's or the blog's share image`
      : 'https://example.com/image.jpg'

  const checks = usePostChecks(draft, effective)
  const toCheck = checks.filter((check) => !check.ok).length
  const words = useMemo(() => countWords(extractTextContent(blogContent)), [blogContent])

  const baseUrl = blogBaseUrl(workspace.settings.custom_endpoint_url, window.API_ENDPOINT)
  const domain = baseUrl.replace(/^https?:\/\//, '')
  const categorySlug = selectedCategory?.slug ?? ''
  const slug = draft.slug?.trim() ?? ''
  const categoryFallback = t`category`
  const state = post ? postState(post) : undefined
  const saving = loading || createMutation.isPending || updateMutation.isPending

  const showField = (path: string[]) => {
    setStep('details')
    // The Details step is shown on the next frame; its field can then take the focus.
    window.requestAnimationFrame(() => {
      form.scrollToField(path, { block: 'center', behavior: 'smooth' })
      form.focusField(path)
    })
  }

  const barTitle = (
    <>
      <span className="truncate text-sm font-semibold text-ink">
        {draft.title?.trim() || (post ? post.settings.title : t`New post`)}
      </span>
      {!post ? (
        <Pill tone="primary">{t`Not created`}</Pill>
      ) : state === 'published' ? (
        <Pill tone="success">{t`Published`}</Pill>
      ) : state === 'scheduled' ? (
        <Pill tone="info">{t`Scheduled`}</Pill>
      ) : (
        <Pill tone="neutral">{t`Draft`}</Pill>
      )}
      {formTouched ? (
        <Pill tone="warning">{t`Unsaved changes`}</Pill>
      ) : post ? (
        <span className="shrink-0 text-xs text-ink-3">{t`Edited ${relative(post.updated_at)}`}</span>
      ) : null}
    </>
  )

  const stepLabel = (text: string, key: PostStep) => (
    <span className="inline-flex items-center gap-1.5" data-step={key}>
      {text}
      {key === 'details' && toCheck > 0 ? (
        <>
          <span aria-hidden="true" className="size-1.5 rounded-full bg-warning-fg" data-step-checks />
          <span className="sr-only">{t`(has checks left)`}</span>
        </>
      ) : null}
    </span>
  )

  const topActions = (
    <>
      {step === 'write' ? (
        <>
          <Tooltip title={t`Undo`}>
            <Button
              type="text"
              aria-label={t`Undo`}
              icon={<Undo size={14} aria-hidden="true" />}
              onClick={() => editorRef.current?.undo()}
              disabled={!canUndo}
            />
          </Tooltip>
          <Tooltip title={t`Redo`}>
            <Button
              type="text"
              aria-label={t`Redo`}
              icon={<Redo size={14} aria-hidden="true" />}
              onClick={() => editorRef.current?.redo()}
              disabled={!canRedo}
            />
          </Tooltip>
        </>
      ) : null}
      {post && state === 'published' && savedCategory ? (
        <Button
          type="text"
          icon={<ExternalLink size={14} aria-hidden="true" />}
          href={`${baseUrl}/${savedCategory.slug}/${post.slug}`}
          target="_blank"
          rel="noopener noreferrer"
        >
          {t`View post`}
        </Button>
      ) : null}
      <Button
        type="primary"
        loading={saving}
        // An untouched post has nothing to save; a new one is created from the defaults.
        disabled={isEditMode && !formTouched}
        onClick={() => form.submit()}
      >
        {isEditMode ? t`Save changes` : t`Create post`}
      </Button>
    </>
  )

  const readingTime = watched.reading_time_minutes
  const authorNames = ((watched.authors ?? []) as BlogAuthor[]).map((author) => author.name).filter(Boolean)

  return (
    <>
      <FullDrawer
        open={open}
        onClose={handleClose}
        backLabel={t`Blog`}
        copyLink={false}
        // Escape belongs to the editor (menus, links): closing by accident would lose the text.
        keyboard={false}
        push={false}
        fill
        ariaLabel={isEditMode ? t`Edit post` : t`New post`}
        barTitle={barTitle}
        barCenter={
          <Segmented<PostStep>
            value={step}
            onChange={setStep}
            aria-label={t`Steps`}
            options={[
              { value: 'details', label: stepLabel(t`Details`, 'details') },
              { value: 'write', label: stepLabel(t`Write`, 'write') }
            ]}
          />
        }
        topActions={topActions}
      >
        <Form
          form={form}
          component={false}
          layout="vertical"
          onFinish={onFinish}
          onFinishFailed={(info) => {
            if (info.errorFields && info.errorFields.length > 0) {
              message.error(t`Please check the form for errors.`)
              // Every field is in Details: show the one to fix.
              setStep('details')
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
          {/* Both steps stay mounted: the editor keeps its text and history, the form its fields. */}
          <div data-post-step="details" hidden={step !== 'details'} className="min-h-0 flex-1 overflow-y-auto">
            <div className="flex flex-col gap-3 px-5 pt-4 pb-6 min-[1200px]:flex-row min-[1200px]:items-start">
              <div className="flex min-w-0 flex-1 flex-col gap-3">
                <Panel title={t`Post`} icon={<FileText size={14} aria-hidden="true" />}>
                  <Form.Item
                    name="title"
                    label={t`Title`}
                    rules={[
                      { required: true, message: t`Please enter a post title` },
                      { max: 500, message: t`Title must be less than 500 characters` }
                    ]}
                  >
                    <Input placeholder={t`Post title`} onChange={handleTitleChange} />
                  </Form.Item>
                  <div className="grid grid-cols-[minmax(0,1fr)_160px] gap-x-3">
                    <Form.Item
                      name="category_id"
                      label={t`Category`}
                      rules={[{ required: true, message: t`Please select a category` }]}
                    >
                      <Select
                        placeholder={t`Select a category`}
                        options={categories.map((cat) => ({
                          label: cat.settings.name,
                          value: cat.id
                        }))}
                      />
                    </Form.Item>
                    <Form.Item
                      name="reading_time_minutes"
                      label={t`Reading time`}
                      rules={[{ required: true, message: t`Please enter reading time` }]}
                    >
                      <InputNumber style={{ width: '100%' }} min={1} max={120} suffix={t`min`} />
                    </Form.Item>
                  </div>
                  <Form.Item
                    name="slug"
                    label={
                      isEditMode ? (
                        <span className="inline-flex items-center gap-1">
                          {t`Slug`}
                          <Lock size={12} aria-hidden="true" />
                        </span>
                      ) : (
                        t`Slug`
                      )
                    }
                    rules={[
                      { required: true, message: t`Please enter a slug` },
                      {
                        pattern: /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
                        message: t`Slug must contain only lowercase letters, numbers, and hyphens`
                      },
                      { max: 100, message: t`Slug must be less than 100 characters` }
                    ]}
                    extra={
                      isEditMode
                        ? t`Fixed once created: links to the post may already be shared`
                        : slugEdited
                          ? t`Lowercase letters, numbers and hyphens`
                          : t`Follows the title until you edit it`
                    }
                  >
                    <Input
                      prefix={<span className="text-ink-3">{`${domain}/${categorySlug || categoryFallback}/`}</span>}
                      placeholder="post-slug"
                      disabled={isEditMode}
                      onChange={handleSlugChange}
                      onBlur={handleSlugBlur}
                    />
                  </Form.Item>
                  <Form.Item
                    name="excerpt"
                    label={t`Excerpt`}
                    extra={t`Shown in post lists, and as the description when no meta description is set`}
                  >
                    <TextArea rows={3} placeholder={t`Brief summary of the post`} showCount maxLength={500} />
                  </Form.Item>
                  <div className="flex items-start gap-3">
                    <div className="min-w-0 flex-1">
                      <Form.Item
                        name="featured_image_url"
                        label={t`Featured image`}
                        extra={t`Also the share image when none is set`}
                      >
                        <ImageURLInput />
                      </Form.Item>
                    </div>
                    <div className="mt-7 w-28 shrink-0">
                      {draft.featured_image_url?.trim() ? (
                        <img
                          src={draft.featured_image_url}
                          alt={t`Featured image`}
                          className="aspect-[1.91/1] w-full rounded-sm border border-line object-cover"
                        />
                      ) : (
                        <div className="flex aspect-[1.91/1] w-full items-center justify-center rounded-sm border border-dashed border-line bg-subtle text-ink-4">
                          <ImageOff size={16} aria-label={t`No featured image`} />
                        </div>
                      )}
                    </div>
                  </div>
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
                    <AuthorsField knownAuthors={knownAuthors} />
                  </Form.Item>
                </Panel>
                <SearchEnginesCard
                  hint={t`How the post appears in search results`}
                  titlePlaceholder={t`Defaults to ${fallbackTitle}`}
                  descriptionPlaceholder={descriptionFallback ?? t`Brief description for search results`}
                  canonicalPlaceholder={`${baseUrl}/${categorySlug || categoryFallback}/${slug || 'post-slug'}`}
                />
                <SocialShareCard
                  hint={t`When a link to the post is shared`}
                  titlePlaceholder={t`Defaults to ${fallbackShareTitle}`}
                  descriptionPlaceholder={descriptionFallback}
                  imagePlaceholder={imagePlaceholder}
                  image={effective.ogImage}
                />
              </div>
              <aside className="flex shrink-0 flex-col gap-3 min-[1200px]:sticky min-[1200px]:top-4 min-[1200px]:w-aside">
                <PostChecks checks={checks} onShowField={showField} />
                <SeoPreview
                  effective={effective}
                  domain={domain}
                  path={[categorySlug, slug]}
                  iconUrl={workspace.settings.blog_settings?.icon_url || workspace.settings.logo_url || undefined}
                />
              </aside>
            </div>
          </div>

          <div
            data-post-step="write"
            hidden={step !== 'write'}
            className="min-h-0 flex-1 overflow-y-auto bg-surface"
            data-editor-container
            // Someone at work in the text: from now on, a change is theirs.
            onKeyDownCapture={markInteracted}
            onPointerDownCapture={markInteracted}
            onPasteCapture={markInteracted}
            onDropCapture={markInteracted}
          >
            <div className="mx-auto grid max-w-[1320px] grid-cols-1 gap-6 px-6 pt-7 pb-10 min-[1400px]:grid-cols-[200px_minmax(0,1fr)_200px]">
              <nav aria-label={t`Table of Contents`} className="hidden min-[1400px]:block">
                {tableOfContents.length > 0 ? (
                  <div className="sticky top-7 flex flex-col gap-1 pt-16">
                    <span className="border-b border-line-soft pb-1.5 text-overline font-semibold tracking-[0.06em] text-ink-3 uppercase">
                      {t`On this page`}
                    </span>
                    <ul className="m-0 flex list-none flex-col gap-0.5 p-0 pt-1">
                      {tableOfContents.map((anchor) => (
                        <li key={anchor.id} className={anchor.level > 2 ? 'pl-3' : undefined}>
                          <button
                            type="button"
                            onClick={() => handleTOCClick(anchor)}
                            className={`w-full cursor-pointer border-0 bg-transparent p-0 py-1 text-left ${
                              anchor.level === 2 ? 'text-sm' : 'text-xs'
                            } ${anchor.isActive ? 'font-medium text-primary-600' : 'text-ink-2 hover:text-ink'}`}
                          >
                            {anchor.textContent}
                          </button>
                        </li>
                      ))}
                    </ul>
                  </div>
                ) : null}
              </nav>

              <div className="mx-auto flex w-full max-w-[768px] min-w-0 flex-col">
                {/* The title is set in Details; here it heads the text as readers will see it, on
                    the editor's own 3rem margin so both start on the same line. */}
                <div className="flex flex-col gap-2 px-12">
                  <h1 className="m-0">
                    <button
                      type="button"
                      onClick={() => showField(['title'])}
                      title={t`Edit the title in Details`}
                      // The editor's headings font: its CSS variable lives inside the editor only.
                      style={{ fontFamily: defaultEditorStyles.headings.fontFamily }}
                      className="notifuse-editor-post-title cursor-pointer border-0 bg-transparent p-0 text-left text-ink"
                    >
                      {draft.title?.trim() || <span className="text-ink-4">{t`Untitled post`}</span>}
                    </button>
                  </h1>
                  <div className="flex flex-wrap items-center gap-2 text-xs text-ink-3">
                    {selectedCategory ? <Pill tone="neutral" dot={false}>{selectedCategory.settings.name}</Pill> : null}
                    {authorNames.length > 0 ? <span>{authorNames.join(', ')}</span> : null}
                    {readingTime ? <span>{t`${readingTime} min read`}</span> : null}
                  </div>
                </div>
                {templateLoading || (post && !blogContent) ? (
                  <div className="flex items-center justify-center py-16">
                    <Space orientation="vertical" align="center">
                      <div>{t`Loading template...`}</div>
                    </Space>
                  </div>
                ) : (
                  <NotifuseEditor
                    key={editorKey}
                    ref={editorRef}
                    placeholder={t`Start writing your blog post...`}
                    initialContent={initialHtml}
                    disableH1={true}
                    showHeader={false}
                    onChange={handleContentChange}
                    onTableOfContentsUpdate={handleTOCUpdate}
                  />
                )}
              </div>

              <aside aria-label={t`This post`} className="hidden min-[1400px]:block">
                <div className="sticky top-7 flex flex-col gap-2 pt-16">
                  <span className="text-overline font-semibold tracking-[0.06em] text-ink-3 uppercase">{t`This post`}</span>
                  <div className="flex justify-between border-t border-line-soft pt-2 text-xs text-ink-2">
                    <span>{t`Words`}</span>
                    <span className="font-semibold text-ink tabular-nums">{words}</span>
                  </div>
                  <div className="flex justify-between border-t border-line-soft pt-2 text-xs text-ink-2">
                    <span>{t`Search and share`}</span>
                    {toCheck > 0 ? (
                      <Button type="link" size="small" onClick={() => setStep('details')}>
                        {plural(toCheck, { one: '# to check', other: '# to check' })}
                      </Button>
                    ) : (
                      <span className="font-semibold text-success-fg">{t`All set`}</span>
                    )}
                  </div>
                </div>
              </aside>
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
      </FullDrawer>
      {conflictModal}
    </>
  )
}
