import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ThemePreview } from './ThemePreview'
import { renderBlogPage } from '../../utils/liquidRenderer'
import { withPreviewStorage } from '../../utils/previewStorage'
import type { BlogThemeFiles } from '../../services/api/blog'

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
})
