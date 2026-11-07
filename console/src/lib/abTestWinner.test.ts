import { describe, it, expect } from 'vitest'
import { canSelectWinner } from './abTestWinner'
import type { Broadcast } from '../services/api/broadcast'

const broadcast = (status: string, autoSendWinner: boolean): Broadcast =>
  ({
    id: 'bc-1',
    status,
    test_settings: { enabled: true, auto_send_winner: autoSendWinner }
  }) as unknown as Broadcast

describe('canSelectWinner', () => {
  // An automatic winner is not chosen when no variation has a tracked send for the
  // metric (the docs say the owner then picks it), and the server accepts a manual
  // choice in test_completed whatever auto_send_winner says.
  it('offers the choice on a completed test with an automatic winner', () => {
    expect(canSelectWinner(broadcast('test_completed', true))).toBe(true)
  })

  it('offers the choice on a completed test without an automatic winner', () => {
    expect(canSelectWinner(broadcast('test_completed', false))).toBe(true)
  })

  it.each(['draft', 'testing', 'processing', 'winner_selected', 'processed'])(
    'does not offer it on a broadcast in %s',
    (status) => {
      expect(canSelectWinner(broadcast(status, false))).toBe(false)
      expect(canSelectWinner(broadcast(status, true))).toBe(false)
    }
  )
})
