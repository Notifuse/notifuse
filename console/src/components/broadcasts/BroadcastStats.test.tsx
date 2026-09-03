import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import { BroadcastStats, type ProgressStats } from './BroadcastStats'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

const state = vi.hoisted(() => ({
  result: {} as Record<string, unknown>
}))

vi.mock('../../services/api/messages_history', () => ({
  getBroadcastStats: vi.fn(() => Promise.resolve(state.result))
}))

const sum = (over: Record<string, number> = {}) => ({
  total_sent: 45,
  total_delivered: 0,
  total_opened: 0,
  total_clicked: 0,
  total_failed: 23,
  total_bounced: 0,
  total_complained: 0,
  total_unsubscribed: 0,
  ...over
})

const renderStats = (onStatsUpdate?: (s: ProgressStats) => void) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ConfigProvider>
        <BroadcastStats
          workspaceId="ws-1"
          broadcastId="bc-1"
          enqueuedCount={68}
          broadcastStatus="processed"
          onStatsUpdate={onStatsUpdate}
        />
      </ConfigProvider>
    </QueryClientProvider>
  )
}

// The tile showed a percentage, so 23 abandoned recipients read as "34%" beside a green
// Complete badge. A count is what the operator has to act on.
describe('BroadcastStats failed tile', () => {
  it('shows how many recipients failed, not just a rate', async () => {
    state.result = { broadcast_id: 'bc-1', stats: sum() }
    renderStats()

    await waitFor(() => expect(screen.getByText('23')).toBeInTheDocument())
    // The rate is a share of what was attempted. Dividing by total_sent, which now
    // means "accepted by the provider", gave 51% for a campaign that lost a third of
    // its recipients — and would pass 100% in a worse outage.
    expect(screen.getByText('34%')).toBeInTheDocument()
  })
})

describe('BroadcastStats queue counts', () => {
  it('passes the queue counts up so the badge can tell finished from abandoned', async () => {
    state.result = {
      broadcast_id: 'bc-1',
      stats: sum(),
      queue: { pending: 2, processing: 1, paused: 0, failed_retrying: 3, failed_terminal: 23 }
    }
    const updates: ProgressStats[] = []
    renderStats((s) => updates.push(s))

    await waitFor(() => expect(updates[updates.length - 1]?.inFlight).toBe(6))
    expect(updates[updates.length - 1].failedTerminal).toBe(23)
  })

  it('leaves the counts undefined when the queue could not be read', async () => {
    // Undefined has to mean "unknown", never "nothing left" — the badge falls back
    // to the message-history arithmetic rather than declaring the campaign finished.
    state.result = { broadcast_id: 'bc-1', stats: sum() }
    const updates: ProgressStats[] = []
    renderStats((s) => updates.push(s))

    // Wait for the response to land before asserting on what it did not carry.
    await waitFor(() => expect(updates[updates.length - 1]?.sentCount).toBe(45))
    expect(updates[updates.length - 1].inFlight).toBeUndefined()
    expect(updates[updates.length - 1].failedTerminal).toBeUndefined()
  })
})
