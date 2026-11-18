import '../../__tests__/resizeObserverMock'
import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { CreateListDrawer } from './ListDrawer'
import { templatesApi } from '../../services/api/template'
import { listsApi } from '../../services/api/list'
import { ApiError } from '../../services/api/errors'
import { LIST_ID_CHECK_DELAY } from './useListIdCheck'
import type { List } from '../../services/api/types'

// The blocker the drawer registers for browser Back: the tests call it as the router would.
type ShouldBlock = (args: { action: string }) => Promise<boolean> | boolean
const router = vi.hoisted(() => ({ shouldBlock: undefined as undefined | ShouldBlock }))
vi.mock('@tanstack/react-router', () => ({
  useBlocker: (opts: { shouldBlockFn: ShouldBlock }) => {
    router.shouldBlock = opts.shouldBlockFn
    return { status: 'idle' }
  }
}))
// The API client imports the router, and the router every page.
vi.mock('../../router', () => ({ router: { navigate: vi.fn() } }))
vi.mock('../../services/api/template', () => ({
  // The window counts the uses of each type the API names.
  TEMPLATE_USAGE_TYPES: ['broadcasts', 'transactional_notifications', 'automations', 'lists'],
  templatesApi: { list: vi.fn(), get: vi.fn(), usage: vi.fn(), compile: vi.fn() }
}))
vi.mock('../../services/api/list', () => ({
  listsApi: { create: vi.fn(), update: vi.fn(), get: vi.fn(), stats: vi.fn() }
}))
vi.mock('../../contexts/AuthContext', () => ({
  // TemplateSelectorInput reads the member's permissions for its drawers
  useWorkspacePermissions: () => ({ permissions: null }),
  // Both providers set, each with its default sender: the window names the one a send uses.
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
vi.mock('../templates/TemplatePreviewDrawer', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>
}))
vi.mock('../templates/CreateTemplateDrawer', () => ({ CreateTemplateDrawer: () => null }))
vi.mock('../broadcasts/detail/TemplatePreviewFrame', () => ({ TemplatePreviewFrame: () => <div data-testid="frame" /> }))

const list: List = {
  id: 'newsletter',
  name: 'Newsletter',
  description: 'Monthly',
  is_double_optin: true,
  is_public: false,
  double_optin_template: { id: 'tpl-optin', version: 1 },
  created_at: '2026-03-03T10:00:00Z',
  updated_at: '2026-03-03T10:00:00Z'
}
const other: List = { ...list, id: 'other', name: 'Other', is_double_optin: false, double_optin_template: undefined }

let queryClient: QueryClient
interface Props {
  list?: List
  open?: boolean
  canReadTemplates?: boolean
  onClose?: () => void
  onCreated?: (list: List) => void
}
function setup(props: Props = {}) {
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onClose = props.onClose ?? vi.fn()
  const onCreated = props.onCreated ?? vi.fn()
  const tree = (p: Props) => (
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <CreateListDrawer
            workspaceId="ws1"
            list={p.list}
            open={p.open ?? true}
            canReadTemplates={p.canReadTemplates ?? true}
            onClose={onClose}
            onCreated={onCreated}
          />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
  const utils = render(tree(props))
  return { onClose, onCreated, rerender: (p: Props) => utils.rerender(tree({ ...props, ...p })) }
}

const drawer = (name: 'New list' | 'Edit list') => screen.getByRole('dialog', { name })
const nameInput = () => screen.getByLabelText('Name')
const idInput = () => screen.getByLabelText('ID')
// antd hides a Form.Item with a class its stylesheet styles: jsdom has no stylesheet.
const idHidden = () => idInput().closest('.ant-form-item')?.classList.contains('ant-form-item-hidden')
const createButton = () => screen.getByRole('button', { name: 'Create list' })
// antd's Form.useWatch hands a change on a beat later: the drawer's derived lines follow then.
const flush = () => act(() => new Promise((resolve) => setTimeout(resolve, 20)))
const change = async (element: HTMLElement, value: string) => {
  fireEvent.change(element, { target: { value } })
  await flush()
}
const typeName = (value: string) => change(nameInput(), value)
const confirmShown = () => screen.queryAllByText('Discard your changes?').length > 0
// Lets the ID check's pause run out and its answer land.
async function checkSettles() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, LIST_ID_CHECK_DELAY + 50))
  })
}

