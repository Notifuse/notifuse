import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { WebhookDelivery } from '../../services/api/webhook_subscription'

const state = vi.hoisted(() => ({
  navigate: vi.fn(),
  search: {} as Record<string, unknown>,
  list: vi.fn(),
  getDeliveries: vi.fn()
}))

vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() }
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => state.navigate,
  useSearch: () => state.search
}))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', settings: { timezone: 'UTC' } }] })
}))
vi.mock('../../services/api/webhook_subscription', () => ({
  webhookSubscriptionApi: { list: state.list, getDeliveries: state.getDeliveries }
}))

import { OutgoingWebhooksTab } from './OutgoingWebhooksTab'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const delivery = (overrides: Partial<WebhookDelivery> = {}): WebhookDelivery => ({
  id: 'd1',
  subscription_id: 'sub1',
  event_type: 'contact.created',
  payload: { contact: { email: 'ann@example.com' } },
  status: 'delivered',
  attempts: 1,
  max_attempts: 10,
  next_attempt_at: '2026-11-12T10:05:00Z',
  last_attempt_at: '2026-11-12T10:00:01Z',
  delivered_at: '2026-11-12T10:00:01Z',
  last_response_status: 200,
  last_response_body: '{"ok":true}',
  created_at: '2026-11-12T10:00:00Z',
  ...overrides
})

const page = (deliveries: WebhookDelivery[], total = deliveries.length, offset = 0) => ({
  deliveries,
  total,
  limit: 50,
  offset
})

const renderTab = (search: Record<string, unknown> = { tab: 'outgoing-webhooks' }) => {
  state.search = search
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <OutgoingWebhooksTab workspaceId="ws1" />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
}

const rows = () => document.querySelectorAll('.ant-table-tbody tr.ant-table-row')

