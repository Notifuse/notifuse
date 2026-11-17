import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import SendTemplateModal from './SendTemplateModal'
import { transactionalNotificationsApi } from '../../services/api/transactional_notifications'
import { listsApi } from '../../services/api/list'
import type { Template, Workspace } from '../../services/api/types'

// antd's Select mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

vi.mock('../../services/api/transactional_notifications', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('../../services/api/transactional_notifications')>()
  return {
    ...actual,
    transactionalNotificationsApi: { ...actual.transactionalNotificationsApi, testTemplate: vi.fn() }
  }
})

vi.mock('../../services/api/list', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/list')>()
  return { ...actual, listsApi: { ...actual.listsApi, list: vi.fn() } }
})

const workspace = {
  id: 'ws1',
  settings: { default_language: 'en' },
  integrations: [
    {
      id: 'int1',
      name: 'SMTP',
      type: 'email',
      email_provider: {
        kind: 'smtp',
        senders: [{ id: 'sender1', email: 'hello@example.com', name: 'Hello' }]
      }
    }
  ]
} as unknown as Workspace

const template = { id: 'tpl1', name: 'Newsletter', category: 'marketing' } as unknown as Template

const lists = [
  { id: 'news', name: 'Monthly news' },
  { id: 'beta', name: 'Beta testers' }
]

interface ModalProps {
  defaultListId?: string
  omitListUnsubscribeHeader?: boolean
  canReadLists?: boolean
  canReadContacts?: boolean
}

function modal(props: ModalProps & { isOpen?: boolean }, queryClient: QueryClient) {
  const { isOpen = true, ...rest } = props
  return (
    <QueryClientProvider client={queryClient}>
      <AntApp>
        <SendTemplateModal
          isOpen={isOpen}
          onClose={() => {}}
          template={template}
          workspace={workspace}
          {...rest}
          canReadContacts={rest.canReadContacts ?? false}
        />
      </AntApp>
    </QueryClientProvider>
  )
}

function renderModal(props: ModalProps) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const result = render(modal(props, queryClient))
  return {
    ...result,
    rerenderModal: (next: ModalProps & { isOpen?: boolean }) =>
      result.rerender(modal(next, queryClient))
  }
}

function selectedList() {
  const field = screen.getByText('Mailing list').closest('.ant-form-item')
  return field?.querySelector('.ant-select-content')?.textContent
}

async function pickList(name: string) {
  const field = screen.getByText('Mailing list').closest('.ant-form-item')
  const select = field?.querySelector('.ant-select')
  expect(select).not.toBeNull()
  fireEvent.mouseDown(select as Element)
  const option = await waitFor(() => {
    const found = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
      (el) => el.textContent === name
    )
    expect(found).toBeDefined()
    return found as Element
  })
  fireEvent.click(option)
}

async function sendTo(email: string) {
  await userEvent.type(screen.getByPlaceholderText('recipient@example.com'), email)
  const buttons = screen.getAllByRole('button', { name: /Send test email/i })
  await userEvent.click(buttons[buttons.length - 1])
  await waitFor(() => expect(transactionalNotificationsApi.testTemplate).toHaveBeenCalled())
  return (transactionalNotificationsApi.testTemplate as Mock).mock.calls[0]
}

