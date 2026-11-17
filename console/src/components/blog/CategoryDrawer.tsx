import { useState } from 'react'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import { App, Button, Dropdown, Form, Input } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ExternalLink, Lock, MoreHorizontal, Tag } from 'lucide-react'
import { blogCategoriesApi, normalizeSlug, type BlogCategory } from '../../services/api/blog'
import type { CreateBlogCategoryRequest, UpdateBlogCategoryRequest } from '../../services/api/blog'
import type { Workspace } from '../../services/api/types'
import { FullDrawer, Panel, Pill } from '../layout'
import { CategoryChecks } from './category/CategoryChecks'
import { SeoPreview } from '../seo/SeoPreview'
import { SearchEnginesCard, SocialShareCard } from '../seo/SeoCards'
import { blogBaseUrl, effectiveCategorySeo, sameDraft, type CategoryDraft } from './category/categorySeo'

const { TextArea } = Input

const SLUG_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

export interface CategoryDrawerProps {
  open: boolean
  onClose: () => void
  /** The category to edit; none to create one. */
  category?: BlogCategory | null
  workspace: Workspace
  /** Its posts, shown in the bar of an existing category. */
  postCount?: number
  /** Asks to delete the category; the page closes the drawer once it is gone. */
  onDelete?: (category: BlogCategory) => void
  /** A category was created: the page can open it. */
  onCreated?: (category: BlogCategory) => void
}

/** A category as the form holds it, every field present so a reset clears what was typed. */
function draftOf(category?: BlogCategory | null): CategoryDraft {
  const seo = category?.settings.seo
  return {
    name: category?.settings.name ?? '',
    slug: category?.slug ?? '',
    description: category?.settings.description ?? '',
    seo: {
      meta_title: seo?.meta_title ?? '',
      meta_description: seo?.meta_description ?? '',
      keywords: seo?.keywords ?? [],
      meta_robots: seo?.meta_robots,
      canonical_url: seo?.canonical_url ?? '',
      og_title: seo?.og_title ?? '',
      og_description: seo?.og_description ?? '',
      og_image: seo?.og_image ?? ''
    }
  }
}

/**
 * Creates or edits a blog category in a full-width drawer, laid out as the broadcast composer:
 * the category and its search and share tags on the left, their checks and a preview on the right.
 */
export function CategoryDrawer(props: CategoryDrawerProps) {
  if (!props.open) return null
  // A fresh form for each category, so nothing typed for one shows on the next.
  return <CategoryEditor key={props.category?.id ?? 'new'} {...props} />
}