describe('OutgoingWebhooksTab', () => {
  beforeEach(() => {
    state.navigate.mockClear()
    state.list.mockResolvedValue({
      subscriptions: [{ id: 'sub1', name: 'CRM sync' }]
    })
    state.getDeliveries.mockReset()
    state.getDeliveries.mockResolvedValue(page([delivery()]))
  })

  it('sends the subscription and status of the URL', async () => {
    renderTab({ tab: 'outgoing-webhooks', subscription_id: 'sub1', status: 'failed' })
    await waitFor(() =>
      expect(state.getDeliveries).toHaveBeenCalledWith('ws1', 'sub1', 50, 0, 'failed')
    )
  })

  it('writes a filter through the router with its tab, as a new history entry', async () => {
    renderTab()
    await screen.findByText('contact.created')
    await userEvent.click(screen.getByRole('button', { name: /Status All/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Failed' }))
    expect(state.navigate).toHaveBeenCalledWith(
      expect.objectContaining({ search: { tab: 'outgoing-webhooks', status: 'failed' } })
    )
    expect(state.navigate.mock.calls[0][0]).not.toHaveProperty('replace')
  })

  it('tells a delivery that will be retried from one that gave up', async () => {
    state.getDeliveries.mockResolvedValue(
      page([
        delivery({ id: 'd1', status: 'failed', attempts: 3, max_attempts: 10 }),
        delivery({ id: 'd2', status: 'failed', attempts: 10, max_attempts: 10 })
      ])
    )
    renderTab()
    await waitFor(() => expect(rows()).toHaveLength(2))
    expect(within(rows()[0] as HTMLElement).getByText('Retrying')).toBeInTheDocument()
    expect(within(rows()[0] as HTMLElement).getByText('3/10')).toBeInTheDocument()
    expect(within(rows()[1] as HTMLElement).getByText('Failed')).toBeInTheDocument()
  })

  it('shows the response code and the error', async () => {
    state.getDeliveries.mockResolvedValue(
      page([
        delivery({
          status: 'failed',
          last_response_status: 503,
          last_error: 'upstream unavailable'
        })
      ])
    )
    renderTab()
    expect(await screen.findByText('503')).toBeInTheDocument()
    expect(screen.getByText('upstream unavailable')).toBeInTheDocument()
  })

  it('names the subscription, or shows its id once it is gone', async () => {
    state.getDeliveries.mockResolvedValue(
      page([delivery({ id: 'd1' }), delivery({ id: 'd2', subscription_id: 'sub-gone' })])
    )
    renderTab()
    await waitFor(() => expect(rows()).toHaveLength(2))
    expect(within(rows()[0] as HTMLElement).getByText('CRM sync')).toBeInTheDocument()
    expect(within(rows()[1] as HTMLElement).getByText('sub-gone')).toBeInTheDocument()
  })

  it('loads more by offset without showing a delivery twice', async () => {
    state.getDeliveries
      .mockResolvedValueOnce(page([delivery({ id: 'd1' }), delivery({ id: 'd2' })], 3))
      // A delivery created in between shifted the offset: d2 comes back.
      .mockResolvedValueOnce(page([delivery({ id: 'd2' }), delivery({ id: 'd3' })], 3, 2))
    renderTab()
    await waitFor(() => expect(rows()).toHaveLength(2))
    fireEvent.click(screen.getByRole('button', { name: 'Load more' }))
    await waitFor(() => expect(rows()).toHaveLength(3))
    expect(state.getDeliveries).toHaveBeenLastCalledWith('ws1', undefined, 50, 2, undefined)
  })

  it('opens a delivery in a side panel', async () => {
    state.getDeliveries.mockResolvedValue(
      page([
        delivery({
          id: 'd9',
          status: 'failed',
          attempts: 2,
          last_response_status: 500,
          last_response_body: 'boom',
          last_error: 'HTTP 500'
        })
      ])
    )
    renderTab()
    fireEvent.click(await screen.findByText('contact.created'))
    const panel = await screen.findByRole('dialog')
    expect(within(panel).getByText('contact.created · CRM sync')).toBeInTheDocument()
    expect(within(panel).getByText('d9')).toBeInTheDocument()
    expect(within(panel).getByText('Next attempt')).toBeInTheDocument()
    expect(within(panel).getByRole('alert')).toHaveTextContent('HTTP 500')
    expect(within(panel).getByText('boom')).toBeInTheDocument()
    expect(within(panel).getByText(/ann@example.com/)).toBeInTheDocument()
    // Its detail has no URL of its own: a copied link would not reopen it.
    expect(within(panel).queryByRole('button', { name: 'Copy link' })).not.toBeInTheDocument()

    fireEvent.click(within(panel).getByRole('button', { name: 'Open subscription' }))
    expect(state.navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId: 'ws1', section: 'webhooks' },
      search: { webhook: 'sub1' }
    })
  })

  it('has no next attempt for a delivered one, and no subscription to open once it is gone', async () => {
    state.getDeliveries.mockResolvedValue(page([delivery({ subscription_id: 'sub-gone' })]))
    renderTab()
    fireEvent.click(await screen.findByText('contact.created'))
    const panel = await screen.findByRole('dialog')
    expect(within(panel).queryByText('Next attempt')).not.toBeInTheDocument()
    expect(within(panel).getByRole('button', { name: 'Open subscription' })).toBeDisabled()
  })

  it('invites to add a webhook when nothing was ever delivered', async () => {
    state.getDeliveries.mockResolvedValue(page([]))
    renderTab()
    expect(await screen.findByText('No webhook deliveries yet')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Add a webhook' }))
    expect(state.navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId: 'ws1', section: 'webhooks' }
    })
  })

  it('offers to clear the filters when they match nothing', async () => {
    state.getDeliveries.mockResolvedValue(page([]))
    renderTab({ tab: 'outgoing-webhooks', status: 'pending' })
    expect(await screen.findByText('No results match')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(state.navigate).toHaveBeenCalledWith(
      expect.objectContaining({ search: { tab: 'outgoing-webhooks' } })
    )
  })

  it('shows the error with a retry', async () => {
    state.getDeliveries.mockRejectedValueOnce(new Error('status must be one of …'))
    renderTab()
    expect(await screen.findByText('status must be one of …')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('contact.created')).toBeInTheDocument()
  })
})
