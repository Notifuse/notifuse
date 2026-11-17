import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TreeNodeInput } from './input'
import { TableSchemas } from './table_schemas'
import { templatesApi } from '../../services/api/template'
import type { TreeNode } from '../../services/api/segment'

// antd's Select mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

vi.mock('../../services/api/template', () => ({
  // The window counts the uses of each type the API names.
  TEMPLATE_USAGE_TYPES: ['broadcasts', 'transactional_notifications', 'automations', 'lists'],
  templatesApi: { list: vi.fn(), get: vi.fn(), usage: vi.fn(), compile: vi.fn() }
}))
vi.mock('../../contexts/AuthContext', () => ({
  // TemplateSelectorInput reads the member's permissions for its drawers
  useWorkspacePermissions: () => ({ permissions: null }),
  // Providers with default senders: a filter's window still names none, it sends nothing.
  useAuth: () => ({
    workspaces: [
      {
        id: 'ws1',
        name: 'Workspace',
        settings: { marketing_email_provider_id: 'mk', transactional_email_provider_id: 'tx' },
        integrations: [
          { id: 'tx', name: 'Postmark', email_provider: { senders: [{ id: 'tx1', name: 'Support', email: 'support@acme.com', is_default: true }] } },
          { id: 'mk', name: 'SES', email_provider: { senders: [{ id: 'mk1', name: 'News', email: 'news@acme.com', is_default: true }] } }
        ]
      }
    ]
  })
}))
vi.mock('../templates/TemplatePreviewDrawer', () => ({ default: () => null }))
vi.mock('../templates/CreateTemplateDrawer', () => ({ CreateTemplateDrawer: () => null }))

const emailTree: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      {
        kind: 'leaf',
        leaf: {
          source: 'contact_timeline',
          contact_timeline: {
            kind: 'email.opened',
            template_id: 'tpl-welcome',
            count_operator: 'at_least',
            count_value: 1,
            timeframe_operator: 'anytime',
            timeframe_values: []
          }
        }
      }
    ]
  }
}

const renderTree = (canReadTemplates?: boolean) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <TreeNodeInput
          value={emailTree}
          onChange={vi.fn()}
          schemas={TableSchemas}
          workspaceId="ws1"
          canReadTemplates={canReadTemplates}
        />
      </I18nProvider>
    </QueryClientProvider>
  )
}

const editFirstCondition = () =>
  fireEvent.click(document.querySelectorAll('[data-icon="pen-to-square"]')[0])

// Without Templates read, templates.list and templates.get answer 403: an email condition names
// its template by ID, and editing it shows that ID locked, without requesting any template.
describe('segment email condition without Templates read', () => {
  beforeEach(() => {
    vi.mocked(templatesApi.list)
      .mockReset()
      .mockResolvedValue({ templates: [{ id: 'tpl-welcome', name: 'Welcome' }] } as never)
    vi.mocked(templatesApi.get)
      .mockReset()
      .mockResolvedValue({ template: { id: 'tpl-welcome', name: 'Welcome' } } as never)
  })

  it('names the template by its ID and requests no template, even while editing', async () => {
    renderTree(false)

    expect(screen.getByText('tpl-welcome')).toBeInTheDocument()
    editFirstCondition()
    const field = await screen.findByDisplayValue('tpl-welcome')
    expect(field).toBeDisabled()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(templatesApi.list).not.toHaveBeenCalled()
    expect(templatesApi.get).not.toHaveBeenCalled()
  })

  it('names the template when the prop is left out', async () => {
    renderTree()

    expect(await screen.findByText('Welcome')).toBeInTheDocument()
    editFirstCondition()
    expect(await screen.findByDisplayValue('Welcome')).not.toBeDisabled()
    expect(templatesApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'tpl-welcome' })
  })
})

// A template filter left empty counts the emails of every template: the window offers that as a
// first row, which empties the filter.
describe('segment email condition template window', () => {
  beforeEach(() => {
    vi.mocked(templatesApi.list)
      .mockReset()
      .mockResolvedValue({ templates: [{ id: 'tpl-welcome', name: 'Welcome', category: 'welcome', email: { subject: 'Hi' } }] } as never)
    vi.mocked(templatesApi.get)
      .mockReset()
      .mockResolvedValue({ template: { id: 'tpl-welcome', name: 'Welcome' } } as never)
    vi.mocked(templatesApi.usage).mockReset().mockResolvedValue({ usage: {}, hidden_types: [] } as never)
    vi.mocked(templatesApi.compile).mockReset().mockResolvedValue({ html: '', mjml: '' } as never)
  })

  it('offers Any template first, and says what the template filters', async () => {
    renderTree()
    editFirstCondition()
    fireEvent.click(await screen.findByDisplayValue('Welcome'))
    // Found by its title: the dialogs around it share the test id antd names them by.
    const dialog = await waitFor(() => {
      const found = screen.getAllByRole('dialog').find((element) => element.textContent?.startsWith('Choose a template'))
      expect(found).toBeDefined()
      return found as HTMLElement
    })
    expect(within(dialog).getByText('The template of the emails this condition counts')).toBeInTheDocument()
    const rows = await within(dialog).findAllByRole('button', { pressed: false })
    expect(rows[0]).toHaveTextContent(/^Any template/)
    expect(within(dialog).getByRole('button', { name: /^Welcome/, pressed: true })).toHaveTextContent('Current')
  })

  it('names no sender: the condition matches emails, it sends none', async () => {
    renderTree()
    editFirstCondition()
    fireEvent.click(await screen.findByDisplayValue('Welcome'))
    const preview = await screen.findByRole('region', { name: 'Preview' })
    expect(within(preview).getByRole('heading', { name: 'Welcome' })).toBeInTheDocument()
    expect(within(preview).queryByText('Sender')).toBeNull()
    expect(within(preview).queryByText(/support@acme\.com|news@acme\.com/)).toBeNull()
  })
})
