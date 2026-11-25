import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { BroadcastFunnel } from './BroadcastFunnel'
import type { MessageHistoryStatusSum } from '../../../services/api/messages_history'

const stats = (over: Partial<MessageHistoryStatusSum> = {}): MessageHistoryStatusSum =>
  ({
    total_sent: 45,
    total_delivered: 40,
    total_opened: 10,
    total_clicked: 4,
    total_failed: 23,
    total_bounced: 2,
    total_complained: 0,
    total_unsubscribed: 1,
    ...over
  }) as MessageHistoryStatusSum

const cell = (label: string) => screen.getByText(label).closest('[data-kpi-cell]') as HTMLElement

describe('BroadcastFunnel', () => {
  it('shows values and rates', () => {
    render(<BroadcastFunnel stats={stats()} smtp={false} untracked={false} onSelect={vi.fn()} />)
    expect(cell('Sent')).toHaveTextContent('45')
    expect(cell('Sent')).toHaveTextContent('100.0%')
    expect(cell('Delivered')).toHaveTextContent('88.9%')
    expect(cell('Opened')).toHaveTextContent('22.2%')
  })

  it('selects the step of each cell and incident', () => {
    const onSelect = vi.fn()
    render(<BroadcastFunnel stats={stats()} smtp={false} untracked={false} onSelect={onSelect} />)
    fireEvent.click(cell('Sent'))
    fireEvent.click(cell('Delivered'))
    fireEvent.click(cell('Opened'))
    fireEvent.click(cell('Clicked'))
    fireEvent.click(screen.getByText('Bounces'))
    fireEvent.click(screen.getByText('Complaints'))
    fireEvent.click(screen.getByText('Unsubscribed'))
    fireEvent.click(screen.getByText('Failed'))
    expect(onSelect.mock.calls.map(([step]) => step)).toEqual([
      'sent',
      'delivered',
      'opened',
      'clicked',
      'bounced',
      'complained',
      'unsubscribed',
      'failed'
    ])
  })

  it('marks the selected cell and the selected incident', () => {
    const { rerender } = render(
      <BroadcastFunnel stats={stats()} smtp={false} untracked={false} selected="opened" onSelect={vi.fn()} />
    )
    expect(cell('Opened')).toHaveAttribute('aria-pressed', 'true')
    expect(cell('Clicked')).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByText('Bounces').closest('button')).toHaveAttribute('aria-pressed', 'false')

    rerender(<BroadcastFunnel stats={stats()} smtp={false} untracked={false} selected="bounced" onSelect={vi.fn()} />)
    expect(cell('Opened')).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByText('Bounces').closest('button')).toHaveAttribute('aria-pressed', 'true')
  })

  it('counts the failed share over sent plus failed', () => {
    render(<BroadcastFunnel stats={stats()} smtp={false} untracked={false} onSelect={vi.fn()} />)
    // 23 / (45 + 23)
    expect(screen.getByText('Failed').closest('button')).toHaveTextContent('23 · 33.8%')
    expect(screen.getByText('Failed').closest('button')!.querySelector('.text-danger-fg')).not.toBeNull()
  })

  it('computes rates over the tracked emails, with the hint', async () => {
    render(
      <BroadcastFunnel stats={stats({ total_opens_tracked: 40 })} smtp={false} untracked={false} onSelect={vi.fn()} />
    )
    expect(cell('Opened')).toHaveTextContent('25.0%')
    fireEvent.mouseEnter(cell('Opened').querySelector('.text-kpi')!)
    expect(await screen.findByText('of 40 tracked')).toBeInTheDocument()
  })

  it('has no hint when every email was tracked', async () => {
    render(
      <BroadcastFunnel stats={stats({ total_opens_tracked: 45 })} smtp={false} untracked={false} onSelect={vi.fn()} />
    )
    fireEvent.mouseEnter(cell('Opened').querySelector('.text-kpi')!)
    await new Promise((resolve) => setTimeout(resolve, 150))
    expect(screen.queryByText(/tracked/)).not.toBeInTheDocument()
  })

  it('shows "—" when nothing was tracked', () => {
    render(
      <BroadcastFunnel stats={stats({ total_opens_tracked: 0 })} smtp={false} untracked={false} onSelect={vi.fn()} />
    )
    expect(cell('Opened')).toHaveTextContent('—')
  })

  it('shows Delivered, Bounces and Complaints as "—", not clickable, with an SMTP provider', async () => {
    render(<BroadcastFunnel stats={stats()} smtp untracked={false} onSelect={vi.fn()} />)
    expect(cell('Delivered').tagName).toBe('DIV')
    expect(cell('Delivered')).toHaveTextContent('—')
    expect(screen.getByText('Bounces').closest('button')).toBeNull()
    expect(screen.getByText('Complaints').closest('button')).toBeNull()
    fireEvent.mouseEnter(cell('Delivered').querySelector('.text-kpi span')!)
    await waitFor(() => expect(screen.getByText(/doesn't support delivery webhooks/)).toBeInTheDocument())
  })

  it('reads opens and clicks as not tracked for a broadcast sent without tracking', () => {
    render(<BroadcastFunnel stats={stats()} smtp={false} untracked onSelect={vi.fn()} />)
    expect(cell('Opened')).toHaveTextContent('Not tracked')
    expect(cell('Clicked')).toHaveTextContent('Not tracked')
    // Nothing to list: the cells are not buttons.
    expect(cell('Opened').tagName).toBe('DIV')
    expect(cell('Clicked').tagName).toBe('DIV')
  })

  it('shows visits and conversions only with web analytics', () => {
    const { rerender } = render(<BroadcastFunnel stats={stats()} smtp={false} untracked={false} onSelect={vi.fn()} />)
    expect(screen.queryByText('Website visits')).not.toBeInTheDocument()
    rerender(
      <BroadcastFunnel
        stats={stats()}
        smtp={false}
        untracked={false}
        web={{ visits: { sessions: 12, medianDuration: 65, conversions: 2, value: 30 }, loading: false, error: false }}
        onSelect={vi.fn()}
      />
    )
    expect(cell('Website visits')).toHaveTextContent('12')
    expect(cell('Conversions')).toHaveTextContent('2')
    expect(cell('Website visits').tagName).toBe('DIV')
  })
})
