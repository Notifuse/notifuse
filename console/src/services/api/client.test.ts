import { describe, it, expect, vi, afterEach } from 'vitest'

// client.ts imports the router only to redirect on a 401; none of these requests get one.
vi.mock('../../router', () => ({ router: { navigate: vi.fn() } }))

import { api, ApiUnreachableError } from './client'

describe('api client, request with no response', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('reports the base URL it tried, keeping the browser message', async () => {
    window.API_ENDPOINT = 'https://old.example.com/'
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))

    const err = await api
      .post('/api/user.signin', { email: 'root@example.com' })
      .catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiUnreachableError)
    expect((err as ApiUnreachableError).message).toBe('Failed to fetch')
    expect((err as ApiUnreachableError).endpoint).toBe('https://old.example.com')
  })

  it('lets an aborted request through untouched', async () => {
    window.API_ENDPOINT = 'https://old.example.com'
    const abort = new DOMException('The operation was aborted.', 'AbortError')
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(abort))

    await expect(api.get('/api/user.me')).rejects.toBe(abort)
  })
})
