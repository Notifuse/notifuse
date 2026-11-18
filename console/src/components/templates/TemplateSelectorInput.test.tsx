import '../../__tests__/resizeObserverMock'
import React from 'react'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'
import TemplateSelectorInput from './TemplateSelectorInput'
import { templatesApi } from '../../services/api/template'
import type { Template } from '../../services/api/types'

// The API client imports the router, and the router every page.
vi.mock('../../router', () => ({ router: { navigate: vi.fn() } }))
// The template API: list and usage for the window, get to resolve the current value, compile for
// the preview.
vi.mock('../../services/api/template', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/template')>()),
  templatesApi: { list: vi.fn(), get: vi.fn(), usage: vi.fn(), compile: vi.fn() }
}))
vi.mock('../broadcasts/detail/TemplatePreviewFrame', () => ({
  TemplatePreviewFrame: ({ template }: { template: Template }) => <div data-testid="frame" data-template={template.id} />
}))

// Provide the current workspace so the component renders past its loading guard
const access = vi.hoisted(() => ({ templatesWrite: true }))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [{ id: 'ws1', name: 'WS', settings: { default_language: 'en' }, integrations: [] }]
  }),
  useWorkspacePermissions: () => ({
    permissions: { templates: { read: true, write: access.templatesWrite } },
    loading: false
  })
}))

// The full preview drawer: its children open it; controlled, it says what it shows.
vi.mock('./TemplatePreviewDrawer', () => ({
  default: ({ children, open, record }: { children?: React.ReactNode; open?: boolean; record: Template }) =>
    open ? <div data-testid="full-preview">{record.name}</div> : <>{children}</>
}))
// The editor: records what it was opened with, so a test can close it as a save or a cancel would.
interface DrawerProps {
  open?: boolean
  canManageSavedBlocks?: boolean
  fromTemplate?: Template
  forceCategory?: string
  startModelId?: string
  successMessage?: string
  onClose?: (saved?: Template) => void
}
const editor = vi.hoisted(() => ({ props: undefined as undefined | DrawerProps }))
vi.mock('./CreateTemplateDrawer', () => ({
  CreateTemplateDrawer: (props: DrawerProps) => {
    editor.props = props
    return props.open ? <div data-testid="editor" /> : null
  }
}))

const makeTemplate = (id: string, name: string, category = 'marketing'): Template =>
  ({ id, name, category, channel: 'email', version: 1, updated_at: '2026-11-01T00:00:00Z', email: { subject: `${name} subject` } }) as unknown as Template

const renderSelector = (props: Partial<React.ComponentProps<typeof TemplateSelectorInput>> = {}) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onChange = props.onChange ?? vi.fn()
  const tree = (extra: Partial<React.ComponentProps<typeof TemplateSelectorInput>>) => (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <TemplateSelectorInput workspaceId="ws1" onChange={onChange} {...props} {...extra} />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
  const { container, rerender } = render(tree({}))
  return { onChange, container, rerender: (extra: Partial<React.ComponentProps<typeof TemplateSelectorInput>>) => rerender(tree(extra)) }
}

const openWindow = async () => {
  await userEvent.click(screen.getByRole('textbox'))
  return screen.findByRole('dialog', { name: /Choose a template/ })
}
const windowShown = () => screen.queryByRole('dialog', { name: /Choose a template/ })

beforeEach(() => {
  vi.clearAllMocks()
  access.templatesWrite = true
  editor.props = undefined
  ;(templatesApi.list as Mock).mockResolvedValue({
    templates: [makeTemplate('tpl-a', 'Template A'), makeTemplate('tpl-b', 'Template B')]
  })
  ;(templatesApi.usage as Mock).mockResolvedValue({ usage: {}, hidden_types: [] })
  ;(templatesApi.compile as Mock).mockResolvedValue({ html: '', mjml: '' })
  ;(templatesApi.get as Mock).mockImplementation(({ id }: { id: string }) =>
    Promise.resolve({ template: makeTemplate(id, id === 'tpl-a' ? 'Template A' : 'Template B') })
  )
})

