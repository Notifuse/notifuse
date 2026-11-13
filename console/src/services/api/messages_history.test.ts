import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { countMessages, listMessages } from './messages_history'

// The service module imports the api client, which pulls in the router.
vi.mock('./client', () => ({
  api: { get: vi.fn() }
}))

const sentQuery = (): URLSearchParams => {
  const [url] = vi.mocked(api.get).mock.calls.at(-1) as [string]
  return new URLSearchParams(url.split('?')[1])
}

beforeEach(() => {
  vi.mocked(api.get).mockReset()
  vi.mocked(api.get).mockResolvedValue({ messages: [], has_more: false })
})

describe('listMessages', () => {
  it('sends the transactional notification filter', async () => {
    await listMessages('ws1', { transactional_notification_id: 'password_reset', limit: 10 })
    const query = sentQuery()
    expect(query.get('workspace_id')).toBe('ws1')
    expect(query.get('transactional_notification_id')).toBe('password_reset')
    expect(query.get('limit')).toBe('10')
  })

  it('leaves the filter out when it is not set', async () => {
    await listMessages('ws1', { limit: 10 })
    expect(sentQuery().has('transactional_notification_id')).toBe(false)
  })

  it('sends the automation filter when it is set', async () => {
    await listMessages('ws1', { automation_id: 'a1', template_id: 'tpl' })
    const query = sentQuery()
    expect(query.get('automation_id')).toBe('a1')
    expect(query.get('template_id')).toBe('tpl')
  })

  it('leaves the automation filter out otherwise', async () => {
    await listMessages('ws1', { limit: 10 })
    expect(sentQuery().has('automation_id')).toBe(false)
  })
})

describe('listMessages with the Logs filters', () => {
  it('sends the origin and the creation bounds', async () => {
    await listMessages('ws1', {
      origin: 'automation',
      created_after: '2026-11-12T10:00:00.000Z',
      created_before: '2026-11-13T10:00:00.000Z'
    })
    const query = sentQuery()
    expect(query.get('origin')).toBe('automation')
    expect(query.get('created_after')).toBe('2026-11-12T10:00:00.000Z')
    expect(query.get('created_before')).toBe('2026-11-13T10:00:00.000Z')
  })

  it('leaves them out when they are not set', async () => {
    await listMessages('ws1', { limit: 50 })
    const query = sentQuery()
    expect(query.has('origin')).toBe(false)
    expect(query.has('created_after')).toBe(false)
    expect(query.has('created_before')).toBe(false)
  })

  it('sends a false status filter', async () => {
    await listMessages('ws1', { is_opened: false })
    expect(sentQuery().get('is_opened')).toBe('false')
  })
})

describe('countMessages', () => {
  it('calls messages.count with the filters', async () => {
    vi.mocked(api.get).mockResolvedValue({ total: 3 })
    const counts = await countMessages('ws1', {
      origin: 'broadcast',
      broadcast_id: 'b1',
      created_after: '2026-11-12T10:00:00.000Z'
    })
    const [url] = vi.mocked(api.get).mock.calls.at(-1) as [string]
    expect(url.startsWith('/api/messages.count?')).toBe(true)
    const query = sentQuery()
    expect(query.get('workspace_id')).toBe('ws1')
    expect(query.get('origin')).toBe('broadcast')
    expect(query.get('broadcast_id')).toBe('b1')
    expect(query.get('created_after')).toBe('2026-11-12T10:00:00.000Z')
    expect(counts).toEqual({ total: 3 })
  })

  it('sends neither a cursor nor a limit', async () => {
    await countMessages('ws1', { contact_email: 'ann@example.com' })
    const query = sentQuery()
    expect(query.has('cursor')).toBe(false)
    expect(query.has('limit')).toBe(false)
  })
})
