import { describe, expect, it } from 'vitest'
import type { BlogCategory } from '../../services/api/blog'
import type { Workspace } from '../../services/api/types'
import { getMockDataForView } from '../../utils/mockBlogData'
import { withWorkspaceData } from './themePreviewData'

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: { custom_endpoint_url: 'https://blog.acme.com/', blog_settings: { title: 'Acme Journal' } }
} as unknown as Workspace

const category = (id: string, slug: string, name: string): BlogCategory => ({
  id,
  slug,
  settings: { name, description: `${name} posts` },
  created_at: '',
  updated_at: ''
})

const categories = [category('c1', 'guides', 'Guides'), category('c2', 'news', 'News')]

describe('withWorkspaceData', () => {
  it("renders the sample posts under the workspace's own name, address and categories", () => {
    const data = withWorkspaceData(getMockDataForView('home'), workspace, categories, undefined)
    expect(data.workspace).toMatchObject({ id: 'ws1', name: 'Acme', blog_title: 'Acme Journal' })
    expect(data.base_url).toBe('https://blog.acme.com')
    expect(data.categories.map((c) => c.name)).toEqual(['Guides', 'News'])
    // No sample category is left anywhere.
    const json = JSON.stringify(data)
    for (const sample of ['Tutorials', 'Technology', 'tutorials', 'My Workspace', 'My Awesome Blog', 'example.com', 'cat-1']) {
      expect(json).not.toContain(sample)
    }
    expect(new Set(data.posts.map((post) => post.category_id))).toEqual(new Set(['c1', 'c2']))
  })

  it('opens a category page on a real category, with its posts only', () => {
    const data = withWorkspaceData(getMockDataForView('category'), workspace, categories, undefined)
    expect(data.category?.name).toBe('Guides')
    expect(data.posts.length).toBeGreaterThan(0)
    expect(data.posts.every((post) => post.category_id === 'c1')).toBe(true)
    expect(data.pagination?.total_count).toBe(data.posts.length)
    expect(data.current_url).toBe('https://blog.acme.com/guides')
    expect(data.page_title).toBe('Guides - Acme Journal')
  })

  it("files the sample post under a real category's address", () => {
    const data = withWorkspaceData(getMockDataForView('post'), workspace, categories, undefined)
    expect(data.post?.category_slug).toBe(data.category?.slug)
    expect(data.current_url).toBe(`https://blog.acme.com/${data.category?.slug}/${data.post?.slug}`)
  })

  it('keeps the sample categories of a blog that has none yet', () => {
    const data = withWorkspaceData(getMockDataForView('home'), workspace, [], undefined)
    expect(data.workspace.name).toBe('Acme')
    expect(data.categories.map((c) => c.name)).toEqual(['Tutorials', 'Technology', 'Design'])
  })
})
