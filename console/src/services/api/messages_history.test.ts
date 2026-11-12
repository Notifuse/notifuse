import { describe, it, expect, vi, beforeEach } from 'vitest'
import { listMessages } from './messages_history'
import { api } from './client'

vi.mock('./client', () => ({ api: { get: vi.fn().mockResolvedValue({ messages: [] }) } }))

beforeEach(() => vi.clearAllMocks())

describe('listMessages', () => {
  it('sends the automation filter when it is set', async () => {
    await listMessages('ws1', { automation_id: 'a1', template_id: 'tpl' })
    const url = (api.get as ReturnType<typeof vi.fn>).mock.calls[0][0] as string
    const params = new URL(url, 'http://x').searchParams
    expect(params.get('automation_id')).toBe('a1')
    expect(params.get('template_id')).toBe('tpl')
  })

  it('leaves it out otherwise', async () => {
    await listMessages('ws1', {})
    const url = (api.get as ReturnType<typeof vi.fn>).mock.calls[0][0] as string
    expect(new URL(url, 'http://x').searchParams.has('automation_id')).toBe(false)
  })
})
