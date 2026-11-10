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
}

function renderModal(props: ModalProps) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AntApp>
        <SendTemplateModal
          isOpen
          onClose={() => {}}
          template={template}
          workspace={workspace}
          {...props}
        />
      </AntApp>
    </QueryClientProvider>
  )
}

async function sendTo(email: string) {
  await userEvent.type(screen.getByPlaceholderText('recipient@example.com'), email)
  const buttons = screen.getAllByRole('button', { name: /Send Test Email/i })
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
      screen.getByText(/Unsubscribing or confirming from this test really changes/i)
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
