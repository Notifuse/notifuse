import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { App } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { THEME_PRESETS } from './themePresets'

vi.mock('@monaco-editor/react', () => ({ default: () => <div data-testid="code-editor" /> }))
vi.mock('./ThemePreview', () => ({ ThemePreview: () => <div data-testid="theme-preview" /> }))
// services/api/blog imports the API client, and with it the router.
vi.mock('../../services/api/blog', () => ({ blogThemesApi: { create: vi.fn(), update: vi.fn() } }))

import { ThemeEditorDrawer } from './ThemeEditorDrawer'

const theme = {
  version: 1,
  published_at: null,
  files: THEME_PRESETS[0].files,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z'
}

describe('ThemeEditorDrawer', () => {
  it('fills the drawer with a flex column down to the editor and the preview, with no height of its own', () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <App>
          <ThemeEditorDrawer open onClose={() => {}} theme={theme} workspaceId="ws1" />
        </App>
      </QueryClientProvider>
    )
    expect(screen.getByTestId('code-editor')).toBeInTheDocument()
    const body = document.querySelector<HTMLElement>('.ant-drawer-body')!
    expect(body.style.display).toBe('flex')
    expect(body.style.flexDirection).toBe('column')
    // Heights guessed from the viewport left a band under the editor once the header grew
    const guessed = Array.from(document.querySelectorAll<HTMLElement>('[style]')).filter((element) =>
      element.style.height.includes('100vh')
    )
    expect(guessed).toEqual([])
    expect(document.querySelector<HTMLElement>('[data-theme-editor-pane]')!.style.height).toBe('100%')
    expect(document.querySelector<HTMLElement>('[data-theme-preview-pane]')!.style.height).toBe('100%')
    // The tab panels themselves take the room their tab bar leaves
    const panels = document.querySelectorAll<HTMLElement>('.ant-tabs-body')
    expect(panels).toHaveLength(2)
    for (const panel of panels) {
      expect(panel.style.height).toBe('100%')
    }
  })
})
