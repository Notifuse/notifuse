import React from 'react'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'
import TemplateSelectorInput from './TemplateSelectorInput'
import { templatesApi } from '../../services/api/template'
import type { Template } from '../../services/api/types'

// Mock the template API — list (for the drawer) and get (to resolve the current value)
vi.mock('../../services/api/template', () => ({
  templatesApi: {
    list: vi.fn(),
    get: vi.fn()
  }
}))

// Provide the current workspace so the component renders past its loading guard
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [{ id: 'ws1', name: 'WS', integrations: [] }]
  })
}))

// Heavy child drawers/popovers are not under test — stub them out
vi.mock('./TemplatePreviewDrawer', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>
}))
vi.mock('./CreateTemplateDrawer', () => ({
  CreateTemplateDrawer: () => null
}))

const makeTemplate = (id: string, name: string): Template =>
  ({ id, name, category: 'marketing', channel: 'email' }) as unknown as Template

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

const Wrapper = ({ value }: { value: string | null }) => (
  <QueryClientProvider client={queryClient}>
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <TemplateSelectorInput value={value} onChange={vi.fn()} workspaceId="ws1" />
        </App>
      </ConfigProvider>
    </I18nProvider>
  </QueryClientProvider>
)

describe('TemplateSelectorInput', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(templatesApi.list as Mock).mockResolvedValue({ templates: [] })
    ;(templatesApi.get as Mock).mockImplementation(
      ({ id }: { id: string }) =>
        Promise.resolve({
          template: makeTemplate(id, id === 'tpl-a' ? 'Template A' : 'Template B')
        })
    )
  })

  // Regression for issue #353: the same instance is reused while `value` changes
  // (switching between email nodes in the automation editor). The displayed
  // template must follow `value`, not stay on the first one resolved.
  it('updates the displayed template when the value prop changes', async () => {
    const { rerender } = render(<Wrapper value="tpl-a" />)
    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()

    rerender(<Wrapper value="tpl-b" />)
    expect(await screen.findByDisplayValue('Template B')).toBeInTheDocument()
  })

  it('clears the displayed template when the value is cleared', async () => {
    const { rerender } = render(<Wrapper value="tpl-a" />)
    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()

    rerender(<Wrapper value={null} />)
    await waitFor(() => {
      expect(screen.queryByDisplayValue('Template A')).not.toBeInTheDocument()
    })
  })
})

// Without Templates read, templates.list and templates.get answer 403: the field shows the stored
// template ID instead, requests nothing, and never changes the value it was given.
describe('TemplateSelectorInput without Templates read', () => {
  const renderSelector = (value: string | null, onChange = vi.fn()) => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const tree = (canReadTemplates: boolean) => (
      <QueryClientProvider client={client}>
        <I18nProvider i18n={i18n}>
          <ConfigProvider>
            <App>
              <TemplateSelectorInput
                value={value}
                onChange={onChange}
                workspaceId="ws1"
                canReadTemplates={canReadTemplates}
              />
            </App>
          </ConfigProvider>
        </I18nProvider>
      </QueryClientProvider>
    )
    const { container, rerender } = render(tree(false))
    return { onChange, container, grant: () => rerender(tree(true)) }
  }

  beforeEach(() => {
    vi.clearAllMocks()
    ;(templatesApi.list as Mock).mockResolvedValue({ templates: [] })
    ;(templatesApi.get as Mock).mockResolvedValue({ template: makeTemplate('tpl-a', 'Template A') })
  })

  it('shows the template ID, disabled, without requesting any template', async () => {
    const { onChange, container } = renderSelector('supabase_magiclink_000001')

    const input = screen.getByDisplayValue('supabase_magiclink_000001')
    expect(input).toBeDisabled()
    // No clear button: clearing is the one way the field could change the stored value.
    expect(container.querySelector('.ant-input-clear-icon')).toBeNull()
    // Give a query or the effect the chance to fire before asserting they did not.
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(templatesApi.list).not.toHaveBeenCalled()
    expect(templatesApi.get).not.toHaveBeenCalled()
    expect(onChange).not.toHaveBeenCalled()
  })

  // The lock says why the field is locked, wherever the selector is used: most callers have no
  // room or no extra of their own for it.
  it('explains with a lock why the stored template is not shown', async () => {
    renderSelector('supabase_magiclink_000001')

    const lock = screen.getByLabelText(
      "You need Templates read permission to see this template's name and preview"
    )
    await userEvent.hover(lock)
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      "You need Templates read permission to see this template's name and preview"
    )
  })

  it('shows the lock hint to keyboard users too', async () => {
    renderSelector('supabase_magiclink_000001')

    await userEvent.tab()
    expect(
      screen.getByLabelText("You need Templates read permission to see this template's name and preview")
    ).toHaveFocus()
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      "You need Templates read permission to see this template's name and preview"
    )
  })

  it('explains with a lock why no template can be chosen', async () => {
    renderSelector(null)

    expect(screen.getByPlaceholderText('Select a template')).toBeDisabled()
    const lock = screen.getByLabelText('You need Templates read permission to choose a template')
    await userEvent.hover(lock)
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      'You need Templates read permission to choose a template'
    )
  })

  it('shows no lock once the permission is granted', async () => {
    const { grant } = renderSelector('tpl-a')
    grant()

    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()
    expect(screen.queryByLabelText(/You need Templates read permission/)).toBeNull()
  })

  it('resolves the template once the permission is granted', async () => {
    const { grant } = renderSelector('tpl-a')
    grant()

    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()
    expect(templatesApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'tpl-a' })
  })

  it('requests the templates when the prop is left out', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <I18nProvider i18n={i18n}>
          <ConfigProvider>
            <App>
              <TemplateSelectorInput value="tpl-a" onChange={vi.fn()} workspaceId="ws1" />
            </App>
          </ConfigProvider>
        </I18nProvider>
      </QueryClientProvider>
    )

    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()
    expect(templatesApi.list).toHaveBeenCalled()
    expect(templatesApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'tpl-a' })
  })
})
