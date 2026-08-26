import {
  createEmptyPermissions,
  grantUnenforcedPermissions,
  type PermissionResource,
  type UserPermissions
} from '../../services/api/permissions'

/**
 * The scopes a Zapier connection actually needs, verb by verb.
 *
 * Exported so the test asserts against this list rather than restating it — a test that
 * repeats the grants cannot catch a missing one, which is how `segments` was left out.
 *
 * - `webhook_subscriptions` read and write: every trigger registers and removes its own REST
 *   Hook when the Zap is turned on and off.
 * - `contacts` read and write: the two actions upsert contacts, and every trigger reads
 *   contacts for the sample data Zapier shows while a Zap is being built.
 * - `lists` read and write: the list picker and the subscribe action.
 * - `segments` read only: the segment picker, and the member lookup behind both segment
 *   triggers. Read is the whole of it — nothing in the app writes a segment — and the two
 *   segment triggers are unusable without it.
 *
 * Nothing else is granted, so a leaked Zapier token cannot read message history or send.
 *
 * This lives beside the screen rather than inside it because it is data, not a component:
 * a non-component export in a `.tsx` file breaks React Fast Refresh for that file, which
 * `react-refresh/only-export-components` reports.
 */
export const ZAPIER_KEY_GRANTS: ReadonlyArray<
  readonly [PermissionResource, { read: boolean; write: boolean }]
> = [
  ['webhook_subscriptions', { read: true, write: true }],
  ['contacts', { read: true, write: true }],
  ['lists', { read: true, write: true }],
  ['segments', { read: true, write: false }]
]

export function buildZapierPermissions(): UserPermissions {
  // Start from a denied set so a resource added to the canonical list later is denied by
  // default rather than silently granted to every Zapier key.
  const permissions = grantUnenforcedPermissions(createEmptyPermissions())
  for (const [resource, verbs] of ZAPIER_KEY_GRANTS) {
    permissions[resource] = { ...verbs }
  }
  return permissions
}
