import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
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
  // One Cancel entry, with no Back to draft beside it: it asks which way out (cancelOrDraft).
  it('offers one Cancel entry that asks for back to draft or cancel', async () => {
    const onAction = vi.fn()
    const scheduled = broadcast({ status: 'scheduled' } as Partial<Broadcast>)
    renderTable({ rows: [scheduled], onAction, permissions: { broadcasts: { read: true, write: true } } as never })
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(screen.queryByText('Back to draft')).toBeNull()
    fireEvent.click(await screen.findByText('Cancel'))
    expect(onAction).toHaveBeenCalledWith('cancelOrDraft', scheduled)
  })

  it('offers no "More actions" on a cancelled broadcast with nothing to do', () => {
    // It used to open an empty menu.
    renderTable({ rows: [broadcast({ status: 'cancelled' } as Partial<Broadcast>)], permissions: { broadcasts: { read: true, write: true } } as never })
    expect(screen.queryByRole('button', { name: 'More actions' })).toBeNull()
  })

  it('cancels a broadcast whose sending started straight from Cancel', async () => {
    const onAction = vi.fn()
    const started = broadcast({ status: 'scheduled', started_at: '2026-11-15T09:00:00Z' } as Partial<Broadcast>)
    renderTable({ rows: [started], onAction, permissions: { broadcasts: { read: true, write: true } } as never })
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
    fireEvent.click(await screen.findByText('Cancel'))
    expect(onAction).toHaveBeenCalledWith('cancel', started)
    expect(screen.queryByText('Back to draft')).toBeNull()
  })

  it('offers Pause on the row at every step of an A/B test', () => {
    const onAction = vi.fn()
    for (const status of ['testing', 'test_completed', 'winner_selected'] as const) {
      const ab = broadcast({ id: status, status, started_at: '2026-11-15T09:00:00Z' } as Partial<Broadcast>)
      const { unmount } = renderTable({ rows: [ab], onAction, permissions: { broadcasts: { read: true, write: true } } as never })
      fireEvent.click(screen.getByRole('button', { name: 'Pause' }))
      expect(onAction).toHaveBeenLastCalledWith('pause', ab)
      unmount()
    }
  })

  it('shows « A/B » after the name on the first line, its variants and test in a popover', async () => {
    renderTable({
      rows: [
        broadcast({
          test_settings: {
            enabled: true,
            sample_percentage: 20,
            auto_send_winner: true,
            auto_send_winner_metric: 'click_rate',
            test_duration_hours: 6,
            variations: [
              { variation_name: 'variation-a', template_id: 't1', template: { name: 'Spring offer' } },
              { variation_name: 'variation-b', template_id: 't2', template: { name: 'Spring offer, short' } }
            ]
          },
          winning_template: 't2'
        } as Partial<Broadcast>)
      ]
    })
    const pill = document.querySelector('[data-ab-pill]') as HTMLElement
    // The name's line, right after it.
    const nameLine = screen.getByText('Black Friday').parentElement as HTMLElement
    expect(nameLine.contains(pill)).toBe(true)
    // Seen: « A/B » alone, the summary being for screen readers only.
    const visible = pill.cloneNode(true) as HTMLElement
    visible.querySelectorAll('.sr-only').forEach((node) => node.remove())
    expect(visible.textContent?.trim()).toBe('A/B')
    // The former summary stays for screen readers.
    expect(pill.querySelector('.sr-only')).toHaveTextContent('A/B · Spring offer, short')

    fireEvent.mouseEnter(pill)
    const popover = await waitFor(() => {
      const node = document.querySelector('[data-ab-popover]')
      expect(node).not.toBeNull()
      return node as HTMLElement
    })
    expect(popover).toHaveTextContent('Spring offer')
    expect(popover).toHaveTextContent('Winner')
    // Each variant by its letter, as in the composer, never by its internal name.
    expect(within(popover).getByText('A')).toBeInTheDocument()
    expect(within(popover).getByText('B')).toBeInTheDocument()
    expect(popover).not.toHaveTextContent('variation-a')
    expect(popover).toHaveTextContent('20 %')
    expect(popover).toHaveTextContent('Click rate')
    expect(popover).toHaveTextContent('6')
  })

  it('lists Website traffic in the row menu without a divider, even with no action above it', async () => {
    renderTable({
      canReadWeb: true,
      rows: [
        broadcast({
          status: 'processed',
          started_at: '2026-11-10T08:00:00Z',
          utm_parameters: { campaign: 'black-friday' }
        } as Partial<Broadcast>)
      ]
    })
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(await screen.findByText('Website traffic')).toBeInTheDocument()
    expect(document.querySelector('.ant-dropdown-menu-item-divider')).toBeNull()
  })

  it('scrolls below the width that leaves the Broadcast column its minimum', () => {
    expect(BROADCAST_NAME_MIN_WIDTH).toBe(200)
    expect(tableWidth(renderTable({}).container)).toBe('1110px')
  })

  it('makes room for the website visits column when it is shown', () => {
    const { container } = renderTable({ canReadWeb: true })
    expect(tableWidth(container)).toBe('1190px')
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
