import { describe, expect, it } from 'vitest'
import { mergeFailures } from './recentFailures'
import type { MessageHistory } from '../../services/api/messages_history'

const message = (id: string, values: Partial<MessageHistory>): MessageHistory =>
  ({
    id,
    contact_email: `${id}@example.com`,
    template_id: 't',
    template_version: 1,
    channel: 'email',
    message_data: { data: {} },
    sent_at: '2026-11-10T08:00:00Z',
    created_at: '2026-11-10T08:00:00Z',
    updated_at: '2026-11-10T08:00:00Z',
    ...values
  }) as MessageHistory

describe('mergeFailures', () => {
  it('sorts the three lists on the time of their event', () => {
    const rows = mergeFailures({
      complained: [message('c', { complained_at: '2026-11-10T14:22:00Z' })],
      bounced: [message('b', { bounced_at: '2026-11-10T14:29:00Z' })],
      failed: [message('f', { failed_at: '2026-11-10T14:24:00Z' })]
    })
    expect(rows.map((row) => [row.message.id, row.kind, row.at])).toEqual([
      ['b', 'bounce', '2026-11-10T14:29:00Z'],
      ['f', 'failed', '2026-11-10T14:24:00Z'],
      ['c', 'complaint', '2026-11-10T14:22:00Z']
    ])
  })

  it('keeps five rows', () => {
    const bounced = Array.from({ length: 5 }, (_, i) => message(`b${i}`, { bounced_at: `2026-11-10T10:0${i}:00Z` }))
    const failed = Array.from({ length: 5 }, (_, i) => message(`f${i}`, { failed_at: `2026-11-10T11:0${i}:00Z` }))
    const rows = mergeFailures({ complained: [], bounced, failed })
    expect(rows).toHaveLength(5)
    expect(rows[0].message.id).toBe('f4')
  })

  it('shows a message in two lists once, as a complaint before a bounce before a failure', () => {
    const both = message('x', { bounced_at: '2026-11-10T09:00:00Z', complained_at: '2026-11-10T12:00:00Z', failed_at: '2026-11-10T13:00:00Z' })
    const rows = mergeFailures({ complained: [both], bounced: [both], failed: [both] })
    expect(rows).toHaveLength(1)
    expect(rows[0].kind).toBe('complaint')
    expect(rows[0].at).toBe('2026-11-10T12:00:00Z')
    const bounceAndFail = message('y', { bounced_at: '2026-11-10T09:00:00Z', failed_at: '2026-11-10T13:00:00Z' })
    expect(mergeFailures({ complained: [], bounced: [bounceAndFail], failed: [bounceAndFail] })[0].kind).toBe('bounce')
  })

  it('gives the provider reason, or none', () => {
    const rows = mergeFailures({
      complained: [],
      bounced: [message('a', { bounced_at: '2026-11-10T09:00:00Z', status_info: '  Mailbox full ' })],
      failed: [message('b', { failed_at: '2026-11-10T08:00:00Z', status_info: '' })]
    })
    expect(rows.map((row) => row.reason)).toEqual(['Mailbox full', null])
  })

  it('accepts missing lists', () => {
    expect(mergeFailures({ complained: undefined, bounced: undefined, failed: undefined })).toEqual([])
  })
})
