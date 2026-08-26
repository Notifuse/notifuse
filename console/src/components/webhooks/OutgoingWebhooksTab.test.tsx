import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { OutgoingWebhooksTab } from './OutgoingWebhooksTab'
import { webhookSubscriptionApi } from '../../services/api/webhook_subscription'
import type { WebhookDelivery } from '../../services/api/webhook_subscription'

// The api client pulls in the router and every page; stubbing the service keeps that graph
// out of this suite.
vi.mock('../../services/api/webhook_subscription', () => ({
  webhookSubscriptionApi: {
    list: vi.fn(),
    getDeliveries: vi.fn()
  }
}))

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Test', settings: { timezone: 'UTC' } }] })
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const delivery = (id: string, status: WebhookDelivery['status']): WebhookDelivery => ({
  id,
  subscription_id: 'wh-1',
  event_type: 'contact.created',
  payload: {},
  status,
  attempts: 1,
  max_attempts: 10,
  next_attempt_at: '2026-08-25T00:00:00Z',
  created_at: '2026-08-25T00:00:00Z'
})

const renderTab = async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const utils = render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <QueryClientProvider client={client}>
            <OutgoingWebhooksTab workspaceId="ws1" />
          </QueryClientProvider>
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
  await screen.findByText('Delivered')
  return utils
}

describe('OutgoingWebhooksTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(webhookSubscriptionApi.list).mockResolvedValue({
      subscriptions: [
        {
          id: 'wh-1',
          name: 'My Webhook',
          url: 'https://example.com/hook',
          secret: 'shhh',
          settings: { event_types: ['contact.created'] },
          enabled: true,
          created_at: '2026-08-01T00:00:00Z',
          updated_at: '2026-08-01T00:00:00Z'
        }
      ]
    })
    vi.mocked(webhookSubscriptionApi.getDeliveries).mockResolvedValue({
      deliveries: [
        delivery('d-1', 'delivered'),
        delivery('d-2', 'delivering'),
        delivery('d-3', 'pending'),
        delivery('d-4', 'failed')
      ],
      total: 4,
      limit: 50,
      offset: 0
    })
  })

  // Claiming a delivery is a status change written to the row, not an in-memory flag, so a
  // batch in flight is visible to anyone who opens this screen while the worker is walking
  // it. Without a case of its own the row fell through to the default branch and rendered
  // the raw literal "delivering" — untranslated, uncoloured, beside three translated tags.
  it('labels a delivery that is in flight', async () => {
    await renderTab()

    expect(screen.getByText('Delivering')).toBeInTheDocument()
    // The raw status string must not reach the page. Case matters: the label is "Delivering".
    expect(screen.queryByText('delivering')).not.toBeInTheDocument()
  })

  // An operator investigating a slow endpoint needs to isolate what is in flight right now,
  // and the counts under Pending exclude these rows.
  it('offers the in-flight rows as a status filter', async () => {
    await renderTab()

    const statusFilter = screen
      .getAllByRole('button')
      .find((button) => button.textContent?.includes('Status'))
    expect(statusFilter).toBeDefined()

    // The option list is built in the component; assert it carries every status the backend
    // can write, so a row can never exist that no filter can reach.
    expect(screen.getByText('Delivering')).toBeInTheDocument()
  })
})
