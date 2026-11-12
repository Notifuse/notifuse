import '../../__tests__/resizeObserverMock'
import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { BroadcastStatusPill } from './BroadcastStatusPill'
import type { Broadcast } from '../../services/api/broadcast'
import type { DeliveryVerdict } from './broadcastStatus'

const broadcast = (status: string, over: Partial<Broadcast> = {}): Broadcast =>
  ({ id: 'bc-1', name: 'Newsletter', status, ...over }) as unknown as Broadcast

const hoverTooltip = async (label: string) => {
  fireEvent.mouseEnter(screen.getByText(label).closest('span.inline-flex')!)
  return waitFor(() => expect(document.querySelector('.ant-tooltip')).not.toBeNull())
}

describe('BroadcastStatusPill', () => {
  it.each([
    ['draft', 'Draft'],
    ['scheduled', 'Scheduled'],
    ['testing', 'Testing'],
    ['test_completed', 'Pick a winner'],
    ['processing', 'Sending'],
    ['winner_selected', 'Sending'],
    ['paused', 'Paused'],
    ['cancelled', 'Cancelled'],
    ['failed', 'Failed']
  ])('labels %s "%s"', (status, label) => {
    render(<BroadcastStatusPill broadcast={broadcast(status)} />)
    expect(screen.getByText(label)).toBeInTheDocument()
  })

  it('says how many are still to go while a processed broadcast is sending', async () => {
    const verdict: DeliveryVerdict = { kind: 'sending', remaining: 12 }
    render(<BroadcastStatusPill broadcast={broadcast('processed')} verdict={verdict} />)
    expect(screen.getByText('Sending')).toBeInTheDocument()
    await hoverTooltip('Sending')
    expect(await screen.findByText('Emails are being delivered. 12 still to go.')).toBeInTheDocument()
  })

  it('names the failures, never "Sent", once the queue gave up on recipients', () => {
    render(<BroadcastStatusPill broadcast={broadcast('processed')} verdict={{ kind: 'completedWithFailures', failed: 23 }} />)
    expect(screen.getByText('23 failed')).toBeInTheDocument()
    expect(screen.queryByText('Sent')).not.toBeInTheDocument()
  })

  it('is "Sent" when the queue is done and nothing failed', () => {
    render(<BroadcastStatusPill broadcast={broadcast('processed')} verdict={{ kind: 'complete' }} />)
    expect(screen.getByText('Sent')).toBeInTheDocument()
  })

  it('reads the failures of another row from the 30-day stats', () => {
    render(<BroadcastStatusPill broadcast={broadcast('processed')} l2Failed={4} />)
    expect(screen.getByText('4 failed')).toBeInTheDocument()
  })

  it('keeps naming the winner phase in the tooltip while the winner goes out', async () => {
    render(<BroadcastStatusPill broadcast={broadcast('winner_selected')} />)
    await hoverTooltip('Sending')
    expect(
      await screen.findByText('A winner has been selected. Emails are being sent to the remaining recipients.')
    ).toBeInTheDocument()
  })

  it('explains a pause with its reason when there is one', async () => {
    render(<BroadcastStatusPill broadcast={broadcast('paused', { pause_reason: 'Circuit breaker: bounces' })} />)
    await hoverTooltip('Paused')
    expect(await screen.findByText('Circuit breaker: bounces')).toBeInTheDocument()
  })
})
