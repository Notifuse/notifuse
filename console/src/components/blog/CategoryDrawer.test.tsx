import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { BlogCategory } from '../../services/api/blog'
import type { Workspace } from '../../services/api/types'

vi.mock('../../services/api/blog', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/blog')>()
  return { ...actual, blogCategoriesApi: { create: vi.fn(), update: vi.fn() } }
})
// The picker is the file manager's own drawer; the field stays a plain URL input here.
vi.mock('../file_manager/context', () => ({ useFileManager: () => ({ SelectFileButton: () => null }) }))

import { CategoryDrawer, type CategoryDrawerProps } from './CategoryDrawer'
import { blogCategoriesApi } from '../../services/api/blog'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: { custom_endpoint_url: 'https://blog.acme.com/', default_language: 'en', languages: ['en'] }
} as unknown as Workspace

const guides = {
  id: 'c1',
  slug: 'guides',
  settings: { name: 'Guides', description: 'How-tos for everyone', seo: { keywords: ['email'] } },
  created_at: '',
  updated_at: ''
} as BlogCategory

function renderDrawer(props: Partial<CategoryDrawerProps> = {}) {
  const handlers = { onClose: vi.fn(), onDelete: vi.fn(), onCreated: vi.fn() }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <QueryClientProvider client={client}>
          <CategoryDrawer open workspace={workspace} {...handlers} {...props} />
        </QueryClientProvider>
      </App>
    </I18nProvider>
  )
  return handlers
}

const field = (label: string) => screen.getByLabelText(label) as HTMLInputElement
const bar = () => document.querySelector('[data-drawer-bar]') as HTMLElement