beforeEach(() => {
  router.shouldBlock = undefined
  vi.mocked(templatesApi.list).mockReset().mockResolvedValue({ templates: [] } as never)
  vi.mocked(templatesApi.usage).mockReset().mockResolvedValue({ usage: {}, hidden_types: [] } as never)
  vi.mocked(templatesApi.compile).mockReset().mockResolvedValue({ html: '', mjml: '' } as never)
  vi.mocked(templatesApi.get)
    .mockReset()
    .mockResolvedValue({ template: { id: 'tpl-optin', name: 'Confirm your subscription', category: 'opt_in' } } as never)
  vi.mocked(listsApi.get).mockReset().mockRejectedValue(new ApiError('List not found', 404))
  vi.mocked(listsApi.create)
    .mockReset()
    .mockImplementation(async (req) => ({ list: { ...req, created_at: '', updated_at: '' } as List }))
  vi.mocked(listsApi.update).mockReset().mockResolvedValue({ list } as never)
  vi.mocked(listsApi.stats)
    .mockReset()
    .mockResolvedValue({ list_id: 'newsletter', stats: { total_active: 8, total_pending: 312, total_unsubscribed: 0, total_bounced: 0, total_complained: 0 } } as never)
})
afterEach(() => vi.restoreAllMocks())

