/**
 * Licence types, mirroring internal/domain/license.go field for field.
 *
 * This module is deliberately free of runtime imports, exactly like services/api/permissions.ts:
 * services/api/errors.ts needs the feature names to label a 402, and errors.ts is the one module
 * that must stay importable without dragging the router graph into a test environment. Anything
 * here that reached for the api client would put that back.
 *
 * Two rules from the backend carry over to every consumer of these types:
 *
 * `tier` is DISPLAY ONLY. Never branch on it. What a deployment may do is `features` and
 * `max_workspaces`, so a plan renamed on the pricing page cannot change what any console does.
 *
 * The console never enforces. The backend refuses a write with 402 whether or not this bundle
 * greyed the button out first; everything here exists so that refusal is not the first a user
 * hears of it.
 */

// The closed whitelist of gated capabilities, kept in the same order as the Go constants.
// A feature string this bundle does not know is not a feature: it gets no label and no
// greying-out, and the backend's own sentence is shown instead of an invented one.
export const LICENSE_FEATURES = [
  'rbac',
  'ses_tenant',
  'sso',
  'audit_logs',
  'template_i18n'
] as const

export type LicenseFeature = (typeof LICENSE_FEATURES)[number]

// Where the deployment sits in the licence lifecycle. `grace` grants everything the key
// grants — only the console says the renewal is late — and `expired` is identical to `none`.
export type LicenseState = 'none' | 'active' | 'grace' | 'expired'

// The `max_workspaces` value meaning "no ceiling". Negative rather than 0 on purpose: 0 is what
// an unfilled struct holds, and a quota check reading that as unlimited is the one mistake that
// would hand an unlicensed deployment more than a licensed one.
export const UNLIMITED_WORKSPACES = -1

/**
 * What this deployment is licensed for, already reconciled against the clock by the backend.
 *
 * The console reads this and never parses a key: the key is a bearer credential and no endpoint
 * hands it back, which is why there is no `key` field here to be tempted by.
 */
export interface Entitlements {
  tier: string
  // Licensee identity, shown as "Licensed to: ACME SAS — billing@acme.com". Display only, and
  // deliberately so: the deterrent against passing a key around is social, not cryptographic.
  org: string
  sub: string
  max_workspaces: number
  features: LicenseFeature[]
  state: LicenseState
  // RFC 3339. Go marshals a zero time as "0001-01-01T00:00:00Z" rather than omitting it, so an
  // unlicensed deployment sends a date rather than nothing — read it through licenseExpiry().
  expires_at: string
}

/** Body of GET /api/licence.get and of a successful POST /api/licence.set. */
export interface LicenseResponse {
  entitlements: Entitlements
}

/** Body of POST /api/licence.set. The envelope, verbatim; nothing else is accepted. */
export interface SetLicenseRequest {
  key: string
}

// The plan a buyer needs for each capability, for the sentence a LOCKED control shows before
// anything is pressed. Restated from internal/domain/license.go (NewFeatureNotLicensedError),
// which this bundle cannot import; TestTheConsoleNamesTheSameTierTheServerDoes pins the two
// tables to each other, so a plan renamed on one side fails a test rather than a customer.
//
// Display only, like `tier` above. What a deployment may do is still `features`; this table
// only says what to buy, and never decides whether a control is locked.
export const LICENSE_REQUIRED_TIER: Record<LicenseFeature, string> = {
  rbac: 'Studio',
  ses_tenant: 'Studio',
  sso: 'Enterprise',
  audit_logs: 'Enterprise',
  template_i18n: 'Studio'
}

// The page the Additional Use Grant pins by URL. Restated from internal/http/utils.go, which
// this bundle cannot import; the two must stay identical, and neither may change without the
// other — a v40 binary carries this URL in the `docs` field of every 402 it ever answers.
export const LICENSE_DOCS_URL = 'https://notifuse.com/licence-features'

// Where a SELF-HOSTED licence is bought, which is not the same page as the Cloud plans. It
// used to point at /pricing: an operator refused a capability clicked "Buy a licence" and
// landed on monthly SaaS tiers that sell none of what they were just refused.
export const LICENSE_PRICING_URL = 'https://notifuse.com/pricing/self-hosted'

/**
 * Whether the deployment holds a key that is still current, counting the grace period.
 *
 * This answers "is there a licence here" for the banner and the "Licensed to" line. It is not a
 * gate: an active Studio key is licensed and still has no right to SSO, so a gate asks
 * hasLicensedFeature.
 */
export function isLicensed(entitlements: Entitlements | null): boolean {
  return entitlements?.state === 'active' || entitlements?.state === 'grace'
}

/**
 * Whether the deployment is licensed for one capability.
 *
 * Unknown entitlements answer TRUE, and that is not a hole. The console never enforces — the
 * backend refuses with 402 regardless — so the only thing this decides is whether a control is
 * greyed out before it is pressed. Answering false while the state is still unknown would hide
 * paid capabilities from the deployments that bought them, which is the one failure mode a
 * licence check must not have. It mirrors the backend's own nil-provider convention.
 */
export function hasLicensedFeature(
  entitlements: Entitlements | null,
  feature: LicenseFeature
): boolean {
  if (!entitlements) return true
  // Array.isArray, not a truthiness check on the field. The Go side returns an empty slice
  // on purpose so this marshals to [] — but "the server always sends []" is a promise made
  // by a different process, possibly an older or newer one, and .includes on null throws
  // inside a render. That is a white screen for the whole console, caused by the one
  // subsystem whose entire design is that its failures cost features and never the product.
  // Unknown reads as licensed here for the same reason a null grant does.
  if (!Array.isArray(entitlements.features)) return true
  return entitlements.features.includes(feature)
}

/**
 * The key's expiry as a Date, or null when there is none to show.
 *
 * Go's zero time marshals as year 1 rather than as an absent field, so an unlicensed deployment
 * reports "0001-01-01T00:00:00Z" — printing that verbatim would tell an operator their licence
 * ran out during the Roman Empire.
 */
export function licenseExpiry(entitlements: Entitlements | null): Date | null {
  if (!entitlements?.expires_at) return null
  const expiry = new Date(entitlements.expires_at)
  if (Number.isNaN(expiry.getTime()) || expiry.getUTCFullYear() <= 1) return null
  return expiry
}