describe('TemplateSelectorInput', () => {
  // Regression for issue #353: the same instance is reused while `value` changes
  // (switching between email nodes in the automation editor). The displayed
  // template must follow `value`, not stay on the first one resolved.
  it('updates the displayed template when the value prop changes', async () => {
    const { rerender } = renderSelector({ value: 'tpl-a' })
    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()

    rerender({ value: 'tpl-b' })
    expect(await screen.findByDisplayValue('Template B')).toBeInTheDocument()
  })

  it('clears the displayed template when the value is cleared', async () => {
    const { rerender } = renderSelector({ value: 'tpl-a' })
    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()

    rerender({ value: null })
    await waitFor(() => {
      expect(screen.queryByDisplayValue('Template A')).not.toBeInTheDocument()
    })
  })

  it('lists the templates only once the window opens', async () => {
    renderSelector({ value: 'tpl-a' })
    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()
    expect(templatesApi.list).not.toHaveBeenCalled()
    const dialog = await openWindow()
    expect(await within(dialog).findByRole('button', { name: /^Template B/ })).toBeInTheDocument()
    expect(templatesApi.list).toHaveBeenCalledWith({ workspace_id: 'ws1', category: undefined, channel: 'email' })
  })

  it('opens the window from the keyboard too', async () => {
    renderSelector({ value: null })
    screen.getByRole('textbox').focus()
    await userEvent.keyboard('{Enter}')
    expect(await screen.findByRole('dialog', { name: /Choose a template/ })).toBeInTheDocument()
  })

  it('shows the subtitle the caller gives in the window', async () => {
    renderSelector({ value: null, subtitle: 'The email this step sends' })
    const dialog = await openWindow()
    expect(within(dialog).getByText('The email this step sends')).toBeInTheDocument()
  })

  it('chooses a template on a double click, and closes the window', async () => {
    const { onChange } = renderSelector({ value: null })
    const dialog = await openWindow()
    await userEvent.dblClick(await within(dialog).findByRole('button', { name: /^Template B/ }))
    expect(onChange).toHaveBeenCalledWith('tpl-b')
    await waitFor(() => expect(windowShown()).toBeNull())
  })

  it('chooses the previewed template with Use this template', async () => {
    const { onChange } = renderSelector({ value: null })
    const dialog = await openWindow()
    await userEvent.click(await within(dialog).findByRole('button', { name: /^Template A/ }))
    await userEvent.click(within(dialog).getByRole('button', { name: 'Use this template' }))
    expect(onChange).toHaveBeenCalledWith('tpl-a')
  })

  it('keeps the value when the window is cancelled', async () => {
    const { onChange } = renderSelector({ value: 'tpl-a' })
    const dialog = await openWindow()
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(windowShown()).toBeNull())
    expect(onChange).not.toHaveBeenCalled()
  })

  it('opens the full preview of a row from its menu, over the window', async () => {
    renderSelector({ value: null })
    const dialog = await openWindow()
    await userEvent.click(await within(dialog).findByRole('button', { name: 'More actions for Template B' }))
    await userEvent.click(await screen.findByRole('menuitem', { name: /Full preview/ }))
    expect(screen.getByTestId('full-preview')).toHaveTextContent('Template B')
    expect(windowShown()).not.toBeNull()
  })

  it('does not open the window while disabled', async () => {
    renderSelector({ value: null, disabled: true })
    await userEvent.click(screen.getByRole('textbox'))
    expect(windowShown()).toBeNull()
  })
})