describe('CreateListDrawer: opening', () => {
  it('stays closed and renders nothing while closed (no trigger button any more)', () => {
    setup({ open: false })
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('opens as « New list » with its buttons in the footer', () => {
    setup()
    expect(drawer('New list')).toBeInTheDocument()
    expect(createButton()).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
  })
})

describe('CreateListDrawer: opening again', () => {
  // The page keeps the drawer mounted, closed: each opening reads the list it is given then.
  it('opens on the list given at each opening, and empty for a new one', async () => {
    const { rerender } = setup({ open: false })
    rerender({ open: true, list })
    expect(drawer('Edit list')).toBeInTheDocument()
    expect(nameInput()).toHaveValue('Newsletter')
    await change(nameInput(), 'Edited')

    rerender({ open: false, list: undefined })
    rerender({ open: true, list: undefined })
    await flush()
    const created = screen.getAllByRole('dialog').find((dialog) => dialog.getAttribute('aria-label') === 'New list' || dialog.textContent?.includes('New list'))
    expect(created).toBeDefined()
    expect(screen.getByRole('button', { name: 'Create list' })).toBeInTheDocument()
    expect(nameInput()).toHaveValue('')
  })
})

describe('CreateListDrawer: the ID on create', () => {
  it('does not open the ID on a submit with no name', async () => {
    setup()
    fireEvent.click(createButton())
    expect(await screen.findByText('Please enter a list name')).toBeInTheDocument()
    expect(idHidden()).toBe(true)
    await typeName('Beta')
    expect(screen.getByText('beta')).toBeInTheDocument()
  })

  it('reads an ID such as constructor as any other', async () => {
    setup()
    await typeName('Constructor')
    await checkSettles()
    expect(screen.getByText('available')).toBeInTheDocument()
    expect(createButton()).toBeEnabled()
  })

  it('derives the ID from the name and shows it under it, checked', async () => {
    setup()
    expect(screen.getByText('ID derived from the name')).toBeInTheDocument()
    await typeName('Newsletter produit')
    expect(screen.getByText('newsletterproduit')).toBeInTheDocument()
    expect(idHidden()).toBe(true)
    await checkSettles()
    expect(listsApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'newsletterproduit' })
    expect(screen.getByText('available')).toBeInTheDocument()
  })

  it('opens the ID with Modify, after which it no longer follows the name', async () => {
    setup()
    await typeName('Newsletter produit')
    fireEvent.click(screen.getByRole('button', { name: 'Modify' }))
    await flush()
    expect(idHidden()).toBe(false)
    await change(idInput(), 'news')
    await typeName('Newsletter produit 2')
    expect(idInput()).toHaveValue('news')
    expect(screen.getByText(/Letters and digits, 32 at most/)).toBeInTheDocument()
  })

  it('opens the ID by itself, required, when the name gives none', async () => {
    setup()
    await typeName('会員ニュース')
    expect(idHidden()).toBe(false)
    expect(screen.getByText('The name gives no ID: enter one.')).toBeInTheDocument()
    fireEvent.click(createButton())
    expect(await screen.findByText('Please enter a list ID')).toBeInTheDocument()
    expect(listsApi.create).not.toHaveBeenCalled()

    await change(idInput(), 'members')
    await checkSettles()
    fireEvent.click(createButton())
    await waitFor(() => expect(listsApi.create).toHaveBeenCalledWith(expect.objectContaining({ id: 'members', name: '会員ニュース' })))
  })

  it('refuses a taken ID while typing: the field opens with the reason and Create is disabled', async () => {
    vi.mocked(listsApi.get).mockResolvedValue({ list } as never)
    setup()
    await typeName('Newsletter')
    expect(createButton()).toBeDisabled() // checking
    await checkSettles()
    expect(idHidden()).toBe(false)
    expect(screen.getByText('A list already has the ID “newsletter”. Change it.')).toBeInTheDocument()
    expect(screen.getByText('already used')).toBeInTheDocument()
    expect(createButton()).toBeDisabled()
  })

  it('keeps the field a taken ID opened while the ID is corrected, or emptied', async () => {
    vi.mocked(listsApi.get).mockImplementation(async ({ id }) => {
      if (id === 'newsletter') return { list } as never
      throw new ApiError('List not found', 404)
    })
    setup()
    await typeName('Newsletter')
    await checkSettles()
    expect(idHidden()).toBe(false)

    await change(idInput(), 'newsletter2')
    expect(idHidden()).toBe(false)
    await checkSettles()
    expect(screen.getByText('available')).toBeInTheDocument()

    await change(idInput(), '')
    expect(idHidden()).toBe(false)
    fireEvent.click(createButton())
    expect(await screen.findByText('Please enter a list ID')).toBeInTheDocument()
    expect(listsApi.create).not.toHaveBeenCalled()
  })

  it('keeps an ID typed by hand when the name changes', async () => {
    setup()
    await typeName('会員')
    await change(idInput(), 'members')
    await typeName('Members 会員')
    expect(idHidden()).toBe(false)
    expect(idInput()).toHaveValue('members')
  })

  it('does not take a failed check for a free ID, and lets Create through', async () => {
    vi.mocked(listsApi.get).mockRejectedValue(new ApiError('Failed to get list', 500))
    setup()
    await typeName('Beta')
    await checkSettles()
    expect(screen.queryByText('available')).toBeNull()
    expect(createButton()).toBeEnabled()
  })

  // Typed and submitted in the same beat: the ID the form holds has not reached the check yet.
  it('does not submit an ID the check has not seen yet', async () => {
    setup()
    fireEvent.change(nameInput(), { target: { value: 'Beta' } })
    fireEvent.submit(nameInput().closest('form') as HTMLFormElement)
    await flush()
    expect(listsApi.create).not.toHaveBeenCalled()
  })

  it('does not submit with Enter in the name while the ID is being checked', async () => {
    setup()
    await typeName('Beta')
    fireEvent.submit(nameInput().closest('form') as HTMLFormElement)
    await act(async () => {})
    expect(listsApi.create).not.toHaveBeenCalled()
    await checkSettles()
    fireEvent.submit(nameInput().closest('form') as HTMLFormElement)
    await waitFor(() => expect(listsApi.create).toHaveBeenCalled())
  })
})

describe('CreateListDrawer: create', () => {
  it('sends every field, refreshes the lists, closes and opens the new list', async () => {
    const invalidate = vi.spyOn(QueryClient.prototype, 'invalidateQueries')
    const { onClose, onCreated } = setup()
    queryClient.setQueryData(['lists', 'ws1'], { lists: [other] })
    await typeName('Beta testers')
    await change(screen.getByLabelText('Description'), 'Early access')
    fireEvent.click(screen.getByRole('switch', { name: 'Public list' }))
    await checkSettles()
    fireEvent.click(createButton())

    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(expect.objectContaining({ id: 'betatesters' })))
    expect(listsApi.create).toHaveBeenCalledWith({
      workspace_id: 'ws1',
      id: 'betatesters',
      name: 'Beta testers',
      description: 'Early access',
      is_public: true,
      is_double_optin: false,
      double_optin_template: undefined
    })
    expect(onClose).toHaveBeenCalled()
    // The page holds the new list before it opens it.
    expect(queryClient.getQueryData<{ lists: List[] }>(['lists', 'ws1'])?.lists.map((l) => l.id)).toEqual(['other', 'betatesters'])
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['lists', 'ws1'] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['lists-with-counts', 'ws1'] })
    // The navigation to the new list is not stopped by the close guard.
    expect(await router.shouldBlock?.({ action: 'PUSH' })).toBe(false)
    expect(confirmShown()).toBe(false)
  })

  it('requires the confirmation template while double opt-in is on', async () => {
    setup()
    await typeName('Beta')
    fireEvent.click(screen.getByRole('switch', { name: 'Double opt-in' }))
    await flush()
    expect(screen.getByText('Choose a confirmation template', { selector: 'span' })).toBeInTheDocument()
    await checkSettles()
    fireEvent.click(createButton())
    expect(await screen.findByText('Choose a confirmation template', { selector: '.ant-form-item-explain-error' })).toBeInTheDocument()
    expect(listsApi.create).not.toHaveBeenCalled()
  })
})

