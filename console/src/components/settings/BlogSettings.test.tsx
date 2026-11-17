import type React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { BlogSettings } from './BlogSettings'
import type { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'

// Blog settings must be saved via the dedicated, blog:write-gated endpoint
// (setBlogSettings) and NOT via the owner-only workspaces.update.
vi.mock('../../services/api/workspace', () => ({
  workspaceService: {
    setBlogSettings: vi.fn().mockResolvedValue({ status: 'success' }),
    update: vi.fn(),
    get: vi.fn().mockResolvedValue({
      workspace: { id: 'ws1', settings: { blog_enabled: true } }
    })
  }
}))

// The save bar guards navigation away from unsaved edits; the suite renders the
// section outside a router, so the blocker is stubbed idle.
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle', proceed: undefined, reset: undefined }),
  Link: ({ children, params }: { children: React.ReactNode; params?: { section?: string } }) => (
    <a href={`/settings/${params?.section ?? ''}`}>{children}</a>
  )
}))

vi.mock('../../services/api/blog', () => ({
  blogThemesApi: {
    list: vi.fn().mockResolvedValue({ themes: [{ version: 1 }] }),
    create: vi.fn(),
    publish: vi.fn()
  }
}))

// Stub heavy child components that pull in their own data/context dependencies
// (file-manager context, react-query, drawers); they are out of scope here.
vi.mock('../blog/RecentThemesTable', () => ({
  RecentThemesTable: () => <div data-testid="recent-themes" />
}))
vi.mock('../common/ImageURLInput', () => ({
  ImageURLInput: () => <input data-testid="image-url-input" />
}))
// The SEO fields are rendered for real: stubbing them hid the fact that the
// section loads the SEO block field by field, so a field the form renders but
// the loader forgets is blanked on the next save.

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

const makeWorkspace = (settingsOverrides: Record<string, unknown> = {}): Workspace =>
  ({
    id: 'ws1',
    name: 'My Blog WS',
    settings: {
      timezone: 'UTC',
      custom_endpoint_url: 'https://blog.example.com',
      blog_enabled: true,
      blog_settings: { title: 'Existing Title' },
      ...settingsOverrides
    }
  }) as unknown as Workspace

const renderComponent = (canManage: boolean, workspace: Workspace | null = makeWorkspace()) =>
  render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <BlogSettings workspace={workspace} onWorkspaceUpdate={vi.fn()} canManage={canManage} />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )

