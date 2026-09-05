/**
 * Licence refusals — the 402 half of the console's error taxonomy.
 *
 * Same shape as the permission half in ./errors.ts, deliberately: one detection function reading
 * the body, one lazy label map, one translated-sentence builder. It lives in its own module for
 * two reasons — ./errors.ts is imported by App.tsx for the retry policy alone, and the licence
 * wording is the one surface expected to be reworded after real operator feedback, which
 * src/i18n/locales/catalogues.test.ts exempts from the eight-language gate by source file.
 *
 * Router-free for the same reason ./errors.ts is: a component that only wants to inspect an
 * error must not drag the router graph into its test environment.
 */
import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'
import { i18n } from '../../i18n'
import { ApiError } from './errors'
import { LICENSE_DOCS_URL } from '../../types/license'

/**
 * A licence refusal, as the backend serialises it.
 *
 * internal/http/utils.go writes it, for a service-layer ErrFeatureNotLicensed and for the
 * workspace ceiling: `{"error":"license_required", …}` with 402.
 */
export interface LicenseRefusal {
  // The gate that refused. A LicenseFeature, or `workspaces` for the quota, which travels in the
  // key's max_ws field rather than as an entry in its feature list. Kept as a bare string so a
  // gate this bundle predates still renders — as the server's own sentence.
  feature: string
  // The plan a buyer needs, absent when no single one can be named: which plan lifts the
  // workspace ceiling depends on how many the deployment already holds.
  requiredTier: string | null
  // The server's English sentence. The fallback, never the first choice.
  message: string
  docs: string
}

// The body field the console switches on. Restated from internal/http/utils.go
// (licenseRequiredCode), which this bundle cannot import; the two must stay identical.
export const LICENSE_REQUIRED_CODE = 'license_required'

// Named for what the user was doing, not for the wire string, so the sentence reads as an
// answer: "SES tenant isolation requires…" rather than "ses_tenant requires…". Lazy descriptors
// for the same reason as RESOURCE_LABELS — module scope runs before any catalog is active.
//
// `workspaces` is not a domain.Feature; it is the name internal/http/utils.go gives the quota so
// the body has something to call what it refused.
export const LICENSE_FEATURE_LABELS: Record<string, MessageDescriptor> = {
  rbac: msg`Custom permissions`,
  ses_tenant: msg`SES tenant isolation`,
  sso: msg`Single sign-on`,
  audit_logs: msg`Audit logs`,
  template_i18n: msg`Template translations`,
  workspaces: msg`Workspaces`
}

/**
 * Reads the licence fields off a parsed error body.
 *
 * Detection is by the `error` field being the agreed code, never by status: 402 is the status of
 * every licence refusal, but reading the status alone would leave the console unable to say what
 * was refused, and the body is the only thing that carries that.
 *
 * A body missing the code returns null and keeps whatever message it came with.
 */
export function licenseRefusalFromBody(body: unknown): LicenseRefusal | null {
  if (!body || typeof body !== 'object') return null

  const { error, feature, required_tier, message, docs } = body as {
    error?: unknown
    feature?: unknown
    required_tier?: unknown
    message?: unknown
    docs?: unknown
  }
  if (error !== LICENSE_REQUIRED_CODE) return null

  return {
    feature: typeof feature === 'string' ? feature : '',
    requiredTier: typeof required_tier === 'string' && required_tier !== '' ? required_tier : null,
    message: typeof message === 'string' ? message : '',
    docs: typeof docs === 'string' && docs !== '' ? docs : LICENSE_DOCS_URL
  }
}

/**
 * Reads the licence fields off a thrown error, for call sites that want the refused gate rather
 * than the sentence — ApiError keeps the parsed body on `data`.
 */
export function licenseRefusal(err: unknown): LicenseRefusal | null {
  return err instanceof ApiError ? licenseRefusalFromBody(err.data) : null
}

/**
 * The console's own sentence for a licence refusal.
 *
 * A gate this bundle has no label for falls through to the server's sentence, the same way an
 * unknown permission resource does — inventing a name for something we cannot name is worse than
 * showing English prose.
 */
export function licenseRefusedMessage(refusal: LicenseRefusal): string {
  const label = LICENSE_FEATURE_LABELS[refusal.feature]
  if (!label) return refusal.message

  // The quota is not a capability that a named plan unlocks: which plan lifts it depends on how
  // many workspaces already exist, so the sentence says what happened rather than what to buy.
  // It also says what did NOT happen, because "quota reached" is exactly the phrasing that makes
  // an operator fear their existing workspaces are next.
  if (refusal.feature === 'workspaces') {
    return i18n._(
      msg`This deployment has reached the number of workspaces its licence allows. Existing workspaces are unaffected; a licence with a higher limit allows more.`
    )
  }

  const capability = i18n._(label)
  return refusal.requiredTier
    ? i18n._(
        msg`${{ capability }} requires a Notifuse ${{ tier: refusal.requiredTier }} licence.`
      )
    : i18n._(msg`${{ capability }} requires a Notifuse licence.`)
}
