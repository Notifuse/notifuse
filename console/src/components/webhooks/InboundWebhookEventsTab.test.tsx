import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { InboundWebhookEventsTab } from './InboundWebhookEventsTab'
import { listInboundWebhookEvents } from '../../services/api/inbound_webhook_event'
import type { InboundWebhookEvent } from '../../services/api/inbound_webhook_event'

// The api client pulls in the router and every page; stubbing the service keeps that graph
// out of this suite.
vi.mock('../../services/api/inbound_webhook_event', () => ({
  listInboundWebhookEvents: vi.fn()
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Test', settings: { timezone: 'UTC' } }] })
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

// The tab writes its filters to the address, and reads them back each time it rebuilds its
// filter options. Those are memoised on `t`, which the shared Lingui mock creates anew on
// every render, where the real one only changes with the locale: with a filter in the
// address, the tab would re-apply it forever. Keep the address out of it.
vi.spyOn(window.history, 'pushState').mockImplementation(() => undefined)

// Mailjet reports an unsubscribe as an event of its own, which the webhook records as an
// unsubscribe rather than a complaint.
const unsubscribeEvent: InboundWebhookEvent = {
  id: 'evt-1',
  type: 'unsubscribe',
  source: 'mailjet',
  integration_id: 'int-1',
  recipient_email: 'reader@example.com',
  message_id: 'msg-1',
  timestamp: '2026-10-01T00:00:00Z',
  raw_payload: '{"event":"unsub"}',
  created_at: '2026-10-01T00:00:00Z'
}

const renderTab = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <QueryClientProvider client={client}>
            <InboundWebhookEventsTab workspaceId="ws1" />
          </QueryClientProvider>
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
}

describe('InboundWebhookEventsTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listInboundWebhookEvents).mockResolvedValue({
      events: [unsubscribeEvent],
      has_more: false
    })
  })

  // Without an entry of its own the row fell through to the raw type: an untranslated,
  // uncoloured "unsubscribe" beside the labelled delivered, bounce and complaint rows.
  it('labels an unsubscribe event, with the unsubscribe icon', async () => {
    renderTab()

    const tag = (await screen.findByText('Unsubscribed')).closest('.ant-tag')
    expect(tag).toHaveClass('ant-tag-red')
    // The ban icon the message history shows for an unsubscribe
    expect(tag?.querySelector('svg[data-icon="ban"]')).not.toBeNull()
    expect(screen.queryByText('unsubscribe')).not.toBeInTheDocument()
  })

  // The option list is built from the labelled types, so a type without an entry could
  // never be isolated, though the API filters on it.
  it('offers unsubscribe events as an event type filter', async () => {
    renderTab()
    await screen.findByText('Unsubscribed')

    fireEvent.click(screen.getByRole('button', { name: 'Event Type' }))
    fireEvent.mouseDown(await screen.findByRole('combobox'))
    fireEvent.click(await screen.findByTitle('Unsubscribed'))
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    await waitFor(() => {
      expect(listInboundWebhookEvents).toHaveBeenLastCalledWith(
        'ws1',
        expect.objectContaining({ event_type: 'unsubscribe' })
      )
    })
  })
})
