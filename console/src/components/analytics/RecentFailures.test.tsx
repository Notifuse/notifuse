import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { MessageHistory, MessageListParams } from '../../services/api/messages_history'

const { listMessages, navigate } = vi.hoisted(() => ({
  listMessages: vi.fn(),
  navigate: vi.fn()
}))
vi.mock('../../services/api/messages_history', () => ({ listMessages }))
vi.mock('./dashboardClient', () => ({
  dashboardAnalytics: { query: vi.fn() },
  DASHBOARD_REFRESH_MS: 60_000
}))
const { linkSearch } = vi.hoisted(() => ({ linkSearch: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  Link: ({ search, children }: { search?: Record<string, unknown>; children: ReactNode }) => {
    linkSearch(search)
    return (
      <a href={`/logs?${new URLSearchParams(search as Record<string, string>).toString()}`}>
        {children}
      </a>
    )
  }
}))

import { RecentFailures } from './RecentFailures'

const message = (id: string, extra: Partial<MessageHistory>): MessageHistory =>
  ({
    id,
    contact_email: `${id}@example.com`,
    template_id: 't',
    template_version: 1,
    channel: 'email',
    message_data: { data: {} },
    sent_at: '2026-11-01T10:00:00Z',
    created_at: '2026-11-01T10:00:00Z',
    updated_at: '2026-11-01T10:00:00Z',
    ...extra
  }) as MessageHistory

const lists = (complained: MessageHistory[], bounced: MessageHistory[], failed: MessageHistory[]) =>
  listMessages.mockImplementation(async (_ws: string, params: MessageListParams) => ({
    messages: params.is_complained ? complained : params.is_bounced ? bounced : failed
  }))

const renderCard = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <I18nProvider i18n={i18n}>
        <RecentFailures workspaceId="ws1" timezone="Europe/Paris" />
      </I18nProvider>
    </QueryClientProvider>
  )

beforeEach(() => {
  listMessages.mockReset()
  navigate.mockReset()
})

describe('RecentFailures', () => {
  it('asks the five latest complaints, bounces and failures', async () => {
    lists([], [], [])
    renderCard()
    await screen.findByText('No failure')

    const params = listMessages.mock.calls.map(([ws, p]) => [ws, p])
    expect(params).toHaveLength(3)
    expect(params).toContainEqual(['ws1', { is_complained: true, limit: 5 }])
    expect(params).toContainEqual(['ws1', { is_bounced: true, limit: 5 }])
    expect(params).toContainEqual(['ws1', { is_failed: true, limit: 5 }])
  })

  it('lists each failure once with its reason and kind', async () => {
    lists(
      [
        message('spam', {
          complained_at: '2026-11-03T09:00:00Z',
          bounced_at: '2026-11-02T09:00:00Z'
        })
      ],
      [
        message('spam', {
          complained_at: '2026-11-03T09:00:00Z',
          bounced_at: '2026-11-02T09:00:00Z'
        }),
        message('full', {
          bounced_at: '2026-11-04T09:00:00Z',
          status_info: 'Mailbox full'
        })
      ],
      [message('down', { failed_at: '2026-11-01T09:00:00Z' })]
    )
    renderCard()

    expect(await screen.findByText('Mailbox full')).toBeInTheDocument()
    expect(screen.getAllByText('spam@example.com')).toHaveLength(1)
    expect(screen.getByText('Marked as spam')).toBeInTheDocument()
    expect(screen.getByText('Complaint')).toBeInTheDocument()
    expect(screen.getByText('Bounce')).toBeInTheDocument()
    // The pill and the reason of a failure without explanation.
    expect(screen.getAllByText('Failed')).toHaveLength(2)

    const order = screen.getAllByText(/@example\.com$/).map((cell) => cell.textContent)
    expect(order).toEqual(['full@example.com', 'spam@example.com', 'down@example.com'])
  })

  it('opens the message in the logs', async () => {
    lists([], [message('full', { bounced_at: '2026-11-04T09:00:00Z' })], [])
    renderCard()

    fireEvent.click(await screen.findByText('full@example.com'))
    expect(navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId: 'ws1' },
      search: { tab: 'messages', id: 'full', message: 'full' }
    })
  })

  it('links to the failed messages of all time in the logs', async () => {
    lists([], [], [])
    renderCard()

    expect((await screen.findByText('View in logs')).closest('a')).toHaveAttribute(
      'href',
      '/logs?tab=messages&is_failed=true&period=all'
    )
    // A boolean: the router would write the string 'true' JSON-quoted, read back as not true.
    // All time: the card lists the latest failures whatever their date, past the tab's 24 hours.
    expect(linkSearch).toHaveBeenLastCalledWith({
      tab: 'messages',
      is_failed: true,
      period: 'all'
    })
  })

  it('offers a retry when a list fails, and asks it again', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    let calls = 0
    listMessages.mockImplementation(async (_ws: string, params: MessageListParams) => {
      if (params.is_bounced && calls++ === 0) throw new Error('boom')
      return { messages: [] }
    })
    renderCard()

    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('No failure')).toBeInTheDocument()
    await waitFor(() => expect(listMessages).toHaveBeenCalledTimes(4))
    expect(warn).toHaveBeenCalled()
    warn.mockRestore()
  })
})
