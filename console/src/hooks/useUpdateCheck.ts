import { useCallback, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '../contexts/AuthContext'
import { browserLocale, browserTimeZone, checkForUpdates, normalizeUpdateCheck, type UpdateCheckResponse } from '../services/updates'

const DAY_MS = 24 * 60 * 60 * 1000
// A failed check (no network to the update service, a firewall, a timeout) waits this long before
// it is tried again, rather than costing a five-second request on every visit.
const FAILURE_RETRY_MS = 6 * 60 * 60 * 1000
export const UPDATE_CHECK_CACHE_KEY = 'notifuse.updateCheck'
export const UPDATE_CHECK_IGNORED_KEY = 'notifuse.updateCheck.ignored'

interface CachedCheck {
  at: number
  version: string
  locale: string
  /** Null records a failed check. */
  response: unknown
}

// What the browser remembers for this version and language: an answer, a recent failure, or
// nothing. A server that was just upgraded, or a console switched to another language, asks
// again. A kept answer goes through the same reading as a fresh one, so an entry written by an
// older console, or edited by hand, cannot reach the components in a shape they do not expect.
// Storage may be missing or throw; the check then simply runs.
type Remembered = { kind: 'answer'; response: UpdateCheckResponse } | { kind: 'failed' } | null

function readCache(version: string, locale: string): Remembered {
  try {
    const raw = localStorage.getItem(UPDATE_CHECK_CACHE_KEY)
    if (!raw) return null
    const cached = JSON.parse(raw) as Partial<CachedCheck>
    if (cached.version !== version || cached.locale !== locale || typeof cached.at !== 'number') return null
    const age = Date.now() - cached.at
    if (cached.response === null) return age <= FAILURE_RETRY_MS ? { kind: 'failed' } : null
    if (age > DAY_MS) return null
    const response = normalizeUpdateCheck(cached.response)
    return response ? { kind: 'answer', response } : null
  } catch {
    return null
  }
}

function writeCache(version: string, locale: string, response: UpdateCheckResponse | null) {
  try {
    const entry: CachedCheck = { at: Date.now(), version, locale, response }
    localStorage.setItem(UPDATE_CHECK_CACHE_KEY, JSON.stringify(entry))
  } catch {
    // Without storage the check runs on each visit to the workspace list.
  }
}

function readIgnored(): string | null {
  try {
    return localStorage.getItem(UPDATE_CHECK_IGNORED_KEY)
  } catch {
    return null
  }
}

export interface UpdateCheckState {
  /** The answer, when there is one to show: null while disabled, loading or after a failure. */
  data: UpdateCheckResponse | null
  /** A newer release exists. */
  available: boolean
  /** The latest release was ignored in this browser and no missing release is a security one. */
  dismissed: boolean
  /** Remember in this browser that the given release should not be announced. */
  ignore: (version: string) => void
}

/**
 * The releases this server is missing, asked of updates.notifuse.com when user.me says this
 * console should ask (root user or workspace owner, CHECK_FOR_UPDATES on). Failures are silent:
 * a missed announcement is not worth an error on the workspace list.
 */
export function useUpdateCheck(): UpdateCheckState {
  const { checkForUpdates: enabled, serverVersion, licenseEntitlements } = useAuth()
  // The browser's language rather than the console's: the notes are in the language the reader
  // reads, English when the console does not speak it.
  const locale = browserLocale()
  const [ignoredVersion, setIgnoredVersion] = useState<string | null>(readIgnored)

  const query = useQuery({
    queryKey: ['updateCheck', serverVersion, locale],
    enabled: enabled && !!serverVersion,
    staleTime: DAY_MS,
    gcTime: DAY_MS,
    retry: false,
    refetchOnWindowFocus: false,
    queryFn: async (): Promise<UpdateCheckResponse | null> => {
      const version = serverVersion as string
      const remembered = readCache(version, locale)
      if (remembered?.kind === 'answer') return remembered.response
      if (remembered?.kind === 'failed') return null
      try {
        const response = await checkForUpdates({
          version,
          locale,
          timezone: browserTimeZone(),
          entitlements: licenseEntitlements
        })
        writeCache(version, locale, response)
        return response
      } catch {
        writeCache(version, locale, null)
        return null
      }
    }
  })

  const ignore = useCallback((version: string) => {
    try {
      localStorage.setItem(UPDATE_CHECK_IGNORED_KEY, version)
    } catch {
      // Ignoring then lasts until the page is reloaded.
    }
    setIgnoredVersion(version)
  }, [])

  const data = enabled && query.data ? query.data : null
  const available = !!data?.update_available
  // A missing security release is never dismissed.
  const dismissed = available && !data?.security && data?.latest === ignoredVersion

  return { data, available, dismissed, ignore }
}
