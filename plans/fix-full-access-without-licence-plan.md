# Full access never needs a licence — plan

## Decision (Pierre, 2026-11-03)

Without a licence covering `rbac`, a permission set granting **full access** is always accepted;
only a **restricted** set needs the licence. The rule is the same for inviting a member, creating
an API key, and changing the permissions of an existing member or API key.

## Current state

| Write path | Route | Gate today |
|---|---|---|
| `InviteMember` (`internal/service/workspace_service.go` ~:817) | `/api/workspaces.inviteMember` | `!grantsFullPermissions(p) && !licensed` ✓ |
| `CreateAPIKey` (~:1494) | `/api/workspaces.createAPIKey` | same rule, nil = full ✓ |
| `AddUserToWorkspace` (~:657) | none (service only) | same rule ✓ |
| `SetUserPermissions` (~:1004) | `/api/workspaces.setUserPermissions` (`internal/http/workspace_handler.go:86`) | **refused outright** when unlicensed, even for a full set ✗ |

The outright refusal shipped in v40.0 (commit `50565f3`), so it is released behaviour and the
CHANGELOG entry is a **Fix**. `main` is at 42.1; `dev` carries the unreleased 42.2 and 42.3, so the
entry goes in the top section, `[42.3]`, and `VERSION` is not bumped.

Console today:

- `EditPermissionsDrawer.tsx`: unlicensed → whole matrix disabled **and** Save disabled; no
  presets. An unlicensed owner cannot put a restricted member back on full access at all.
- `WorkspaceMembers.tsx`, invite drawer: matrix disabled, but the `Grant all` / `Revoke all`
  presets stay live, so `Revoke all` builds a restricted set that the server then refuses (402).
- `WorkspaceMembers.tsx`, create-API-key drawer: no licence notice, nothing greyed; a restricted
  set is only refused by the server on Create.
- `LicenceGateNotice.tsx` rbac fallback ("any member can still be given full access") is false
  today for existing members, and becomes true with this change. Wording kept as is.

No SQL table and no environment variable are touched.

## Backend

### `internal/service/workspace_service.go` — `SetUserPermissions`

Replace

```go
if !s.isLicensedFor(domain.FeatureRBAC) { return domain.NewFeatureNotLicensedError(domain.FeatureRBAC) }
```

with

```go
if !grantsFullPermissions(permissions) && !s.isLicensedFor(domain.FeatureRBAC) { ... }
```

Order kept: authenticate → owner check → `permissions.Validate()` → licence gate → read the target
membership row. A non-owner (or a non-member, refused by `AuthenticateUserForWorkspace`) never
reaches the gate, so nobody outside the owner role learns the licence state through a 402.
The gate stays before `GetUserWorkspace`, so a refused write never reads the target row.
Comment above it rewritten: full access is never gated; a restricted set is; stored restricted
rows keep being enforced; nil/empty maps are not full (they deny everything) so they are gated,
unlike `CreateAPIKey`'s nil-means-full contract.

`grantsFullPermissions` compares over `domain.AllPermissionResources` only, so a full set plus the
opt-in `audit_logs` grant is still full and accepted unlicensed, exactly as for invitations and
API keys today.

Tests — `internal/service/workspace_service_test.go`, `TestWorkspaceService_RBACLicenceGate`:

- replace "SetUserPermissions is refused even when the map is full" with
  **"SetUserPermissions accepts full access without the licence"**: unlicensed, owner, full map →
  `GetUserWorkspace`, `UpdateUserWorkspacePermissions` (written map == `domain.NewFullPermissions()`),
  `GetSessionsByUserID` called; no error.
- **full access plus the opt-in audit_logs grant is accepted without the licence**.
- rename "refused outright" to **"SetUserPermissions refuses a restricted set without the
  licence"**: unlicensed, restricted → `FeatureNotLicensedError{rbac}`, no repository call
  (gomock fails on any unexpected call).
- **an empty map is refused without the licence** (empty is not full on this path).
- keep **"writes the grant when licensed"** (restricted, licensed) and add **"licensed, full set
  accepted"**.
