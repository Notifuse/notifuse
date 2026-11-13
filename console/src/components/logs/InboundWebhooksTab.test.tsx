import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { InboundWebhookEvent } from '../../services/api/inbound_webhook_event'

const state = vi.hoisted(() => ({
  navigate: vi.fn(),
  search: {} as Record<string, unknown>,
  list: vi.fn()
}))

vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => state.navigate,
  useSearch: () => state.search,
  Link: ({
    children,
    search,
    onClick
  }: {
    children: ReactNode
    search: Record<string, string>
    onClick?: (event: { stopPropagation: () => void }) => void
  }) => (
    <a href={`/logs?${new URLSearchParams(search).toString()}`} onClick={onClick}>
      {children}
    </a>
  )
}))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', settings: { timezone: 'UTC' } }] })
}))
vi.mock('../../services/api/inbound_webhook_event', () => ({
  listInboundWebhookEvents: state.list
}))

import { InboundWebhooksTab } from './InboundWebhooksTab'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const event = (overrides: Partial<InboundWebhookEvent> = {}): InboundWebhookEvent => ({
  id: 'e1',
  type: 'delivered',
  source: 'postmark',
  integration_id: 'int_1',
  recipient_email: 'ann@example.com',
  message_id: 'm1',
  timestamp: '2026-11-13T09:00:00Z',
  raw_payload: '{"RecordType":"Delivery"}',
  created_at: '2026-11-13T09:00:01Z',
  ...overrides
})

const readMessages = { message_history: { read: true, write: false } }

const renderTab = (
  search: Record<string, unknown> = { tab: 'incoming-webhooks' },
  permissions: Record<string, { read: boolean; write: boolean }> | null = readMessages
) => {
  state.search = search
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <InboundWebhooksTab workspaceId="ws1" permissions={permissions as never} />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
}

const lastList = () => state.list.mock.calls.at(-1)?.[1] as Record<string, unknown>
const lastNavigation = () =>
  state.navigate.mock.calls.at(-1)?.[0] as {
    search: Record<string, unknown>
  }