// A segment condition or a trigger: an empty value means every template.
describe('TemplateSelectorInput with Any template', () => {
  it('empties the value when Any template is chosen, as the clear cross does', async () => {
    const { onChange } = renderSelector({ value: 'tpl-a', anyLabel: 'Any template', placeholder: 'Any template' })
    const dialog = await openWindow()
    await userEvent.dblClick(await within(dialog).findByRole('button', { name: /^Any template/ }))
    expect(onChange).toHaveBeenCalledWith(null)
    await waitFor(() => expect(windowShown()).toBeNull())
  })

  it('has no Any template row when the field cannot be emptied', async () => {
    renderSelector({ value: 'tpl-a', anyLabel: 'Any template', clearable: false })
    const dialog = await openWindow()
    await within(dialog).findByRole('button', { name: /^Template A/ })
    expect(within(dialog).queryByRole('button', { name: /^Any template/ })).toBeNull()
  })
})

// "New template" and "Duplicate and edit" go through the editor, the window closed meanwhile:
// a save comes back with the new template chosen, a cancel back to the window with nothing chosen.
describe('TemplateSelectorInput creating a template', () => {
  it('chooses the template created, closes everything, and has the editor say so', async () => {
    ;(templatesApi.list as Mock).mockResolvedValue({ templates: [makeTemplate('confirm', 'Confirm', 'opt_in')] })
    const { onChange } = renderSelector({ value: null, category: 'opt_in' })
    const dialog = await openWindow()
    await within(dialog).findByRole('button', { name: /^Confirm/ })
    await userEvent.click(within(dialog).getByRole('button', { name: 'New template' }))

    expect(screen.getByTestId('editor')).toBeInTheDocument()
    await waitFor(() => expect(windowShown()).toBeNull())
    expect(editor.props).toMatchObject({ open: true, forceCategory: 'opt_in', successMessage: 'Template created and selected', canManageSavedBlocks: true })
    expect(editor.props?.fromTemplate).toBeUndefined()

    act(() => editor.props?.onClose?.(makeTemplate('confirm-new', 'Confirm new', 'opt_in')))
    expect(onChange).toHaveBeenCalledWith('confirm-new')
    expect(screen.queryByTestId('editor')).toBeNull()
    expect(windowShown()).toBeNull()
  })

  it('shows the created template in the field', async () => {
    const Controlled = () => {
      const [value, setValue] = React.useState<string | null>(null)
      return <TemplateSelectorInput workspaceId="ws1" value={value} onChange={setValue} />
    }
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <App>
          <Controlled />
        </App>
      </QueryClientProvider>
    )
    const dialog = await openWindow()
    await within(dialog).findByRole('button', { name: /^Template A/ })
    await userEvent.click(within(dialog).getByRole('button', { name: 'New template' }))
    act(() => editor.props?.onClose?.(makeTemplate('fresh', 'Fresh template')))
    expect(await screen.findByDisplayValue('Fresh template')).toBeInTheDocument()
    // Already known: not read again.
    expect(templatesApi.get).not.toHaveBeenCalled()
  })

  it('goes back to the window, nothing chosen, when the editor is cancelled', async () => {
    const { onChange } = renderSelector({ value: 'tpl-a' })
    const dialog = await openWindow()
    await within(dialog).findByRole('button', { name: /^Template A/ })
    await userEvent.click(within(dialog).getByRole('button', { name: 'New template' }))

    act(() => editor.props?.onClose?.())
    expect(await screen.findByRole('dialog', { name: /Choose a template/ })).toBeInTheDocument()
    expect(screen.queryByTestId('editor')).toBeNull()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('duplicates a row into the editor, and chooses the copy once saved', async () => {
    const { onChange } = renderSelector({ value: null })
    const dialog = await openWindow()
    await userEvent.click(await within(dialog).findByRole('button', { name: 'More actions for Template B' }))
    await userEvent.click(await screen.findByRole('menuitem', { name: /Duplicate and edit/ }))
    expect(editor.props?.fromTemplate).toMatchObject({ id: 'tpl-b' })

    act(() => editor.props?.onClose?.(makeTemplate('tpl-b-copy', 'Template B copy')))
    expect(onChange).toHaveBeenCalledWith('tpl-b-copy')
  })

  it('starts the editor on a gallery model when the imposed category has no template', async () => {
    ;(templatesApi.list as Mock).mockResolvedValue({ templates: [] })
    renderSelector({ value: null, category: 'opt_in' })
    const dialog = await openWindow()
    await userEvent.click(await within(dialog).findByRole('button', { name: /Subscription confirmation/ }))
    expect(editor.props).toMatchObject({ open: true, startModelId: 'confirm', forceCategory: 'opt_in' })
  })

  it('cannot create nor duplicate without Templates write', async () => {
    access.templatesWrite = false
    renderSelector({ value: null })
    const dialog = await openWindow()
    await within(dialog).findByRole('button', { name: /^Template A/ })
    expect(within(dialog).getByRole('button', { name: 'New template' })).toBeDisabled()
    await userEvent.click(within(dialog).getByRole('button', { name: 'More actions for Template A' }))
    expect(await screen.findByRole('menuitem', { name: /Duplicate and edit/ })).toHaveAttribute('aria-disabled', 'true')
    expect(editor.props).toBeUndefined()
  })
})

// Without Templates read, templates.list and templates.get answer 403: the field shows the stored
// template ID instead, requests nothing, and never changes the value it was given.
describe('TemplateSelectorInput without Templates read', () => {
  it('shows the template ID, disabled, without requesting any template', async () => {
    const { onChange, container } = renderSelector({ value: 'supabase_magiclink_000001', canReadTemplates: false })

    const input = screen.getByDisplayValue('supabase_magiclink_000001')
    expect(input).toBeDisabled()
    // No clear button: clearing is the one way the field could change the stored value.
    expect(container.querySelector('.ant-input-clear-icon')).toBeNull()
    await userEvent.click(input)
    // Give a query or the effect the chance to fire before asserting they did not.
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(windowShown()).toBeNull()
    expect(templatesApi.list).not.toHaveBeenCalled()
    expect(templatesApi.get).not.toHaveBeenCalled()
    expect(onChange).not.toHaveBeenCalled()
  })

  // The lock says why the field is locked, wherever the selector is used: most callers have no
  // room or no extra of their own for it.
  it('explains with a lock why the stored template is not shown', async () => {
    renderSelector({ value: 'supabase_magiclink_000001', canReadTemplates: false })

    const lock = screen.getByLabelText("You need Templates read permission to see this template's name and preview")
    await userEvent.hover(lock)
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      "You need Templates read permission to see this template's name and preview"
    )
  })

  it('shows the lock hint to keyboard users too', async () => {
    renderSelector({ value: 'supabase_magiclink_000001', canReadTemplates: false })

    await userEvent.tab()
    expect(screen.getByLabelText("You need Templates read permission to see this template's name and preview")).toHaveFocus()
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      "You need Templates read permission to see this template's name and preview"
    )
  })

  it('explains with a lock why no template can be chosen', async () => {
    renderSelector({ value: null, canReadTemplates: false })

    expect(screen.getByPlaceholderText('Select a template')).toBeDisabled()
    const lock = screen.getByLabelText('You need Templates read permission to choose a template')
    await userEvent.hover(lock)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('You need Templates read permission to choose a template')
  })

  it('shows no lock once the permission is granted, and resolves the template', async () => {
    const { rerender } = renderSelector({ value: 'tpl-a', canReadTemplates: false })
    rerender({ canReadTemplates: true })

    expect(await screen.findByDisplayValue('Template A')).toBeInTheDocument()
    expect(screen.queryByLabelText(/You need Templates read permission/)).toBeNull()
    expect(templatesApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'tpl-a' })
  })
})