- **a non-owner gets ErrUnauthorized, not the licence refusal**, unlicensed, restricted set:
  pins the ordering (owner check before gate).
- **an invalid map is refused by Validate before the licence**, unlicensed: unknown resource →
  validation error, not `ErrFeatureNotLicensed`.

### `internal/http/workspace_handler.go`

No code change (402 mapping for this handler already goes through `writeServiceError`).

Tests — `internal/http/workspace_handler_test.go`, new `TestWorkspaceHandler_HandleSetUserPermissions_LicenceRule`
with **raw JSON bodies** (string literals, not `json.Marshal` of the struct):

- full-access body built from `domain.AllPermissionResources` → the mock receives a map equal to
  `domain.NewFullPermissions()` → 200 `{"status":"success"}`.
- restricted body → mock returns `NewFeatureNotLicensedError(rbac)` → 402 with
  `error=license_required`, `feature=rbac`, `required_tier`, `docs`.
- body with an unknown resource → 400, service never called.

### Integration — `tests/integration/license_gate_test.go`

New subtest in `TestLicenseGates`, **"an unlicensed server puts a restricted member back on full
access and still refuses a restricted set"**, on `unlicensed(nil)`:

1. root signs in, creates a workspace.
2. creates an API key with no permissions (full access, allowed unlicensed) → key user id.
3. simulates a licence that lapsed: writes a restricted map directly into
   `user_workspaces.permissions` for that key (through the system DB the suite exposes); asserts
   the key now gets 403 on a resource outside the map.
4. `POST /api/workspaces.setUserPermissions` with a restricted set → **402**, `license_required`,
   `feature=rbac`; stored row unchanged.
5. same call with full access → **200**; the stored row equals full access; the key's token now
   gets 200 on the resource it was denied.

Run with `INTEGRATION_TESTS=true go test -tags integration,licdev ./tests/integration -run TestLicenseGates`.

## Console

### `console/src/services/api/permissions.ts`

Add `grantsFullPermissions(p: StoredPermissions | null | undefined): boolean` — read and write on
every resource of `ALL_PERMISSION_RESOURCES`, mirroring the backend predicate (opt-in resources
ignored). Test: `console/src/services/api/permissions.test.ts` (full → true; full + audit_logs →
true; one verb missing → false; empty/null → false; a missing resource → false).

### `console/src/components/settings/PermissionScopePresets.tsx` (moved out of WorkspaceMembers)

`Grant all` / `Revoke all`, plus `fullAccessOnly?: boolean`: when true, `Revoke all` is disabled
(it can only produce a restricted set); `Grant all` stays live. `Grant all` now receives the
current value and keeps its opt-in grants (`audit_logs`): replacing the map with
`createFullPermissions()` would silently strip Audit logs read from an existing member once the
presets are offered in the member editor (Gemini remark 3).

### `console/src/components/settings/PermissionsMatrix.tsx`

New prop `fullAccessOnly?: boolean`. When true, a switch is disabled unless flipping it leaves
the set at full access (`grantsFullPermissions(next)`), so:

- on a full set, every switch of `ALL_PERMISSION_RESOURCES` is greyed (turning one off narrows),
  and the opt-in Audit logs read switch stays live (it never affects "full");
- on a restricted set, a switch whose flip completes full access stays live; the rest are greyed.

`disabled` keeps its meaning (everything locked). The next-map construction is factored out of
`setPermission` so the predicate and the change use the same map.

### `EditPermissionsDrawer.tsx`

Unlicensed: notice shown; presets shown with `fullAccessOnly`; matrix `fullAccessOnly` (no longer
`disabled`); Save enabled only when the set is full access (`locked && !grantsFullPermissions`).
Licensed: unchanged behaviour (presets added on both, so a licensed owner also gets Grant/Revoke).

### `WorkspaceMembers.tsx`

- Invite drawer: matrix `fullAccessOnly` instead of `disabled`; presets `fullAccessOnly`; Send
  disabled when unlicensed and the set is not full.
