import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import type { BlogPost } from '../../services/api/blog'
import type { Workspace } from '../../services/api/types'

const editor = vi.hoisted(() => ({ mounts: 0 }))

// services/api/client imports the router, which imports every page and cycles back here.
vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() },
  ApiError: class ApiError extends Error {}
}))
vi.mock('../../services/api/blog', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/blog')>()),
  blogCategoriesApi: {
    list: vi.fn(async () => ({
      categories: [
        { id: 'c1', slug: 'guides', settings: { name: 'Guides', description: 'How-tos' } },
        { id: 'c2', slug: 'news', settings: { name: 'News' } }
      ],
      total_count: 2
    }))
  },
  blogPostsApi: { create: vi.fn(), update: vi.fn(async () => ({})) }
}))
vi.mock('../../services/api/template', () => ({
  templatesApi: {
    get: vi.fn(async () => ({
      template: { id: 'tpl1', version: 3, web: { content: { type: 'doc', content: [{ type: 'paragraph' }] } } }
    })),
    update: vi.fn(async () => ({})),
    create: vi.fn()
  }
}))
// Tiptap does not run in jsdom. The stand-in counts its mounts, to show it survives the steps; like
// the real editor, it emits the text it loaded, normalised its own way, as it mounts; a click edits it.
vi.mock('../blog_editor', async () => {
  const { useEffect } = await import('react')
  return {
    NotifuseEditor: ({ onChange }: { onChange?: (json: Record<string, unknown>) => void }) => {
      useEffect(() => {
        editor.mounts += 1
        onChange?.({ type: 'doc', content: [{ type: 'paragraph', attrs: { textAlign: 'left', class: 'normalised' } }] })
        // eslint-disable-next-line react-hooks/exhaustive-deps
      }, [])
      return (
        <button
          type="button"
          data-testid="editor"
          onClick={() => onChange?.({ type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'edited' }] }] })}
        />
      )
    },
    DEFAULT_INITIAL_CONTENT: { type: 'doc', content: [] }
  }
})
vi.mock('./BlogAIAssistant', () => ({ BlogAIAssistant: () => null }))
vi.mock('../common/ImageURLInput', () => ({ ImageURLInput: () => null }))

import { PostDrawer } from './PostDrawer'
import { blogPostsApi } from '../../services/api/blog'
import { templatesApi } from '../../services/api/template'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: { custom_endpoint_url: 'https://blog.acme.com' }
} as unknown as Workspace

const post = {
  id: 'p1',
  category_id: 'c1',
  slug: 'subject-lines',
  published_at: '2026-10-01T09:00:00Z',
  created_at: '2026-10-01T09:00:00Z',
  updated_at: '2026-11-15T09:00:00Z',
  settings: {
    title: 'Subject lines',
    excerpt: 'Six patterns',
    featured_image_url: 'https://cdn.example.com/hero.jpg',
    authors: [{ name: 'Aiko Tanaka' }],
    reading_time_minutes: 7,
    template: { template_id: 'tpl1', template_version: 3 },
    seo: { meta_robots: 'noindex,follow', keywords: ['email'] }
  }
} as unknown as BlogPost

function renderDrawer(props: { post?: BlogPost } = {}) {
  const onClose = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <App>
          <PostDrawer open onClose={onClose} workspace={workspace} post={props.post ?? null} />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
  return { onClose }
}

const step = (name: 'details' | 'write') => document.querySelector(`[data-post-step="${name}"]`) as HTMLElement
const bar = () => document.querySelector('[data-drawer-bar]') as HTMLElement

