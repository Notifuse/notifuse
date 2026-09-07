import { describe, it, expect } from 'vitest'
import {
  ALL_PERMISSION_RESOURCES,
  OPT_IN_PERMISSION_RESOURCES,
  PERMISSION_DESCRIPTORS,
  createEmptyPermissions,
  createFullPermissions,
  grantUnenforcedPermissions,
  isPermissionEnforced
} from './permissions'

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
