import { useContext } from 'react'
import { LicenseContext, LicenseContextValue } from '../contexts/licenseState'
import {
  hasLicensedFeature,
  isLicensed,
  licenseExpiry,
  type LicenseFeature
} from '../types/license'

export interface UseLicenseResult extends LicenseContextValue {
  /**
   * Whether this deployment is licensed for one capability, for greying a control out before it
   * is pressed.
   *
   * Advisory only, exactly like hasAccess() in WorkspaceLayout: the backend refuses with 402
   * whatever this answers. Unknown entitlements therefore answer TRUE — a console that hid paid
   * capabilities from the deployments that bought them because it had not been told would be the
   * one failure mode this must not have.
   */
  has: (feature: LicenseFeature) => boolean

  /** Whether a key is installed and current, counting the grace period. Not a gate — ask has(). */
  licensed: boolean

  /** The key's expiry, or null when there is none to show. */
  expiresAt: Date | null
}

/**
 * The licence half of the "may I" question, sibling to hasAccess() in WorkspaceLayout.
 *
 * hasAccess answers what this USER may do in this WORKSPACE; this answers what the DEPLOYMENT is
 * licensed for. The two are independent and both advisory: the backend enforces permissions with
 * 403 and licensing with 402, and neither refusal depends on the console having predicted it.
 *
 * Outside a LicenseProvider it returns the unknown state rather than throwing, so a page that
 * renders on its own — every page under test does — behaves exactly as it did before licensing
 * existed.
 */
export function useLicense(): UseLicenseResult {
  const context = useContext(LicenseContext)

  return {
    ...context,
    has: (feature: LicenseFeature) => hasLicensedFeature(context.entitlements, feature),
    licensed: isLicensed(context.entitlements),
    expiresAt: licenseExpiry(context.entitlements)
  }
}