describe('CreateListDrawer: a failed save', () => {
  it('stays in the drawer with the message, no toast, and a retry clears it', async () => {
    vi.mocked(listsApi.create).mockRejectedValueOnce(new ApiError('Failed to create list', 500))
    const { onClose } = setup()
    await typeName('Beta')
    await checkSettles()
    fireEvent.click(createButton())
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('The list could not be created.')
    expect(alert).not.toHaveTextContent('Failed to create list')
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.click(createButton())
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('adds the server message when it says more', async () => {
    vi.mocked(listsApi.create).mockRejectedValueOnce(new ApiError('invalid create list request: name is required', 400))
    setup()
    await typeName('Beta')
    await checkSettles()
    fireEvent.click(createButton())
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'The list could not be created. invalid create list request: name is required'
    )
  })

  it.each([
    ['list_id_exists', 'A list already has the ID “beta”. Change it.'],
    ['list_id_deleted', 'The ID “beta” belonged to a deleted list and cannot be reused. Choose another.']
  ])('puts a %s refusal on the ID, and keeps it there', async (code, text) => {
    vi.mocked(listsApi.create).mockRejectedValue(new ApiError('List id already exists', 400, { error: 'x', code }))
    setup()
    await typeName('Beta')
    await checkSettles()
    fireEvent.click(createButton())
    expect(await screen.findByText(text)).toBeInTheDocument()
    expect(idHidden()).toBe(false)
    expect(screen.queryByRole('alert')).toBeNull()
    // lists.get still finds no list under it: the refusal is the answer shown.
    expect(screen.queryByText('available')).toBeNull()
    expect(createButton()).toBeDisabled()

    // Another ID clears it; typing the refused one again brings it back.
    await change(idInput(), 'beta2')
    expect(screen.queryByText(text)).toBeNull()
    await change(idInput(), 'beta')
    expect(screen.getByText(text)).toBeInTheDocument()
    expect(createButton()).toBeDisabled()
  })
})

describe('CreateListDrawer: edit', () => {
  it('fills the form, shows the ID read-only with a copy button and the creation date', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    setup({ list })
    expect(drawer('Edit list')).toBeInTheDocument()
    expect(nameInput()).toHaveValue('Newsletter')
    expect(screen.queryByLabelText('ID')).toBeNull()
    expect(screen.getByText('newsletter')).toBeInTheDocument()
    expect(screen.getByText('Cannot be changed, used by the API and forms.')).toBeInTheDocument()
    expect(screen.getByText(/^Created on .*2026/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('newsletter'))
    expect(await screen.findByText('ID copied')).toBeInTheDocument()
    expect(listsApi.get).not.toHaveBeenCalled()
  })

  it('saves the list it opened with, even after the page moves to another list', async () => {
    const { rerender, onClose } = setup({ list, canReadTemplates: false })
    rerender({ list: other })
    await change(nameInput(), 'Newsletter FR')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(listsApi.update).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'newsletter', name: 'Newsletter FR', double_optin_template: { id: 'tpl-optin', version: 1 } })
      )
    )
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('says the pending contacts stay pending when double opt-in is turned off', async () => {
    setup({ list })
    fireEvent.click(screen.getByRole('switch', { name: 'Double opt-in' }))
    expect(
      await screen.findByText('Turning double opt-in off does not change the 312 “Pending” contacts: they stay pending until they confirm.')
    ).toBeInTheDocument()
  })

  it('says it without the count while the stats load, and not at all with none pending', async () => {
    vi.mocked(listsApi.stats).mockReturnValue(new Promise(() => {}))
    setup({ list })
    fireEvent.click(screen.getByRole('switch', { name: 'Double opt-in' }))
    await flush()
    expect(
      screen.getByText('Turning double opt-in off does not change the “Pending” contacts: they stay pending until they confirm.')
    ).toBeInTheDocument()
  })

  it('says nothing when no contact is pending, or when the list never had double opt-in', async () => {
    vi.mocked(listsApi.stats).mockResolvedValue({ list_id: 'newsletter', stats: { total_pending: 0 } } as never)
    setup({ list })
    await act(async () => {})
    fireEvent.click(screen.getByRole('switch', { name: 'Double opt-in' }))
    await act(async () => {})
    expect(screen.queryByText(/stay pending until they confirm/)).toBeNull()
  })

  it('does not ask for stats of a list without double opt-in', async () => {
    setup({ list: other })
    await act(async () => {})
    expect(listsApi.stats).not.toHaveBeenCalled()
  })
})

