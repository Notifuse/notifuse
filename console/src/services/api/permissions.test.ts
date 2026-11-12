import { describe, it, expect } from 'vitest'
import {
  ALL_PERMISSION_RESOURCES,
  OPT_IN_PERMISSION_RESOURCES,
  PERMISSION_DESCRIPTORS,
  createEmptyPermissions,
  createFullPermissions,
  grantUnenforcedPermissions,
  grantsFullPermissions,
  isPermissionEnforced,
  ENFORCED_PERMISSION_VERBS,
  countEnforcedGrants,
  canSendTestEmail,
  canCreateTransactionalNotification,
  canEditTransactionalNotification
} from './permissions'
import type { PermissionType } from './permissions'

// What the members list counts: the verbs that gate something, over every resource the matrix
// offers. The unenforced verbs are always granted and gate nothing, so counting them would only
// shift both sides of the badge.
describe('ENFORCED_PERMISSION_VERBS', () => {
  const allPairs = [...ALL_PERMISSION_RESOURCES, ...OPT_IN_PERMISSION_RESOURCES].flatMap((resource) =>
    (['read', 'write'] as PermissionType[]).map((verb) => [resource, verb] as const)
  )

  // Pinned against literals rather than rebuilt from isPermissionEnforced, so a change to the
  // unenforced list or to the resources shows up here instead of moving both sides.
  it('holds every canonical and opt-in verb except the four no endpoint enforces', () => {
    expect(ALL_PERMISSION_RESOURCES).toHaveLength(14)
    expect(OPT_IN_PERMISSION_RESOURCES).toEqual(['audit_logs'])
    const excluded = allPairs.filter(
      ([resource, verb]) => !ENFORCED_PERMISSION_VERBS.some(([r, v]) => r === resource && v === verb)
    )
    expect(excluded.map(([resource, verb]) => `${resource}.${verb}`).sort()).toEqual([
      'audit_logs.write',
      'llm.read',
      'message_history.write',
      'webhook_events.write'
    ])
    expect(ENFORCED_PERMISSION_VERBS).toHaveLength(26)
  })

  it('includes the opt-in Audit logs read and leaves out its unenforced write', () => {
    expect(ENFORCED_PERMISSION_VERBS).toContainEqual(['audit_logs', 'read'])
    expect(ENFORCED_PERMISSION_VERBS).not.toContainEqual(['audit_logs', 'write'])
  })
})

describe('countEnforcedGrants', () => {
  const canonicalEnforced = ENFORCED_PERMISSION_VERBS.filter(([resource]) =>
    ALL_PERMISSION_RESOURCES.includes(resource)
  ).length

  it('counts every canonical enforced verb of a full set, and no unenforced one', () => {
    expect(countEnforcedGrants(grantUnenforcedPermissions(createFullPermissions()))).toBe(
      canonicalEnforced
    )
  })

  it('counts nothing for the set Revoke all saves', () => {
    expect(countEnforcedGrants(grantUnenforcedPermissions(createEmptyPermissions()))).toBe(0)
  })

  it('counts the opt-in Audit logs read', () => {
    expect(countEnforcedGrants({ audit_logs: { read: true, write: true } })).toBe(1)
  })

  it('counts nothing for an empty, null or undefined map', () => {
    expect(countEnforcedGrants({})).toBe(0)
    expect(countEnforcedGrants(null)).toBe(0)
    expect(countEnforcedGrants(undefined)).toBe(0)
  })
})

// Mirrors grantsFullPermissions in internal/service/permissions_predicate.go: the console greys
// out what the server would refuse without the rbac licence, so the two must agree on "full".
describe('grantsFullPermissions', () => {
  it('is true for read and write on every canonical resource', () => {
    expect(grantsFullPermissions(createFullPermissions())).toBe(true)
  })

  it('ignores opt-in resources, which are never part of full access', () => {
    const permissions = {
      ...createFullPermissions(),
      audit_logs: { read: true, write: true }
    }
    expect(grantsFullPermissions(permissions)).toBe(true)
    expect(grantsFullPermissions({ ...permissions, audit_logs: { read: false, write: false } })).toBe(
      true
    )
  })

  it('is false when one verb is missing', () => {
    const permissions = createFullPermissions()
    permissions.contacts = { read: true, write: false }
    expect(grantsFullPermissions(permissions)).toBe(false)
  })

  it('is false when a canonical resource is absent from the map', () => {
    const permissions: Partial<ReturnType<typeof createFullPermissions>> = createFullPermissions()
    delete permissions.segments
    expect(grantsFullPermissions(permissions)).toBe(false)
  })

  it('is false for an empty, null or undefined map', () => {
    expect(grantsFullPermissions({})).toBe(false)
    expect(grantsFullPermissions(null)).toBe(false)
    expect(grantsFullPermissions(undefined)).toBe(false)
    expect(grantsFullPermissions(createEmptyPermissions())).toBe(false)
  })
})

describe('opt-in permission resources', () => {
  it('are never part of full access', () => {
    const full = createFullPermissions()
    for (const resource of OPT_IN_PERMISSION_RESOURCES) {
      expect(ALL_PERMISSION_RESOURCES).not.toContain(resource)
      expect(full[resource]).toBeUndefined()
    }
  })

  it('read is enforced, write is not', () => {
    expect(isPermissionEnforced('audit_logs', 'read')).toBe(true)
    expect(isPermissionEnforced('audit_logs', 'write')).toBe(false)
    const granted = grantUnenforcedPermissions({
      ...createEmptyPermissions(),
      audit_logs: { read: true, write: false }
    })
    expect(granted.audit_logs).toEqual({ read: true, write: true })
  })

  it('has a descriptor naming the read endpoints', () => {
    const descriptor = PERMISSION_DESCRIPTORS.audit_logs
    expect(descriptor.read.endpoints.map((e) => e.endpoint)).toEqual([
      '/api/auditLogs.list',
      '/api/auditLogs.get',
      '/api/auditLogs.export'
    ])
    expect(descriptor.write.endpoints).toEqual([])
  })
})

