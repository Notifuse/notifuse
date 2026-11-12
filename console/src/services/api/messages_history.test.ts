import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { listMessages } from './messages_history'

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
