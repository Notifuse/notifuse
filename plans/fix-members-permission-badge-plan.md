# Members list permission badge — plan

Follow-up to Notifuse/notifuse#482 (full access without a licence). Branch `fix/members-permission-badge`
from `dev` once #482 is merged; PR to `dev`. No SQL, no environment variable, no backend change.

## Problem

`console/src/components/settings/WorkspaceMembers.tsx`, Permissions column:

```ts
const totalPermissions = ALL_PERMISSION_RESOURCES.length * 2
const activePermissions = Object.values(permissions).reduce(...read + write of every stored key...)
if (activePermissions === 0) -> red "No Access" (with popover)
if (activePermissions === totalPermissions) -> green "Full Access" (no popover)
else -> blue `${active}/${total}` (with popover)
```

The numerator counts every stored key, including the opt-in `audit_logs` resource (never part of
full access) and the verbs no endpoint enforces (`llm.read`, `message_history.write`,
`webhook_events.write`, `audit_logs.write` — `UNENFORCED_PERMISSIONS` in `permissions.ts`), which
the console stores as granted on every save (`grantUnenforcedPermissions`). The denominator counts
canonical resources only. Consequences, all reachable from the console:

1. A member put back on full access through the editor (the flow #482 enables without a licence)
   stores `audit_logs.write = true` and shows `29/28` instead of Full Access.
2. A row one canonical verb short plus the stored `audit_logs.write` reaches 28/28 and shows
   **Full Access** while it is not (and the server would refuse to call it full).
3. A row saved after Revoke all holds the four unenforced verbs and shows `4/28`, never No Access.
4. Full access plus Audit logs read shows `30/28`.

## Rule

One definition, the server's: **Full Access = `grantsFullPermissions`** (read and write on every
canonical resource; opt-in resources ignored).

The count measures what the grant lets the holder do, so it counts **enforced verbs only**, over
**canonical and opt-in resources**: numerator = enforced verbs granted, denominator = every enforced
verb the matrix offers. Unenforced verbs are always granted and gate nothing, so counting them
would only shift both sides.

Evaluated in this order (Gemini remark 1): Full Access first, then No Access, then the count — the
count is never compared with the total to decide Full Access.

| Row | Badge |
|---|---|
| `grantsFullPermissions` true, no opt-in grant | green Full Access (no popover, as today) |
| `grantsFullPermissions` true, an enforced opt-in verb granted (Audit logs read) | green Full Access **with the popover** showing the matrix, so the audit grant stays visible from the list |
| no enforced verb granted on any resource | red No Access (popover) |
| otherwise | blue `${granted}/${total}` over enforced verbs (popover) |

No new user-facing string (the catalogue test requires every new string in eight languages).

Helpers in `console/src/services/api/permissions.ts`, next to `grantsFullPermissions`:

- `ENFORCED_PERMISSION_VERBS`: `[resource, verb]` pairs over `[...ALL_PERMISSION_RESOURCES,
  ...OPT_IN_PERMISSION_RESOURCES]` × `read|write` filtered by `isPermissionEnforced`.
- `countEnforcedGrants(p: StoredPermissions | null | undefined): number`.

`WorkspaceMembers.tsx` uses them; `activePermissions` / `totalPermissions` derived from them only.
Comments: on `OPT_IN_PERMISSION_RESOURCES` (`permissions.ts`, "the Full Access tag counts it") and
its twin on `OptInPermissionResources` (`internal/domain/workspace.go`, "the console's Full Access
tag counts it") say the tag *checks* the canonical list (Gemini remark 4; comment-only Go change).

## Tests (written first)

`console/src/services/api/permissions.test.ts`:
- `ENFORCED_PERMISSION_VERBS` holds only pairs where `isPermissionEnforced` is true, includes
  `audit_logs.read`, and its length equals the number of `(resource, verb)` pairs over canonical
  plus opt-in resources for which `isPermissionEnforced` is true (derived in the test, so
  `UNENFORCED_PERMISSIONS` stays private — Gemini remark 2).
- `countEnforcedGrants`: full set → canonical enforced count; Revoke all shape → 0; audit read only → 1;
  null/empty → 0; unenforced verbs never counted.

`console/src/components/settings/WorkspaceMembers.test.tsx` (counts computed from the helpers, never
a literal, per CLAUDE.md):
- full access plus the stored `audit_logs.write` (the shape the editor saves) → Full Access.
- full access plus Audit logs read → Full Access, and hovering shows the popover with the Audit logs row.
- plain full access → Full Access and no popover on hover (Gemini remark 2).
- one canonical verb short plus `audit_logs.write` → not Full Access, count = total − 1.
- the Revoke all shape (`grantUnenforcedPermissions(createEmptyPermissions())`) → No Access.
- Audit logs read only → not No Access, count 1/total.
- existing tests updated: "6/28" and "1/28" become computed from `ENFORCED_PERMISSION_VERBS`.
- end to end through the drawer: put a restricted member back on full access unlicensed, save, and
  the member's new map (as the API mock receives it) renders Full Access.

## Gemini review

Raw: `/mnt/project-files/reviews/fix-members-permission-badge-gemini-brut-plan.md`; verification:
`/mnt/project-files/reviews/fix-members-permission-badge-gemini-verif-plan.md`. Remarks 1, 2 and 4
integrated. Remark on the read-only popover matrix showing unenforced verbs as stored (not as
granted) is pre-existing display and left as is.

## CHANGELOG

One line in the unreleased top section (42.3 if still unreleased when this lands), as an
Improvement: the members list shows Full Access for every full-access member and API key, including
one that can also read audit logs, No Access for one whose permissions were all revoked, and counts
only permissions that grant access.

## Commands

```
cd console && npx vitest run && npx tsc -b --noEmit && npx eslint --max-warnings 0 <touched files> && npm run lingui:extract
```
(no backend change, so no Go suite is required; `make test-unit` is still run once before the PR is
announced ready, as CI does.)