describe('CreateListDrawer: the notification center preview', () => {
  it('shows a public list as the contact sees it, following the form', async () => {
    setup()
    expect(screen.queryByRole('region', { name: 'In the notification center' })).toBeNull()
    await typeName('Product news')
    fireEvent.click(screen.getByRole('switch', { name: 'Public list' }))
    await flush()
    const preview = screen.getByRole('region', { name: 'In the notification center' })
    expect(within(preview).getByText('Product news')).toBeInTheDocument()
    await change(screen.getByLabelText('Description'), 'Once a month.')
    expect(within(preview).getByText('Once a month.')).toBeInTheDocument()
  })
})

describe('CreateListDrawer: the double opt-in template', () => {
  // Without Templates read, templates.list and templates.get answer 403: the card shows the
  // stored ID, locked, and saving keeps the template.
  it("shows the template's ID without Templates read, requests nothing, and saves it unchanged", async () => {
    setup({ list, canReadTemplates: false })
    expect(screen.getByText('tpl-optin')).toBeInTheDocument()
    await act(async () => {})
    expect(templatesApi.list).not.toHaveBeenCalled()
    expect(templatesApi.get).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(listsApi.update).toHaveBeenCalled())
    expect(vi.mocked(listsApi.update).mock.calls[0][0].double_optin_template).toEqual({ id: 'tpl-optin', version: 1 })
  })

  it("shows the template's name with Templates read, and its category", async () => {
    setup({ list })
    expect(await screen.findByText('Confirm your subscription')).toBeInTheDocument()
    expect(screen.getByText('Opt-in category · sent by the marketing provider')).toBeInTheDocument()
  })

  // The window lists the Opt-in templates alone, says what the template is for, and names the
  // marketing provider's sender.
  it('opens the window on the Opt-in category with Change, naming the marketing sender', async () => {
    const optin = { id: 'tpl-optin', name: 'Confirm your subscription', category: 'opt_in', email: { subject: 'Confirm' } }
    vi.mocked(templatesApi.list).mockResolvedValue({ templates: [optin] } as never)
    vi.mocked(templatesApi.get).mockResolvedValue({ template: optin } as never)
    setup({ list })
    await screen.findByText('Confirm your subscription')
    fireEvent.click(screen.getByRole('button', { name: 'Change' }))
    const dialog = await waitFor(() => {
      const found = screen.getAllByRole('dialog').find((element) => element.textContent?.startsWith('Choose a template'))
      expect(found).toBeDefined()
      return found as HTMLElement
    })
    expect(within(dialog).getByText('The confirmation email each new subscriber receives')).toBeInTheDocument()
    await waitFor(() =>
      expect(templatesApi.list).toHaveBeenCalledWith({ workspace_id: 'ws1', category: 'opt_in', channel: 'email' })
    )
    const preview = await screen.findByRole('region', { name: 'Preview' })
    expect(await within(preview).findByText('News · news@acme.com')).toBeInTheDocument()
    expect(within(preview).queryByText('Support · support@acme.com')).toBeNull()
  })
})

