import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App, ConfigProvider } from 'antd'
import { MessageHistoryTab } from './MessageHistoryTab'

const STORAGE_KEY = 'message_columns_visibility'

// vi.mock factories are hoisted above every top-level binding, so the fixture the
// listMessages mock resolves to has to be hoisted with them.
const state = vi.hoisted(() => ({
  sentMessage: {
    id: 'msg-1',
    contact_email: 'bob@example.com',
    template_id: 'tpl-1',
    template_version: 1,
    channel: 'email',
    message_data: { data: {}, metadata: { order_id: '9912' } },
    sent_at: '2026-08-01T10:00:00Z',
    created_at: '2026-08-01T10:00:00Z',
    updated_at: '2026-08-01T10:00:00Z'
  }
}))

vi.mock('../../services/api/messages_history', async () => {
  const actual = await vi.importActual<typeof import('../../services/api/messages_history')>(
    '../../services/api/messages_history'
  )
  return {
    ...actual,
    listMessages: vi.fn().mockResolvedValue({ messages: [state.sentMessage], has_more: false })
  }
})

vi.mock('../../services/api/broadcast', () => ({
  broadcastApi: { list: vi.fn().mockResolvedValue({ broadcasts: [] }) }
}))

vi.mock('../../services/api/list', () => ({
  listsApi: { list: vi.fn().mockResolvedValue({ lists: [] }) }
}))

vi.mock('../../services/api/template', () => ({
  templatesApi: { get: vi.fn().mockResolvedValue({ template: null }) }
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [
      { id: 'ws-1', name: 'Test Workspace', settings: { timezone: 'UTC' } }
    ]
  })
}))

const renderTab = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ConfigProvider>
        <App>
          <MessageHistoryTab workspaceId="ws-1" />
        </App>
      </ConfigProvider>
    </QueryClientProvider>
  )
}

describe('MessageHistoryTab metadata column default', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('shows the metadata column on a first visit', async () => {
    // The column is deliberately absent from DEFAULT_VISIBLE_COLUMNS, so an unset key
    // reads as visible — the same way the contact drawer and the failed-messages card
    // show every column by passing no map at all.
    renderTab()

    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())
    expect(screen.getByText('Metadata')).toBeInTheDocument()
    expect(screen.getByText(/order_id/)).toBeInTheDocument()
  })

  it('shows the column for a user whose stored map predates it', async () => {
    // Stored maps written before this column existed carry no `metadata` key, and the
    // merge with DEFAULT_VISIBLE_COLUMNS does not add one.
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ id: true, external_id: false, contact_email: true, created_at: true })
    )
    renderTab()

    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())
    expect(screen.getByText('Metadata')).toBeInTheDocument()
  })

  it('hides the column once the user turns it off', async () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ metadata: false }))
    renderTab()

    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())
    expect(screen.queryByText('Metadata')).not.toBeInTheDocument()
    expect(screen.queryByText(/order_id/)).not.toBeInTheDocument()
  })
})

// The Error column is where a send failure explains itself, so it is on by default.
// It is keyed on `status_info` — the field it renders — rather than on `error`, which
// is the name of the column that never worked: every stored map written before this
// carries `error: false`, and that value records the old default, not a choice.
describe('MessageHistoryTab error column default', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('shows the error column on a first visit', async () => {
    renderTab()

    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())
    expect(screen.getByText('Error')).toBeInTheDocument()
  })

  it('ignores the stale preference left by the column that never worked', async () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ id: true, error: false }))
    renderTab()

    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())
    expect(screen.getByText('Error')).toBeInTheDocument()
  })

  it('hides the column once the user turns it off', async () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ status_info: false }))
    renderTab()

    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())
    expect(screen.queryByText('Error')).not.toBeInTheDocument()
  })
})

// The broadcast link stats open the log on the recipients who clicked one link. The
// server takes that filter only within a broadcast, so it lives and dies with it.
describe('MessageHistoryTab clicked link filter', () => {
  const pricing = 'https://shop.example.com/pricing?utm_source=news'

  beforeEach(async () => {
    localStorage.clear()
    const { listMessages } = await import('../../services/api/messages_history')
    vi.mocked(listMessages).mockClear()
  })

  const lastListParams = async () => {
    const { listMessages } = await import('../../services/api/messages_history')
    const calls = vi.mocked(listMessages).mock.calls
    return calls[calls.length - 1]?.[1]
  }

  it('sends the clicked link from the URL along with its broadcast', async () => {
    window.history.replaceState({}, '', `/logs?broadcast_id=bc-1&clicked_url=${encodeURIComponent(pricing)}`)
    renderTab()

    await waitFor(async () =>
      expect(await lastListParams()).toMatchObject({ broadcast_id: 'bc-1', clicked_url: pricing })
    )
    expect(screen.getByText(`Clicked link: ${pricing}`)).toBeInTheDocument()
  })

  it('offers no clicked link filter outside a broadcast', async () => {
    window.history.replaceState({}, '', '/logs')
    renderTab()

    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())
    expect(screen.getByText('Broadcast ID')).toBeInTheDocument()
    expect(screen.queryByText('Clicked link')).not.toBeInTheDocument()
  })

  it('drops the clicked link when the broadcast filter is cleared', async () => {
    window.history.replaceState({}, '', `/logs?broadcast_id=bc-1&clicked_url=${encodeURIComponent(pricing)}`)
    renderTab()

    await waitFor(() => expect(screen.getByText('Broadcast ID: bc-1')).toBeInTheDocument())
    fireEvent.click(screen.getByText('Broadcast ID: bc-1'))
    fireEvent.click(await screen.findByRole('button', { name: 'Clear' }))

    await waitFor(async () => {
      const params = await lastListParams()
      expect(params).not.toHaveProperty('broadcast_id')
      expect(params).not.toHaveProperty('clicked_url')
    })
    expect(screen.queryByText(`Clicked link: ${pricing}`)).not.toBeInTheDocument()
  })

  it('filters on the transactional notification from the URL, and lets it be removed', async () => {
    window.history.replaceState({}, '', '/logs?transactional_notification_id=password_reset')
    renderTab()

    await waitFor(async () =>
      expect(await lastListParams()).toMatchObject({ transactional_notification_id: 'password_reset' })
    )
    fireEvent.click(screen.getByText('Transactional notification ID: password_reset'))
    fireEvent.click(await screen.findByRole('button', { name: 'Clear' }))

    await waitFor(async () => expect(await lastListParams()).not.toHaveProperty('transactional_notification_id'))
  })
})
