import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BroadcastRecipients, type BroadcastRecipientsProps } from './BroadcastRecipients'
import { listMessages, type MessageHistory, type MessageHistoryStatusSum } from '../../../services/api/messages_history'
import type { Broadcast } from '../../../services/api/broadcast'

vi.mock('../../../services/api/messages_history', () => ({ listMessages: vi.fn() }))
vi.mock('../../logs/MessageDetail', () => ({
  MessageDetail: ({ id, copyLink }: { id: string; copyLink?: boolean }) => (
    <div data-testid="message-detail" data-id={id} data-copy-link={String(copyLink)} />
  )
}))
const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigate }))

const SENT = '2026-11-24T14:27:00Z'

const message = (index: number, over: Partial<MessageHistory> = {}): MessageHistory =>
  ({
    id: `m${index}`,
    contact_email: `r${index}@example.com`,
    broadcast_id: 'bc1',
    template_id: 'tpl-a',
    template_version: 1,
    channel: 'email',
    message_data: { data: {} },
    sent_at: SENT,
    delivered_at: SENT,
    opened_at: '2026-11-24T14:39:00Z',
    created_at: SENT,
    updated_at: SENT,
    ...over
  }) as MessageHistory

const page = (count: number, more = false, from = 0) => ({
  messages: Array.from({ length: count }, (_, i) => message(from + i)),
  has_more: more,
  next_cursor: more ? `cursor-${from + count}` : undefined
})

const stats = (over: Partial<MessageHistoryStatusSum> = {}): MessageHistoryStatusSum =>
  ({
    total_sent: 1000,
    total_delivered: 990,
    total_opened: 400,
    total_clicked: 90,
    total_bounced: 4,
    total_complained: 1,
    total_unsubscribed: 3,
    total_failed: 2,
    ...over
  }) as MessageHistoryStatusSum

const broadcast = { id: 'bc1', name: 'November news' } as Broadcast

const renderList = (props: Partial<BroadcastRecipientsProps> = {}) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const all: BroadcastRecipientsProps = {
    workspaceId: 'ws1',
    permissions: null,
    broadcast,
    timezone: 'UTC',
    step: 'opened',
    stats: stats(),
    variationStats: {},
    variants: [],
    smtp: false,
    lists: [],
    refetchInterval: false,
    ...props
  }
  const view = render(
    <QueryClientProvider client={client}>
      <BroadcastRecipients {...all} />
    </QueryClientProvider>
  )
  return {
    ...view,
    rerenderList: (next: Partial<BroadcastRecipientsProps>) =>
      view.rerender(
        <QueryClientProvider client={client}>
          <BroadcastRecipients {...all} {...next} />
        </QueryClientProvider>
      )
  }
}

const lastCall = () => (listMessages as Mock).mock.calls.at(-1)!
const bodyRows = () => document.querySelectorAll('.ant-table-tbody tr.ant-table-row')

