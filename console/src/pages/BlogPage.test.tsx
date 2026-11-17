import type React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { BlogCategory, BlogPostCounts } from '../services/api/blog'

const state = vi.hoisted(() => ({
  search: {} as Record<string, string | undefined>,
  permissions: {} as Record<string, { read: boolean; write: boolean }>,
  webAnalytics: false,
  navigate: vi.fn()
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => state.search,
  useNavigate: () => state.navigate
}))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [
      { id: 'ws1', name: 'Acme', settings: { web_analytics: { enabled: state.webAnalytics } } }
    ]
  }),
  useWorkspacePermissions: () => ({ permissions: state.permissions, loading: false })
}))
vi.mock('../services/api/blog', () => ({
  blogCategoriesApi: { list: vi.fn(), delete: vi.fn() },
  blogPostsApi: { counts: vi.fn() }
}))

// The table and the drawers are tested on their own; here they only say what the page gave them.
vi.mock('../components/blog/PostsTable', () => ({
  PostsTable: (props: {
    canWritePosts: boolean
    editRefusal?: string
    showViews: boolean
    counts?: BlogPostCounts
    rail?: React.ReactNode
  }) => (
    <>
      {props.rail}
      <div
        data-testid="posts-table"
        data-can-write={String(props.canWritePosts)}
        data-edit-refusal={props.editRefusal ?? ''}
        data-views={String(props.showViews)}
        data-total={props.counts?.total ?? ''}
      />
    </>
  )
}))
vi.mock('../components/blog/CategoryDrawer', () => ({
  CategoryDrawer: (props: {
    open: boolean
    category?: BlogCategory | null
    postCount?: number
    onDelete?: (category: BlogCategory) => void
    onCreated?: (category: BlogCategory) => void
  }) =>
    props.open ? (
      <div data-testid="category-drawer" data-category={props.category?.id ?? 'new'} data-posts={props.postCount ?? ''}>
        <button type="button" onClick={() => props.onCreated?.({ id: 'c9' } as BlogCategory)}>
          drawer-created
        </button>
        <button type="button" onClick={() => props.category && props.onDelete?.(props.category)}>
          drawer-delete
        </button>
      </div>
    ) : null
}))
vi.mock('../components/blog/DeleteCategoryModal', () => ({
  DeleteCategoryModal: (props: { open: boolean; onConfirm: () => void }) =>
    props.open ? (
      <button type="button" onClick={props.onConfirm}>
        confirm-delete
      </button>
    ) : null
}))
vi.mock('../components/blog/PostDrawer', () => ({ PostDrawer: () => null }))
vi.mock('../components/seo/MissingMetaTagsWarning', () => ({ MissingMetaTagsWarning: () => null }))

import { BlogPage } from './BlogPage'
import { blogCategoriesApi, blogPostsApi } from '../services/api/blog'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const categories = [
  { id: 'c1', slug: 'news', settings: { name: 'News' } }
] as unknown as BlogCategory[]
const counts = (overrides: Partial<BlogPostCounts> = {}): BlogPostCounts => ({
  all_total: 9,
  total: 9,
  published: 5,
  scheduled: 1,
  draft: 4,
  by_category: { c1: 6 },
  ...overrides
})

const renderPage = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const tree = () => (
    <I18nProvider i18n={i18n}>
      <App>
        <QueryClientProvider client={client}>
          <BlogPage />
        </QueryClientProvider>
      </App>
    </I18nProvider>
  )
  return { ...render(tree()), tree, client }
}

