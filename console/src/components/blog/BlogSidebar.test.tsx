import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BlogSidebar } from './BlogSidebar'
import type { BlogCategory } from '../../services/api/blog'

const categories = [
  { id: 'c1', slug: 'news', settings: { name: 'News' }, created_at: '', updated_at: '' },
  { id: 'c2', slug: 'guides', settings: { name: 'Guides' }, created_at: '', updated_at: '' }
] as unknown as BlogCategory[]

vi.mock('../../services/api/blog', () => ({
  blogCategoriesApi: { list: vi.fn(() => Promise.resolve({ categories })) }
}))

function renderSidebar(activeCategoryId: string | null = null) {
  const handlers = {
    onCategoryChange: vi.fn(),
    onNewCategory: vi.fn(),
    onEditCategory: vi.fn(),
    onDeleteCategory: vi.fn()
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <BlogSidebar workspaceId="ws1" activeCategoryId={activeCategoryId} {...handlers} />
    </QueryClientProvider>
  )
  return handlers
}

describe('BlogSidebar', () => {
  it('shows all posts, current when no category is picked', async () => {
    const { onCategoryChange } = renderSidebar()
    await screen.findByRole('button', { name: /^News/ })
    const all = screen.getByRole('button', { name: 'All Posts' })
    expect(all).toHaveAttribute('aria-current', 'page')
    fireEvent.click(all)
    expect(onCategoryChange).toHaveBeenCalledWith(null)
  })

  it('picks a category by its id, and marks the current one', async () => {
    const { onCategoryChange } = renderSidebar('c2')
    fireEvent.click(await screen.findByRole('button', { name: /^News/ }))
    expect(onCategoryChange).toHaveBeenCalledWith('c1')
    expect(screen.getByRole('button', { name: /^Guides/ })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('button', { name: 'All Posts' })).not.toHaveAttribute('aria-current')
  })

  it('edits and deletes a category without selecting it', async () => {
    const { onCategoryChange, onEditCategory, onDeleteCategory } = renderSidebar()
    fireEvent.click(await screen.findByRole('button', { name: 'Edit News' }))
    expect(onEditCategory).toHaveBeenCalledWith(categories[0])
    fireEvent.click(screen.getByRole('button', { name: 'Delete Guides' }))
    expect(onDeleteCategory).toHaveBeenCalledWith(categories[1])
    expect(onCategoryChange).not.toHaveBeenCalled()
  })

  it('creates a category from the section action', async () => {
    const { onNewCategory } = renderSidebar()
    fireEvent.click(await screen.findByRole('button', { name: 'New Category' }))
    expect(onNewCategory).toHaveBeenCalledTimes(1)
  })

  it('nests no button inside another', async () => {
    renderSidebar()
    await screen.findByRole('button', { name: /^News/ })
    expect(document.querySelector('button button')).toBeNull()
  })
})
