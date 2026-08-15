import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { annotationService, ANNOTATIONS_QUERY_KEY, type Annotation } from './annotation'

// The service module imports the api client, which pulls in the router.
vi.mock('./client', () => ({
  api: { post: vi.fn(), get: vi.fn() }
}))

const annotation: Annotation = {
  id: 'a1',
  annotated_at: '2026-08-15T09:00:00Z',
  timezone: 'Asia/Tokyo',
  title: 'Launch',
  color: '#3b82f6',
  source: 'manual',
  created_at: '2026-08-15T09:00:00Z',
  updated_at: '2026-08-15T09:00:00Z'
}

/** The query string the client actually sent, parsed back into params. */
const sentQuery = (call: string): URLSearchParams => {
  const [, query] = call.split('?')
  return new URLSearchParams(query)
}

beforeEach(() => {
  vi.mocked(api.get).mockReset()
  vi.mocked(api.post).mockReset()
})

describe('annotationService.list', () => {
  it('sends only the workspace when no filter is given', async () => {
    vi.mocked(api.get).mockResolvedValue({ annotations: [] })

    await annotationService.list({ workspace_id: 'ws1' })

    const url = vi.mocked(api.get).mock.calls[0][0]
    expect(url.startsWith('/api/annotations.list?')).toBe(true)
    expect([...sentQuery(url).keys()]).toEqual(['workspace_id'])
    expect(sentQuery(url).get('workspace_id')).toBe('ws1')
  })

  it('serialises sources as a single comma-separated param', async () => {
    vi.mocked(api.get).mockResolvedValue({ annotations: [] })

    await annotationService.list({
      workspace_id: 'ws1',
      start: '2026-08-01T00:00:00Z',
      end: '2026-08-31T23:59:59Z',
      sources: ['manual', 'broadcast'],
      limit: 500
    })

    const params = sentQuery(vi.mocked(api.get).mock.calls[0][0])
    expect(params.getAll('sources')).toEqual(['manual,broadcast'])
    expect(params.get('start')).toBe('2026-08-01T00:00:00Z')
    expect(params.get('end')).toBe('2026-08-31T23:59:59Z')
    expect(params.get('limit')).toBe('500')
  })

  it('omits an empty sources array rather than sending a blank param', async () => {
    vi.mocked(api.get).mockResolvedValue({ annotations: [] })

    await annotationService.list({ workspace_id: 'ws1', sources: [] })

    expect(sentQuery(vi.mocked(api.get).mock.calls[0][0]).has('sources')).toBe(false)
  })

  it('unwraps the annotations envelope', async () => {
    vi.mocked(api.get).mockResolvedValue({ annotations: [annotation] })

    await expect(annotationService.list({ workspace_id: 'ws1' })).resolves.toEqual([annotation])
  })
})

describe('annotationService.get', () => {
  it('sends the workspace and id, and unwraps the envelope', async () => {
    vi.mocked(api.get).mockResolvedValue({ annotation })

    await expect(annotationService.get('ws1', 'a1')).resolves.toEqual(annotation)

    const url = vi.mocked(api.get).mock.calls[0][0]
    expect(url.startsWith('/api/annotations.get?')).toBe(true)
    const params = sentQuery(url)
    expect(params.get('workspace_id')).toBe('ws1')
    expect(params.get('id')).toBe('a1')
  })
})

describe('annotationService.create', () => {
  it('posts the request body and unwraps the envelope', async () => {
    vi.mocked(api.post).mockResolvedValue({ annotation })

    const params = {
      workspace_id: 'ws1',
      annotated_at: '2026-08-15T09:00:00Z',
      timezone: 'Asia/Tokyo',
      title: 'Launch'
    }
    await expect(annotationService.create(params)).resolves.toEqual(annotation)
    expect(api.post).toHaveBeenCalledWith('/api/annotations.create', params)
  })
})

describe('annotationService.update', () => {
  it('posts the request body and unwraps the envelope', async () => {
    vi.mocked(api.post).mockResolvedValue({ annotation })

    const params = {
      workspace_id: 'ws1',
      id: 'a1',
      annotated_at: '2026-08-15T09:00:00Z',
      title: 'Launch, renamed'
    }
    await expect(annotationService.update(params)).resolves.toEqual(annotation)
    expect(api.post).toHaveBeenCalledWith('/api/annotations.update', params)
  })
})

describe('annotationService.delete', () => {
  it('posts the workspace and id', async () => {
    vi.mocked(api.post).mockResolvedValue({ success: true })

    await annotationService.delete('ws1', 'a1')

    expect(api.post).toHaveBeenCalledWith('/api/annotations.delete', {
      workspace_id: 'ws1',
      id: 'a1'
    })
  })
})

describe('ANNOTATIONS_QUERY_KEY', () => {
  it('is the prefix the tab invalidates on', () => {
    expect(ANNOTATIONS_QUERY_KEY).toBe('annotations')
  })
})
