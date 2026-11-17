import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ThemePreview } from './ThemePreview'
import { renderBlogPage } from '../../utils/liquidRenderer'
import { withPreviewStorage } from '../../utils/previewStorage'
import type { BlogCategory, BlogThemeFiles } from '../../services/api/blog'
import type { Workspace } from '../../services/api/types'

vi.mock('../../utils/liquidRenderer', () => ({
  renderBlogPage: vi.fn()
}))

const files = {} as BlogThemeFiles
const html =
  '<!DOCTYPE html><html lang="en"><head><title>T</title></head>' +
  '<body><script>theme()</script></body></html>'

describe('ThemePreview', () => {
  it('runs theme scripts in an origin of their own, with storage they can use there', async () => {
    vi.mocked(renderBlogPage).mockResolvedValue({ success: true, html })
    render(<ThemePreview files={files} view="home" />)

    const frame = await screen.findByTitle('Blog Preview')
    expect(frame.getAttribute('sandbox')).toBe('allow-scripts')
    expect(frame.getAttribute('srcdoc')).toBe(withPreviewStorage(html))
  })

  it("renders the theme with the workspace's name and categories, not the sample ones", async () => {
    vi.mocked(renderBlogPage).mockResolvedValue({ success: true, html })
    const workspace = { id: 'ws1', name: 'Acme', settings: {} } as unknown as Workspace
    const categories = [
      { id: 'c1', slug: 'guides', settings: { name: 'Guides' }, created_at: '', updated_at: '' }
    ] as BlogCategory[]
    render(<ThemePreview files={files} view="home" workspace={workspace} categories={categories} />)

    await screen.findByTitle('Blog Preview')
    const data = vi.mocked(renderBlogPage).mock.calls.at(-1)?.[2]
    expect(data?.workspace.name).toBe('Acme')
    expect(data?.categories.map((category) => category.name)).toEqual(['Guides'])
  })
})