describe('CreateListDrawer: closing', () => {
  it('closes at once when nothing changed, by ×, Cancel or Escape', () => {
    const { onClose } = setup({ list })
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    fireEvent.keyDown(drawer('Edit list'), { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(3)
    expect(confirmShown()).toBe(false)
  })

  it('is not dirty after a name typed then erased', async () => {
    const { onClose } = setup()
    await typeName('Beta')
    await typeName('')
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onClose).toHaveBeenCalled()
  })

  it('asks before dropping changes: Keep editing keeps them, Discard closes', async () => {
    const { onClose } = setup({ list })
    await change(nameInput(), 'Changed')
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(confirmShown()).toBe(true)
    // A second request while the window is open does not stack another.
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(document.querySelectorAll('.ant-modal-confirm')).toHaveLength(1)

    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    await waitFor(() => expect(confirmShown()).toBe(false))
    expect(onClose).not.toHaveBeenCalled()
    expect(nameInput()).toHaveValue('Changed')

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Discard' }))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('asks on browser Back when dirty, and closes then lets it through when clean', async () => {
    const first = setup({ list })
    await change(nameInput(), 'Changed')
    let blocked: Promise<boolean> | boolean | undefined
    act(() => {
      blocked = router.shouldBlock?.({ action: 'BACK' })
    })
    expect(confirmShown()).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    expect(await blocked).toBe(true)
    expect(first.onClose).not.toHaveBeenCalled()

    await change(nameInput(), 'Newsletter')
    let clean: Promise<boolean> | boolean | undefined
    act(() => {
      clean = router.shouldBlock?.({ action: 'BACK' })
    })
    expect(await clean).toBe(false)
    expect(first.onClose).toHaveBeenCalled()
  })

  it("lets the page's own replace navigations through without closing or asking", async () => {
    const { onClose } = setup({ list })
    await change(nameInput(), 'Changed')
    expect(await router.shouldBlock?.({ action: 'REPLACE' })).toBe(false)
    expect(onClose).not.toHaveBeenCalled()
    expect(confirmShown()).toBe(false)
  })

  it('ignores close requests while saving', async () => {
    vi.mocked(listsApi.update).mockReturnValue(new Promise(() => {}))
    const { onClose } = setup({ list, canReadTemplates: false })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(listsApi.update).toHaveBeenCalled())
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).not.toHaveBeenCalled()
    expect(await router.shouldBlock?.({ action: 'BACK' })).toBe(true)
  })
})
