import { describe, expect, it, vi, beforeEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { MessageHistory } from '../../services/api/messages_history'

const { listMessages, useQuerySpy } = vi.hoisted(() => ({ listMessages: vi.fn(), useQuerySpy: vi.fn() }))
vi.mock('../../services/api/messages_history', () => ({ listMessages }))
vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQuery: (options: Parameters<typeof actual.useQuery>[0]) => {
      useQuerySpy(options)
      return actual.useQuery(options)
    }
  }
})
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    search,
    children,
    ...rest
  }: {
    search?: Record<string, string>
    children: ReactNode
    'aria-label'?: string
  }) => (
    <a href={`/logs?${new URLSearchParams(search).toString()}`} aria-label={rest['aria-label']}>
      {children}
    </a>
  )
}))

import { RecentSends } from './RecentSends'

const now = new Date('2026-11-12T09:00:00Z')

const message = (id: string, extra: Partial<MessageHistory> = {}): MessageHistory =>
  ({
    id,
    contact_email: `${id}@example.com`,
    template_id: 't',
    template_version: 1,
    channel: 'email',
    message_data: { data: {} },
    sent_at: '2026-11-12T08:29:03Z',
    created_at: '2026-11-12T08:29:03Z',
    updated_at: '2026-11-12T08:29:03Z',
    ...extra
  }) as MessageHistory

let client: QueryClient
const renderPanel = (canReadMessages = true) =>
  render(
    <QueryClientProvider client={(client = new QueryClient({ defaultOptions: { queries: { retry: false } } }))}>
      <I18nProvider i18n={i18n}>
        <RecentSends
          workspaceId="ws1"
          notificationId="password_reset"
          timezone="Europe/Paris"
          canReadMessages={canReadMessages}
          now={now}
        />
      </I18nProvider>
    </QueryClientProvider>
  )

beforeEach(() => {
  listMessages.mockReset()
  useQuerySpy.mockReset()
})

describe('RecentSends', () => {
  it('lists the latest sends with their time, recipient, status and external ID', async () => {
    listMessages.mockResolvedValue({
      messages: [
        message('m1', {
          delivered_at: '2026-11-12T08:29:05Z',
          opened_at: '2026-11-12T08:40:00Z',
          external_id: 'order-42'
        }),
        message('m2', {
          failed_at: '2026-11-12T08:29:04Z',
          status_info: 'Quota exceeded',
          sent_at: '2026-11-10T08:00:00Z'
        })
      ],
      has_more: false
    })
    renderPanel()

    expect(await screen.findByText('m1@example.com')).toBeInTheDocument()
    expect(listMessages).toHaveBeenCalledWith('ws1', { transactional_notification_id: 'password_reset', limit: 10 })
    expect(screen.getByText('09:29:03')).toBeInTheDocument()
    expect(screen.getByText('Nov 10, 09:00')).toBeInTheDocument()
    expect(screen.getByText('Opened')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('order-42')).toBeInTheDocument()
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('links each send to its message in the logs, and the panel to all of them', async () => {
    listMessages.mockResolvedValue({ messages: [message('m1')], has_more: false })
    renderPanel()

    await screen.findByText('m1@example.com')
    expect(screen.getByRole('link', { name: 'Open in logs' })).toHaveAttribute(
      'href',
      '/logs?tab=messages&id=m1&message=m1'
    )
    expect(screen.getByText('View in logs').closest('a')).toHaveAttribute(
      'href',
      '/logs?tab=messages&transactional_notification_id=password_reset'
    )
  })

  it('reads the list again every 30 seconds', async () => {
    listMessages.mockResolvedValue({ messages: [], has_more: false })
    renderPanel()

    expect(await screen.findByText('No sends yet')).toBeInTheDocument()
    expect(useQuerySpy.mock.calls.at(-1)?.[0]).toMatchObject({ refetchInterval: 30_000 })
  })

  it('keeps the rows on screen when a refresh fails', async () => {
    listMessages.mockResolvedValueOnce({ messages: [message('ada')], has_more: false })
    listMessages.mockRejectedValueOnce(new Error('boom'))
    renderPanel()
    expect(await screen.findByText('ada@example.com')).toBeInTheDocument()

    await act(() => client.refetchQueries())
    // The query tells React of the failure on a later tick.
    await act(() => new Promise((resolve) => setTimeout(resolve, 50)))
    expect(listMessages).toHaveBeenCalledTimes(2)
    expect(client.getQueryState(['transactional-recent', 'ws1', 'password_reset'])?.status).toBe('error')
    expect(screen.getByText('ada@example.com')).toBeInTheDocument()
    expect(screen.queryByText("Couldn't load")).not.toBeInTheDocument()
  })

  it('says why sends are hidden without access to message history, and asks nothing', () => {
    renderPanel(false)
    expect(screen.getByText('You need access to message history to see sends')).toBeInTheDocument()
    expect(listMessages).not.toHaveBeenCalled()
  })
})
