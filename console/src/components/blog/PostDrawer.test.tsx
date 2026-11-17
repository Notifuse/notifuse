import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import { activateDayjsLocale } from '../../i18n/dayjsLocales'
import type { Workspace } from '../../services/api/types'
import { PostDrawer } from './PostDrawer'

// services/api/client imports the router, which imports every page and cycles back here.
vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() },
  ApiError: class ApiError extends Error {}
}))

vi.mock('../../services/api/blog', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/blog')>()),
  blogCategoriesApi: { list: vi.fn(async () => ({ categories: [], total_count: 0 })) }
}))

// The editor, the assistant and the image field are not what this file tests: Tiptap does not run in
// jsdom, and the image field needs the file manager.
vi.mock('../blog_editor', () => ({
  NotifuseEditor: ({ initialContent }: { initialContent?: string }) => (
    <div data-testid="editor" data-initial-content={initialContent ?? ''} />
  ),
  DEFAULT_INITIAL_CONTENT: '<h2>Editor demo</h2><p>Sample text</p>'
}))
vi.mock('./BlogAIAssistant', () => ({ BlogAIAssistant: () => null }))
vi.mock('../common/ImageURLInput', () => ({ ImageURLInput: () => null }))

const workspace = { id: 'ws1', name: 'Workspace', settings: {} } as unknown as Workspace

const renderDrawer = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <App>
          <PostDrawer open onClose={vi.fn()} workspace={workspace} />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )

beforeEach(() => {
  localStorage.clear()
  i18n.load('fr', {})
  i18n.activate('fr')
  activateDayjsLocale('fr')
})

afterEach(() => {
  i18n.activate('en')
  activateDayjsLocale('en')
})

describe('PostDrawer in French', () => {
  it('dates the unsaved draft it offers to restore in French', async () => {
    localStorage.setItem(
      'blog-post-draft-new-ws1',
      JSON.stringify({
        content: { type: 'doc', content: [] },
        formValues: { title: 'Brouillon' },
        savedAt: new Date(2026, 10, 14, 10, 30).getTime()
      })
    )

    renderDrawer()

    // "lll" in French: day, short month, year, then the 24-hour time
    expect(
      await screen.findByText(/^Found unsaved changes from 14 nov\. 2026 10:30$/)
    ).toBeInTheDocument()
  })
})

describe('PostDrawer, new post', () => {
  it('opens the editor of a new post empty, without the editor demo text', async () => {
    renderDrawer()
    const editor = await screen.findByTestId('editor')
    expect(editor).toHaveAttribute('data-initial-content', '')
  })
})
