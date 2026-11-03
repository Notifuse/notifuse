import { describe, it, expect } from 'vitest'
import {
  ALL_PERMISSION_RESOURCES,
  OPT_IN_PERMISSION_RESOURCES,
  PERMISSION_DESCRIPTORS,
  createEmptyPermissions,
  createFullPermissions,
  grantUnenforcedPermissions,
  grantsFullPermissions,
  isPermissionEnforced
} from './permissions'

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
