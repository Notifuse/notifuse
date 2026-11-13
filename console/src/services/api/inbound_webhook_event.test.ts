import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { listInboundWebhookEvents } from './inbound_webhook_event'

vi.mock('./client', () => ({
  api: { get: vi.fn() }
}))

const sentQuery = (): URLSearchParams => {
  const [url] = vi.mocked(api.get).mock.calls.at(-1) as [string]
  return new URLSearchParams(url.split('?')[1])
}

beforeEach(() => {
  vi.mocked(api.get).mockReset()
  vi.mocked(api.get).mockResolvedValue({ events: [], has_more: false })
})

describe('listInboundWebhookEvents', () => {
  it('sends the source, the type, the search and the lower bound', async () => {
    await listInboundWebhookEvents('ws1', {
      source: 'postmark',
      event_type: 'reply',
      recipient_email: 'ann@example.com',
      timestamp_after: '2026-11-12T10:00:00.000Z',
      limit: 50,
      cursor: 'c1'
    })
    const query = sentQuery()
    expect(query.get('workspace_id')).toBe('ws1')
    expect(query.get('source')).toBe('postmark')
    expect(query.get('event_type')).toBe('reply')
    expect(query.get('recipient_email')).toBe('ann@example.com')
    expect(query.get('timestamp_after')).toBe('2026-11-12T10:00:00.000Z')
    expect(query.get('limit')).toBe('50')
    expect(query.get('cursor')).toBe('c1')
  })

  it('leaves the source out when it is not set', async () => {
    await listInboundWebhookEvents('ws1', { limit: 50 })
    expect(sentQuery().has('source')).toBe(false)
  })
})