describe('BroadcastRecipients', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(listMessages as Mock).mockResolvedValue(page(5))
  })

  it('lists the first opens, latest first, five at a time', async () => {
    renderList()
    expect(await screen.findByText('r0@example.com')).toBeInTheDocument()
    expect(lastCall()).toEqual([
      'ws1',
      { broadcast_id: 'bc1', is_opened: true, order_by: 'opened_at', order: 'desc', cursor: undefined, limit: 5 }
    ])
    expect(bodyRows()).toHaveLength(5)
    expect(screen.getByRole('heading', { name: 'Opened · 400 recipients' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: /First opened/ })).toBeInTheDocument()
    expect(screen.getByText('Showing 5 of 400')).toBeInTheDocument()
    // 12 minutes after the send.
    expect(screen.getAllByText('+12 min')).toHaveLength(5)
  })

  it('lists what the Sent cell counts: the messages sent without failing, with no delay', async () => {
    renderList({ step: 'sent' })
    await screen.findByText('r0@example.com')
    expect(lastCall()[1]).toMatchObject({ is_failed: false, order_by: 'sent_at' })
    expect(lastCall()[1]).not.toHaveProperty('is_sent')
    expect(screen.queryByText(/^\+/)).not.toBeInTheDocument()
  })

  it('shows no delay that would be negative', async () => {
    ;(listMessages as Mock).mockResolvedValue({
      messages: [message(0, { opened_at: '2026-11-24T14:20:00Z' })],
      has_more: false
    })
    renderList()
    await screen.findByText('r0@example.com')
    expect(screen.queryByText(/^\+/)).not.toBeInTheDocument()
  })

  it('turns the order over from the date column header, and back', async () => {
    renderList()
    await screen.findByText('r0@example.com')
    const header = () => screen.getByRole('columnheader', { name: /First opened/ })
    expect(header()).toHaveAttribute('aria-sort', 'descending')
    await userEvent.click(header())
    await waitFor(() => expect(lastCall()[1]).toMatchObject({ order_by: 'opened_at', order: 'asc' }))
    expect(header()).toHaveAttribute('aria-sort', 'ascending')
    await userEvent.click(header())
    await waitFor(() => expect(header()).toHaveAttribute('aria-sort', 'descending'))
  })

  it('shows an erased contact as such, not the stand-in address', async () => {
    ;(listMessages as Mock).mockResolvedValue({ messages: [message(0, { contact_email: 'DELETED_EMAIL' })], has_more: false })
    renderList()
    expect(await screen.findByText('Contact erased')).toBeInTheDocument()
    expect(screen.queryByText('DELETED_EMAIL')).not.toBeInTheDocument()
  })

  it('searches an exact email on Enter, lower-cased, and clears it without Enter', async () => {
    renderList()
    await screen.findByText('r0@example.com')
    const input = screen.getByRole('textbox', { name: 'Exact email' })

    await userEvent.type(input, 'Alice@Example.com {Enter}')
    await waitFor(() => expect(lastCall()[1]).toMatchObject({ contact_email: 'alice@example.com' }))
    expect(screen.queryByText('Showing 5 of 400')).not.toBeInTheDocument()
    expect(screen.getByText('Showing 5 recipients')).toBeInTheDocument()

    // Cleared: the whole step again (from the cache), with its total.
    await userEvent.clear(input)
    expect(await screen.findByText('Showing 5 of 400')).toBeInTheDocument()
  })

  it('does not search a partial email, and says why', async () => {
    renderList()
    await screen.findByText('r0@example.com')
    const calls = (listMessages as Mock).mock.calls.length
    const input = screen.getByRole('textbox', { name: 'Exact email' })
    await userEvent.type(input, 'alice{Enter}')
    const hint = await screen.findByText('Enter a full email address')
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(input).toHaveAttribute('aria-describedby', hint.id)
    expect((listMessages as Mock).mock.calls.length).toBe(calls)

    await userEvent.keyboard('{Escape}')
    expect(screen.queryByText('Enter a full email address')).not.toBeInTheDocument()
    expect(input).not.toHaveAttribute('aria-invalid')
  })

  it('says when no recipient of the step has the email searched', async () => {
    renderList()
    await screen.findByText('r0@example.com')
    ;(listMessages as Mock).mockResolvedValue({ messages: [], has_more: false })
    await userEvent.type(screen.getByRole('textbox', { name: 'Exact email' }), 'bob@example.com{Enter}')
    expect(await screen.findByText('No results match')).toBeInTheDocument()
    expect(screen.queryByText('No recipients at this step yet')).not.toBeInTheDocument()
  })

  it('shows 20 more rows at a time, and folds back to five without a request', async () => {
    ;(listMessages as Mock)
      .mockResolvedValueOnce(page(5, true))
      .mockResolvedValueOnce(page(20, true, 5))
      .mockResolvedValueOnce(page(20, true, 5))
    renderList()
    await screen.findByText('r0@example.com')
    expect(screen.queryByRole('button', { name: 'Show less' })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
    await waitFor(() => expect(bodyRows()).toHaveLength(25))
    expect(lastCall()[1]).toMatchObject({ cursor: 'cursor-5', limit: 20 })
    // Still more to load: both buttons.
    expect(screen.getByRole('button', { name: 'Show more' })).toBeInTheDocument()

    const calls = (listMessages as Mock).mock.calls.length
    await userEvent.click(screen.getByRole('button', { name: 'Show less' }))
    await waitFor(() => expect(bodyRows()).toHaveLength(5))
    expect((listMessages as Mock).mock.calls.length).toBe(calls)
    expect(screen.queryByRole('button', { name: 'Show less' })).not.toBeInTheDocument()

    // The pages behind the first left the cache: showing more loads them again.
    await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
    await waitFor(() => expect(bodyRows()).toHaveLength(25))
    expect((listMessages as Mock).mock.calls.length).toBe(calls + 1)
  })

  it('keeps the rows loaded when "Show more" fails, and says so', async () => {
    ;(listMessages as Mock).mockResolvedValueOnce(page(5, true)).mockRejectedValueOnce(new Error('down'))
    renderList()
    await screen.findByText('r0@example.com')
    await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
    expect(await screen.findByText("Couldn't load the recipients")).toBeInTheDocument()
    expect(bodyRows()).toHaveLength(5)
    expect(screen.getByRole('button', { name: 'Show more' })).toBeInTheDocument()
  })

  it('has no "Show more" when everything is shown', async () => {
    renderList()
    await screen.findByText('r0@example.com')
    expect(screen.queryByRole('button', { name: 'Show more' })).not.toBeInTheDocument()
  })

  it('lists one variant of an A/B test, with that variant’s count', async () => {
    renderList({
      variants: [
        { letter: 'A', templateId: 'tpl-a' },
        { letter: 'B', templateId: 'tpl-b' }
      ],
      variationStats: { 'tpl-a': stats({ total_opened: 210 }), 'tpl-b': stats({ total_opened: 190 }) }
    })
    await screen.findByText('r0@example.com')
    await userEvent.click(screen.getByText('B'))
    await waitFor(() => expect(lastCall()[1]).toMatchObject({ template_id: 'tpl-b' }))
    expect(screen.getByRole('heading', { name: 'Opened · 190 recipients' })).toBeInTheDocument()
  })

  it('opens the logs on the same step and variant', async () => {
    renderList({ step: 'clicked', variants: [{ letter: 'A', templateId: 'tpl-a' }] })
    await screen.findByText('r0@example.com')
    await userEvent.click(screen.getByText('A'))
    await userEvent.click(screen.getByRole('button', { name: 'Open in logs' }))
    expect(navigate).toHaveBeenCalledWith({
      to: '/console/workspace/ws1/logs?broadcast_id=bc1&is_clicked=true&template_id=tpl-a'
    })
  })

  it('opens the logs of Sent without the failed messages', async () => {
    renderList({ step: 'sent' })
    await screen.findByText('r0@example.com')
    await userEvent.click(screen.getByRole('button', { name: 'Open in logs' }))
    expect(navigate).toHaveBeenCalledWith({ to: '/console/workspace/ws1/logs?broadcast_id=bc1&is_failed=false' })
  })

  it('opens a message without the copy-link button', async () => {
    renderList()
    await screen.findByText('r0@example.com')
    const row = screen.getByText('r2@example.com').closest('tr') as HTMLElement
    await userEvent.click(within(row).getByRole('button', { name: 'View message' }))
    const detail = screen.getByTestId('message-detail')
    expect(detail).toHaveAttribute('data-id', 'm2')
    expect(detail).toHaveAttribute('data-copy-link', 'false')
  })

  it('says when there is nobody at this step', async () => {
    ;(listMessages as Mock).mockResolvedValue({ messages: [], has_more: false })
    renderList({ step: 'complained' })
    expect(await screen.findByText('No recipients at this step yet')).toBeInTheDocument()
  })

  it('says when the list could not load, with a retry and no count', async () => {
    ;(listMessages as Mock).mockRejectedValueOnce(new Error('down'))
    renderList()
    expect(await screen.findByText("Couldn't load the recipients")).toBeInTheDocument()
    expect(screen.queryByText(/^Showing/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Show more' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('r0@example.com')).toBeInTheDocument()
  })

  it('reloads while sending only while its first page alone is loaded', async () => {
    ;(listMessages as Mock).mockResolvedValue(page(5, true))
    renderList({ refetchInterval: 40 })
    await screen.findByText('r0@example.com')
    await waitFor(() => expect((listMessages as Mock).mock.calls.length).toBeGreaterThanOrEqual(3))

    await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
    await waitFor(() => expect(lastCall()[1]).toMatchObject({ limit: 20 }))
    const calls = (listMessages as Mock).mock.calls.length
    await new Promise((resolve) => setTimeout(resolve, 200))
    expect((listMessages as Mock).mock.calls.length).toBe(calls)

    // Folded back to its first page, the list reloads again.
    await userEvent.click(screen.getByRole('button', { name: 'Show less' }))
    await waitFor(() => expect((listMessages as Mock).mock.calls.length).toBeGreaterThan(calls))
    expect(lastCall()[1]).toMatchObject({ limit: 5, cursor: undefined })
  })

  it('does not reload when the broadcast is not sending', async () => {
    renderList({ refetchInterval: false })
    await screen.findByText('r0@example.com')
    await new Promise((resolve) => setTimeout(resolve, 150))
    expect(listMessages).toHaveBeenCalledTimes(1)
  })

  it('leaves a list it no longer shows on its first page', async () => {
    ;(listMessages as Mock).mockResolvedValueOnce(page(5, true)).mockResolvedValueOnce(page(20, false, 5))
    const { rerenderList } = renderList()
    await screen.findByText('r0@example.com')
    await userEvent.click(screen.getByRole('button', { name: 'Show more' }))
    await waitFor(() => expect(bodyRows()).toHaveLength(25))

    ;(listMessages as Mock).mockResolvedValue(page(5, true))
    rerenderList({ step: 'clicked' })
    await waitFor(() => expect(lastCall()[1]).toMatchObject({ is_clicked: true, order_by: 'clicked_at', limit: 5 }))
    await waitFor(() => expect(bodyRows()).toHaveLength(5))

    // Back on Opened: its first page only, from the cache.
    rerenderList({ step: 'opened' })
    await waitFor(() => expect(screen.getByRole('heading', { name: /^Opened/ })).toBeInTheDocument())
    expect(bodyRows()).toHaveLength(5)
  })
})
