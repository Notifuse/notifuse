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
  useBlocker: () => ({ status: 'idle', proceed: undefined, reset: undefined })
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
// SEOSettingsForm is rendered for real: stubbing it hid the fact that the
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
    expect(screen.queryByRole('button', { name: /Save Changes/i })).toBeNull()
    expect(screen.queryByRole('button', { name: /Enable Blog/i })).toBeNull()
    // The read-only status is shown instead.
    expect(screen.getByText(/Disabled/)).toBeInTheDocument()
  })

  it('renders the editable form when the user can manage and the blog is enabled', () => {
    renderComponent(true)
    expect(screen.getByPlaceholderText('My Blog WS')).toBeInTheDocument()
  })

  it('holds the save bar back until the form is touched', () => {
    renderComponent(true)
    expect(screen.queryByRole('button', { name: /Save Changes/i })).toBeNull()
    expect(screen.queryByText('You have unsaved changes')).toBeNull()
  })

  it('raises the save bar as soon as a field changes', () => {
    renderComponent(true)

    fireEvent.change(screen.getByPlaceholderText('My Blog WS'), {
      target: { value: 'New Blog Title' }
    })

    expect(screen.getByText('You have unsaved changes')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Save Changes/i })).toBeEnabled()
  })

  it('restores the stored values on Discard instead of emptying the form', () => {
    // resetFields() would blank the form: the values are pushed in through
    // setFieldsValue, so there are no initialValues to fall back on.
    renderComponent(true)

    const titleInput = screen.getByPlaceholderText('My Blog WS')
    fireEvent.change(titleInput, { target: { value: 'New Blog Title' } })
    fireEvent.click(screen.getByRole('button', { name: /^Discard$/i }))

    expect(titleInput).toHaveValue('Existing Title')
    expect(screen.queryByText('You have unsaved changes')).toBeNull()
  })

  it('loads the stored canonical URL and keeps it on save', async () => {
    // Every SEO key the form renders has to be listed in the loader: the save
    // replaces blog_settings wholesale, so a field left unloaded reaches the
    // backend empty and wipes what was stored.
    renderComponent(
      true,
      makeWorkspace({
        blog_settings: {
          title: 'Existing Title',
          seo: { canonical_url: 'https://example.com/canonical' }
        }
      })
    )

    expect(screen.getByPlaceholderText('https://example.com/original-post')).toHaveValue(
      'https://example.com/canonical'
    )

    fireEvent.change(screen.getByPlaceholderText('My Blog WS'), {
      target: { value: 'New Blog Title' }
    })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() =>
      expect(workspaceService.setBlogSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          blog_settings: expect.objectContaining({
            seo: expect.objectContaining({ canonical_url: 'https://example.com/canonical' })
          })
        })
      )
    )
  })

  it('restores the stored canonical URL on Discard', () => {
    renderComponent(
      true,
      makeWorkspace({
        blog_settings: {
          title: 'Existing Title',
          seo: { canonical_url: 'https://example.com/canonical' }
        }
      })
    )

    const canonical = screen.getByPlaceholderText('https://example.com/original-post')
    fireEvent.change(canonical, { target: { value: 'https://example.com/other' } })
    fireEvent.click(screen.getByRole('button', { name: /^Discard$/i }))

    expect(canonical).toHaveValue('https://example.com/canonical')
  })

  it('saves on Cmd/Ctrl+S once there are changes to save', async () => {
    renderComponent(true)

    fireEvent.keyDown(window, { key: 's', metaKey: true })
    expect(workspaceService.setBlogSettings).not.toHaveBeenCalled()

    fireEvent.change(screen.getByPlaceholderText('My Blog WS'), {
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
    const titleInput = screen.getByPlaceholderText('My Blog WS')
    fireEvent.change(titleInput, { target: { value: 'New Blog Title' } })

    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

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

    fireEvent.change(screen.getByPlaceholderText('My Blog WS'), {
      target: { value: 'New Blog Title' }
    })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() => expect(workspaceService.setBlogSettings).toHaveBeenCalled())
    const payload = vi.mocked(workspaceService.setBlogSettings).mock.calls[0][0]
    expect(payload).not.toHaveProperty('blog_enabled')
    expect(payload.blog_settings).toEqual(
      expect.objectContaining({ title: 'New Blog Title' })
    )
  })

  it('sends blog_enabled false when the blog is disabled from the confirmation', async () => {
    renderComponent(true)

    fireEvent.click(screen.getByRole('button', { name: /Disable Blog/i }))
    // okText repeats the trigger's label, so scope the confirm click to the dialog.
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Disable Blog/i }))

    await waitFor(() =>
      expect(workspaceService.setBlogSettings).toHaveBeenCalledWith(
        expect.objectContaining({ blog_enabled: false })
      )
    )
  })

  it('sends blog_enabled true from the Enable Blog button', async () => {
    renderComponent(true, makeWorkspace({ blog_enabled: false }))

    const enable = screen.getByRole('button', { name: /Enable Blog/i })
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
