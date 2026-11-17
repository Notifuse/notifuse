import { beforeEach, describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { THEME_PRESETS } from './themePresets'
import type { BlogTheme } from '../../services/api/blog'

// Monaco does not run in jsdom: a textarea stands in, so a file can be edited.
vi.mock('@monaco-editor/react', () => ({
  default: (props: { value?: string; path?: string; onChange?: (v: string) => void }) => (
    <textarea data-testid="code-editor" data-path={props.path} value={props.value} onChange={(e) => props.onChange?.(e.target.value)} />
  )
}))
vi.mock('./ThemePreview', () => ({ ThemePreview: (props: { view: string }) => <div data-testid="theme-preview" data-view={props.view} /> }))
// services/api/blog imports the API client, and with it the router.
vi.mock('../../services/api/blog', () => ({
  blogThemesApi: { create: vi.fn(), update: vi.fn(), publish: vi.fn(async () => ({})) }
}))

import { ThemeEditorDrawer } from './ThemeEditorDrawer'
import { blogThemesApi } from '../../services/api/blog'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const theme = (published: boolean): BlogTheme =>
  ({
    version: 3,
    published_at: published ? '2026-11-10T00:00:00Z' : null,
    files: THEME_PRESETS[0].files,
    notes: '',
    created_at: '2026-11-01T00:00:00Z',
    updated_at: '2026-11-01T00:00:00Z'
  }) as BlogTheme

function renderEditor(current: BlogTheme | null) {
  const onClose = vi.fn()
  render(
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient()}>
        <App>
          <ThemeEditorDrawer open onClose={onClose} theme={current} workspaceId="ws1" />
        </App>
      </QueryClientProvider>
    </I18nProvider>
  )
  return { onClose }
}

// One more character in the open file (typing into a 6,000-character textarea is too slow here).
const edit = () => {
  const editor = screen.getByTestId('code-editor') as HTMLTextAreaElement
  fireEvent.change(editor, { target: { value: `${editor.value} ` } })
}
const bar = () => document.querySelector('[data-drawer-bar]') as HTMLElement
const dialog = () => document.querySelector('.ant-modal-wrap:not([style*="display: none"]) .ant-modal') as HTMLElement

describe('ThemeEditorDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('shows the files, the code and the preview side by side, filling the height', () => {
    renderEditor(theme(true))
    expect(within(bar()).getByText('Theme v3')).toBeInTheDocument()
    expect(within(bar()).getByText('Published')).toBeInTheDocument()
    expect(document.querySelectorAll('[data-file]')).toHaveLength(8)
    expect(screen.getByTestId('code-editor')).toHaveAttribute('data-path', 'home.liquid')
    expect(screen.getByTestId('theme-preview')).toHaveAttribute('data-view', 'home')
    // Heights guessed from the viewport left a band under the editor once the bar grew.
    const guessed = [...document.querySelectorAll<HTMLElement>('[style]')].filter((el) => el.style.height.includes('100vh'))
    expect(guessed).toEqual([])
    expect(document.querySelector<HTMLElement>('[data-theme-editor-pane]')!.style.height).toBe('100%')
    expect(document.querySelector<HTMLElement>('[data-theme-preview-pane]')!.style.height).toBe('100%')
  })

  it('switches between code, split and preview', async () => {
    const user = userEvent.setup()
    renderEditor(theme(true))
    await user.click(within(bar()).getByText('Code'))
    expect(screen.queryByTestId('theme-preview')).toBeNull()
    expect(screen.getByTestId('code-editor')).toBeInTheDocument()
    await user.click(within(bar()).getByText('Preview'))
    expect(screen.queryByTestId('code-editor')).toBeNull()
    expect(document.querySelector('[data-theme-files]')).toBeNull()
    expect(screen.getByTestId('theme-preview')).toBeInTheDocument()
  })

  it('opens a file from the column and marks it once changed', async () => {
    const user = userEvent.setup()
    renderEditor(theme(true))
    await user.click(screen.getByRole('button', { name: /^styles\.css/ }))
    expect(screen.getByTestId('code-editor')).toHaveAttribute('data-path', 'styles.css')
    edit()
    expect(screen.getByRole('button', { name: /^styles\.css/ })).toHaveTextContent('(changed)')
    expect(within(bar()).getByText('Unsaved changes')).toBeInTheDocument()
  })

  it('saves a published theme as a new version, without publishing it', async () => {
    vi.mocked(blogThemesApi.create).mockResolvedValue({ theme: { ...theme(false), version: 4 } } as never)
    const user = userEvent.setup()
    renderEditor(theme(true))
    edit()
    await user.click(within(bar()).getByRole('button', { name: 'Save as a new version' }))
    await user.click(within(dialog()).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(blogThemesApi.create).toHaveBeenCalledWith('ws1', expect.objectContaining({ notes: 'Edited from v3' })))
    expect(blogThemesApi.publish).not.toHaveBeenCalled()
  })

  it('saves and publishes the version it saved, after warning it goes live', async () => {
    vi.mocked(blogThemesApi.create).mockResolvedValue({ theme: { ...theme(false), version: 4 } } as never)
    const user = userEvent.setup()
    renderEditor(theme(true))
    edit()
    await user.click(within(bar()).getByRole('button', { name: 'Save and publish' }))
    expect(dialog()).toHaveTextContent('the currently published theme will be unpublished automatically')
    await user.click(within(dialog()).getByRole('button', { name: 'Publish' }))
    await waitFor(() => expect(blogThemesApi.publish).toHaveBeenCalledWith('ws1', { version: 4 }))
  })

  it('publishes an unchanged draft as it is, and offers nothing to publish on an unchanged published theme', async () => {
    const user = userEvent.setup()
    renderEditor(theme(false))
    await user.click(within(bar()).getByRole('button', { name: 'Publish' }))
    await user.click(within(dialog()).getByRole('button', { name: 'Publish' }))
    await waitFor(() => expect(blogThemesApi.publish).toHaveBeenCalledWith('ws1', { version: 3 }))
    expect(blogThemesApi.create).not.toHaveBeenCalled()
    expect(blogThemesApi.update).not.toHaveBeenCalled()
  })

  it('cannot publish a published theme it has not changed', () => {
    renderEditor(theme(true))
    expect(within(bar()).getByRole('button', { name: 'Publish' })).toBeDisabled()
    expect(within(bar()).getByRole('button', { name: 'Save as a new version' })).toBeDisabled()
  })

  it('closes on the saved version when publishing it fails, so a retry cannot save it twice', async () => {
    vi.mocked(blogThemesApi.create).mockResolvedValue({ theme: { ...theme(false), version: 4 } } as never)
    vi.mocked(blogThemesApi.publish).mockRejectedValueOnce(new Error('boom'))
    const user = userEvent.setup()
    const { onClose } = renderEditor(theme(true))
    edit()
    await user.click(within(bar()).getByRole('button', { name: 'Save and publish' }))
    await user.click(within(dialog()).getByRole('button', { name: 'Publish' }))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(blogThemesApi.create).toHaveBeenCalledTimes(1)
    expect(await screen.findByText('Saved as v4, but it could not be published. Publish it from the Themes list.')).toBeInTheDocument()
  })
})