describe('InboundWebhooksTab', () => {
  beforeEach(() => {
    state.navigate.mockClear()
    state.list.mockReset()
    state.list.mockResolvedValue({ events: [event()], has_more: false })
  })

  it('lists the last 24 hours by default, 50 at a time', async () => {
    const before = Date.now()
    renderTab()
    await screen.findByText('ann@example.com')
    const params = lastList()
    expect(params.limit).toBe(50)
    const after = new Date(params.timestamp_after as string).getTime()
    expect(before - after).toBeGreaterThanOrEqual(24 * 3600_000 - 60_000)
    expect(before - after).toBeLessThanOrEqual(24 * 3600_000 + 60_000)
  })

  it('sends the source, the type and the recipient of the URL, over all time for a recipient', async () => {
    renderTab({
      tab: 'incoming-webhooks',
      source: 'ses',
      event_type: 'bounce',
      recipient_email: 'ann@example.com'
    })
    await screen.findByText('ann@example.com')
    expect(lastList()).toMatchObject({
      source: 'ses',
      event_type: 'bounce',
      recipient_email: 'ann@example.com'
    })
    expect(lastList().timestamp_after).toBeUndefined()
  })

  it('does not send an email cut short, which the server would refuse', async () => {
    renderTab({ tab: 'incoming-webhooks', recipient_email: 'ann@' })
    await screen.findByText('ann@example.com')
    expect(lastList().recipient_email).toBeUndefined()
  })

  it('writes the source chosen in the URL', async () => {
    renderTab()
    await screen.findByText('ann@example.com')
    await userEvent.click(screen.getByRole('button', { name: /Source All/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Amazon SES' }))
    expect(lastNavigation().search).toMatchObject({ source: 'ses' })
  })

  it('names a bounce with its type, and an erased contact as such', async () => {
    state.list.mockResolvedValue({
      events: [
        event({ type: 'bounce', bounce_type: 'Permanent' }),
        event({ id: 'e2', recipient_email: 'DELETED_EMAIL' })
      ],
      has_more: false
    })
    renderTab()
    expect(await screen.findByText('Bounce · Permanent')).toBeInTheDocument()
    expect(screen.getByText('Contact erased')).toBeInTheDocument()
  })

  it('links a delivery event to its message, which needs message history read', async () => {
    state.list.mockResolvedValue({
      events: [event(), event({ id: 'e2', type: 'reply', message_id: 'reply-own-id' })],
      has_more: false
    })
    const { unmount } = renderTab()
    const link = await screen.findByRole('link', { name: 'm1' })
    expect(link).toHaveAttribute('href', '/logs?tab=messages&id=m1&message=m1')
    // A reply's message ID is the reply's own: no message of the workspace to open.
    expect(screen.getByText('reply-own-id').closest('a')).toBeNull()
    unmount()

    renderTab({ tab: 'incoming-webhooks' }, {})
    await screen.findAllByText('ann@example.com')
    expect(screen.queryByRole('link', { name: 'm1' })).toBeNull()
    expect(screen.getByText('m1')).toBeInTheDocument()
  })

  it('opens the detail with the raw payload, and goes to the next event with J', async () => {
    state.list.mockResolvedValue({
      events: [
        event({ type: 'bounce', bounce_type: 'Permanent', bounce_diagnostic: '550 no such user' }),
        event({ id: 'e2', recipient_email: 'bob@example.com' })
      ],
      has_more: false
    })
    renderTab()
    fireEvent.click(await screen.findByText('Bounce · Permanent'))
    const panel = await screen.findByRole('dialog')
    expect(within(panel).getByText('int_1')).toBeInTheDocument()
    expect(within(panel).getByText('550 no such user')).toBeInTheDocument()
    expect(within(panel).getByText('Raw payload')).toBeInTheDocument()
    expect(within(panel).getByText(/RecordType/)).toBeInTheDocument()
    expect(within(panel).queryByRole('button', { name: 'Copy link' })).not.toBeInTheDocument()

    fireEvent.keyDown(document, { key: 'j' })
    await waitFor(() => expect(within(panel).getByText('e2')).toBeInTheDocument())
  })

  it('counts the events shown and loads more', async () => {
    state.list.mockResolvedValueOnce({ events: [event()], has_more: true, next_cursor: 'c2' })
    state.list.mockResolvedValueOnce({
      events: [event({ id: 'e2', recipient_email: 'bob@example.com' })],
      has_more: false
    })
    renderTab()
    expect(await screen.findByText('1 event')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Load more' }))
    expect(await screen.findByText('2 events')).toBeInTheDocument()
    expect(lastList()).toMatchObject({ cursor: 'c2' })
    expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull()
  })

  it('says why the list is empty, with a way forward', async () => {
    state.list.mockResolvedValue({ events: [], has_more: false })
    const { unmount } = renderTab()
    expect(await screen.findByText('Nothing in the last 24 hours')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show all time' }))
    expect(lastNavigation().search).toMatchObject({ period: 'all' })
    unmount()

    const second = renderTab({ tab: 'incoming-webhooks', period: 'all' })
    expect(await screen.findByText('No webhook events received yet')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Set up an email provider' }))
    expect(state.navigate).toHaveBeenLastCalledWith({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId: 'ws1', section: 'integrations' }
    })
    second.unmount()

    renderTab({ tab: 'incoming-webhooks', period: '7d', source: 'ses' })
    expect(await screen.findByText('No results match')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(lastNavigation().search).toEqual({ tab: 'incoming-webhooks', period: '7d' })
  })

  it('shows the error of the server, with a retry', async () => {
    state.list.mockRejectedValueOnce(new Error('database is down'))
    renderTab()
    expect(await screen.findByText('database is down')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('ann@example.com')).toBeInTheDocument()
  })
})
