import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BlogSidebar } from './BlogSidebar'
import type { BlogCategory, BlogPostCounts } from '../../services/api/blog'

const state = vi.hoisted(() => ({ categories: [] as unknown[] }))
const categories = [
  { id: 'c1', slug: 'news', settings: { name: 'News' }, created_at: '', updated_at: '' },
  { id: 'c2', slug: 'guides', settings: { name: 'Guides' }, created_at: '', updated_at: '' }
] as unknown as BlogCategory[]

vi.mock('../../services/api/blog', () => ({
  blogCategoriesApi: { list: vi.fn(() => Promise.resolve({ categories: state.categories })) }
}))

const counts: BlogPostCounts = {
  all_total: 12,
  total: 3,
  published: 2,
  scheduled: 1,
  draft: 1,
  by_category: { c1: 9 }
}

function renderSidebar(
  props: Partial<{
    activeCategoryId: string | null
    counts: BlogPostCounts
    canManageCategories: boolean
    withCategories: boolean
  }> = {}
) {
  state.categories = props.withCategories === false ? [] : categories
  const handlers = {
    onCategoryChange: vi.fn(),
    onNewCategory: vi.fn(),
    onEditCategory: vi.fn(),
    onDeleteCategory: vi.fn()
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <BlogSidebar
        workspaceId="ws1"
        activeCategoryId={props.activeCategoryId ?? null}
        counts={props.counts}
        canManageCategories={props.canManageCategories ?? true}
        {...handlers}
      />
    </QueryClientProvider>
  )
  return handlers
}

const itemOf = (name: RegExp) =>
  screen.getByRole('button', { name }).closest('[data-rail-item]') as HTMLElement

describe('BlogSidebar', () => {
  it('shows all posts, current when no category is picked', async () => {
    const { onCategoryChange } = renderSidebar()
    await screen.findByRole('button', { name: /^News/ })
    const all = screen.getByRole('button', { name: /^All posts/ })
    expect(all).toHaveAttribute('aria-current', 'page')
    fireEvent.click(all)
    expect(onCategoryChange).toHaveBeenCalledWith(null)
  })

  it('picks a category by its id, and marks the current one', async () => {
    const { onCategoryChange } = renderSidebar({ activeCategoryId: 'c2' })
    fireEvent.click(await screen.findByRole('button', { name: /^News/ }))
    expect(onCategoryChange).toHaveBeenCalledWith('c1')
    expect(screen.getByRole('button', { name: /^Guides/ })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('button', { name: /^All posts/ })).not.toHaveAttribute('aria-current')
  })

  it('counts every post beside All posts and each category, 0 for a category without any', async () => {
    renderSidebar({ counts, activeCategoryId: 'c1' })
    await screen.findByRole('button', { name: /^News/ })
    // all_total stays the count of every post, whatever the category picked
    expect(itemOf(/^All posts/)).toHaveTextContent('12')
    expect(itemOf(/^News/)).toHaveTextContent('9')
    expect(itemOf(/^Guides/)).toHaveTextContent('0')
  })

  it('shows no number while the counts are not there, rather than zeros', async () => {
    renderSidebar()
    await screen.findByRole('button', { name: /^News/ })
    expect(itemOf(/^All posts/).textContent).toBe('All posts')
    expect(itemOf(/^Guides/).textContent).not.toMatch(/\d/)
  })

  it('edits and deletes a category from its menu without selecting it', async () => {
    const { onCategoryChange, onEditCategory, onDeleteCategory } = renderSidebar()
    fireEvent.click(await screen.findByRole('button', { name: 'Actions for News' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
    expect(onEditCategory).toHaveBeenCalledWith(categories[0])

    fireEvent.click(screen.getByRole('button', { name: 'Actions for Guides' }))
    const menus = await screen.findAllByRole('menu')
    fireEvent.click(within(menus[menus.length - 1]).getByRole('menuitem', { name: 'Delete' }))
    expect(onDeleteCategory).toHaveBeenCalledWith(categories[1])
    expect(onCategoryChange).not.toHaveBeenCalled()
  })

  it('creates a category from the section action', async () => {
    const { onNewCategory } = renderSidebar()
    fireEvent.click(await screen.findByRole('button', { name: 'New category' }))
    expect(onNewCategory).toHaveBeenCalledTimes(1)
  })

  it('offers no category action without Blog write', async () => {
    renderSidebar({ canManageCategories: false })
    await screen.findByRole('button', { name: /^News/ })
    expect(screen.queryByRole('button', { name: 'New category' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Actions for News' })).not.toBeInTheDocument()
  })

  it('says when there is no category yet', async () => {
    renderSidebar({ withCategories: false })
    expect(await screen.findByText('No categories yet')).toBeInTheDocument()
  })

  it('nests no button inside another', async () => {
    renderSidebar()
    await screen.findByRole('button', { name: /^News/ })
    expect(document.querySelector('button button')).toBeNull()
  })
})