describe('CategoryDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('creates a category: the slug follows the name, and the request carries what was typed', async () => {
    const created = { ...guides, id: 'c9', slug: 'customer-stories' }
    vi.mocked(blogCategoriesApi.create).mockResolvedValue({ category: created })
    const user = userEvent.setup()
    const { onCreated, onClose } = renderDrawer()

    expect(within(bar()).getByText('New category')).toBeInTheDocument()
    expect(within(bar()).getByText('Not created')).toBeInTheDocument()
    await user.type(field('Name'), 'Customer Stories')
    expect(field('Slug')).toHaveValue('customer-stories')
    await user.type(field('Meta title'), 'Stories from our customers')
    await user.click(screen.getByRole('button', { name: 'Create category' }))

    await waitFor(() => expect(blogCategoriesApi.create).toHaveBeenCalledTimes(1))
    const [workspaceId, request] = vi.mocked(blogCategoriesApi.create).mock.calls[0]
    expect(workspaceId).toBe('ws1')
    expect(request).toMatchObject({
      name: 'Customer Stories',
      slug: 'customer-stories',
      seo: { meta_title: 'Stories from our customers' }
    })
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(created))
    expect(onClose).toHaveBeenCalled()
  })

  it('stops following the name once the slug is typed in', async () => {
    const user = userEvent.setup()
    renderDrawer()
    await user.type(field('Name'), 'News')
    await user.clear(field('Slug'))
    await user.type(field('Slug'), 'press')
    await user.type(field('Name'), ' and updates')
    expect(field('Slug')).toHaveValue('press')
    expect(screen.queryByText('Follows the name until you edit it')).not.toBeInTheDocument()
  })

  it('refuses to create without a name', async () => {
    const user = userEvent.setup()
    renderDrawer()
    await user.click(screen.getByRole('button', { name: 'Create category' }))
    expect(await screen.findByText('Please enter a category name')).toBeInTheDocument()
    expect(blogCategoriesApi.create).not.toHaveBeenCalled()
  })

  it('edits a category: slug locked, Save once something changed, and stays open on what was saved', async () => {
    vi.mocked(blogCategoriesApi.update).mockImplementation(async (_ws, request) => ({
      category: { ...guides, settings: { name: request.name, description: request.description, seo: request.seo } }
    }))
    const user = userEvent.setup()
    const { onClose } = renderDrawer({ category: guides, postCount: 4 })

    expect(within(bar()).getByText('Guides')).toBeInTheDocument()
    expect(within(bar()).getByText('4 posts')).toBeInTheDocument()
    expect(field('Slug')).toBeDisabled()
    expect(screen.getByText("Fixed once created: its posts' links depend on it")).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()

    await user.type(field('Meta description'), 'Step-by-step guides')
    expect(within(bar()).getByText('Unsaved changes')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(blogCategoriesApi.update).toHaveBeenCalledTimes(1))
    expect(vi.mocked(blogCategoriesApi.update).mock.calls[0][1]).toMatchObject({
      id: 'c1',
      name: 'Guides',
      slug: 'guides',
      description: 'How-tos for everyone',
      seo: { meta_description: 'Step-by-step guides', keywords: ['email'] }
    })
    await waitFor(() => expect(within(bar()).queryByText('Unsaved changes')).not.toBeInTheDocument())
    expect(onClose).not.toHaveBeenCalled()
  })

  it('discards the changes, also clearing a field that was empty', async () => {
    const user = userEvent.setup()
    renderDrawer({ category: guides })
    await user.type(field('Meta title'), 'Our guides')
    await user.clear(field('Description'))
    await user.click(screen.getByRole('button', { name: 'Discard' }))
    expect(field('Meta title')).toHaveValue('')
    expect(field('Description')).toHaveValue('How-tos for everyone')
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })

  it('closes at once without changes, and asks first with some', async () => {
    const user = userEvent.setup()
    const { onClose } = renderDrawer({ category: guides })
    await user.click(screen.getByRole('button', { name: 'Blog' }))
    expect(onClose).toHaveBeenCalledTimes(1)

    await user.type(field('Meta title'), 'Our guides')
    await user.click(screen.getByRole('button', { name: 'Blog' }))
    expect((await screen.findAllByText('Close without saving?')).length).toBeGreaterThan(0)
    expect(onClose).toHaveBeenCalledTimes(1)
    // The confirmation's own Close, not the drawer's.
    await user.click(within(document.querySelector('.ant-modal-confirm') as HTMLElement).getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('lists each empty tag with what the page shows instead, as the default theme writes it', async () => {
    renderDrawer({ category: guides })
    const check = (key: string) => document.querySelector(`[data-check="${key}"]`) as HTMLElement
    expect(check('name')).toHaveAttribute('data-ok', 'true')
    expect(check('meta_title')).toHaveTextContent('Uses “Guides - Acme”')
    expect(check('meta_description')).toHaveTextContent('Uses “How-tos for everyone”')
    // The share title is the name alone.
    expect(check('og_title')).toHaveTextContent('Uses “Guides”')
    expect(check('og_image')).toHaveTextContent('Links show without a picture')
    expect(screen.getByText('5 to check')).toBeInTheDocument()
  })

  it('brings the field of a check into focus', async () => {
    const user = userEvent.setup()
    renderDrawer({ category: guides })
    const metaTitle = document.querySelector('[data-check="meta_title"]') as HTMLElement
    await user.click(within(metaTitle).getByRole('button', { name: 'Add' }))
    expect(field('Meta title')).toHaveFocus()
  })

  it('brings the share image field into focus from its check', async () => {
    const user = userEvent.setup()
    renderDrawer({ category: guides })
    const ogImage = document.querySelector('[data-check="og_image"]') as HTMLElement
    await user.click(within(ogImage).getByRole('button', { name: 'Add' }))
    expect(field('Share image')).toHaveFocus()
  })

  it('previews the search result and the shared link', async () => {
    const user = userEvent.setup()
    renderDrawer({ category: guides })
    const search = document.querySelector('[data-preview="search"]') as HTMLElement
    expect(search).toHaveTextContent('blog.acme.com › guides')
    expect(search).toHaveTextContent('Guides - Acme')

    await user.click(screen.getByText('Social'))
    const social = document.querySelector('[data-preview="social"]') as HTMLElement
    expect(social).toHaveTextContent('Guides')
    expect(social).toHaveTextContent('No share image')
  })

  it('opens the public page and deletes from the bar', async () => {
    const user = userEvent.setup()
    const { onDelete } = renderDrawer({ category: guides })
    expect(screen.getByRole('link', { name: /View page/ })).toHaveAttribute('href', 'https://blog.acme.com/guides')
    await user.click(screen.getByRole('button', { name: 'More actions' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete category' }))
    expect(onDelete).toHaveBeenCalledWith(guides)
  })

  it('offers no indexing field, and keeps the one a category already has when it is saved', async () => {
    vi.mocked(blogCategoriesApi.update).mockResolvedValue({ category: guides })
    const user = userEvent.setup()
    const indexed = { ...guides, settings: { ...guides.settings, seo: { meta_robots: 'noindex,follow' } } }
    renderDrawer({ category: indexed })
    expect(screen.queryByLabelText('Indexing')).not.toBeInTheDocument()

    await user.type(field('Meta title'), 'Our guides')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(blogCategoriesApi.update).toHaveBeenCalledTimes(1))
    expect(vi.mocked(blogCategoriesApi.update).mock.calls[0][1].seo).toMatchObject({
      meta_title: 'Our guides',
      meta_robots: 'noindex,follow'
    })
  })

  it('acts on the category as saved once the drawer stays open after a save', async () => {
    const renamed = { ...guides, settings: { ...guides.settings, name: 'How-to guides' } }
    vi.mocked(blogCategoriesApi.update).mockResolvedValue({ category: renamed })
    const user = userEvent.setup()
    const { onDelete } = renderDrawer({ category: guides })
    await user.clear(field('Name'))
    await user.type(field('Name'), 'How-to guides')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(blogCategoriesApi.update).toHaveBeenCalled())
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled())
    await user.click(screen.getByRole('button', { name: 'More actions' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete category' }))
    expect(onDelete).toHaveBeenCalledWith(renamed)
  })
})
