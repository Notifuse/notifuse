import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { templatesApi } from './template'

vi.mock('./client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

describe('templatesApi.usage', () => {
  beforeEach(() =>
    vi.mocked(api.get).mockReset().mockResolvedValue({ usage: {}, hidden_types: [] })
  )

  it('asks for every template without template_id', async () => {
    await templatesApi.usage({ workspace_id: 'ws 1' })
    expect(api.get).toHaveBeenCalledWith('/api/templates.usage?workspace_id=ws+1')
  })

  it('asks for one template, encoded', async () => {
    await templatesApi.usage({ workspace_id: 'ws1', template_id: 'a&b' })
    expect(api.get).toHaveBeenCalledWith('/api/templates.usage?workspace_id=ws1&template_id=a%26b')
  })
})
