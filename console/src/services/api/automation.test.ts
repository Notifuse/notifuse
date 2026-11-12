import { describe, it, expect, vi, beforeEach } from 'vitest'
import { automationApi } from './automation'
import { api } from './client'

vi.mock('./client', () => ({
  api: { get: vi.fn().mockResolvedValue({}), post: vi.fn().mockResolvedValue({ success: true }) }
}))

beforeEach(() => vi.clearAllMocks())

describe('automationApi', () => {
  it('lists every automation with its journey counts, without a limit', async () => {
    await automationApi.list({ workspace_id: 'ws1', with_journey_counts: true })
    expect(api.get).toHaveBeenCalledWith('/api/automations.list?workspace_id=ws1&with_journey_counts=true')
  })

  it('lists without journey counts unless asked', async () => {
    await automationApi.list({ workspace_id: 'ws1' })
    expect(api.get).toHaveBeenCalledWith('/api/automations.list?workspace_id=ws1')
  })

  it('reads the step results over a period', async () => {
    await automationApi.nodeStats({
      workspace_id: 'ws1',
      automation_id: 'a1',
      since: '2026-10-12T22:00:00.000Z',
      until: '2026-11-11T22:59:59.999Z'
    })
    expect(api.get).toHaveBeenCalledWith(
      '/api/automations.nodeStats?workspace_id=ws1&automation_id=a1&since=2026-10-12T22%3A00%3A00.000Z&until=2026-11-11T22%3A59%3A59.999Z'
    )
  })

  it('reads the step results of all time without bounds', async () => {
    await automationApi.nodeStats({ workspace_id: 'ws1', automation_id: 'a1' })
    expect(api.get).toHaveBeenCalledWith('/api/automations.nodeStats?workspace_id=ws1&automation_id=a1')
  })

  it('activates and pauses by id, answering success', async () => {
    await expect(automationApi.activate({ workspace_id: 'ws1', automation_id: 'a1' })).resolves.toEqual({ success: true })
    expect(api.post).toHaveBeenCalledWith('/api/automations.activate', { workspace_id: 'ws1', automation_id: 'a1' })
    await automationApi.pause({ workspace_id: 'ws1', automation_id: 'a1' })
    expect(api.post).toHaveBeenLastCalledWith('/api/automations.pause', { workspace_id: 'ws1', automation_id: 'a1' })
  })
})