// transactional.testTemplate checks transactional:write, then reads the template with the
// member's own context (templates:read). Its contact upsert is system-scoped, so contacts:write
// plays no part; tests/integration/transactional_handler_test.go pins both against the server.
describe('canSendTestEmail', () => {
  it('needs templates read and transactional write', () => {
    expect(canSendTestEmail({ templates: { read: true, write: false }, transactional: { read: false, write: true } })).toBe(true)
  })

  it.each([
    ['without templates read', { templates: { read: false, write: true }, transactional: { read: true, write: true } }],
    ['without transactional write', { templates: { read: true, write: true }, transactional: { read: true, write: false } }],
    ['with the contacts write the buttons used to ask for', { templates: { read: true, write: false }, contacts: { read: true, write: true } }],
    ['with no permissions', {}]
  ])('is false %s', (_label, permissions) => {
    expect(canSendTestEmail(permissions)).toBe(false)
  })

  it.each([null, undefined])('is false for %s', (permissions) => {
    expect(canSendTestEmail(permissions)).toBe(false)
  })
})

// transactional.create reads the chosen template with the member's own context (templates:read).
// transactional.update does so only when the body carries channels, which the console's drawer
// sends unless an integration manages the notification. tests/integration/transactional_handler_test.go
// pins these requirements against the server.
describe('canCreateTransactionalNotification', () => {
  it('needs transactional write and templates read', () => {
    expect(canCreateTransactionalNotification({ templates: { read: true, write: false }, transactional: { read: false, write: true } })).toBe(true)
  })

  it.each([
    ['without templates read', { templates: { read: false, write: true }, transactional: { read: true, write: true } }],
    ['without transactional write', { templates: { read: true, write: true }, transactional: { read: true, write: false } }]
  ])('is false %s', (_label, permissions) => {
    expect(canCreateTransactionalNotification(permissions)).toBe(false)
  })

  it.each([null, undefined])('is false for %s', (permissions) => {
    expect(canCreateTransactionalNotification(permissions)).toBe(false)
  })
})

describe('canEditTransactionalNotification', () => {
  const both = { templates: { read: true, write: false }, transactional: { read: false, write: true } }
  const writeOnly = { templates: { read: false, write: false }, transactional: { read: true, write: true } }
  const readTemplatesOnly = { templates: { read: true, write: true }, transactional: { read: true, write: false } }

  it('needs transactional write and templates read for a notification no integration manages', () => {
    expect(canEditTransactionalNotification(both, {})).toBe(true)
    expect(canEditTransactionalNotification(writeOnly, {})).toBe(false)
  })

  // The console sends no channels for a managed notification, so its template is not read.
  it('needs transactional write only for a notification an integration manages', () => {
    expect(canEditTransactionalNotification(writeOnly, { integration_id: 'supabase' })).toBe(true)
  })

  it('treats an empty integration id as not managed, as the drawer and the server do', () => {
    expect(canEditTransactionalNotification(writeOnly, { integration_id: '' })).toBe(false)
    expect(canEditTransactionalNotification(writeOnly, { integration_id: null })).toBe(false)
  })

  it('is false without transactional write, even for a managed notification', () => {
    expect(canEditTransactionalNotification(readTemplatesOnly, { integration_id: 'supabase' })).toBe(false)
    expect(canEditTransactionalNotification(readTemplatesOnly, {})).toBe(false)
  })

  it.each([null, undefined])('is false for %s', (permissions) => {
    expect(canEditTransactionalNotification(permissions, { integration_id: 'supabase' })).toBe(false)
  })
})

describe('templates read catalogue', () => {
  it('lists the transactional endpoints that read a template with the member context', () => {
    const endpoints = PERMISSION_DESCRIPTORS.templates.read.endpoints.map((e) => e.endpoint)
    expect(endpoints).toEqual(
      expect.arrayContaining([
        '/api/transactional.testTemplate',
        '/api/transactional.create',
        '/api/transactional.update'
      ])
    )
  })
  it('lists templates.usage, which answers under Templates read', () => {
    const endpoints = PERMISSION_DESCRIPTORS.templates.read.endpoints.map((e) => e.endpoint)
    expect(endpoints).toContain('/api/templates.usage')
  })
})

describe('blog read catalogue', () => {
  it('lists blogPosts.counts, which answers under Blog read', () => {
    const endpoints = PERMISSION_DESCRIPTORS.blog.read.endpoints.map((e) => e.endpoint)
    expect(endpoints).toContain('/api/blogPosts.counts')
  })
})

// broadcasts.estimateAudience answers to broadcasts read, plus lists read (and segments read
// for an audience with segments), checked by the service.
describe('broadcasts permission descriptor', () => {
  it('names the audience estimate among the read endpoints', () => {
    const endpoints = PERMISSION_DESCRIPTORS.broadcasts.read.endpoints.map((e) => e.endpoint)
    expect(endpoints).toContain('/api/broadcasts.estimateAudience')
  })
})
