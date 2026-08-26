import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from './client'
import { workspaceService, type CreateAPIKeyRequest } from './workspace'
import {
  ALL_PERMISSION_RESOURCES,
  createEmptyPermissions,
  createFullPermissions
} from './permissions'

// The service module imports the api client, which pulls in the router.
vi.mock('./client', () => ({
  api: { post: vi.fn(), get: vi.fn() }
}))

beforeEach(() => {
  vi.mocked(api.post).mockReset()
  vi.mocked(api.post).mockResolvedValue({ token: 'tok', email: 'sender@api.example.com' })
})

/** The body the client hands to `fetch`: `api.post` JSON-encodes whatever it is given. */
const sentBody = (): Record<string, unknown> =>
  JSON.parse(JSON.stringify(vi.mocked(api.post).mock.calls[0][1]))

describe('workspaceService.createAPIKey', () => {
  it('serialises the scope when one is given', async () => {
    const permissions = createFullPermissions()
    permissions.contacts = { read: true, write: false }

    await workspaceService.createAPIKey({
      workspace_id: 'ws1',
      email_prefix: 'zapier',
      permissions
    })

    expect(vi.mocked(api.post).mock.calls[0][0]).toBe('/api/workspaces.createAPIKey')
    const body = sentBody()
    expect(body.workspace_id).toBe('ws1')
    expect(body.email_prefix).toBe('zapier')
    expect(body.permissions).toEqual(permissions)
  })

  it('omits the scope entirely when none is given, which the server reads as full access', async () => {
    await workspaceService.createAPIKey({ workspace_id: 'ws1', email_prefix: 'legacy' })

    // Not `permissions: null` and not an empty map — an empty map is a real grant of nothing on
    // the server, so the key has to be absent from the encoded body for the default to hold.
    expect(Object.keys(sentBody()).sort()).toEqual(['email_prefix', 'workspace_id'])
  })

  it('drops an explicitly undefined scope rather than encoding it', async () => {
    const request: CreateAPIKeyRequest = {
      workspace_id: 'ws1',
      email_prefix: 'legacy',
      permissions: undefined
    }

    await workspaceService.createAPIKey(request)

    expect('permissions' in sentBody()).toBe(false)
  })

  it('carries a narrow integration scope through untouched', async () => {
    // A narrow grant is still a complete map with three resources switched on — exactly what the
    // Zapier screen builds. Naming only the three would be a different request: the server denies
    // whatever a scope leaves out, so a short map is indistinguishable from a deliberate denial.
    const permissions = createEmptyPermissions()
    permissions.contacts = { read: true, write: true }
    permissions.lists = { read: true, write: true }
    permissions.webhook_subscriptions = { read: true, write: true }

    await workspaceService.createAPIKey({
      workspace_id: 'ws1',
      email_prefix: 'zapier',
      permissions
    })

    const sent = sentBody().permissions as Record<string, unknown>
    expect(sent).toEqual(permissions)
    // Nothing is trimmed on the way out: a denied resource has to reach the wire as an explicit
    // `false`, not by being dropped from the body.
    expect(Object.keys(sent).sort()).toEqual([...ALL_PERMISSION_RESOURCES].sort())
  })
})

describe('workspaceService.inviteMember', () => {
  it('sends the grant it is handed, naming every canonical resource', async () => {
    vi.mocked(api.post).mockResolvedValue({ status: 'ok', message: '' })

    await workspaceService.inviteMember({
      workspace_id: 'ws1',
      email: 'member@example.com',
      permissions: createFullPermissions()
    })

    expect(vi.mocked(api.post).mock.calls[0][0]).toBe('/api/workspaces.inviteMember')
    const permissions = sentBody().permissions as Record<string, unknown>
    // A resource the map leaves out is denied on the server, so a short map is a silent strip.
    expect(Object.keys(permissions).sort()).toEqual([...ALL_PERMISSION_RESOURCES].sort())
  })
})
