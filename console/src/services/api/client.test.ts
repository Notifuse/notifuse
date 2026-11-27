import { describe, it, expect, vi, afterEach } from 'vitest'

// client.ts imports the router only to redirect on a 401; none of these requests get one.
vi.mock('../../router', () => ({ router: { navigate: vi.fn() } }))

import { api, ApiError, ApiUnreachableError } from './client'

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

describe('api client, error answer with no message of its own', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  const answer = (status: number, body: string, contentType: string) =>
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response(body, { status, headers: { 'Content-Type': contentType } }))
    )

  // What Traefik answers while a redeploy has removed the old container and the new one is not
  // registered yet; nginx and load balancers send their own HTML pages for the same statuses.
  it.each([
    [502, 'Bad Gateway'],
    [503, 'no available server'],
    [504, 'Gateway Timeout']
  ])('says the server is not responding for a %i the API did not write', async (status, body) => {
    answer(status, body, 'text/plain; charset=utf-8')

    const err = await api
      .post('/api/user.twoFactor.verify', { challenge_token: 'c', code: '123456' })
      .catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(status)
    expect((err as ApiError).message).toBe("The server isn't responding. Try again in a few seconds.")
    expect((err as ApiError).data).toBeNull()
  })

  it("keeps the API's own message on a 503 it wrote", async () => {
    answer(503, JSON.stringify({ error: 'OIDC is not available' }), 'application/json')

    const err = await api.get('/api/oidc.config').catch((e: unknown) => e)

    expect((err as ApiError).message).toBe('OIDC is not available')
  })

  it('says the server is not responding for a gateway JSON body with no error key', async () => {
    answer(502, JSON.stringify({ message: 'upstream connect error' }), 'application/json')

    const err = await api.get('/api/user.me').catch((e: unknown) => e)

    expect((err as ApiError).message).toBe("The server isn't responding. Try again in a few seconds.")
  })

  it.each([
    [500, '<html><body>Internal Server Error</body></html>', 'text/html'],
    [404, '404 page not found', 'text/plain'],
    [413, '<html><body>Request Entity Too Large</body></html>', 'text/html']
  ])('keeps the generic sentence for a %i with no message', async (status, body, contentType) => {
    answer(status, body, contentType)

    const err = await api.post('/api/contacts.import', {}).catch((e: unknown) => e)

    expect((err as ApiError).status).toBe(status)
    expect((err as ApiError).message).toBe('An error occurred')
  })
})
