import { describe, it, expect, vi, beforeEach } from 'vitest'
import { listsApi } from './list'
import { api } from './client'

vi.mock('./client', () => ({
  api: { get: vi.fn().mockResolvedValue({ lists: [] }), post: vi.fn() }
}))

describe('listsApi.list', () => {
  beforeEach(() => vi.mocked(api.get).mockClear())

  it('asks for the counts only when told to', async () => {
    await listsApi.list({ workspace_id: 'ws' })
    expect(vi.mocked(api.get).mock.calls[0][0]).toBe('/api/lists.list?workspace_id=ws')

    await listsApi.list({ workspace_id: 'ws', with_counts: true })
    expect(vi.mocked(api.get).mock.calls[1][0]).toBe('/api/lists.list?workspace_id=ws&with_counts=true')
  })
})
