import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Workspace } from '../../../services/api/types'

vi.mock('../../../services/api/analytics', () => ({ analyticsService: { query: vi.fn() } }))
vi.mock('../../../services/api/web_analytics', () => ({ webAnalyticsService: {} }))
vi.mock('../../../services/api/contact_timeline', () => ({ contactTimelineApi: { list: vi.fn() } }))
vi.mock('../../../services/api/messages_history', () => ({
  listMessages: vi.fn(async () => ({ messages: [{ id: 'm1' }], next_cursor: 'next', has_more: true }))
}))
vi.mock('../../messages/MessageHistoryTable', () => ({
  MessageHistoryTable: ({ messages, nextCursor }: { messages: unknown[]; nextCursor?: string }) => (
    <div>
      {messages.length} messages, more: {String(!!nextCursor)}
    </div>
  )
}))

import { listMessages } from '../../../services/api/messages_history'
import { ContactMessagesTab } from './ContactMessagesTab'

describe('ContactMessagesTab', () => {
  it('lists the contact messages page by page', async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ContactMessagesTab workspace={{ id: 'ws' } as Workspace} email="ann@x.com" />
      </QueryClientProvider>
    )
    expect(await screen.findByText('1 messages, more: true')).toBeInTheDocument()
    await waitFor(() =>
      expect(listMessages).toHaveBeenCalledWith('ws', {
        contact_email: 'ann@x.com',
        limit: 10,
        cursor: undefined
      })
    )
  })
})
