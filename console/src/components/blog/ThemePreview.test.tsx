import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ThemePreview } from './ThemePreview'
import type { BlogThemeFiles } from '../../services/api/blog'

vi.mock('../../utils/liquidRenderer', () => ({
  renderBlogPage: vi.fn(async () => ({ success: true, html: '<p>Blog</p>' }))
}))

const files = {} as BlogThemeFiles

describe('ThemePreview', () => {
  it('runs theme scripts in an origin of their own', async () => {
    render(<ThemePreview files={files} view="home" />)

    const frame = await screen.findByTitle('Blog Preview')
    expect(frame.getAttribute('srcdoc')).toBe('<p>Blog</p>')
    expect(frame.getAttribute('sandbox')).toBe('allow-scripts')
  })
})