describe('BlogSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a read-only view (no editor) when the user cannot manage', () => {
    renderComponent(false, makeWorkspace({ blog_enabled: false }))
    // No editor controls are rendered for non-managers.
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Turn on the blog/i })).toBeNull()
    // The read-only status is shown instead.
    expect(screen.getByText(/Disabled/)).toBeInTheDocument()
  })

  it('renders the editable form when the user can manage and the blog is enabled', () => {
    renderComponent(true)
    expect(screen.getByLabelText('Title')).toBeInTheDocument()
  })

  it('holds the save bar back until the form is touched', () => {
    renderComponent(true)
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
    expect(screen.queryByText('Unsaved changes')).toBeNull()
  })

  it('raises the save bar as soon as a field changes', () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: 'New Blog Title' }
    })

    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  })

  it('restores the stored values on Discard instead of emptying the form', () => {
    // resetFields() would blank the form: the values are pushed in through
    // setFieldsValue, so there are no initialValues to fall back on.
    renderComponent(true)

    const titleInput = screen.getByLabelText('Title')
    fireEvent.change(titleInput, { target: { value: 'New Blog Title' } })
    fireEvent.click(screen.getByRole('button', { name: /^Discard$/i }))

    expect(titleInput).toHaveValue('Existing Title')
    expect(screen.queryByText('Unsaved changes')).toBeNull()
  })

  it('shows no canonical URL field, and lets the stored one go on save', async () => {
    // The server no longer hands a blog-wide canonical to themes (it pointed every page of an
    // older theme at one address); the console stops resending it, so the next save clears it.
    renderComponent(
      true,
      makeWorkspace({
        blog_settings: { title: 'Existing Title', seo: { canonical_url: 'https://example.com/canonical' } }
      })
    )
    expect(screen.queryByText('Canonical URL')).toBeNull()

    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'New Blog Title' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(workspaceService.setBlogSettings).toHaveBeenCalled())
    const sent = vi.mocked(workspaceService.setBlogSettings).mock.calls[0][0]
    expect(sent.blog_settings?.title).toBe('New Blog Title')
    expect(sent.blog_settings?.seo).not.toHaveProperty('canonical_url')
  })

  it('turns the blog back on with every setting it had', async () => {
    // Every blog_settings field is unmounted while the blog is off: the save must read the
    // whole store, or it replaces the stored settings with the title alone.
    renderComponent(
      true,
      makeWorkspace({
        blog_enabled: false,
        blog_settings: {
          title: 'Acme blog',
          logo_url: 'https://cdn.example.com/logo.png',
          home_page_size: 10,
          feed_summary_only: true,
          seo: { meta_title: 'Acme', meta_robots: 'noindex,follow' }
        }
      })
    )
    const turnOn = screen.getByRole('button', { name: /Turn on the blog/i })
    await waitFor(() => expect(turnOn).toBeEnabled())
    fireEvent.click(turnOn)
    await waitFor(() => expect(workspaceService.setBlogSettings).toHaveBeenCalled())
    expect(vi.mocked(workspaceService.setBlogSettings).mock.calls[0][0]).toMatchObject({
      blog_enabled: true,
      blog_settings: {
        title: 'Acme blog',
        logo_url: 'https://cdn.example.com/logo.png',
        home_page_size: 10,
        feed_summary_only: true,
        seo: { meta_title: 'Acme', meta_robots: 'noindex,follow' }
      }
    })
  })

  it('loads and restores the stored meta title on Discard', () => {
    renderComponent(true, makeWorkspace({ blog_settings: { title: 'Existing Title', seo: { meta_title: 'Acme blog' } } }))
    const metaTitle = screen.getByLabelText('Meta title')
    expect(metaTitle).toHaveValue('Acme blog')
    fireEvent.change(metaTitle, { target: { value: 'Other' } })
    fireEvent.click(screen.getByRole('button', { name: /^Discard$/i }))
    expect(metaTitle).toHaveValue('Acme blog')
  })

  it('saves the feed content as the summary-only flag', async () => {
    renderComponent(true)
    fireEvent.click(screen.getByText('Excerpt only'))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(workspaceService.setBlogSettings).toHaveBeenCalledWith(
        expect.objectContaining({ blog_settings: expect.objectContaining({ feed_summary_only: true }) })
      )
    )
  })

  it('points to General when the workspace has no custom domain, and cannot turn the blog on', () => {
    renderComponent(true, makeWorkspace({ blog_enabled: false, custom_endpoint_url: '' }))
    expect(screen.getByRole('link', { name: 'Set it in General' })).toHaveAttribute('href', '/settings/general')
    expect(screen.getByRole('button', { name: /Turn on the blog/i })).toBeDisabled()
  })

  it('saves on Cmd/Ctrl+S once there are changes to save', async () => {
    renderComponent(true)

    fireEvent.keyDown(window, { key: 's', metaKey: true })
    expect(workspaceService.setBlogSettings).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: 'New Blog Title' }
    })
    fireEvent.keyDown(window, { key: 's', metaKey: true })

    await waitFor(() =>
      expect(workspaceService.setBlogSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          blog_settings: expect.objectContaining({ title: 'New Blog Title' })
        })
      )
    )
  })

  it('saves via setBlogSettings (not workspace.update)', async () => {
    renderComponent(true)

    // Change the blog title to mark the form as touched (enables the Save button).
    const titleInput = screen.getByLabelText('Title')
    fireEvent.change(titleInput, { target: { value: 'New Blog Title' } })

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(workspaceService.setBlogSettings).toHaveBeenCalledWith(
        expect.objectContaining({ workspace_id: 'ws1' })
      )
    })
    expect(workspaceService.update).not.toHaveBeenCalled()
  })

  // The save endpoint leaves the stored flag alone when the body names no
  // blog_enabled, so editing the title must not carry an opinion about whether
  // the blog is on. Only the two controls that exist to toggle it may say so.
  it('leaves blog_enabled out of the payload on an ordinary settings save', async () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: 'New Blog Title' }
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(workspaceService.setBlogSettings).toHaveBeenCalled())
    const payload = vi.mocked(workspaceService.setBlogSettings).mock.calls[0][0]
    expect(payload).not.toHaveProperty('blog_enabled')
    expect(payload.blog_settings).toEqual(
      expect.objectContaining({ title: 'New Blog Title' })
    )
  })

  it('sends blog_enabled false when the blog is disabled from the confirmation', async () => {
    renderComponent(true)

    fireEvent.click(screen.getByRole('button', { name: /Turn off the blog/i }))
    // okText repeats the trigger's label, so scope the confirm click to the dialog.
    const dialog = await screen.findByRole('dialog')
    // Turning off keeps the settings: the server only clears the flag.
    expect(dialog).toHaveTextContent('Its posts and settings are kept')
    fireEvent.click(within(dialog).getByRole('button', { name: /Turn off the blog/i }))

    await waitFor(() =>
      expect(workspaceService.setBlogSettings).toHaveBeenCalledWith(
        expect.objectContaining({ blog_enabled: false })
      )
    )
  })

  it('sends blog_enabled true from the Turn on button', async () => {
    renderComponent(true, makeWorkspace({ blog_enabled: false }))

    const enable = screen.getByRole('button', { name: /Turn on the blog/i })
    // The button stays disabled until the themes query settles.
    await waitFor(() => expect(enable).toBeEnabled())
    fireEvent.click(enable)

    await waitFor(() =>
      expect(workspaceService.setBlogSettings).toHaveBeenCalledWith(
        expect.objectContaining({ blog_enabled: true })
      )
    )
  })
})
