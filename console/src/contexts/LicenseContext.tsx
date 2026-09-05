import { useCallback, useEffect, useState, ReactNode } from 'react'
import { useAuth } from './AuthContext'
import { isRootUser } from '../services/api/auth'
import { licenseApi } from '../services/api/license'
import { LicenseContext } from './licenseState'
import type { Entitlements, LicenseResponse } from '../types/license'

/**
 * Resolves the licence state from the cheapest source that can answer, in this order:
 *
 *  1. `/api/user.me`, when the server puts `entitlements` in it. Free — the console already
 *     makes that call — and it is the only source that reaches a non-root user before a gate
 *     refuses them something.
 *  2. `/api/licence.get`, for a root user only. The endpoint is root-only on the server, so
 *     calling it for everyone would put a 403 in the log of every console load for no gain.
 *
 * A refused write still explains itself: services/api/licenseErrors.ts turns the 402 into a
 * sentence naming the capability. It does not feed back into this state, because nothing here
 * changes when one write is refused — the gates are per-capability, and a console that
 * restricted itself on the first 402 would be inventing a limit the backend never imposed.
 *
 * Every failure leaves the state unknown, and unknown restricts nothing.
 */
export function LicenseProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, user, licenseEntitlements } = useAuth()
  const [entitlements, setEntitlements] = useState<Entitlements | null>(null)
  const [loading, setLoading] = useState(false)

  const canManageLicense = isRootUser(user?.email)

  // Source 1.
  useEffect(() => {
    if (licenseEntitlements === null) return
    setEntitlements(licenseEntitlements)
  }, [licenseEntitlements])

  // A signed-out console knows nothing about the licence, and must not keep reporting the one
  // the previous session was on.
  useEffect(() => {
    if (!isAuthenticated) setEntitlements(null)
  }, [isAuthenticated])

  // Throws on failure, and the two callers treat that differently on purpose.
  //
  // The automatic read below swallows it: a background licence read that fails leaves the
  // console exactly as usable as it was, and turning that into a toast would make the
  // licence subsystem the loudest thing on a page it has no business interrupting.
  //
  // The Refresh button does not. Somebody asked, so silence reads as "the click did
  // nothing" — and the state Refresh exists to escape is precisely the one where the read
  // has been failing.
  const fetchLicense = useCallback(async () => {
    setLoading(true)
    try {
      const response = await licenseApi.get()
      setEntitlements(response.entitlements)
    } finally {
      setLoading(false)
    }
  }, [])

  // Source 2.
  useEffect(() => {
    if (!isAuthenticated || !canManageLicense) return
    if (licenseEntitlements !== null) return
    void fetchLicense().catch((error) => {
      console.error('Failed to read the licence state', error)
    })
  }, [isAuthenticated, canManageLicense, licenseEntitlements, fetchLicense])

  const adopt = useCallback((response: LicenseResponse) => {
    setEntitlements(response.entitlements)
  }, [])

  return (
    <LicenseContext.Provider
      value={{
        entitlements,
        loading,
        canManageLicense,
        refresh: fetchLicense,
        adopt
      }}
    >
      {children}
    </LicenseContext.Provider>
  )
}