- Create-API-key drawer: add `LicenceGateNotice feature="rbac"`; matrix and presets
  `fullAccessOnly` when unlicensed; Create disabled when unlicensed and the set is not full.

- Permissions column badge: unchanged in the end. Gemini remark 4 (a full row holding Audit
  logs shows `29/28`) was implemented, then reverted to keep this change on the licence rule after the third high
  review kept finding display edge cases in that badge (see "Known, not treated").

### `LicenceGateNotice.tsx`

Text unchanged (it is now accurate). Update the docblock comment only if needed.

### Console tests (Vitest)

`EditPermissionsDrawer.test.tsx`:
- unlicensed, member holding a restricted map: notice shown; Save disabled; `Revoke all` disabled;
  `Grant all` enabled; clicking `Grant all` enables Save; Save posts a map where every canonical
  resource is read+write.
- unlicensed, member on full access: every canonical-resource switch disabled, Save enabled.
- licensed: switches live, both presets live, Save enabled (existing test kept).
- the existing error-reporting tests keep running (unknown licence = licensed).

`WorkspaceMembers.test.tsx`:
- invite, unlicensed: canonical switches disabled, `Revoke all` disabled, Send enabled on the
  default full set (replaces "locks the matrix" assertion that every switch is disabled — the
  opt-in Audit logs read switch stays live).
- API key, unlicensed: notice shown; canonical switches disabled; `Revoke all` disabled; Create
  enabled with the full default and posts full access.
- API key, licensed / unknown licence: existing tests unchanged (nothing greyed).

`LicenceGateNotice.test.tsx`: unchanged (still asserts "can still be given full access").

i18n: `npm run lingui:extract` (new strings, if any; plus the FR translation in `fr.po` for any
licence-surface string), `npm run lingui:compile` is not committed output — follow what the repo
commits.

## CHANGELOG

In `## [42.3]` (unreleased, top), one **Fix** bullet, 2–4 sentences:

> **Fix**: Without a licence covering granular permissions, an owner can now set a member's or an
> API key's permissions back to full access; only a restricted set needs the licence, as for
> invitations and new API keys. The permission editor, the invitation form and the API key form
> now grey out only the choices that would narrow access, and keep "Grant all" available.

## Docs and website

- `docs/self-hosting/licence.mdx:17` and `features/admins-api-keys.mdx:21` already state the rule.
- `docs/self-hosting/licence.mdx:94` (on dev) says "saving in the permission editor (for members
  and API keys) … are refused" — contradicted by the new code. Separate PR on Notifuse/docs
  (branch `docs/full-access-without-licence` from `dev`, PR to `dev`): "saving a restricted set in
  the permission editor (for members and API keys)".
- `website/src/data/licensing.ts:93-96, :125+` bodies say "a permission set that is not full
  access", consistent; the `refusedAt` summary "Saving permissions" is imprecise but not wrong
  enough to need a change in this work — reported, not edited.

## Gemini review

Raw answer: `/mnt/project-files/reviews/fix-full-access-without-licence-gemini-brut-plan.md`;
verification: `/mnt/project-files/reviews/fix-full-access-without-licence-gemini-verif-plan.md`.
Remark 3 integrated above; remark 4 implemented then reverted (badge out of the scope of this
decision). Known, not treated: resending a restricted invitation without
the licence still answers 402 (deliberate: a resend must not widen the stored grant, and the
licence sentence is shown); the invite drawer keeps its last permissions after closing; the members list badge counts
unenforceable and opt-in verbs, so a full row holding Audit logs shows `29/28` or `30/28` and a
revoked row shows a count rather than No Access (pre-existing on dev); review findings on the
console structure (repeated lock logic in three drawers, unused `disabled` matrix prop, per-switch
map rebuild, no tooltip on a switch greyed by the licence) are listed in the PR.

## Test commands

```
make test-service
make test-http
cd console && npm test
INTEGRATION_TESTS=true go test -tags integration,licdev ./tests/integration -run 'TestLicenseGates' -v
```
