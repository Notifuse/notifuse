import '../../__tests__/resizeObserverMock'
import { describe, expect, it, vi, beforeEach, type Mock } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { Template } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import { templatesApi } from '../../services/api/template'
import { TemplateSelectorModal, type TemplateSelectorModalProps } from './TemplateSelectorModal'

// The API client imports the router, and the router every page.
vi.mock('../../router', () => ({ router: { navigate: vi.fn() } }))
// The thumbnails compile through the server: the frames only say which template they show.
vi.mock('../broadcasts/detail/TemplatePreviewFrame', () => ({
  TemplatePreviewFrame: ({ template }: { template: Template }) => <div data-testid="frame" data-template={template.id} />
}))
vi.mock('../../services/api/template', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/template')>()),
  templatesApi: { list: vi.fn(), usage: vi.fn(), compile: vi.fn() }
}))

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: { default_language: 'en', languages: ['en', 'fr'], transactional_email_provider_id: 'p1' },
  integrations: [
    { id: 'p1', email_provider: { senders: [{ id: 's1', name: 'Acme Support', email: 'support@acme.com', is_default: true }] } }
  ]
} as unknown as Workspace

const tree = { id: 'root', type: 'mjml', children: [] }
const reset = {
  id: 'pwd',
  name: 'Password Reset',
  version: 3,
  category: 'transactional',
  updated_at: '2026-11-13T00:00:00Z',
  email: { subject: 'Reset your password, {{contact.first_name}}', subject_preview: 'This link expires in one hour', visual_editor_tree: tree },
  translations: { fr: { email: { subject: 'Réinitialisez votre mot de passe', visual_editor_tree: tree } } },
  test_data: { contact: { first_name: 'Jane' }, reset_url: 'https://acme.com/reset' }
} as unknown as Template
const cart = {
  id: 'cart-b',
  name: 'Cart Recovery B',
  version: 1,
  category: 'marketing',
  updated_at: '2026-11-09T00:00:00Z',
  email: { subject: "Here's 10% off your cart", visual_editor_tree: tree }
} as unknown as Template
const welcome = {
  id: 'welcome',
  name: 'Welcome Email',
  version: 1,
  category: 'welcome',
  updated_at: '2026-11-01T00:00:00Z',
  email: { subject: 'Welcome to our community', visual_editor_tree: tree }
} as unknown as Template

function renderModal(props: Partial<TemplateSelectorModalProps> = {}) {
  const handlers = {
    onCancel: vi.fn(),
    onPick: vi.fn(),
    onNew: vi.fn(),
    onDuplicate: vi.fn(),
    onStartModel: vi.fn(),
    onFullPreview: vi.fn()
  }
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AntApp>
        <TemplateSelectorModal workspace={workspace} canWrite {...handlers} {...props} />
      </AntApp>
    </QueryClientProvider>
  )
  return handlers
}

const list = () => screen.findByRole('list', { name: 'Templates' })
const row = (name: RegExp) => within(screen.getByRole('list', { name: 'Templates' })).getByRole('button', { name })
const rowNames = () =>
  within(screen.getByRole('list', { name: 'Templates' }))
    .getAllByRole('listitem')
    .map((item) => item.querySelector('.font-semibold')?.textContent)
const pane = () => screen.getByRole('region', { name: 'Preview' })

