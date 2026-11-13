import { describe, it, expect } from 'vitest'
import { summarizeAccess } from './accessSummary'
import {
  createEmptyPermissions,
  createFullPermissions,
  ENFORCED_PERMISSION_VERBS,
  type StoredPermissions
} from '../../services/api/permissions'
import type { WorkspaceMember } from '../../services/api/workspace'

const member = (overrides: Partial<WorkspaceMember>): WorkspaceMember => ({
  user_id: 'u1',
  workspace_id: 'w1',
  role: 'member',
  email: 'a@b.c',
  type: 'user',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  permissions: createEmptyPermissions(),
  ...overrides
})

const withGrants = (grants: StoredPermissions): StoredPermissions => ({ ...createEmptyPermissions(), ...grants })

describe('summarizeAccess', () => {
  it('reads an owner as owner, whatever its permissions', () => {
    expect(summarizeAccess(member({ role: 'owner', permissions: undefined }))).toEqual({ kind: 'owner' })
  })

  it('reads an invitation as pending', () => {
    expect(
      summarizeAccess(member({ user_id: '', invitation_id: 'i1', invitation_expires_at: '2026-12-01T00:00:00Z' }))
    ).toEqual({ kind: 'pending' })
  })

  it('reads full access, and says when an opt-in permission comes on top', () => {
    expect(summarizeAccess(member({ permissions: createFullPermissions() }))).toEqual({ kind: 'full', optIn: false })
    expect(
      summarizeAccess(
        member({ permissions: { ...createFullPermissions(), audit_logs: { read: true, write: false } } })
      )
    ).toEqual({ kind: 'full', optIn: true })
  })

  it('reads no enforced grant as no access, a missing map included', () => {
    expect(summarizeAccess(member({ permissions: createEmptyPermissions() }))).toEqual({ kind: 'none' })
    expect(summarizeAccess(member({ permissions: undefined }))).toEqual({ kind: 'none' })
  })

  it('names the first two resources with a grant and counts the others', () => {
    const summary = summarizeAccess(
      member({
        permissions: withGrants({
          contacts: { read: true, write: true },
          transactional: { read: true, write: false },
          templates: { read: true, write: false },
          blog: { read: true, write: false },
          segments: { read: true, write: false }
        })
      })
    )
    expect(summary).toMatchObject({
      kind: 'partial',
      resources: ['contacts', 'segments'],
      more: 3,
      readOnly: false
    })
  })

  it('lists the resources in the matrix order, opt-in ones last', () => {
    const summary = summarizeAccess(
      member({
        permissions: withGrants({
          audit_logs: { read: true, write: false },
          webhook_events: { read: true, write: false }
        })
      })
    )
    expect(summary).toMatchObject({ kind: 'partial', resources: ['webhook_events', 'audit_logs'], more: 0 })
  })

  it('names Audit logs for a grant limited to them, not an empty summary', () => {
    const summary = summarizeAccess(member({ permissions: withGrants({ audit_logs: { read: true, write: false } }) }))
    expect(summary).toMatchObject({ kind: 'partial', resources: ['audit_logs'], more: 0, readOnly: true })
  })

  it('says read only without any write, and not with one', () => {
    expect(
      summarizeAccess(member({ permissions: withGrants({ contacts: { read: true, write: false } }) }))
    ).toMatchObject({ readOnly: true })
    expect(
      summarizeAccess(
        member({
          permissions: withGrants({ contacts: { read: true, write: false }, lists: { read: false, write: true } })
        })
      )
    ).toMatchObject({ readOnly: false })
  })

  it('keeps the count of granted verbs over the enforced ones, for the tooltip', () => {
    const summary = summarizeAccess(
      member({ permissions: withGrants({ contacts: { read: true, write: true }, lists: { read: true, write: false } }) })
    )
    expect(summary).toMatchObject({ kind: 'partial', active: 3, total: ENFORCED_PERMISSION_VERBS.length })
  })
})