describe('SendTemplateModal mailing list', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(listsApi.list as Mock).mockResolvedValue({ lists })
    ;(transactionalNotificationsApi.testTemplate as Mock).mockResolvedValue({ success: true })
  })

  it('preselects the default list, labels it and sends the test for it', async () => {
    renderModal({ canReadLists: true, defaultListId: 'news' })

    expect(await screen.findByText('Monthly news (broadcast audience)')).toBeInTheDocument()
    expect(listsApi.list).toHaveBeenCalledWith({ workspace_id: 'ws1' })
    expect(
      screen.getByText(/or a bounce or complaint on it, really changes the recipient's subscription/i)
    ).toBeInTheDocument()

    const call = await sendTo('me@example.com')
    expect(call[7]).toEqual({ listId: 'news', omitListUnsubscribeHeader: false })
  })

  it('keeps the list but says there is no header for a notice broadcast', async () => {
    renderModal({ canReadLists: true, defaultListId: 'news', omitListUnsubscribeHeader: true })

    expect(await screen.findByText('Monthly news (broadcast audience)')).toBeInTheDocument()
    expect(
      screen.getByText(/also goes to the list's unsubscribed contacts/i)
    ).toBeInTheDocument()

    const call = await sendTo('me@example.com')
    expect(call[7]).toEqual({ listId: 'news', omitListUnsubscribeHeader: true })
  })

  it('sends the test for a list picked from the field when no list is preselected', async () => {
    renderModal({ canReadLists: true })

    expect(await screen.findByText('None (no unsubscribe link)')).toBeInTheDocument()
    await waitFor(() => expect(listsApi.list).toHaveBeenCalled())
    await pickList('Beta testers')
    expect(
      await screen.findByText(/and the List-Unsubscribe header, as a broadcast to this list does/i)
    ).toBeInTheDocument()

    const call = await sendTo('me@example.com')
    expect(call[7]).toEqual({ listId: 'beta', omitListUnsubscribeHeader: false })
  })

  it('keeps the header for another list picked in a notice broadcast test', async () => {
    renderModal({ canReadLists: true, defaultListId: 'news', omitListUnsubscribeHeader: true })

    expect(await screen.findByText('Monthly news (broadcast audience)')).toBeInTheDocument()
    await pickList('Beta testers')
    expect(
      await screen.findByText(/and the List-Unsubscribe header, as a broadcast to this list does/i)
    ).toBeInTheDocument()

    const call = await sendTo('me@example.com')
    expect(call[7]).toEqual({ listId: 'beta', omitListUnsubscribeHeader: false })
  })

  it('goes back to the default list when the dialog is opened again', async () => {
    const { rerenderModal } = renderModal({ canReadLists: true, defaultListId: 'news' })

    expect(await screen.findByText('Monthly news (broadcast audience)')).toBeInTheDocument()
    await pickList('Beta testers')
    await waitFor(() => expect(selectedList()).toBe('Beta testers'))

    rerenderModal({ canReadLists: true, defaultListId: 'news', isOpen: false })
    rerenderModal({ canReadLists: true, defaultListId: 'news', isOpen: true })
    await waitFor(() => expect(selectedList()).toBe('Monthly news (broadcast audience)'))

    const call = await sendTo('me@example.com')
    expect(call[7]).toEqual({ listId: 'news', omitListUnsubscribeHeader: false })
  })

  it('waits for the lists before a test for the preselected list can be sent', async () => {
    ;(listsApi.list as Mock).mockReturnValue(new Promise(() => {}))
    renderModal({ canReadLists: true, defaultListId: 'news' })

    await userEvent.type(screen.getByPlaceholderText('recipient@example.com'), 'me@example.com')
    const buttons = screen.getAllByRole('button', { name: /Send test email/i })
    expect(buttons[buttons.length - 1]).toBeDisabled()
  })

  it('selects nothing when the default list is not one of the workspace lists', async () => {
    renderModal({ canReadLists: true, defaultListId: 'deleted' })

    await waitFor(() => expect(listsApi.list).toHaveBeenCalled())
    expect(await screen.findByText('None (no unsubscribe link)')).toBeInTheDocument()
    expect(
      screen.getByText(/Without a list, the test email has no working unsubscribe link/i)
    ).toBeInTheDocument()

    const call = await sendTo('me@example.com')
    expect(call[7]).toBeUndefined()
  })

  it('sends no list once the preselected list is cleared', async () => {
    const { container } = renderModal({ canReadLists: true, defaultListId: 'news' })
    expect(await screen.findByText('Monthly news (broadcast audience)')).toBeInTheDocument()

    const clear = container.ownerDocument.querySelector('.ant-select-clear')
    expect(clear).not.toBeNull()
    fireEvent.mouseDown(clear as Element)
    fireEvent.click(clear as Element)
    expect(await screen.findByText('None (no unsubscribe link)')).toBeInTheDocument()

    const call = await sendTo('me@example.com')
    expect(call[7]).toBeUndefined()
  })

  it.each([
    ['false', false],
    ['absent', undefined]
  ])('hides the field and loads no lists when canReadLists is %s', async (_label, canReadLists) => {
    renderModal({ canReadLists, defaultListId: 'news' })

    expect(screen.queryByText('Mailing list')).not.toBeInTheDocument()
    expect(listsApi.list).not.toHaveBeenCalled()

    const call = await sendTo('me@example.com')
    expect(call[7]).toBeUndefined()
  })
})

