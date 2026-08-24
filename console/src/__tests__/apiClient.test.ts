import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// vi.mock is hoisted above the imports, so the spy has to be hoisted with it.
const { navigate } = vi.hoisted(() => ({ navigate: vi.fn() }))

// The real router module pulls in every page component; the 401 branch only needs
// to know whether navigate() was called.
vi.mock('../router', () => ({
  router: { navigate }
}))

import { api, ApiError } from '../services/api/client'

function jsonResponse(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body
  } as unknown as Response
}

function goTo(url: string) {
  window.history.replaceState(null, '', url)
}

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const error = await promise.catch((e: unknown) => e)
  expect(error).toBeInstanceOf(ApiError)
  return error as ApiError
}

describe('api client 401 handling', () => {
  beforeEach(() => {
    navigate.mockClear()
    localStorage.clear()
    localStorage.setItem('auth_token', 'stale-token')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    goTo('/')
  })

  // The one-click demo link is /console/signin?email=…, and a stale token makes the
  // opening user.me call 401 while that page is still booting. Redirecting from
  // here would rewrite the URL to a bare /console/signin and throw the email away,
  // so the visitor would land on an empty form and only succeed on a second try.
  it('keeps the sign-in URL and its search params when a 401 arrives on the sign-in page', async () => {
    goTo('/console/signin?email=demo@notifuse.com')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(401, { error: 'Session expired or invalid' }))
    )

    await expect(api.get('/api/user.me')).rejects.toBeInstanceOf(ApiError)

    expect(navigate).not.toHaveBeenCalled()
    expect(window.location.pathname).toBe('/console/signin')
    expect(window.location.search).toBe('?email=demo@notifuse.com')
    expect(localStorage.getItem('auth_token')).toBeNull()
  })

  it('still redirects to sign-in when a 401 arrives anywhere else', async () => {
    goTo('/console/workspace/demo/contacts')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(401, { error: 'Session expired or invalid' }))
    )

    await expect(api.get('/api/contacts.list')).rejects.toBeInstanceOf(ApiError)

    expect(navigate).toHaveBeenCalledWith({ to: '/console/signin' })
    expect(localStorage.getItem('auth_token')).toBeNull()
  })

  it('leaves the session alone for non-auth failures', async () => {
    goTo('/console/signin?email=demo@notifuse.com')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(500, { error: 'Failed to verify session' }))
    )

    await expect(api.get('/api/user.me')).rejects.toBeInstanceOf(ApiError)

    expect(navigate).not.toHaveBeenCalled()
    expect(localStorage.getItem('auth_token')).toBe('stale-token')
  })

  it('treats a "Session expired" body as a 401 regardless of status', async () => {
    goTo('/console/workspace/demo/contacts')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(403, { error: 'Session expired' }))
    )

    await expect(api.get('/api/contacts.list')).rejects.toBeInstanceOf(ApiError)

    expect(navigate).toHaveBeenCalledWith({ to: '/console/signin' })
    expect(localStorage.getItem('auth_token')).toBeNull()
  })
})

describe('api client permission denials', () => {
  beforeEach(() => {
    navigate.mockClear()
    localStorage.clear()
    localStorage.setItem('auth_token', 'valid-token')
    goTo('/console/workspace/demo/contacts')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    goTo('/')
  })

  it('replaces the server prose with a translated message, keeping the body on the error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(403, {
          error: 'user does not have write permission on contacts',
          resource: 'contacts',
          permission: 'write'
        })
      )
    )

    const error = await rejection(api.post('/api/contacts.import', {}))

    expect(error.message).toBe('You do not have write access to Contacts.')
    expect(error.data).toEqual({
      error: 'user does not have write permission on contacts',
      resource: 'contacts',
      permission: 'write'
    })
    expect(localStorage.getItem('auth_token')).toBe('valid-token')
  })

  // The quota errors answer 403 too, and CreateWorkspacePage and WorkspaceMembers match on their
  // message to offer an upgrade instead of a generic failure. Their bodies carry no resource, so
  // the rewrite must not reach them.
  it('leaves a quota 403 message untouched', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(403, { error: 'team member limit reached for your plan' })
      )
    )

    const error = await rejection(api.post('/api/workspaces.inviteMember', {}))

    expect(error.message).toBe('team member limit reached for your plan')
    expect(error.message).toContain('team member limit')
  })

  it('leaves a workspace-access 403 message untouched', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(403, { error: 'user is not a member of workspace' }))
    )

    const error = await rejection(api.get('/api/contacts.list'))

    expect(error.message).toBe('user is not a member of workspace')
  })
})