describe('BlogPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.search = {}
    state.permissions = {
      workspace: { read: true, write: true },
      blog: { read: true, write: true },
      templates: { read: true, write: true }
    }
    state.webAnalytics = false
    vi.mocked(blogCategoriesApi.list).mockResolvedValue({ categories } as never)
    vi.mocked(blogPostsApi.counts).mockResolvedValue(counts())
  })

  it('counts the posts of the category picked, and gives the counts to the rail and the table', async () => {
    state.search = { category_id: 'c1' }
    vi.mocked(blogPostsApi.counts).mockResolvedValue(counts({ total: 6 }))
    renderPage()
    await waitFor(() =>
      expect(screen.getByTestId('posts-table')).toHaveAttribute('data-total', '6')
    )
    expect(blogPostsApi.counts).toHaveBeenCalledWith('ws1', 'c1', undefined)
    // All posts keeps the count of every post inside a category
    expect(screen.getByRole('button', { name: /^All posts/ })).toHaveTextContent('9')
  })

  it("counts the tabs with the list's search, the rail still over every post", async () => {
    state.search = { q: ' launch ' }
    renderPage()
    await screen.findByTestId('posts-table')
    await waitFor(() => expect(blogPostsApi.counts).toHaveBeenCalledWith('ws1', undefined, 'launch'))
    expect(screen.getByRole('button', { name: /^All posts/ })).toHaveTextContent('9')
  })

  it('offers the category actions with Blog write only', async () => {
    state.permissions = {
      workspace: { read: true, write: true },
      blog: { read: true, write: false }
    }
    renderPage()
    await screen.findByTestId('posts-table')
    expect(screen.queryByRole('button', { name: 'New category' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Actions for News' })).not.toBeInTheDocument()
  })

  it('lets posts follow Blog write, as the server does, whatever Workspace write', async () => {
    state.permissions = {
      workspace: { read: true, write: false },
      blog: { read: true, write: true },
      templates: { read: true, write: true }
    }
    const { unmount } = renderPage()
    expect(await screen.findByTestId('posts-table')).toHaveAttribute('data-can-write', 'true')
    unmount()

    state.permissions = {
      workspace: { read: true, write: true },
      blog: { read: true, write: false },
      templates: { read: true, write: true }
    }
    renderPage()
    expect(await screen.findByTestId('posts-table')).toHaveAttribute('data-can-write', 'false')
  })

  it("says why a post's content cannot be edited without Templates read or write", async () => {
    state.permissions = {
      workspace: { read: true, write: true },
      blog: { read: true, write: true },
      templates: { read: true, write: false }
    }
    const { unmount } = renderPage()
    expect(await screen.findByTestId('posts-table')).toHaveAttribute(
      'data-edit-refusal',
      'Writing or editing a post also needs Templates read and write: its content is a template.'
    )
    unmount()

    state.permissions = {
      workspace: { read: true, write: true },
      blog: { read: true, write: true },
      templates: { read: true, write: true }
    }
    renderPage()
    expect(await screen.findByTestId('posts-table')).toHaveAttribute('data-edit-refusal', '')
  })

  it('shows the first-category state, its button with Blog write only', async () => {
    vi.mocked(blogCategoriesApi.list).mockResolvedValue({ categories: [] } as never)
    const { unmount } = renderPage()
    expect(
      await screen.findByRole('button', { name: /Create Your First Category/ })
    ).toBeInTheDocument()
    unmount()

    state.permissions = {
      workspace: { read: true, write: true },
      blog: { read: true, write: false }
    }
    renderPage()
    expect((await screen.findAllByText('No categories yet')).length).toBeGreaterThan(0)
    expect(
      screen.queryByRole('button', { name: /Create Your First Category/ })
    ).not.toBeInTheDocument()
  })

  it('shows the first-post state when the blog has no post at all', async () => {
    vi.mocked(blogPostsApi.counts).mockResolvedValue(counts({ all_total: 0, total: 0 }))
    renderPage()
    expect(await screen.findByRole('button', { name: /Create Your First Post/ })).toBeEnabled()
    expect(screen.queryByTestId('posts-table')).not.toBeInTheDocument()
  })

  it('keeps the table while the counts load', async () => {
    vi.mocked(blogPostsApi.counts).mockReturnValue(new Promise(() => {}))
    renderPage()
    expect(await screen.findByTestId('posts-table')).toHaveAttribute('data-total', '')
  })

  it('keeps the table when the counts fail', async () => {
    vi.mocked(blogPostsApi.counts).mockRejectedValue(new Error('boom'))
    const { client } = renderPage()
    await waitFor(() =>
      expect(client.getQueryState(['blog-posts', 'ws1', 'counts', null, null])?.status).toBe('error')
    )
    expect(screen.getByTestId('posts-table')).toHaveAttribute('data-total', '')
  })

  it('keeps the rail counts while another category loads, and gives the table none', async () => {
    vi.mocked(blogPostsApi.counts).mockImplementation((_ws, categoryId) =>
      categoryId ? new Promise(() => {}) : Promise.resolve(counts())
    )
    const view = renderPage()
    await waitFor(() =>
      expect(screen.getByTestId('posts-table')).toHaveAttribute('data-total', '9')
    )
    state.search = { category_id: 'c1' }
    view.rerender(view.tree())
    await waitFor(() => expect(blogPostsApi.counts).toHaveBeenCalledWith('ws1', 'c1', undefined))
    expect(screen.getByRole('button', { name: /^All posts/ })).toHaveTextContent('9')
    expect(screen.getByTestId('posts-table')).toHaveAttribute('data-total', '')
  })

  it.each([
    [true, { read: true, write: false }, 'true'],
    [false, { read: true, write: false }, 'false'],
    [true, { read: false, write: false }, 'false']
  ])(
    'shows the views with web analytics %s and its read right %o: %s',
    async (enabled, right, shown) => {
      state.webAnalytics = enabled
      state.permissions = { ...state.permissions, web_analytics: right }
      renderPage()
      expect(await screen.findByTestId('posts-table')).toHaveAttribute('data-views', shown)
    }
  )

  it('opens a category it created, by its id', async () => {
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'New category' }))
    expect(screen.getByTestId('category-drawer')).toHaveAttribute('data-category', 'new')
    fireEvent.click(screen.getByRole('button', { name: 'drawer-created' }))
    const update = state.navigate.mock.calls.at(-1)?.[0].search as (prev: object) => object
    expect(update({ status: 'draft' })).toEqual({ status: 'draft', category_id: 'c9' })
  })

  it('gives the drawer its post count, and closes it once its category is deleted from it', async () => {
    vi.mocked(blogCategoriesApi.delete).mockResolvedValue({ success: true } as never)
    renderPage()
    await waitFor(() => expect(screen.getByRole('button', { name: /^News/ })).toHaveTextContent('6'))
    fireEvent.click(screen.getByRole('button', { name: 'Actions for News' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
    expect(screen.getByTestId('category-drawer')).toHaveAttribute('data-posts', '6')

    fireEvent.click(screen.getByRole('button', { name: 'drawer-delete' }))
    fireEvent.click(screen.getByRole('button', { name: 'confirm-delete' }))
    await waitFor(() => expect(blogCategoriesApi.delete).toHaveBeenCalledWith('ws1', { id: 'c1' }))
    await waitFor(() => expect(screen.queryByTestId('category-drawer')).not.toBeInTheDocument())
  })
})