// The list drawer shows the double opt-in template as a card: the same window, editor and preview
// behind it.
describe('TemplateSelectorInput as a card', () => {
  const card = () => document.querySelector('[data-template-card]') as HTMLElement

  it('shows the name, the category and sender line, and a thumbnail', async () => {
    renderSelector({ value: 'tpl-a', variant: 'card', category: 'opt_in', sendsWith: 'marketing' })
    expect(await within(card()).findByText('Template A')).toBeInTheDocument()
    expect(within(card()).getByText('Opt-in category · sent by the marketing provider')).toBeInTheDocument()
    expect(within(card()).getByTestId('frame')).toHaveAttribute('data-template', 'tpl-a')
    expect(within(card()).getByRole('button', { name: 'Preview' })).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).toBeNull()
  })

  it('opens the window on its category with Change', async () => {
    renderSelector({ value: 'tpl-a', variant: 'card', category: 'opt_in' })
    await within(card()).findByText('Template A')
    await userEvent.click(within(card()).getByRole('button', { name: 'Change' }))
    expect(await screen.findByRole('dialog', { name: /Choose a template/ })).toBeInTheDocument()
    await waitFor(() =>
      expect(templatesApi.list).toHaveBeenCalledWith({ workspace_id: 'ws1', category: 'opt_in', channel: 'email' })
    )
  })

  it('offers Choose on an empty card, which opens the window and picks', async () => {
    const { onChange } = renderSelector({ value: null, variant: 'card', placeholder: 'Choose a confirmation template' })
    expect(within(card()).getByText('Choose a confirmation template')).toBeInTheDocument()
    expect(within(card()).queryByRole('button', { name: 'Preview' })).toBeNull()
    await userEvent.click(within(card()).getByRole('button', { name: 'Choose' }))
    const dialog = await screen.findByRole('dialog', { name: /Choose a template/ })
    await userEvent.dblClick(await within(dialog).findByRole('button', { name: /^Template B/ }))
    expect(onChange).toHaveBeenCalledWith('tpl-b')
  })

  it('shows the stored ID, locked, without Templates read, and requests nothing', async () => {
    renderSelector({ value: 'tpl-a', variant: 'card', canReadTemplates: false })
    expect(within(card()).getByText('tpl-a')).toBeInTheDocument()
    expect(within(card()).getByText('You need Templates read permission to see its name and preview')).toBeInTheDocument()
    expect(within(card()).queryByRole('button')).toBeNull()
    await act(async () => {})
    expect(templatesApi.get).not.toHaveBeenCalled()
  })

  it('says what reading is needed to choose, without Templates read and no value', () => {
    renderSelector({ value: null, variant: 'card', canReadTemplates: false, placeholder: 'Choose a confirmation template' })
    expect(within(card()).getByText('You need Templates read permission to choose a template')).toBeInTheDocument()
    expect(within(card()).queryByRole('button')).toBeNull()
  })

  it('shows a skeleton while the template resolves, with Change already there', async () => {
    ;(templatesApi.get as Mock).mockReturnValue(new Promise(() => {}))
    renderSelector({ value: 'tpl-a', variant: 'card' })
    expect(card().querySelector('.ant-skeleton')).not.toBeNull()
    expect(within(card()).getByRole('button', { name: 'Change' })).toBeInTheDocument()
    expect(within(card()).queryByRole('button', { name: 'Preview' })).toBeNull()
  })

  it('says when the template cannot be found, and still offers Change', async () => {
    ;(templatesApi.get as Mock).mockRejectedValue(new Error('Template not found'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    renderSelector({ value: 'tpl-gone', variant: 'card' })
    expect(await within(card()).findByText('This template cannot be found')).toBeInTheDocument()
    expect(within(card()).getByText('tpl-gone')).toBeInTheDocument()
    expect(within(card()).getByRole('button', { name: 'Change' })).toBeInTheDocument()
    expect(within(card()).queryByRole('button', { name: 'Preview' })).toBeNull()
  })
})
