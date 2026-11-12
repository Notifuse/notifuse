import { describe, expect, it } from 'vitest'
import { messageStatus } from './messageStatus'

const at = '2026-11-12T10:00:00Z'

describe('messageStatus', () => {
  it.each([
    [{ failed_at: at }, 'failed', 'danger'],
    [{ complained_at: at }, 'complained', 'danger'],
    [{ bounced_at: at }, 'bounced', 'warning'],
    [{ unsubscribed_at: at }, 'unsubscribed', 'warning'],
    [{ clicked_at: at }, 'clicked', 'primary'],
    [{ opened_at: at }, 'opened', 'info'],
    [{ delivered_at: at }, 'delivered', 'success'],
    [{}, 'sent', 'neutral']
  ] as const)('reads %o as %s', (events, key, tone) => {
    expect(messageStatus(events)).toEqual({ key, tone })
  })

  it('keeps the most advanced event when several are set', () => {
    expect(messageStatus({ delivered_at: at, opened_at: at, clicked_at: at }).key).toBe('clicked')
    expect(messageStatus({ delivered_at: at, opened_at: at, unsubscribed_at: at }).key).toBe('unsubscribed')
    expect(messageStatus({ delivered_at: at, complained_at: at, bounced_at: at }).key).toBe('complained')
    expect(messageStatus({ delivered_at: at, failed_at: at }).key).toBe('failed')
  })

  it('leaves a message opened without a delivery event as opened', () => {
    expect(messageStatus({ opened_at: at }).key).toBe('opened')
  })
})
