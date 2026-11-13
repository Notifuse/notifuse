import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PostsTable } from './PostsTable'
import { blogPostsApi } from '../../services/api/blog'
import { analyticsService } from '../../services/api/analytics'
import type {
  BlogCategory,
  BlogPost,
  BlogPostCounts,
  BlogPostListResponse
} from '../../services/api/blog'
import type { Workspace } from '../../services/api/types'

vi.mock('../../services/api/blog', () => ({
  blogPostsApi: { list: vi.fn(), delete: vi.fn(), unpublish: vi.fn() }
}))
vi.mock('../../services/api/analytics', () => ({
  analyticsService: { query: vi.fn() }
}))

// The address the table reads its filters from; tests change it and re-render.
const router = vi.hoisted(() => ({ search: {} as Record<string, string | undefined> }))
const navigate = vi.hoisted(() => vi.fn())
vi.mock('@tanstack/react-router', () => ({
  useSearch: () => router.search,
  useNavigate: () => navigate
}))

// The drawers and modals pull in the editor; here they only say what the table opened.
vi.mock('./PostDrawer', () => ({
  PostDrawer: (props: { open: boolean; post: BlogPost | null }) =>
    props.open ? <div data-testid="post-drawer" data-post={props.post?.id ?? 'new'} /> : null
}))
vi.mock('./DeletePostModal', () => ({
  DeletePostModal: (props: { open: boolean; post: BlogPost | null }) =>
    props.open ? <div data-testid="delete-post" data-post={props.post?.id} /> : null
}))
vi.mock('./PublishModal', () => ({
  PublishModal: (props: { visible: boolean; post: BlogPost | null }) =>
    props.visible ? <div data-testid="publish-post" data-post={props.post?.id} /> : null
}))
vi.mock('../seo/MissingMetaTagsWarning', () => ({
  MissingMetaTagsWarning: () => <span data-testid="seo-warning" />
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const DAY = 24 * 60 * 60 * 1000
const workspace = {
  id: 'ws1',
  name: 'Test',
  settings: { timezone: 'UTC', custom_endpoint_url: 'https://blog.acme.io' }
} as unknown as Workspace
const categories = [
  { id: 'c1', slug: 'news', settings: { name: 'News' } },
  { id: 'c2', slug: 'guides', settings: { name: 'Guides' } }
] as unknown as BlogCategory[]

const post = (n: number, overrides: Partial<BlogPost> = {}, settings: object = {}): BlogPost =>
  ({
    id: `post-${n}`,
    category_id: 'c1',
    slug: `post-${n}`,
    settings: {
      title: `Post ${n}`,
      template: { template_id: 'tpl', template_version: 1 },
      authors: [],
      reading_time_minutes: 0,
      ...settings
    },
    published_at: null,
    created_at: '2026-11-01T00:00:00Z',
    updated_at: '2026-11-01T00:00:00Z',
    ...overrides
  }) as unknown as BlogPost

// As the server answers: the posts of the page and the total, no next-page flag.
const page = (from: number, count: number, total: number): BlogPostListResponse => ({
  posts: Array.from({ length: count }, (_, i) => post(from + i)),
  total_count: total
})

const counts: BlogPostCounts = {
  all_total: 9,
  total: 7,
  published: 4,
  scheduled: 1,
  draft: 3,
  by_category: {}
}

interface Props {
  canWritePosts?: boolean
  showViews?: boolean
  counts?: BlogPostCounts
}

const tree = (client: QueryClient, props: Props) => (
  <I18nProvider i18n={i18n}>
    <ConfigProvider>
      <App>
        <QueryClientProvider client={client}>
          <PostsTable
            workspace={workspace}
            categories={categories}
            counts={props.counts}
            canWritePosts={props.canWritePosts ?? true}
            showViews={props.showViews ?? false}
          />
        </QueryClientProvider>
      </App>
    </ConfigProvider>
  </I18nProvider>
)

const renderTable = (props: Props = {}) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const utils = render(tree(client, props))
  return { ...utils, rerenderTable: () => utils.rerender(tree(client, props)) }
}

const listCalls = () => vi.mocked(blogPostsApi.list).mock.calls.map(([, params]) => params)
const rowOf = (title: string) => screen.getByText(title).closest('tr') as HTMLElement
const lastSearch = () => {
  const call = navigate.mock.calls[navigate.mock.calls.length - 1][0]
  return call.search({})
}

const openMenu = async (title: string) => {
  fireEvent.click(within(rowOf(title)).getByRole('button', { name: 'More actions' }))
  const menus = await screen.findAllByRole('menu')
  return menus[menus.length - 1]
}

describe('PostsTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    router.search = {}
    vi.mocked(blogPostsApi.list).mockResolvedValue(page(1, 3, 3))
    vi.mocked(analyticsService.query).mockResolvedValue({
      data: [],
      meta: { total: 0, query: '', params: [] }
    })
  })

  it('asks the server for the first 50 posts of the status, category and search of the URL', async () => {
    router.search = { status: 'scheduled', category_id: 'c1', q: 'launch' }
    renderTable()
    await screen.findByText('Post 1')
    expect(listCalls()[0]).toEqual({
      status: 'scheduled',
      category_id: 'c1',
      search: 'launch',
      limit: 50,
      page: 1
    })
  })

  it('appends the next page on Load more, and counts what is shown', async () => {
    vi.mocked(blogPostsApi.list)
      .mockResolvedValueOnce(page(1, 50, 60))
      .mockResolvedValueOnce(page(51, 10, 60))
    renderTable()
    await screen.findByText('Post 1')
    expect(screen.getByText('50 of 60 posts')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Load more' }))
    expect(await screen.findByText('Post 60')).toBeInTheDocument()
    expect(screen.getByText('Post 1')).toBeInTheDocument()
    expect(listCalls()[1]).toMatchObject({ page: 2, limit: 50 })
    expect(screen.getByText('60 of 60 posts')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument()
  })

  it('counts each status in its tab, from the counts of the page', async () => {
    renderTable({ counts })
    await screen.findByText('Post 1')
    const tabs = screen.getByRole('radiogroup')
    for (const [label, count] of [
      ['All', '7'],
      ['Published', '4'],
      ['Scheduled', '1'],
      ['Drafts', '3']
    ]) {
      expect(within(tabs).getByText(label).parentElement).toHaveTextContent(`${label}${count}`)
    }
  })

  it('writes the status picked to the URL, and leaves it out for All', async () => {
    router.search = { status: 'draft' }
    renderTable({ counts })
    await screen.findByText('Post 1')
    const tabs = screen.getByRole('radiogroup')
    fireEvent.click(within(tabs).getByText('Scheduled'))
    expect(lastSearch()).toEqual({ status: 'scheduled' })
    fireEvent.click(within(tabs).getByText('All'))
    expect(lastSearch()).toEqual({ status: undefined })
  })

  it('marks a post scheduled until its date, with the days left', async () => {
    const soon = new Date(Date.now() + 3 * DAY + 3600_000).toISOString()
    const past = new Date(Date.now() - 2 * DAY).toISOString()
    vi.mocked(blogPostsApi.list).mockResolvedValue({
      posts: [
        post(1, { published_at: soon }),
        post(2, { published_at: past }),
        post(3, { updated_at: past })
      ],
      total_count: 3
    })
    renderTable()
    await screen.findByText('Post 1')
    expect(within(rowOf('Post 1')).getByText('Scheduled')).toBeInTheDocument()
    expect(within(rowOf('Post 1')).getByText('Scheduled in 3 days')).toBeInTheDocument()
    expect(within(rowOf('Post 2')).getByText('Published')).toBeInTheDocument()
    expect(within(rowOf('Post 2')).getByText('2 days ago')).toBeInTheDocument()
    expect(within(rowOf('Post 3')).getByText('Draft')).toBeInTheDocument()
    expect(within(rowOf('Post 3')).getByText('Edited 2 days ago')).toBeInTheDocument()
    // The SEO warning of each post stays beside its status
    expect(within(rowOf('Post 1')).getByTestId('seo-warning')).toBeInTheDocument()
  })

  it('shows the reading time, and nothing at 0', async () => {
    vi.mocked(blogPostsApi.list).mockResolvedValue({
      posts: [post(1, {}, { reading_time_minutes: 5 }), post(2)],
      total_count: 2
    })
    renderTable()
    await screen.findByText('Post 1')
    expect(within(rowOf('Post 1')).getByText('5 min read')).toBeInTheDocument()
    expect(within(rowOf('Post 2')).queryByText(/min read/)).not.toBeInTheDocument()
  })

  it('puts the slug and the featured image in the title tooltip', async () => {
    vi.mocked(blogPostsApi.list).mockResolvedValue({
      posts: [post(1, {}, { featured_image_url: 'https://cdn.acme.io/cover.png' })],
      total_count: 1
    })
    const { container } = renderTable()
    fireEvent.mouseEnter(await screen.findByText('Post 1'))
    expect(await screen.findByText('post-1')).toBeInTheDocument()
    await waitFor(() =>
      expect(document.querySelector('img[src="https://cdn.acme.io/cover.png"]')).not.toBeNull()
    )
    // Neither under the title nor beside it any more
    expect(container.querySelector('img')).toBeNull()
  })

  it('names the first author, with the others counted', async () => {
    vi.mocked(blogPostsApi.list).mockResolvedValue({
      posts: [
        post(1, {}, { authors: [{ name: 'Ada' }, { name: 'Linus' }, { name: 'Grace' }] }),
        post(2, {}, { authors: [{ name: 'Ada' }] }),
        post(3)
      ],
      total_count: 3
    })
    renderTable()
    await screen.findByText('Post 1')
    expect(rowOf('Post 1')).toHaveTextContent('Ada +2')
    expect(rowOf('Post 2')).toHaveTextContent('Ada')
    expect(rowOf('Post 2')).not.toHaveTextContent('+')
    expect(within(rowOf('Post 3')).getAllByText('—').length).toBeGreaterThan(0)
  })

  it('names the category of each post, and hides the column inside a category', async () => {
    vi.mocked(blogPostsApi.list).mockResolvedValue({
      posts: [post(1), post(2, { category_id: null }), post(3, { category_id: 'gone' })],
      total_count: 3
    })
    const { unmount } = renderTable()
    await screen.findByText('Post 1')
    expect(within(rowOf('Post 1')).getByText('News')).toBeInTheDocument()
    expect(within(rowOf('Post 2')).getByText('Uncategorized')).toBeInTheDocument()
    expect(within(rowOf('Post 3')).getByText('Unknown')).toBeInTheDocument()
    unmount()

    router.search = { category_id: 'c1' }
    renderTable()
    await screen.findByText('Post 1')
    expect(screen.queryByRole('columnheader', { name: 'Category' })).not.toBeInTheDocument()
  })

  it('titles the view after its category, with the number of posts it holds', async () => {
    router.search = { category_id: 'c2' }
    vi.mocked(blogPostsApi.list).mockResolvedValue(page(1, 3, 42))
    renderTable()
    await screen.findByText('Post 1')
    expect(screen.getByRole('heading', { name: 'Guides' }).parentElement).toHaveTextContent(
      'Guides42'
    )
    expect(screen.getByRole('link', { name: /blog\.acme\.io\/guides/ })).toHaveAttribute(
      'href',
      'https://blog.acme.io/guides'
    )
  })

  it('links the blog on its domain, from All posts', async () => {
    renderTable()
    await screen.findByText('Post 1')
    expect(screen.getByRole('heading', { name: /All posts/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /blog\.acme\.io/ })).toHaveAttribute(
      'href',
      'https://blog.acme.io'
    )
  })

  it('opens a post on a row click, and a new one from New post', async () => {
    renderTable()
    fireEvent.click(await screen.findByText('Post 2'))
    expect(screen.getByTestId('post-drawer')).toHaveAttribute('data-post', 'post-2')
    fireEvent.click(screen.getByRole('button', { name: /New post/ }))
    expect(screen.getByTestId('post-drawer')).toHaveAttribute('data-post', 'new')
  })

  it('offers Open on web for a live post only, and Unpublish for a scheduled one', async () => {
    const soon = new Date(Date.now() + 3 * DAY).toISOString()
    const past = new Date(Date.now() - 2 * DAY).toISOString()
    vi.mocked(blogPostsApi.list).mockResolvedValue({
      posts: [post(1, { published_at: soon }), post(2, { published_at: past }), post(3)],
      total_count: 3
    })
    renderTable()
    await screen.findByText('Post 1')

    const scheduled = await openMenu('Post 1')
    expect(within(scheduled).queryByRole('menuitem', { name: 'Open on web' })).toBeNull()
    expect(within(scheduled).getByRole('menuitem', { name: 'Unpublish' })).toBeInTheDocument()

    const live = await openMenu('Post 2')
    expect(within(live).getByRole('menuitem', { name: 'Open on web' })).toBeInTheDocument()

    const draft = await openMenu('Post 3')
    fireEvent.click(within(draft).getByRole('menuitem', { name: 'Publish' }))
    expect(screen.getByTestId('publish-post')).toHaveAttribute('data-post', 'post-3')
  })

  it('asks before unpublishing, and says a scheduled post will not go out on its date', async () => {
    const soon = new Date(Date.now() + 3 * DAY).toISOString()
    const past = new Date(Date.now() - 2 * DAY).toISOString()
    vi.mocked(blogPostsApi.list).mockResolvedValue({
      posts: [post(1, { published_at: soon }), post(2, { published_at: past })],
      total_count: 2
    })
    vi.mocked(blogPostsApi.unpublish).mockResolvedValue({} as never)
    renderTable()
    await screen.findByText('Post 1')

    fireEvent.click(within(await openMenu('Post 1')).getByRole('menuitem', { name: 'Unpublish' }))
    expect(
      await screen.findByText('The post goes back to drafts and will not be published on its date.')
    ).toBeInTheDocument()
    expect(blogPostsApi.unpublish).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Unpublish' }))
    await waitFor(() =>
      expect(blogPostsApi.unpublish).toHaveBeenCalledWith('ws1', { id: 'post-1' })
    )

    fireEvent.click(within(await openMenu('Post 2')).getByRole('menuitem', { name: 'Unpublish' }))
    expect(
      await screen.findByText('Are you sure you want to unpublish this post?')
    ).toBeInTheDocument()
  })

  it('deletes from the row menu without opening the post', async () => {
    renderTable()
    await screen.findByText('Post 1')
    fireEvent.click(within(await openMenu('Post 1')).getByRole('menuitem', { name: 'Delete' }))
    expect(screen.getByTestId('delete-post')).toHaveAttribute('data-post', 'post-1')
    expect(screen.queryByTestId('post-drawer')).not.toBeInTheDocument()
  })

  it('without Workspace write, has no menu, no row click, and disables New post and Theme', async () => {
    renderTable({ canWritePosts: false })
    fireEvent.click(await screen.findByText('Post 1'))
    expect(screen.queryByTestId('post-drawer')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'More actions' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /New post/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /Theme/ })).toBeDisabled()
  })

  it('shows no Views column without web analytics', async () => {
    renderTable({ showViews: false })
    await screen.findByText('Post 1')
    expect(screen.queryByText('Views 30d')).not.toBeInTheDocument()
    expect(analyticsService.query).not.toHaveBeenCalled()
  })

  it('counts the 30-day views of the live posts loaded', async () => {
    const past = new Date(Date.now() - 2 * DAY).toISOString()
    const soon = new Date(Date.now() + 2 * DAY).toISOString()
    vi.mocked(blogPostsApi.list).mockResolvedValue({
      posts: [
        post(1, { published_at: past }),
        post(2, { published_at: soon }),
        post(3),
        post(4, { published_at: past, category_id: 'c2' })
      ],
      total_count: 4
    })
    vi.mocked(analyticsService.query).mockResolvedValue({
      data: [{ page_path: '/news/post-1', page_count: 1234 }],
      meta: { total: 1, query: '', params: [] }
    })
    renderTable({ showViews: true })
    await screen.findByText('Post 1')
    await waitFor(() => expect(within(rowOf('Post 1')).getByText('1,234')).toBeInTheDocument())

    const [query, workspaceId] = vi.mocked(analyticsService.query).mock.calls[0]
    expect(workspaceId).toBe('ws1')
    expect(query.filters?.[0]).toEqual({
      member: 'page_path',
      operator: 'in',
      values: ['/news/post-1', '/guides/post-4']
    })
    expect(query.filters?.[1]).toMatchObject({ member: 'entered_at', operator: 'inDateRange' })
    // No views recorded: a dash in the last data cell, after the author's
    const cells = within(rowOf('Post 4')).getAllByRole('cell')
    expect(cells[cells.length - 2]).toHaveTextContent('—')
  })

  it('says when the views could not be loaded', async () => {
    const past = new Date(Date.now() - 2 * DAY).toISOString()
    vi.mocked(blogPostsApi.list).mockResolvedValue({
      posts: [post(1, { published_at: past })],
      total_count: 1
    })
    vi.mocked(analyticsService.query).mockRejectedValue(new Error('boom'))
    renderTable({ showViews: true })
    await screen.findByText('Post 1')
    await waitFor(() => expect(analyticsService.query).toHaveBeenCalled())
    const cells = within(rowOf('Post 1')).getAllByRole('cell')
    const dash = within(cells[cells.length - 2]).getByText('—')
    fireEvent.mouseEnter(dash)
    expect(await screen.findByText("Couldn't load")).toBeInTheDocument()
  })

  it.each([
    [{}, 'No posts yet'],
    [{ status: 'draft' }, 'No draft posts'],
    [{ status: 'published' }, 'No published posts'],
    [{ status: 'scheduled' }, 'No scheduled posts'],
    [{ category_id: 'c1' }, 'No posts in this category'],
    [{ status: 'draft', q: 'zzz' }, 'No posts match your search']
  ])('says why the view %o is empty', async (search, text) => {
    router.search = search
    vi.mocked(blogPostsApi.list).mockResolvedValue({ posts: null, total_count: 0 })
    renderTable()
    expect(await screen.findByText(text)).toBeInTheDocument()
  })

  it('offers a retry when the posts could not be loaded', async () => {
    vi.mocked(blogPostsApi.list).mockRejectedValueOnce(new Error('database is down'))
    renderTable()
    expect(await screen.findByText("Couldn't load the posts")).toBeInTheDocument()
    vi.mocked(blogPostsApi.list).mockResolvedValue(page(1, 1, 1))
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Post 1')).toBeInTheDocument()
  })

  it('keeps the loaded posts and says so when the next page fails', async () => {
    vi.mocked(blogPostsApi.list)
      .mockResolvedValueOnce(page(1, 50, 60))
      .mockRejectedValueOnce(new Error('timeout'))
    renderTable()
    await screen.findByText('Post 1')
    fireEvent.click(screen.getByRole('button', { name: 'Load more' }))
    expect(await screen.findByText('timeout')).toBeInTheDocument()
    expect(screen.getByText('Post 1')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Load more' })).toBeInTheDocument()
  })

  it('asks the server for the title typed, 300 ms after the last keystroke', async () => {
    renderTable()
    await screen.findByText('Post 1')
    vi.useFakeTimers()
    try {
      fireEvent.change(screen.getByPlaceholderText('Title'), { target: { value: 'launch ' } })
      expect(navigate).not.toHaveBeenCalled()
      vi.advanceTimersByTime(300)
      expect(navigate).toHaveBeenCalledWith(expect.objectContaining({ replace: true }))
      expect(lastSearch()).toEqual({ q: 'launch' })
    } finally {
      vi.useRealTimers()
    }
  })
})
