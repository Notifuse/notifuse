import { createContext } from 'react'
import type { Entitlements, LicenseResponse } from '../types/license'

/**
 * What the console knows about this deployment's licence.
 *
 * Split from the provider next door and free of every runtime import but React, for the reason
 * services/api/errors.ts and services/api/permissions.ts are: the provider reaches the api client,
 * the api client imports the router, and the router builds the whole route graph at module scope.
 * A layout that only wants to grey a button out would otherwise drag that graph into any test
 * that mocks @tanstack/react-router shallowly — which is most of them.
 *
 * Licensing is a property of the deployment — one Postgres database — not of a workspace, which
 * is why this sits above workspace selection rather than inside WorkspaceLayout.
 *
 * The console never enforces. The backend refuses a walled write with 402 whether or not this
 * bundle greyed the button out first; everything here exists so that refusal is explained
 * instead of arriving bare.
 */
export interface LicenseContextValue {
  // What the deployment is licensed for, or null when the console has not been told. Null is
  // "not told", never "unlicensed": /api/licence.get is root-only, so most sessions legitimately
  // cannot read it.
  entitlements: Entitlements | null

  // Whether the licence read is still in flight. Used to keep the banner from flashing on the
  // first paint of a perfectly licensed console.
  loading: boolean

  // Whether this user may install a key. Root-only, mirroring requireRootUser on the server.
  canManageLicense: boolean

  refresh: () => Promise<void>

  // Adopts the state a licence endpoint just answered with, so the banner repaints from the same
  // round trip that installed the key rather than from a follow-up read that could race the swap.
  adopt: (response: LicenseResponse) => void
}

// What every consumer sees when no provider is mounted, and what the provider itself starts from.
//
// Not throwing, unlike useAuth. A hook that threw would take out every page that renders outside
// the provider — which is every page in the component tests — and, worse, it would mean a licence
// subsystem that failed to mount could break a console that has nothing wrong with its licence.
// Failing safe here means unknown state and no restrictions, exactly like the backend's nil
// entitlement provider.
export const UNKNOWN_LICENSE: LicenseContextValue = {
  entitlements: null,
  loading: false,
  canManageLicense: false,
  refresh: async () => {},
  adopt: () => {}
}

export const LicenseContext = createContext<LicenseContextValue>(UNKNOWN_LICENSE)
