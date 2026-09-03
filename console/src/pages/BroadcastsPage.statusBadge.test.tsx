import '../__tests__/resizeObserverMock'
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { StatusBadge } from './BroadcastsPage'
import type { ProgressStats } from '../components/broadcasts/BroadcastStats'
import type { Broadcast } from '../services/api/broadcast'

const broadcast = (status: string): Broadcast =>
  ({ id: 'bc-1', name: 'Newsletter', status }) as unknown as Broadcast

const renderBadge = (status: string, progressStats?: ProgressStats) =>
  render(
    <ConfigProvider>
      <StatusBadge broadcast={broadcast(status)} progressStats={progressStats} />
    </ConfigProvider>
  )

const stats = (over: Partial<ProgressStats>): ProgressStats => ({
  remaining: 0,
  processed: 68,
  enqueuedCount: 68,
  sentCount: 45,
  failedCount: 23,
  ...over
})

// A broadcast is finished when its queue is empty, not when the arithmetic on message
// history happens to reach zero. sent_at is stamped on the first attempt whatever its
// outcome, so a recipient the provider refused counted as both sent and failed, which
// drove `remaining` to zero and painted a green "Complete" over a campaign that never
// reached 23 people.
describe('StatusBadge for a processed broadcast', () => {
  it('is still sending while anything is in flight', () => {
    renderBadge('processed', stats({ inFlight: 12, failedTerminal: 0 }))

    expect(screen.getByText(/Sending 12 remaining/)).toBeInTheDocument()
  })

  it('is complete only when the queue is empty and nothing was given up on', () => {
    renderBadge('processed', stats({ inFlight: 0, failedTerminal: 0, failedCount: 0 }))

    expect(screen.getByText('Complete')).toBeInTheDocument()
  })

  it('names the recipients it gave up on', () => {
    renderBadge('processed', stats({ inFlight: 0, failedTerminal: 23 }))

    expect(screen.getByText(/23 failed/)).toBeInTheDocument()
    expect(screen.queryByText('Complete')).not.toBeInTheDocument()
  })

  it('counts a row still retrying as in flight, not as given up on', () => {
    // failed_retrying rows come back on their own; only a row that has spent every
    // attempt means a recipient was abandoned.
    renderBadge('processed', stats({ inFlight: 2, failedTerminal: 0, failedCount: 2 }))

    expect(screen.getByText(/Sending 2 remaining/)).toBeInTheDocument()
  })

  it('falls back to the old arithmetic when the queue could not be read', () => {
    // The counts are absent when the queue is unreachable. The page keeps working on
    // what message history knows rather than showing nothing.
    renderBadge('processed', stats({ inFlight: undefined, failedTerminal: undefined, remaining: 5 }))

    expect(screen.getByText(/Sending 5 remaining/)).toBeInTheDocument()
  })

  it('still names the failures once the abandoned rows have been swept', () => {
    // Terminal queue rows are deleted after their retention window. Reading the
    // verdict from them alone would repaint this campaign green a week later — the
    // exact bug being fixed, on a timer — so the count comes from message history.
    renderBadge('processed', stats({ inFlight: 0, failedTerminal: 0, failedCount: 23 }))

    expect(screen.getByText(/23 failed/)).toBeInTheDocument()
    expect(screen.queryByText('Complete')).not.toBeInTheDocument()
  })
})

// winner_selected is a phase, not an ending: the orchestrator moves the broadcast to
// 'processed' once the remaining recipients are enqueued, and that is where the
// delivery verdict applies. The phase keeps its own badge so an A/B campaign can still
// be told apart from an ordinary send.
describe('StatusBadge for the A/B winner phase', () => {
  it('keeps naming the phase while the winner is going out', () => {
    renderBadge('winner_selected', stats({ inFlight: 4, failedTerminal: 0 }))

    expect(screen.getByText('Winner Selected')).toBeInTheDocument()
  })

  it('reports the failures once the phase has finished and the broadcast is processed', () => {
    renderBadge('processed', stats({ inFlight: 0, failedTerminal: 4, failedCount: 4 }))

    expect(screen.getByText(/4 failed/)).toBeInTheDocument()
  })
})
