import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { filesApi } from './files'

// The service module imports the api client, which pulls in the router.
vi.mock('./client', () => ({
  api: { post: vi.fn(), get: vi.fn() }
}))

beforeEach(() => {
  vi.mocked(api.get).mockReset()
  vi.mocked(api.post).mockReset()
})

const sentQuery = (url: string): URLSearchParams => new URLSearchParams(url.split('?')[1])

describe('filesApi', () => {
  it('lists the whole bucket without a prefix parameter', async () => {
    vi.mocked(api.get).mockResolvedValue({ objects: [], capped: false, cut_short: false })

    await filesApi.list('ws1')

    const url = vi.mocked(api.get).mock.calls[0][0]
    expect(url.startsWith('/api/files.list?')).toBe(true)
    expect([...sentQuery(url).keys()]).toEqual(['workspace_id'])
  })

  it('encodes a prefix with spaces and plus signs', async () => {
    vi.mocked(api.get).mockResolvedValue({ objects: [], capped: false, cut_short: false })

    await filesApi.list('ws1', 'my folder/a+b/')

    expect(sentQuery(vi.mocked(api.get).mock.calls[0][0]).get('prefix')).toBe('my folder/a+b/')
  })

  it('posts each write to its endpoint with the workspace', async () => {
    vi.mocked(api.post).mockResolvedValue({})

    await filesApi.presignUpload({ workspace_id: 'ws1', key: 'a.png', content_type: 'image/png', size: 3 })
    await filesApi.createFolder('ws1', 'photos/')
    await filesApi.delete('ws1', ['a.png'])
    await filesApi.deleteFolder('ws1', 'photos/')
    await filesApi.testSettings('ws1', { endpoint: 'https://s3.example.com', access_key: 'AK', bucket: 'b' })

    expect(vi.mocked(api.post).mock.calls).toEqual([
      ['/api/files.presignUpload', { workspace_id: 'ws1', key: 'a.png', content_type: 'image/png', size: 3 }],
      ['/api/files.createFolder', { workspace_id: 'ws1', key: 'photos/' }],
      ['/api/files.delete', { workspace_id: 'ws1', keys: ['a.png'] }],
      ['/api/files.deleteFolder', { workspace_id: 'ws1', prefix: 'photos/' }],
      [
        '/api/workspaces.testFileManager',
        { workspace_id: 'ws1', settings: { endpoint: 'https://s3.example.com', access_key: 'AK', bucket: 'b' } }
      ]
    ])
  })
})
