import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { blogPostsApi } from './blog'

vi.mock('./client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

describe('blogPostsApi.list', () => {
  beforeEach(() => vi.mocked(api.get).mockReset().mockResolvedValue({ posts: [], total_count: 0 }))

  it('sends the status, the category, the page and the limit', async () => {
    await blogPostsApi.list('ws1', { status: 'scheduled', category_id: 'c1', page: 2, limit: 50 })
    expect(api.get).toHaveBeenCalledWith(
      '/api/blogPosts.list?workspace_id=ws1&category_id=c1&status=scheduled&limit=50&page=2'
    )
  })

  it('encodes the title search', async () => {
    await blogPostsApi.list('ws1', { search: 'A&B 50%' })
    expect(api.get).toHaveBeenCalledWith('/api/blogPosts.list?workspace_id=ws1&search=A%26B+50%25')
  })

  it('leaves out an empty search', async () => {
    await blogPostsApi.list('ws1', { search: '' })
    expect(api.get).toHaveBeenCalledWith('/api/blogPosts.list?workspace_id=ws1')
  })
})

describe('blogPostsApi.counts', () => {
  beforeEach(() =>
    vi
      .mocked(api.get)
      .mockReset()
      .mockResolvedValue({
        all_total: 0,
        total: 0,
        published: 0,
        scheduled: 0,
        draft: 0,
        by_category: {}
      })
  )

  it('counts every post without a category', async () => {
    await blogPostsApi.counts('ws1')
    expect(api.get).toHaveBeenCalledWith('/api/blogPosts.counts?workspace_id=ws1')
  })

  it('counts the posts of one category', async () => {
    await blogPostsApi.counts('ws1', 'c1')
    expect(api.get).toHaveBeenCalledWith('/api/blogPosts.counts?workspace_id=ws1&category_id=c1')
  })
})