describe('PostDrawer steps', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    editor.mounts = 0
  })

  it('opens on Details, then Write, keeping the editor mounted across the steps', async () => {
    const user = userEvent.setup()
    renderDrawer()
    expect(step('details')).toBeVisible()
    expect(step('write')).not.toBeVisible()

    await user.click(within(bar()).getByText('Write'))
    expect(step('write')).toBeVisible()
    expect(await screen.findByTestId('editor')).toBeInTheDocument()
    await user.click(within(bar()).getByText('Details'))
    await user.click(within(bar()).getByText('Write'))
    expect(editor.mounts).toBe(1)
  })

  it('makes the slug follow the title until it is typed in', async () => {
    const user = userEvent.setup()
    renderDrawer()
    await user.type(screen.getByLabelText('Title'), 'Subject Lines')
    expect(screen.getByLabelText('Slug')).toHaveValue('subject-lines')
    await user.clear(screen.getByLabelText('Slug'))
    await user.type(screen.getByLabelText('Slug'), 'lines')
    await user.type(screen.getByLabelText('Title'), ' that work')
    expect(screen.getByLabelText('Slug')).toHaveValue('lines')
  })

  it('goes back to Details when a save from Write misses a field', async () => {
    const user = userEvent.setup()
    renderDrawer()
    await user.click(within(bar()).getByText('Write'))
    await user.click(screen.getByRole('button', { name: 'Create post' }))
    await waitFor(() => expect(step('details')).toBeVisible())
    expect(await screen.findByText('Please enter a post title')).toBeInTheDocument()
    expect(blogPostsApi.create).not.toHaveBeenCalled()
  })

  it('saves an edited post with its tags, keeping the indexing it already has', async () => {
    const user = userEvent.setup()
    renderDrawer({ post })
    expect(within(bar()).getByText('Published')).toBeInTheDocument()
    // The address needs the category's slug, once the categories have loaded.
    expect(await within(bar()).findByRole('link', { name: /View post/ })).toHaveAttribute(
      'href',
      'https://blog.acme.com/guides/subject-lines'
    )
    expect(screen.queryByLabelText('Indexing')).not.toBeInTheDocument()
    await waitFor(() => expect(screen.getByLabelText('Title')).toHaveValue('Subject lines'))

    // Nothing changed yet: nothing to save.
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
    await user.type(screen.getByLabelText('Meta title'), 'Better subject lines')
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(blogPostsApi.update).toHaveBeenCalledTimes(1))
    expect(templatesApi.update).toHaveBeenCalledWith(expect.objectContaining({ id: 'tpl1', base_version: 3 }))
    expect(vi.mocked(blogPostsApi.update).mock.calls[0][1]).toMatchObject({
      id: 'p1',
      slug: 'subject-lines',
      category_id: 'c1',
      seo: { meta_title: 'Better subject lines', keywords: ['email'], meta_robots: 'noindex,follow' }
    })
  })

  it('checks the tags as the theme falls back: the featured image is the share image', async () => {
    renderDrawer({ post })
    const check = (key: string) => document.querySelector(`[data-check="${key}"]`) as HTMLElement
    await waitFor(() => expect(check('meta_title')).toHaveTextContent('Uses “Subject lines - Acme”'))
    expect(check('basics')).toHaveAttribute('data-ok', 'true')
    expect(check('meta_description')).toHaveTextContent('Uses “Six patterns”')
    expect(check('og_image')).toHaveAttribute('data-ok', 'true')
    expect(check('og_image')).toHaveTextContent('The featured image')
  })

  it('edits the title from the heading of the Write step', async () => {
    const user = userEvent.setup()
    renderDrawer({ post })
    await user.click(within(bar()).getByText('Write'))
    await user.click(await screen.findByRole('button', { name: 'Subject lines' }))
    await waitFor(() => expect(step('details')).toBeVisible())
    await waitFor(() => expect(screen.getByLabelText('Title')).toHaveFocus())
  })

  it('lets a hyphen be typed in the slug, and trims a trailing one when the field is left', async () => {
    const user = userEvent.setup()
    renderDrawer()
    const slug = screen.getByLabelText('Slug')
    await user.type(slug, 'my-post')
    expect(slug).toHaveValue('my-post')
    await user.type(slug, '-')
    expect(slug).toHaveValue('my-post-')
    await user.tab()
    expect(slug).toHaveValue('my-post')
  })

  it('keeps the slug of a restored draft its own', async () => {
    localStorage.setItem(
      'blog-post-draft-new-ws1',
      JSON.stringify({ content: { type: 'doc', content: [] }, formValues: { title: 'Hello', slug: 'custom' }, savedAt: Date.now() })
    )
    const user = userEvent.setup()
    renderDrawer()
    await user.click(await screen.findByRole('button', { name: 'Yes' }))
    await waitFor(() => expect(screen.getByLabelText('Slug')).toHaveValue('custom'))
    await user.type(screen.getByLabelText('Title'), ' world')
    expect(screen.getByLabelText('Slug')).toHaveValue('custom')
  })

  it("opens the post at its saved category's address, whatever category is picked", async () => {
    const user = userEvent.setup()
    renderDrawer({ post })
    const link = await within(bar()).findByRole('link', { name: /View post/ })
    await user.click(screen.getByLabelText('Category'))
    await user.click(await screen.findByTitle('News'))
    expect(link).toHaveAttribute('href', 'https://blog.acme.com/guides/subject-lines')
  })

  // A post created through the API without template_version is at 0: it renders the template's
  // latest version, and templates.get answers with that version's real number.
  const latestPost = {
    ...post,
    settings: { ...post.settings, template: { template_id: 'tpl1', template_version: 0 } }
  } as BlogPost

  it('opens the text of a post that follows the latest template version', async () => {
    const user = userEvent.setup()
    renderDrawer({ post: latestPost })
    await user.click(within(bar()).getByText('Write'))
    expect(await screen.findByTestId('editor')).toBeInTheDocument()
    expect(screen.queryByText('Loading template...')).toBeNull()
    expect(templatesApi.get).toHaveBeenCalledWith(expect.objectContaining({ id: 'tpl1', version: 0 }))
  })

  it('saves a post that follows the latest template version without pinning it', async () => {
    const user = userEvent.setup()
    renderDrawer({ post: latestPost })
    await user.click(within(bar()).getByText('Write'))
    await user.click(await screen.findByTestId('editor'))
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(blogPostsApi.update).toHaveBeenCalledTimes(1))
    // The base is the revision the editor loaded, not 0, so a concurrent save is still caught.
    expect(templatesApi.update).toHaveBeenCalledWith(expect.objectContaining({ id: 'tpl1', base_version: 3 }))
    expect(vi.mocked(blogPostsApi.update).mock.calls[0][1]).toMatchObject({ template_id: 'tpl1', template_version: 0 })
  })

  it("takes the editor's first output as the text as loaded, and an edit in it as a change", async () => {
    const user = userEvent.setup()
    renderDrawer({ post })
    await user.click(within(bar()).getByText('Write'))
    const editorStandIn = await screen.findByTestId('editor')
    expect(within(bar()).queryByText('Unsaved changes')).toBeNull()
    await user.click(editorStandIn)
    expect(within(bar()).getByText('Unsaved changes')).toBeInTheDocument()
  })
})
