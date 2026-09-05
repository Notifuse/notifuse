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

// The 402 half of the error taxonomy, driven through the client rather than tested in
// isolation.
//
// licenseErrors.ts is well covered on its own and so is every component that renders a
// refusal, but nothing exercised the wire between them: handleResponse decides whether a body
// is a licence refusal at all, and substitutes the console's sentence for the server's. Delete
// that and every 402 in the product shows the user the raw wire code `license_required` as its
// error message, with the whole suite green.
describe('api client licence refusals', () => {
  beforeEach(() => {
    navigate.mockClear()
    localStorage.clear()
    localStorage.setItem('auth_token', 'valid-token')
    goTo('/console/workspace/demo/settings')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    goTo('/')
  })

  // The exact body internal/http/utils.go writeLicenseRequired emits.
  const refusalBody = {
    error: 'license_required',
    feature: 'ses_tenant',
    required_tier: 'Studio',
    message: 'SES tenant isolation requires a Notifuse licence (Studio or above).',
    docs: 'https://notifuse.com/licence-features'
  }

  it('names the capability instead of showing the wire code', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(402, refusalBody)))

    const error = await rejection(api.post('/api/ses.enableTenantIsolation', {}))

    expect(error.status).toBe(402)
    expect(error.message).toBe('SES tenant isolation requires a Notifuse Studio licence.')
    // Never the raw code, which is what a dropped branch would surface.
    expect(error.message).not.toContain('license_required')
    // The body travels untouched, so a component that wants the feature or the docs link
    // still reads them off `data`.
    expect(error.data).toEqual(refusalBody)
  })

  // The workspace ceiling carries no required_tier — which plan lifts it depends on how many
  // workspaces already exist — so its sentence has to stand without one.
  it('explains the workspace ceiling without quoting a plan', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(402, {
          error: 'license_required',
          feature: 'workspaces',
          message: 'workspace quota reached: 3 workspaces exist (limit: 3)',
          docs: 'https://notifuse.com/licence-features'
        })
      )
    )

    const error = await rejection(api.post('/api/workspaces.create', {}))

    expect(error.message).toContain('workspaces its licence allows')
    expect(error.message).toContain('Existing workspaces are unaffected')
  })

  // A gate this bundle has no label for falls through to the server's sentence rather than
  // inventing a name for something it cannot name.
  it('falls back to the server sentence for a capability it does not know', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(402, {
          error: 'license_required',
          feature: 'a_capability_from_2028',
          message: 'That capability requires a Notifuse licence.',
          docs: 'https://notifuse.com/licence-features'
        })
      )
    )

    const error = await rejection(api.post('/api/anything', {}))
    expect(error.message).toBe('That capability requires a Notifuse licence.')
  })

  // Detection is by the `error` field, never by the status. The two refusals are different
  // questions — 403 says the signed-in user lacks a grant, which no amount of money fixes —
  // and handleResponse computes the licence refusal only when there is no permission denial.
  it('does not read a permission denial as a licence refusal', async () => {
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

    expect(error.status).toBe(403)
    expect(error.message).toBe('You do not have write access to Contacts.')
    expect(error.message).not.toContain('licence')
  })

  // And the reverse: a 402 whose body is not the agreed code keeps whatever message it came
  // with, so an unrelated payment error is not dressed up as a licence refusal.
  it('leaves a 402 that is not a licence refusal alone', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse(402, { error: 'card declined' }))
    )

    const error = await rejection(api.post('/api/anything', {}))
    expect(error.message).toBe('card declined')
  })
})