describe('SendTemplateModal sender', () => {
  const senders = [
    { id: 's-first', email: 'first@example.com', name: 'First', is_default: false },
    { id: 's-default', email: 'default@example.com', name: 'Default', is_default: true },
    { id: 's-template', email: 'template@example.com', name: 'Template', is_default: false },
    { id: 's-fr', email: 'fr@example.com', name: 'French', is_default: false }
  ]
  const withSenders = {
    ...workspace,
    settings: { default_language: 'en', marketing_email_provider_id: 'int2' },
    integrations: [
      workspace.integrations![0],
      { id: 'int2', name: 'Postmark', type: 'email', email_provider: { kind: 'postmark', senders } }
    ]
  } as unknown as Workspace

  const open = (tpl: Template) => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <AntApp>
          <SendTemplateModal isOpen onClose={() => {}} template={tpl} workspace={withSenders} canReadContacts={false} />
        </AntApp>
      </QueryClientProvider>
    )
  }
  const sentSender = async () => (await sendTo('a@b.co'))[3]

  beforeEach(() => {
    vi.clearAllMocks()
    ;(transactionalNotificationsApi.testTemplate as Mock).mockResolvedValue({ success: true })
  })

  it("preselects the template's sender in the default integration, not the first integration's", async () => {
    open({ ...template, email: { sender_id: 's-template' } } as unknown as Template)
    expect(await sentSender()).toBe('s-template')
  })

  it("preselects the integration's default sender when the template's is not one of its senders", async () => {
    open({ ...template, email: { sender_id: 'elsewhere' } } as unknown as Template)
    expect(await sentSender()).toBe('s-default')
  })

  it("preselects the open translation's sender", async () => {
    open({
      ...template,
      email: { sender_id: 's-template' },
      translations: { fr: { email: { sender_id: 's-fr' } } }
    } as unknown as Template)
    const field = (await screen.findByText('Language')).closest('.ant-form-item')
    fireEvent.mouseDown(field!.querySelector('.ant-select')!)
    const option = await waitFor(() => {
      const found = Array.from(document.querySelectorAll('.ant-select-item-option')).find((el) =>
        /fr|French|Français/i.test(el.textContent ?? '')
      )
      expect(found).toBeDefined()
      return found as Element
    })
    fireEvent.click(option)
    expect(await sentSender()).toBe('s-fr')
  })
})

describe('SendTemplateModal cc and bcc', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(transactionalNotificationsApi.testTemplate as Mock).mockResolvedValue({ success: true })
  })

  // cc and bcc need contacts:read on the server, so a member without it is not offered them.
  it('hides cc and bcc without canReadContacts', async () => {
    renderModal({ canReadContacts: false })

    // The options are shown without a click
    expect(screen.getByText('From Name (override)')).toBeInTheDocument()
    expect(screen.queryByText('CC Recipients')).not.toBeInTheDocument()
    expect(screen.queryByText('BCC Recipients')).not.toBeInTheDocument()
  })

  it('sends the cc typed with Contacts read, and none once that permission is gone', async () => {
    const { rerenderModal } = renderModal({ canReadContacts: true })
    const ccInput = screen
      .getByText('CC Recipients')
      .closest('.ant-form-item')
      ?.querySelector('input') as HTMLInputElement
    await userEvent.type(ccInput, 'copy@example.com,')
    const bccInput = screen
      .getByText('BCC Recipients')
      .closest('.ant-form-item')
      ?.querySelector('input') as HTMLInputElement
    await userEvent.type(bccInput, 'hidden@example.com,')
    await waitFor(() => expect(screen.getAllByText('copy@example.com').length).toBeGreaterThan(0))

    rerenderModal({ canReadContacts: false })
    expect(screen.queryByText('CC Recipients')).not.toBeInTheDocument()

    const call = await sendTo('me@example.com')
    expect(call[6]).toMatchObject({ cc: [], bcc: [] })
  })

  it('shows cc and bcc when canReadContacts is true', async () => {
    renderModal({ canReadContacts: true })

    expect(screen.getByText('CC Recipients')).toBeInTheDocument()
    expect(screen.getByText('BCC Recipients')).toBeInTheDocument()
  })
})

describe('SendTemplateModal panel', () => {
  it('opens as a side panel with every option shown, and no link to reveal them', () => {
    renderModal({ canReadContacts: true })
    expect(document.querySelector('.ant-drawer')).not.toBeNull()
    expect(document.querySelector('.ant-modal')).toBeNull()
    for (const label of ['From Name (override)', 'CC Recipients', 'BCC Recipients', 'Reply-To', 'Attachments']) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
    expect(screen.queryByText(/add from name/i)).not.toBeInTheDocument()
    expect(screen.getByText('Options')).toBeInTheDocument()
  })
})
