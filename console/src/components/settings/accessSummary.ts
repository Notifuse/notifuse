import {
  ALL_PERMISSION_RESOURCES,
  OPT_IN_PERMISSION_RESOURCES,
  ENFORCED_PERMISSION_VERBS,
  countEnforcedGrants,
  createEmptyPermissions,
  grantsFullPermissions,
  grantUnenforcedPermissions,
  type PermissionResource,
  type StoredPermissions
} from '../../services/api/permissions'
import type { WorkspaceMember } from '../../services/api/workspace'

/** How many resources the summary of a partial grant names before "+n". */
export const ACCESS_SUMMARY_NAMED = 2

export type AccessSummary =
  | { kind: 'owner' }
  | { kind: 'pending' }
  /** `optIn`: an opt-in permission (Audit logs) comes on top, so the details stay one hover away. */
  | { kind: 'full'; optIn: boolean }
  | { kind: 'none' }
  | {
      kind: 'partial'
      /** The first resources holding an enforced grant, in the matrix order. */
      resources: PermissionResource[]
      /** Resources with a grant beyond the named ones. */
      more: number
      /** No enforced write on any resource. */
      readOnly: boolean
      /** Enforced verbs granted, over all of them. */
      active: number
      total: number
    }

/**
 * What the Access column says about a member or an API key. Full access is the server's
 * definition, with the unenforced verbs counted as granted as the editor saves them; it is checked
 * before the count and never inferred from it. Only enforced verbs count: an unenforced verb gates
 * nothing.
 */
export function summarizeAccess(member: WorkspaceMember): AccessSummary {
  if (member.role === 'owner') return { kind: 'owner' }
  if (member.invitation_expires_at) return { kind: 'pending' }

  // The row's own map may be partial or null: a resource it does not mention is denied.
  const permissions: StoredPermissions = member.permissions ?? createEmptyPermissions()

  if (grantsFullPermissions(grantUnenforcedPermissions({ ...createEmptyPermissions(), ...permissions }))) {
    const optIn = ENFORCED_PERMISSION_VERBS.some(
      ([resource, verb]) => OPT_IN_PERMISSION_RESOURCES.includes(resource) && permissions[resource]?.[verb] === true
    )
    return { kind: 'full', optIn }
  }

  const active = countEnforcedGrants(permissions)
  if (active === 0) return { kind: 'none' }

  const granted = (resource: PermissionResource, verb: 'read' | 'write') =>
    permissions[resource]?.[verb] === true &&
    ENFORCED_PERMISSION_VERBS.some(([r, v]) => r === resource && v === verb)

  const withGrant = [...ALL_PERMISSION_RESOURCES, ...OPT_IN_PERMISSION_RESOURCES].filter(
    (resource) => granted(resource, 'read') || granted(resource, 'write')
  )
  return {
    kind: 'partial',
    resources: withGrant.slice(0, ACCESS_SUMMARY_NAMED),
    more: Math.max(0, withGrant.length - ACCESS_SUMMARY_NAMED),
    readOnly: !withGrant.some((resource) => granted(resource, 'write')),
    active,
    total: ENFORCED_PERMISSION_VERBS.length
  }
}
