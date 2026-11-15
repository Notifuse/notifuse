import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { BroadcastsTable, type BroadcastsTableProps } from './BroadcastsTable'
import { BROADCAST_NAME_MIN_WIDTH, minTableWidth } from './tableWidth'
import type { Workspace } from '../../services/api/types'
import type { Broadcast } from '../../services/api/broadcast'
import type { ProgressStats } from './broadcastStatus'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useRouter: () => ({ buildLocation: () => ({ href: '#' }) })
}))

const props = (over: Partial<BroadcastsTableProps> = {}): BroadcastsTableProps => ({
  workspace: {
    id: 'ws',
    settings: { timezone: 'UTC' }
  } as unknown as Workspace,
  rows: [],
  loading: false,
  permissions: null,
  canReadHistory: true,
  canReadWeb: false,
  pageStats: {
    stats: new Map(),
    statsLoading: false,
    statsError: false,
    verdicts: new Map(),
    progress: new Map(),
    tasks: new Map(),
    visits: new Map(),
    visitsLoading: false,
    visitsError: false,
    verdictLive: false
  },
  estimates: new Map(),
  lists: [],
  segments: [],
  onOpen: vi.fn(),
  onCompose: vi.fn(),
  onAction: vi.fn(),
  onOpenDetail: vi.fn(),
  onDelete: vi.fn(),
  ...over
})

const tableWidth = (container: HTMLElement) => container.querySelector('table')?.style.width

describe('minTableWidth', () => {
  it('adds the minimum of the column without a width to the fixed widths', () => {
    expect(minTableWidth([{}, { width: 120 }, { width: 80 }], 200)).toBe(400)
  })

  it('counts only widths given in pixels', () => {
    expect(minTableWidth([{}, { width: 120 }, { width: '10%' }], 200)).toBe(320)
  })
})

const broadcast = (over: Partial<Broadcast>): Broadcast =>
  ({
    id: 'b1',
    name: 'Black Friday',
    status: 'processed',
    audience: { list: 'l1', exclude_unsubscribed: true },
    test_settings: { enabled: false, variations: [] },
    updated_at: '2026-11-10T08:00:00Z',
    ...over
  }) as unknown as Broadcast

const renderTable = (over: Partial<BroadcastsTableProps>) =>
  render(
    <App>
      <BroadcastsTable {...props(over)} />
    </App>
  )

describe('BroadcastsTable', () => {
  it('scrolls below the width that leaves the Broadcast column its minimum', () => {
    expect(BROADCAST_NAME_MIN_WIDTH).toBe(200)
    expect(tableWidth(renderTable({}).container)).toBe('1094px')
  })

  it('makes room for the website visits column when it is shown', () => {
    const { container } = renderTable({ canReadWeb: true })
    expect(tableWidth(container)).toBe('1174px')
    expect(container.querySelector('thead')).toHaveTextContent('Site visits')
  })

  it('writes a scheduled date on one line and its time and zone on the next', () => {
    renderTable({
      rows: [
        broadcast({
          status: 'scheduled',
          schedule: {
            is_scheduled: true,
            scheduled_date: '2026-11-27',
            scheduled_time: '08:30',
            timezone: 'UTC',
            use_recipient_timezone: false
          }
        })
      ]
    })
    const date = screen.getByText('Nov 27, 2026')
    const time = screen.getByText(/^8:30\sAM UTC$/)
    expect(date).not.toBe(time)
    expect(date.parentElement).toBe(time.parentElement)
  })

  it('truncates a status pill too long for its column, and starts its tooltip with the whole label', async () => {
    renderTable({ rows: [broadcast({ status: 'test_completed' })] })
    const label = screen.getByText('Pick a winner')
    expect(label.className).toContain('truncate')
    fireEvent.mouseEnter(label)
    expect(await screen.findByText(/A\/B test has completed/)).toBeInTheDocument()
    expect(screen.getAllByText('Pick a winner')).toHaveLength(2)
  })

  it('shows a send in progress as a bar and its share, the counts in a tooltip', async () => {
    const progress = { sentCount: 30, failedCount: 0, inFlight: 70 } as ProgressStats
    const { container } = renderTable({
      rows: [broadcast({ started_at: '2026-11-10T08:00:00Z' })],
      pageStats: {
        ...props().pageStats,
        verdicts: new Map([['b1', { kind: 'sending', remaining: 70 }]]),
        progress: new Map([['b1', progress]]),
        tasks: new Map([['b1', { state: { send_broadcast: { enqueued_count: 100 } } }]]) as never
      }
    })
    expect(container.querySelector('[data-sending-progress]')).not.toBeNull()
    expect(screen.getByText('30.0%')).toBeInTheDocument()
    expect(screen.getByText('Sending emails...').closest('.sr-only')).not.toBeNull()
    fireEvent.mouseEnter(container.querySelector('[data-sending-progress]')!)
    await waitFor(() => expect(document.querySelector('.ant-tooltip')).toHaveTextContent('30 of 100'))
  })

  it('shows the start date once nothing is left in flight', () => {
    const progress = { sentCount: 100, failedCount: 0, inFlight: 0 } as ProgressStats
    const { container } = renderTable({
      rows: [broadcast({ started_at: '2026-11-10T08:00:00Z' })],
      pageStats: {
        ...props().pageStats,
        verdicts: new Map([['b1', { kind: 'sending', remaining: 0 }]]),
        progress: new Map([['b1', progress]]),
        tasks: new Map([['b1', { state: { send_broadcast: { enqueued_count: 100 } } }]]) as never
      }
    })
    expect(container.querySelector('[data-sending-progress]')).toBeNull()
    expect(screen.getByText('Nov 10, 2026, 8:00 AM')).toBeInTheDocument()
  })
})