function CategoryEditor({ onClose, category: opened, workspace, postCount, onDelete, onCreated }: CategoryDrawerProps) {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const [form] = Form.useForm<CategoryDraft>()
  // The category as last saved: the drawer stays open after a save, and the page's copy is the
  // one it was opened with, so the bar, View page and Delete read this one.
  const [category, setCategory] = useState<BlogCategory | null | undefined>(opened)
  const editing = !!category

  const [baseline, setBaseline] = useState<CategoryDraft>(() => draftOf(category))
  // A new category's slug follows its name until the slug is typed in.
  const [slugEdited, setSlugEdited] = useState(false)
  // The watch answers {} until the first change (the form is not connected when it first reads),
  // so it is laid over the values the form started from; a cleared field comes through as ''.
  const watched = Form.useWatch([], { form, preserve: true }) as CategoryDraft | undefined
  const draft: CategoryDraft = { ...baseline, ...watched, seo: { ...baseline.seo, ...watched?.seo } }
  const dirty = !sameDraft(draft, baseline)

  const defaults = { workspaceName: workspace.name, seo: workspace.settings.blog_settings?.seo }
  const effective = effectiveCategorySeo(draft, defaults)
  // What an empty field falls back to, for its placeholder.
  const fallback = effectiveCategorySeo({ ...draft, seo: {} }, defaults)
  const fallbackTitle = fallback.title
  const fallbackShareTitle = fallback.ogTitle
  const hasDescription = !!draft.description?.trim()
  const descriptionFallback = hasDescription
    ? t`Defaults to the description`
    : fallback.description
      ? t`Defaults to the blog's description`
      : undefined

  const baseUrl = blogBaseUrl(workspace.settings.custom_endpoint_url, window.API_ENDPOINT)
  const domain = baseUrl.replace(/^https?:\/\//, '')
  const slug = draft.slug?.trim() ?? ''

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['blog-categories', workspace.id] })
    // Its slug is in the posts' links and the 30-day views; its counts sit under the posts' key.
    void queryClient.invalidateQueries({ queryKey: ['blog-posts', workspace.id] })
  }

  const createMutation = useMutation({
    mutationFn: (data: CreateBlogCategoryRequest) => blogCategoriesApi.create(workspace.id, data),
    onSuccess: (response) => {
      message.success(t`Category created successfully`)
      invalidate()
      onCreated?.(response.category)
      onClose()
    },
    onError: (error: Error) => {
      message.error(t`Failed to create category: ${error.message}`)
    }
  })

  const updateMutation = useMutation({
    mutationFn: (data: UpdateBlogCategoryRequest) => blogCategoriesApi.update(workspace.id, data),
    onSuccess: (response) => {
      message.success(t`Category updated successfully`)
      invalidate()
      // Stays open on what was saved: the bar no longer shows unsaved changes.
      setBaseline(draftOf(response.category))
      setCategory(response.category)
    },
    onError: (error: Error) => {
      message.error(t`Failed to update category: ${error.message}`)
    }
  })
  const saving = createMutation.isPending || updateMutation.isPending

  const onFinish = (values: CategoryDraft) => {
    const fields = {
      name: values.name ?? '',
      slug: values.slug ?? '',
      description: values.description,
      // The form leaves out a saved tag it has no field for (meta_robots: the theme reads the
      // blog's own, never a category's), and antd submits only the fields it shows.
      seo: { ...values.seo, ...(baseline.seo?.meta_robots ? { meta_robots: baseline.seo.meta_robots } : {}) }
    }
    if (category) updateMutation.mutate({ id: category.id, ...fields })
    else createMutation.mutate(fields)
  }

  const requestClose = () => {
    if (!dirty) {
      onClose()
      return
    }
    modal.confirm({
      title: t`Close without saving?`,
      content: editing ? t`Your changes to this category will be lost.` : t`This category is not created yet.`,
      okText: t`Close`,
      cancelText: t`Keep editing`,
      okButtonProps: { danger: true },
      onOk: onClose
    })
  }

  const showField = (path: string[]) => {
    form.scrollToField(path, { block: 'center', behavior: 'smooth' })
    form.focusField(path)
  }

  const barTitle = (
    <>
      <span className="truncate text-sm font-semibold text-ink">
        {editing ? draft.name?.trim() || category.settings.name : t`New category`}
      </span>
      {editing && postCount !== undefined ? (
        <span className="shrink-0 text-xs text-ink-3">
          {plural(postCount, { one: '# post', other: '# posts' })}
        </span>
      ) : null}
      {!editing ? <Pill tone="primary">{t`Not created`}</Pill> : dirty ? <Pill tone="warning">{t`Unsaved changes`}</Pill> : null}
    </>
  )

  const topActions = editing ? (
    <>
      <Button
        type="text"
        icon={<ExternalLink size={14} aria-hidden="true" />}
        href={`${baseUrl}/${category.slug}`}
        target="_blank"
        rel="noopener noreferrer"
      >
        {t`View page`}
      </Button>
      {onDelete ? (
        <Dropdown
          trigger={['click']}
          menu={{
            items: [{ key: 'delete', label: t`Delete category`, danger: true, onClick: () => onDelete(category) }]
          }}
        >
          <Button type="text" aria-label={t`More actions`} icon={<MoreHorizontal size={14} aria-hidden="true" />} />
        </Dropdown>
      ) : null}
      {/* The baseline names every field, so discarding also clears what was typed in an empty one. */}
      {dirty ? <Button onClick={() => form.setFieldsValue(baseline)}>{t`Discard`}</Button> : null}
      <Button type="primary" disabled={!dirty} loading={saving} onClick={() => form.submit()}>
        {t`Save changes`}
      </Button>
    </>
  ) : (
    <Button type="primary" loading={saving} onClick={() => form.submit()}>
      {t`Create category`}
    </Button>
  )

  return (
    <FullDrawer
      open
      onClose={requestClose}
      backLabel={t`Blog`}
      copyLink={false}
      // The file manager opens a drawer of its own; the editor must not slide aside.
      push={false}
      ariaLabel={editing ? t`Edit category` : t`New category`}
      barTitle={barTitle}
      topActions={topActions}
    >
      <Form form={form} layout="vertical" initialValues={baseline} onFinish={onFinish}>
        <div className="flex flex-col gap-3 min-[1200px]:flex-row min-[1200px]:items-start">
          <div className="flex min-w-0 flex-1 flex-col gap-3">
            <Panel title={t`Category`} icon={<Tag size={14} aria-hidden="true" />}>
              <div className="grid grid-cols-2 gap-x-3">
                <Form.Item
                  name="name"
                  label={t`Name`}
                  rules={[
                    { required: true, message: t`Please enter a category name` },
                    { max: 255, message: t`Name must be less than 255 characters` }
                  ]}
                >
                  <Input
                    autoFocus={!editing}
                    placeholder={t`e.g., Product Updates`}
                    onChange={(e) => {
                      if (!editing && !slugEdited) form.setFieldValue('slug', normalizeSlug(e.target.value))
                    }}
                  />
                </Form.Item>
                <Form.Item
                  name="slug"
                  label={
                    editing ? (
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
                    { pattern: SLUG_PATTERN, message: t`Slug must contain only lowercase letters, numbers, and hyphens` },
                    { max: 100, message: t`Slug must be less than 100 characters` }
                  ]}
                  extra={
                    editing
                      ? t`Fixed once created: its posts' links depend on it`
                      : slugEdited
                        ? t`Lowercase letters, numbers and hyphens`
                        : t`Follows the name until you edit it`
                  }
                >
                  <Input
                    prefix={<span className="text-ink-3">{domain}/</span>}
                    placeholder="product-updates"
                    disabled={editing}
                    onChange={() => setSlugEdited(true)}
                  />
                </Form.Item>
                <div className="col-span-2">
                  <Form.Item name="description" label={t`Description`} extra={t`Shown at the top of the category page`}>
                    <TextArea rows={3} showCount maxLength={500} placeholder={t`Brief description of this category`} />
                  </Form.Item>
                </div>
              </div>
            </Panel>

            <SearchEnginesCard
              hint={t`How the category page appears in search results`}
              titlePlaceholder={t`Defaults to ${fallbackTitle}`}
              descriptionPlaceholder={descriptionFallback ?? t`Brief description for search results`}
              canonicalPlaceholder={`${baseUrl}/${slug || 'slug'}`}
            />
            <SocialShareCard
              hint={t`When a link to the category is shared`}
              titlePlaceholder={t`Defaults to ${fallbackShareTitle}`}
              descriptionPlaceholder={descriptionFallback}
              imagePlaceholder={fallback.ogImage ? t`Defaults to the blog's share image` : 'https://example.com/image.jpg'}
              image={effective.ogImage}
            />
          </div>

          <aside className="flex shrink-0 flex-col gap-3 min-[1200px]:sticky min-[1200px]:top-16 min-[1200px]:w-aside">
            <CategoryChecks draft={draft} effective={effective} editing={editing} onShowField={showField} />
            <SeoPreview
              effective={effective}
              domain={domain}
              path={[slug]}
              iconUrl={workspace.settings.blog_settings?.icon_url || workspace.settings.logo_url || undefined}
            />
          </aside>
        </div>
      </Form>
    </FullDrawer>
  )
}
