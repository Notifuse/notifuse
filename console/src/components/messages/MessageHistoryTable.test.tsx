import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App, ConfigProvider } from 'antd'
import { MessageHistoryTable } from './MessageHistoryTable'
import type { MessageHistory, MessageData } from '../../services/api/messages_history'
import type { Workspace } from '../../services/api/types'

// The preview button behind the actions column fetches a template per row. Nothing here
// asserts on it, and letting it run would put an unmocked request in every test.
vi.mock('../../services/api/template', () => ({
  templatesApi: { get: vi.fn().mockResolvedValue({ template: null }) }
}))

const workspace = {
  id: 'ws-1',
  name: 'Test Workspace',
  settings: { timezone: 'UTC' }
} as unknown as Workspace

// `message_data` is typed as required, but the wire shape is looser than the type: contact
// erasure resets the column to {}, so `data` is genuinely absent on redacted rows.
const message = (id: string, messageData: Partial<MessageData>): MessageHistory =>
  ({
    id,
    contact_email: 'bob@example.com',
    template_id: 'tpl-1',
    template_version: 1,
    channel: 'email',
    message_data: messageData,
    sent_at: '2026-08-01T10:00:00Z',
    created_at: '2026-08-01T10:00:00Z',
    updated_at: '2026-08-01T10:00:00Z'
  }) as MessageHistory

const renderTable = (messages: MessageHistory[], visibleColumns?: Record<string, boolean>) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ConfigProvider>
        <App>
          <MessageHistoryTable
            messages={messages}
            loading={false}
            isLoadingMore={false}
            onLoadMore={() => {}}
            workspace={workspace}
            visibleColumns={visibleColumns}
          />
        </App>
      </ConfigProvider>
    </QueryClientProvider>
  )
}

describe('MessageHistoryTable metadata column', () => {
  it('renders a tag per metadata pair', () => {
    renderTable([message('m1', { data: {}, metadata: { order_id: '9912', tier: 'gold' } })])

    expect(screen.getByText(/order_id/)).toBeInTheDocument()
    expect(screen.getByText(/9912/)).toBeInTheDocument()
    expect(screen.getByText(/tier/)).toBeInTheDocument()
    expect(screen.getByText(/gold/)).toBeInTheDocument()
  })

  it('collapses the pairs past the first two into a +N tag', () => {
    renderTable([
      message('m1', { data: {}, metadata: { a: '1', b: '2', c: '3', d: '4' } })
    ])

    expect(screen.getByText(/^\+2$/)).toBeInTheDocument()
    // The collapsed pairs are not rendered inline; they live in the tooltip.
    expect(screen.queryByText(/^c:/)).not.toBeInTheDocument()
  })

  it('lists every pair in the tooltip', async () => {
    renderTable([
      message('m1', { data: {}, metadata: { a: '1', b: '2', c: 'buried' } })
    ])

    fireEvent.mouseEnter(screen.getByText(/^a:/).closest('span[class*="cursor-help"]')!)

    await waitFor(() => expect(screen.getByText('buried')).toBeInTheDocument())
  })

  it('serializes metadata values that are not scalars', () => {
    renderTable([message('m1', { data: {}, metadata: { items: ['a', 'b'] } })])

    expect(screen.getByText(/\["a","b"\]/)).toBeInTheDocument()
  })

  it('truncates a long metadata value', () => {
    const long = 'x'.repeat(60)
    renderTable([message('m1', { data: {}, metadata: { note: long } })])

    expect(screen.queryByText(new RegExp(long))).not.toBeInTheDocument()
    expect(screen.getByText(/x{24}\.\.\./)).toBeInTheDocument()
  })

  it('renders a placeholder when the message carries no metadata', () => {
    // The broadcast and automation shape: those paths never set metadata.
    renderTable([message('m1', { data: { first_name: 'Bob' } })])

    expect(screen.getByText('Metadata')).toBeInTheDocument()
    expect(screen.queryByText(/^\+/)).not.toBeInTheDocument()
  })

  it('treats an empty metadata object as no metadata', () => {
    renderTable([message('m1', { data: {}, metadata: {} })])

    expect(screen.getByText('Metadata')).toBeInTheDocument()
    expect(screen.queryByText(/^\+/)).not.toBeInTheDocument()
  })

  it('renders a row whose message_data was blanked by contact erasure', () => {
    // DeleteForEmail resets message_data to '{}', so neither `data` nor `metadata` is there.
    expect(() => renderTable([message('m1', {})])).not.toThrow()
    expect(screen.getByText('Metadata')).toBeInTheDocument()
  })

  it('hides the column when visibleColumns.metadata is false', () => {
    renderTable([message('m1', { data: {}, metadata: { order_id: '9912' } })], {
      metadata: false
    })

    expect(screen.queryByText('Metadata')).not.toBeInTheDocument()
    expect(screen.queryByText(/order_id/)).not.toBeInTheDocument()
  })

  it('shows the column when no visibility map is supplied', () => {
    // ContactDetailsDrawer and FailedMessagesTable pass none. The Logs page does pass one,
    // but leaves `metadata` out of it, so the column reads as visible there too.
    renderTable([message('m1', { data: {}, metadata: { order_id: '9912' } })])

    expect(screen.getByText('Metadata')).toBeInTheDocument()
    expect(screen.getByText(/order_id/)).toBeInTheDocument()
  })
})

// The Error column is the only place a send failure explains itself. It rendered blank
// for every message because it read `record.error`, a field the API has never returned
// — the wire name is `status_info`.
describe('MessageHistoryTable error column', () => {
  const failed = (id: string, statusInfo?: string): MessageHistory =>
    ({
      ...message(id, { data: {} }),
      failed_at: '2026-08-01T10:00:05Z',
      status_info: statusInfo
    }) as MessageHistory

  it("shows the provider's own explanation", () => {
    renderTable([failed('m1', '452 4.3.1 quota exceeded')])

    expect(screen.getByText('452 4.3.1 quota exceeded')).toBeInTheDocument()
  })

  it('truncates a long explanation, keeping the code that leads it', () => {
    const long = `550 5.1.1 ${'x'.repeat(120)}`
    renderTable([failed('m1', long)])

    const cell = screen.getByText(/^550 5\.1\.1 x+…$/)
    expect(cell.textContent!.length).toBeLessThan(long.length)
  })

  it('reveals the whole explanation on hover', async () => {
    const long = `452 4.3.1 ${'x'.repeat(60)} buried-tail`
    renderTable([failed('m1', long)])

    fireEvent.mouseEnter(screen.getByText(/^452 4\.3\.1 x+…$/))

    await waitFor(() => expect(screen.getByText(long)).toBeInTheDocument())
  })

  it('renders nothing when the send carried no explanation', () => {
    renderTable([failed('m1', undefined)])

    expect(screen.queryByText(/rejected with code/)).not.toBeInTheDocument()
  })
})
