import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PostsTable } from './PostsTable'
import { blogPostsApi, blogCategoriesApi } from '../../services/api/blog'
import type { BlogPost, BlogPostListResponse } from '../../services/api/blog'

vi.mock('../../services/api/blog', () => ({
  blogPostsApi: { list: vi.fn(), delete: vi.fn(), unpublish: vi.fn() },
  blogCategoriesApi: { list: vi.fn(), delete: vi.fn() }
}))

// The address the table reads its filters from; tests change it and re-render.
const router = vi.hoisted(() => ({
  search: {} as Record<string, string | undefined>
}))
vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => router.search,
  useNavigate: () => vi.fn()
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Test', settings: {} }] }),
  useWorkspacePermissions: () => ({
    permissions: { workspace: { write: true } }
  })
}))

// The drawers and modals pull in the editor; the table does not need them here.
vi.mock('./PostDrawer', () => ({ PostDrawer: () => null }))
vi.mock('./DeletePostModal', () => ({ DeletePostModal: () => null }))
vi.mock('./CategoryDrawer', () => ({ CategoryDrawer: () => null }))
vi.mock('./DeleteCategoryModal', () => ({ DeleteCategoryModal: () => null }))
vi.mock('./PublishModal', () => ({ PublishModal: () => null }))
vi.mock('../seo/MissingMetaTagsWarning', () => ({
  MissingMetaTagsWarning: () => null
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const post = (n: number): BlogPost =>
  ({
    id: `post-${n}`,
    category_id: null,
    slug: `post-${n}`,
    settings: {
      title: `Post ${n}`,
      template: { template_id: 'tpl', template_version: 1 },
      authors: [],
      reading_time_minutes: 1
    },
    published_at: null,
    created_at: '2026-11-01T00:00:00Z',
    updated_at: '2026-11-01T00:00:00Z'
  }) as unknown as BlogPost

const page = (from: number, count: number, total: number): BlogPostListResponse => ({
  posts: Array.from({ length: count }, (_, i) => post(from + i)),
  total_count: total
})

const tree = (client: QueryClient) => (
  <I18nProvider i18n={i18n}>
    <ConfigProvider>
      <App>
        <QueryClientProvider client={client}>
          <PostsTable />
        </QueryClientProvider>
      </App>
    </ConfigProvider>
  </I18nProvider>
)

const renderTable = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } }
  })
  const utils = render(tree(client))
  return {
    ...utils,
    client,
    rerenderTable: () => utils.rerender(tree(client))
  }
}

const listCalls = () => vi.mocked(blogPostsApi.list).mock.calls.map(([, params]) => params)

describe('PostsTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    router.search = {}
    vi.mocked(blogCategoriesApi.list).mockResolvedValue({
      categories: []
    } as never)
  })

  it('asks the server for the first page of posts', async () => {
    vi.mocked(blogPostsApi.list).mockResolvedValue(page(1, 3, 3))
    renderTable()

    await screen.findByText('Post 1')
    expect(listCalls()[0]).toMatchObject({ status: 'all', limit: 50, page: 1 })
  })

  it('asks the server for the page the user picks, past the first 100 posts', async () => {
    vi.mocked(blogPostsApi.list).mockImplementation(async (_ws, params) =>
      params.page === 3 ? page(101, 20, 120) : page(1, 50, 120)
    )
    renderTable()

    await screen.findByText('Post 1')
    fireEvent.click(screen.getByTitle('3'))

    await screen.findByText('Post 101')
    expect(listCalls()).toContainEqual(expect.objectContaining({ limit: 50, page: 3 }))
    expect(screen.getByText('Total 120 posts')).toBeTruthy()
  })

  it('goes back to the first page when the status filter changes', async () => {
    vi.mocked(blogPostsApi.list).mockImplementation(async (_ws, params) =>
      params.page === 2 ? page(51, 10, 60) : page(1, 50, 60)
    )
    const { rerenderTable } = renderTable()

    await screen.findByText('Post 1')
    fireEvent.click(screen.getByTitle('2'))
    await screen.findByText('Post 51')

    router.search = { status: 'draft' }
    rerenderTable()

    await waitFor(() =>
      expect(listCalls().at(-1)).toMatchObject({
        status: 'draft',
        limit: 50,
        page: 1
      })
    )
  })

  it('moves to the last page when the current one comes back empty', async () => {
    // Page 2 held only one post; once it is deleted the server answers page 2 with no posts.
    let deleted = false
    vi.mocked(blogPostsApi.list).mockImplementation(async (_ws, params) => {
      if (params.page === 2) return deleted ? page(0, 0, 50) : page(51, 1, 51)
      return page(1, 50, deleted ? 50 : 51)
    })
    const { client } = renderTable()

    await screen.findByText('Post 1')
    fireEvent.click(screen.getByTitle('2'))
    await screen.findByText('Post 51')

    deleted = true
    await client.invalidateQueries({ queryKey: ['blog-posts', 'ws1'] })

    await screen.findByText('Post 1')
    expect(listCalls().at(-1)).toMatchObject({ page: 1 })
    expect(screen.queryByText('No posts yet')).toBeNull()
  })

  it('shows the empty state only when there are no posts at all', async () => {
    vi.mocked(blogPostsApi.list).mockResolvedValue(page(0, 0, 0))
    renderTable()

    await screen.findByText('No posts yet')
  })

  it('does not share its cache entry with the blog page probe', async () => {
    vi.mocked(blogPostsApi.list).mockResolvedValue(page(1, 3, 3))
    const { client } = renderTable()

    await screen.findByText('Post 1')
    // BlogPage.tsx asks for one post under ['blog-posts', ws, 'all', undefined].
    expect(client.getQueryData(['blog-posts', 'ws1', 'all', undefined])).toBeUndefined()
  })
})