describe('TemplateSelectorModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(templatesApi.list as Mock).mockResolvedValue({ templates: [welcome, cart, reset] })
    ;(templatesApi.usage as Mock).mockImplementation(({ template_id }: { template_id?: string }) =>
      Promise.resolve(
        template_id
          ? {
              usage: { pwd: { transactional_notifications: 1 } },
              references: [{ type: 'transactional_notification', id: 'n1', name: 'Password Reset Email' }],
              hidden_types: []
            }
          : { usage: { pwd: { transactional_notifications: 1 }, 'cart-b': { automations: 1 } }, hidden_types: [] }
      )
    )
    ;(templatesApi.compile as Mock).mockImplementation((request: { subject: string }) =>
      Promise.resolve({ html: '<p>x</p>', mjml: '', subject: `Rendered: ${request.subject}` })
    )
  })

  it('lists the templates last updated first, with their category, languages and uses', async () => {
    renderModal()
    await list()
    expect(rowNames()).toEqual(['Password Reset', 'Cart Recovery B', 'Welcome Email'])
    const resetRow = row(/^Password Reset/)
    expect(within(resetRow).getByText('Transactional')).toBeInTheDocument()
    expect(within(resetRow).getByText('EN · FR')).toBeInTheDocument()
    expect(await within(resetRow).findByText('1 notification')).toBeInTheDocument()
    expect(within(row(/^Cart Recovery B/)).getByText('1 automation')).toBeInTheDocument()
    // « Not in use »: a sent broadcast is not counted, and the tooltip says so.
    expect(within(row(/^Welcome Email/)).getByText('Not in use')).toBeInTheDocument()
    expect(within(row(/^Welcome Email/)).queryByText('Not used')).toBeNull()
    expect(screen.getByText('3 templates')).toBeInTheDocument()
    // Every template's counts in one call, without a template_id.
    expect(templatesApi.usage).toHaveBeenCalledWith({ workspace_id: 'ws1' })
    expect(templatesApi.list).toHaveBeenCalledWith({ workspace_id: 'ws1', category: undefined, channel: 'email' })
  })

  it('marks the current template and opens on its preview', async () => {
    renderModal({ value: 'cart-b' })
    await list()
    expect(within(row(/^Cart Recovery B/)).getByText('Current')).toBeInTheDocument()
    expect(row(/^Cart Recovery B/)).toHaveAttribute('aria-pressed', 'true')
    expect(within(pane()).getByRole('heading', { name: 'Cart Recovery B' })).toBeInTheDocument()
  })

  it('searches the subjects as well as the names', async () => {
    renderModal()
    await list()
    await userEvent.type(screen.getByRole('textbox', { name: 'Search by name or subject' }), 'community')
    expect(rowNames()).toEqual(['Welcome Email'])
    expect(screen.getByText('1 of 3 templates')).toBeInTheDocument()
    expect(screen.getByText('Name or subject contains “community”')).toBeInTheDocument()
  })

  it('says when nothing matches the search, and clears it', async () => {
    renderModal()
    await list()
    await userEvent.type(screen.getByRole('textbox', { name: 'Search by name or subject' }), 'refund')
    expect(screen.getByText('No template matches “refund”')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(rowNames()).toHaveLength(3)
  })

  it('does not keep a template the search hides in the preview', async () => {
    // The pane used to keep the previewed template, with "Use this template" choosing it, after a
    // search had taken it out of the list.
    const { onPick } = renderModal({ value: 'pwd' })
    await list()
    expect(within(pane()).getByRole('heading', { name: 'Password Reset' })).toBeInTheDocument()

    const search = screen.getByRole('textbox', { name: 'Search by name or subject' })
    await userEvent.type(search, 'community')
    expect(rowNames()).toEqual(['Welcome Email'])
    expect(within(pane()).queryByRole('heading', { name: 'Password Reset' })).toBeNull()
    expect(within(pane()).queryByRole('button', { name: 'Use this template' })).toBeNull()
    expect(within(pane()).getByText('Pick a template to preview it')).toBeInTheDocument()

    // Clearing the search brings the template, and its preview, back.
    await userEvent.clear(search)
    expect(within(pane()).getByRole('heading', { name: 'Password Reset' })).toBeInTheDocument()
    expect(onPick).not.toHaveBeenCalled()
  })

  it('locks the category the caller imposes, and asks the API for it alone', async () => {
    ;(templatesApi.list as Mock).mockResolvedValue({ templates: [reset] })
    renderModal({ category: 'transactional' })
    await list()
    expect(templatesApi.list).toHaveBeenCalledWith({ workspace_id: 'ws1', category: 'transactional', channel: 'email' })
    const select = screen.getByRole('combobox', { name: 'Category' })
    expect(select).toBeDisabled()
    expect(select.closest('.ant-select')).toHaveTextContent('Transactional')
  })

  it('leaves the category free when the caller imposes none', async () => {
    renderModal()
    await list()
    expect(screen.getByRole('combobox', { name: 'Category' })).not.toBeDisabled()
  })

  it('follows the pointed row, and uses the one previewed', async () => {
    const { onPick } = renderModal({ value: 'pwd' })
    await list()
    expect(within(pane()).getByRole('heading', { name: 'Password Reset' })).toBeInTheDocument()

    await userEvent.hover(row(/^Cart Recovery B/))
    await waitFor(() => expect(within(pane()).getByRole('heading', { name: 'Cart Recovery B' })).toBeInTheDocument())
    expect(within(pane()).getByTestId('frame')).toHaveAttribute('data-template', 'cart-b')

    await userEvent.click(row(/^Welcome Email/))
    expect(within(pane()).getByRole('heading', { name: 'Welcome Email' })).toBeInTheDocument()
    expect(onPick).not.toHaveBeenCalled()

    await userEvent.click(within(pane()).getByRole('button', { name: 'Use this template' }))
    expect(onPick).toHaveBeenCalledWith(welcome)
  })

  it('uses a template on a double click', async () => {
    const { onPick } = renderModal()
    await list()
    await userEvent.dblClick(row(/^Cart Recovery B/))
    expect(onPick).toHaveBeenCalledWith(cart)
  })

  it('asks to pick a template before anything is pointed at', async () => {
    renderModal()
    await list()
    expect(within(pane()).getByText('Pick a template to preview it')).toBeInTheDocument()
  })

  it('shows the rendered subject, sender, languages, variables and what uses the template', async () => {
    renderModal({ value: 'pwd', sendsWith: 'transactional' })
    await list()
    expect(await within(pane()).findByText('Rendered: Reset your password, {{contact.first_name}}')).toBeInTheDocument()
    expect(within(pane()).getByText('This link expires in one hour')).toBeInTheDocument()
    expect(within(pane()).getByText('Acme Support · support@acme.com')).toBeInTheDocument()
    expect(within(pane()).getByText('reset_url')).toBeInTheDocument()
    // The variables every send fills in are left out.
    expect(within(pane()).queryByText('contact.first_name')).toBeNull()
    expect(await within(pane()).findByText('Password Reset Email')).toBeInTheDocument()
    expect(within(pane()).getByText('(Notification)')).toBeInTheDocument()
    expect(templatesApi.usage).toHaveBeenCalledWith({ workspace_id: 'ws1', template_id: 'pwd' })
  })

  it('names a broadcast that uses the template a broadcast', async () => {
    // "Used by" called it a Newsletter, a word the Broadcasts screens don't use.
    ;(templatesApi.usage as Mock).mockImplementation(({ template_id }: { template_id?: string }) =>
      Promise.resolve(
        template_id
          ? { usage: { 'cart-b': { broadcasts: 1 } }, references: [{ type: 'broadcast', id: 'b1', name: 'Black Friday', status: 'draft' }], hidden_types: [] }
          : { usage: { 'cart-b': { broadcasts: 1 } }, hidden_types: [] }
      )
    )
    renderModal({ value: 'cart-b' })
    await list()
    expect(await within(row(/^Cart Recovery B/)).findByText('1 broadcast')).toBeInTheDocument()
    expect(await within(pane()).findByText('Black Friday')).toBeInTheDocument()
    expect(within(pane()).getByText('(Broadcast · Draft)')).toBeInTheDocument()
  })

  it('previews a translation in its own language', async () => {
    renderModal({ value: 'pwd' })
    await list()
    await userEvent.click(within(pane()).getByText('French'))
    expect(await within(pane()).findByText('Rendered: Réinitialisez votre mot de passe')).toBeInTheDocument()
    expect(within(pane()).getByTestId('frame')).toHaveAttribute('data-template', 'pwd@fr')
  })

  it('opens the full preview in the language the pane shows', async () => {
    // It opened in the default language after a translation had been picked.
    const { onFullPreview } = renderModal({ value: 'pwd' })
    await list()
    await userEvent.click(within(pane()).getByText('French'))
    await userEvent.click(within(pane()).getByRole('button', { name: 'Full preview' }))
    expect(onFullPreview).toHaveBeenCalledWith(reset, 'fr')
  })

  it('does not ask what uses a template nothing uses', async () => {
    renderModal({ value: 'welcome' })
    await list()
    await within(pane()).findByText('Rendered: Welcome to our community')
    expect(within(pane()).queryByText('Used by')).toBeNull()
    expect(templatesApi.usage).not.toHaveBeenCalledWith({ workspace_id: 'ws1', template_id: 'welcome' })
  })

  it('opens the full preview and duplicates from a row menu', async () => {
    const { onFullPreview, onDuplicate } = renderModal()
    await list()
    await userEvent.click(screen.getByRole('button', { name: 'More actions for Cart Recovery B' }))
    await userEvent.click(await screen.findByRole('menuitem', { name: /Full preview/ }))
    expect(onFullPreview).toHaveBeenCalledWith(cart)

    await userEvent.click(screen.getByRole('button', { name: 'More actions for Welcome Email' }))
    const menus = await screen.findAllByRole('menuitem', { name: /Duplicate and edit/ })
    await userEvent.click(menus[menus.length - 1])
    expect(onDuplicate).toHaveBeenCalledWith(welcome)
  })

  it('creates a new template from the header', async () => {
    const { onNew } = renderModal()
    await list()
    await userEvent.click(screen.getByRole('button', { name: 'New template' }))
    expect(onNew).toHaveBeenCalled()
  })

  it('disables New template and Duplicate and edit without Templates write', async () => {
    const { onNew, onDuplicate } = renderModal({ canWrite: false })
    await list()
    expect(screen.getByRole('button', { name: 'New template' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'New template' }))
    expect(onNew).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'More actions for Welcome Email' }))
    const duplicate = await screen.findByRole('menuitem', { name: /Duplicate and edit/ })
    expect(duplicate).toHaveAttribute('aria-disabled', 'true')
    await userEvent.click(duplicate)
    expect(onDuplicate).not.toHaveBeenCalled()
  })

  it('lists the same languages on a row as in its preview: a translation without content is left out', async () => {
    const draftDe = {
      ...reset,
      translations: { ...reset.translations, de: { email: { subject: 'Passwort zurücksetzen' } } }
    } as unknown as Template
    ;(templatesApi.list as Mock).mockResolvedValue({ templates: [draftDe] })
    renderModal({ value: 'pwd' })
    await list()
    expect(within(row(/^Password Reset/)).getByText('EN · FR')).toBeInTheDocument()
    expect(within(pane()).getByText('French')).toBeInTheDocument()
    expect(within(pane()).queryByText('German')).toBeNull()
  })

  describe('the sender the template gets', () => {
    const twoProviders = {
      ...workspace,
      settings: { ...workspace.settings, marketing_email_provider_id: 'mk', transactional_email_provider_id: 'tx' },
      integrations: [
        {
          id: 'tx',
          name: 'Postmark',
          email_provider: {
            senders: [
              { id: 'tx-default', name: 'Acme Support', email: 'support@acme.com', is_default: true },
              { id: 'tx-billing', name: 'Billing', email: 'billing@acme.com', is_default: false }
            ]
          }
        },
        {
          id: 'mk',
          name: 'SES',
          email_provider: { senders: [{ id: 'mk-default', name: 'Acme News', email: 'news@acme.com', is_default: true }] }
        },
        { id: 'other', name: 'SMTP', email_provider: { senders: [{ id: 'o1', name: 'Ops', email: 'ops@acme.com', is_default: true }] } }
      ]
    } as unknown as Workspace
    const sender = () => within(pane()).getByText('Sender').nextElementSibling

    it('is the transactional provider’s for a notification, whatever the template’s category', async () => {
      renderModal({ workspace: twoProviders, value: 'cart-b', sendsWith: 'transactional' })
      await list()
      expect(sender()).toHaveTextContent('Acme Support · support@acme.com')
    })

    it('is the marketing provider’s for a marketing send, whatever the template’s category', async () => {
      renderModal({ workspace: twoProviders, value: 'pwd', sendsWith: 'marketing' })
      await list()
      expect(sender()).toHaveTextContent('Acme News · news@acme.com')
    })

    it('is the one of the integration an automation step names', async () => {
      renderModal({ workspace: twoProviders, value: 'pwd', sendsWith: { integrationId: 'other' } })
      await list()
      expect(sender()).toHaveTextContent('Ops · ops@acme.com')
    })

    it('looks for the template’s own sender in the sending provider alone', async () => {
      const billing = { ...reset, email: { ...reset.email, sender_id: 'tx-billing' } } as unknown as Template
      ;(templatesApi.list as Mock).mockResolvedValue({ templates: [billing] })
      renderModal({ workspace: twoProviders, value: 'pwd', sendsWith: 'marketing' })
      await list()
      expect(sender()).toHaveTextContent('Acme News · news@acme.com')
    })

    it('says when the sending provider has no sender for it', async () => {
      const none = { ...twoProviders, settings: { ...twoProviders.settings, marketing_email_provider_id: undefined } } as unknown as Workspace
      renderModal({ workspace: none, value: 'pwd', sendsWith: 'marketing' })
      await list()
      expect(sender()).toHaveTextContent('No sender')
    })

    it('is not shown for a filter, which sends nothing', async () => {
      renderModal({ workspace: twoProviders, value: 'pwd', anyLabel: 'Any template' })
      await list()
      expect(within(pane()).getByRole('heading', { name: 'Password Reset' })).toBeInTheDocument()
      expect(within(pane()).queryByText('Sender')).toBeNull()
      expect(within(pane()).queryByText(/support@acme\.com|news@acme\.com/)).toBeNull()
    })
  })

  describe('an empty value meaning every template', () => {
    it('lists Any template first, current when the value is empty, and chooses null', async () => {
      const { onPick } = renderModal({ anyLabel: 'Any template' })
      await list()
      expect(rowNames()[0]).toBe('Any template')
      const any = row(/^Any template/)
      expect(within(any).getByText('Current')).toBeInTheDocument()
      expect(within(pane()).getByText('Every template')).toBeInTheDocument()
      await userEvent.click(within(pane()).getByRole('button', { name: 'Match any template' }))
      expect(onPick).toHaveBeenCalledWith(null)
    })

    it('keeps Any template previewed when the window opens under a resting pointer', async () => {
      // Opened from a segment condition, the window put a row under the pointer that had clicked
      // the field: Chrome sent it an enter event and the pane previewed that template while "Any
      // template" was Current, with "Use this template" choosing it.
      renderModal({ anyLabel: 'Any template' })
      await list()
      fireEvent.mouseOver(row(/^Cart Recovery B/))
      fireEvent.mouseEnter(row(/^Cart Recovery B/))
      await new Promise((resolve) => setTimeout(resolve, 300))
      expect(within(pane()).getByText('Every template')).toBeInTheDocument()
      expect(within(pane()).queryByRole('button', { name: 'Use this template' })).toBeNull()

      // Once the pointer moves, the preview follows it.
      fireEvent.mouseMove(row(/^Cart Recovery B/))
      await waitFor(() => expect(within(pane()).getByRole('heading', { name: 'Cart Recovery B' })).toBeInTheDocument())
    })

    it('chooses null on a double click of Any template', async () => {
      const { onPick } = renderModal({ anyLabel: 'Any template', value: 'pwd' })
      await list()
      expect(within(row(/^Any template/)).queryByText('Current')).toBeNull()
      await userEvent.dblClick(row(/^Any template/))
      expect(onPick).toHaveBeenCalledWith(null)
    })

    it('still offers Any template when the workspace has no template at all', async () => {
      ;(templatesApi.list as Mock).mockResolvedValue({ templates: [] })
      const { onPick } = renderModal({ anyLabel: 'Any template' })
      await list()
      expect(rowNames()).toEqual(['Any template'])
      expect(within(row(/^Any template/)).getByText('Current')).toBeInTheDocument()
      await userEvent.dblClick(row(/^Any template/))
      expect(onPick).toHaveBeenCalledWith(null)
    })

    it('has no Any template row for a caller that does not ask for one', async () => {
      renderModal()
      await list()
      expect(rowNames()).not.toContain('Any template')
    })
  })

  describe('an imposed category without any template', () => {
    beforeEach(() => {
      ;(templatesApi.list as Mock).mockResolvedValue({ templates: [] })
    })

    it("shows the category's gallery models, and starts from the one clicked", async () => {
      const { onStartModel } = renderModal({ category: 'opt_in' })
      expect(await screen.findByText('No template in the Opt-in category yet')).toBeInTheDocument()
      const models = screen.getByRole('list', { name: 'Models' })
      const names = within(models).getAllByRole('button').map((button) => button.getAttribute('data-model'))
      expect(names).toEqual(['confirm', 'blank'])
      await userEvent.click(within(models).getByRole('button', { name: /Subscription confirmation/ }))
      expect(onStartModel).toHaveBeenCalledWith('confirm')
    })

    it('shows Simple under the imposed category, not a category to choose', async () => {
      renderModal({ category: 'opt_in' })
      const models = await screen.findByRole('list', { name: 'Models' })
      const blank = within(models).getByRole('button', { name: /Simple/ })
      expect(within(blank).getByText('Opt-in')).toBeInTheDocument()
      expect(within(models).queryByText('Category to choose')).toBeNull()
    })

    it('says a member without Templates write cannot create one, and shows no models', async () => {
      renderModal({ category: 'opt_in', canWrite: false })
      expect(await screen.findByText('You need Templates write permission to create one. Ask a workspace owner to add one.')).toBeInTheDocument()
      expect(screen.queryByRole('list', { name: 'Models' })).toBeNull()
      for (const button of screen.getAllByRole('button', { name: 'New template' })) expect(button).toBeDisabled()
    })
  })

  describe('in an automation Email node', () => {
    it('says a Marketing template skips the contacts unsubscribed from the list', async () => {
      renderModal({ value: 'cart-b', listStatusNotice: true })
      await list()
      expect(within(pane()).getByText(/^Marketing template: in an automation tied to a list/)).toBeInTheDocument()
    })

    it('says a Transactional template is sent to them anyway', async () => {
      renderModal({ value: 'pwd', listStatusNotice: true })
      await list()
      expect(within(pane()).getByText(/^Transactional template: sent even to contacts unsubscribed/)).toBeInTheDocument()
    })

    it('says nothing of it elsewhere', async () => {
      renderModal({ value: 'cart-b' })
      await list()
      expect(within(pane()).queryByText(/in an automation tied to a list/)).toBeNull()
    })
  })

  it('shows the subtitle the caller gives', async () => {
    renderModal({ subtitle: 'Email template of Password Reset Email' })
    expect(screen.getByText('Choose a template')).toBeInTheDocument()
    expect(screen.getByText('Email template of Password Reset Email')).toBeInTheDocument()
  })
})
