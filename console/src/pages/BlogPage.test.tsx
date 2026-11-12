import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { BlogCategory, BlogPostCounts } from '../services/api/blog'

const state = vi.hoisted(() => ({
  search: {} as Record<string, string | undefined>,
  permissions: {} as Record<string, { read: boolean; write: boolean }>,
  webAnalytics: false
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => state.search,
  useNavigate: () => vi.fn()
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
  PostsTable: (props: { canWritePosts: boolean; showViews: boolean; counts?: BlogPostCounts }) => (
    <div
      data-testid="posts-table"
      data-can-write={String(props.canWritePosts)}
      data-views={String(props.showViews)}
      data-total={props.counts?.total ?? ''}
    />
  )
}))
vi.mock('../components/blog/CategoryDrawer', () => ({ CategoryDrawer: () => null }))
vi.mock('../components/blog/DeleteCategoryModal', () => ({ DeleteCategoryModal: () => null }))
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

const renderPage = () =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <QueryClientProvider
          client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
        >
          <BlogPage />
        </QueryClientProvider>
      </App>
    </I18nProvider>
  )

describe('BlogPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.search = {}
    state.permissions = {
      workspace: { read: true, write: true },
      blog: { read: true, write: true }
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
    expect(blogPostsApi.counts).toHaveBeenCalledWith('ws1', 'c1')
    // All posts keeps the count of every post inside a category
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

  it('lets posts follow Workspace write, as before', async () => {
    state.permissions = {
      workspace: { read: true, write: false },
      blog: { read: true, write: true }
    }
    renderPage()
    expect(await screen.findByTestId('posts-table')).toHaveAttribute('data-can-write', 'false')
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

  it('keeps the table while the counts load or fail', async () => {
    vi.mocked(blogPostsApi.counts).mockRejectedValue(new Error('boom'))
    renderPage()
    expect(await screen.findByTestId('posts-table')).toHaveAttribute('data-total', '')
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
})
