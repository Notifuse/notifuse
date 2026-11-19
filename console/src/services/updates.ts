import type { Entitlements } from '../types/license'

/**
 * The update service. The console of the root user and of workspace owners asks it, when
 * CHECK_FOR_UPDATES is on, for the Notifuse releases this server is missing.
 */
export const UPDATES_ENDPOINT = 'https://updates.notifuse.com/check'

/** One missing release, its texts already in the requested language (English otherwise). */
export interface UpdateRelease {
  version: string
  released_at: string
  security: boolean
  security_note: string
  summary: string
  highlights: string[]
  before_upgrade: string[]
  changelog_url: string
}

export interface UpdateCheckResponse {
  current: string
  latest: string
  update_available: boolean
  /** How many releases are missing. */
  behind: number
  /** True when one of the missing releases fixes a vulnerability. */
  security: boolean
  upgrade_url: string
  /** The missing releases, newest first. */
  releases: UpdateRelease[]
}

export interface UpdateCheckRequest {
  version: string
  locale: string
  /** The browser's IANA time zone, such as Europe/Paris; empty when the browser gives none. */
  timezone: string
  entitlements: Entitlements | null
}

/** The console languages the update notes may be written in; English is the fallback. */
const UPDATE_NOTE_LOCALES = ['en', 'fr', 'es', 'de', 'ca', 'pt-BR', 'ja', 'it'] as const

/**
 * The language to ask the update notes in: the first of the browser's languages the console
 * speaks, matched exactly (pt-BR) or by its base (fr-CA to fr, any Portuguese to pt-BR), else
 * English.
 */
export function browserLocale(languages?: readonly string[]): string {
  let preferred: readonly string[] = languages ?? []
  if (!languages && typeof navigator !== 'undefined') {
    preferred = navigator.languages?.length ? navigator.languages : [navigator.language]
  }
  for (const tag of preferred) {
    if (!tag) continue
    const lower = tag.toLowerCase()
    const exact = UPDATE_NOTE_LOCALES.find((locale) => locale.toLowerCase() === lower)
    if (exact) return exact
    const base = lower.split('-')[0]
    if (base === 'pt') return 'pt-BR'
    const byBase = UPDATE_NOTE_LOCALES.find((locale) => locale === base)
    if (byBase) return byBase
  }
  return 'en'
}

/** The browser's IANA time zone, or an empty string when Intl cannot tell. */
export function browserTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || ''
  } catch {
    return ''
  }
}

/**
 * Asks the update service. A plain fetch, not the api client: the client would attach this
 * server's session token, which must never leave for another host.
 */
export async function checkForUpdates(request: UpdateCheckRequest): Promise<UpdateCheckResponse> {
  const body: Record<string, unknown> = { version: request.version, locale: request.locale }
  if (request.timezone) {
    body.timezone = request.timezone
  }
  if (request.entitlements) {
    body.entitlements = request.entitlements
  }
  const response = await fetch(UPDATES_ENDPOINT, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    credentials: 'omit',
    signal: AbortSignal.timeout(5000)
  })
  if (!response.ok) {
    throw new Error(`Update check failed with status ${response.status}`)
  }
  const normalized = normalizeUpdateCheck(await response.json())
  if (!normalized) {
    throw new Error('Update check returned an answer this console cannot read')
  }
  return normalized
}

const asString = (value: unknown): string => (typeof value === 'string' ? value : '')
const asStrings = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string' && item !== '') : []

/**
 * Reads an answer of the update service, or one kept in the browser, into the shape the
 * components rely on: every list an array, every text a string. Releases without a version are
 * dropped, and the summary fields (behind, security, update_available) are recomputed from the
 * releases, so the card and the panel can never disagree with them. Null when the answer is not
 * an object with a list of releases.
 */
export function normalizeUpdateCheck(raw: unknown): UpdateCheckResponse | null {
  if (!raw || typeof raw !== 'object') return null
  const answer = raw as Record<string, unknown>
  if (!Array.isArray(answer.releases)) return null

  const releases: UpdateRelease[] = answer.releases
    .filter((item): item is Record<string, unknown> => !!item && typeof item === 'object')
    .filter((item) => asString(item.version) !== '')
    .map((item) => ({
      version: asString(item.version),
      released_at: asString(item.released_at),
      security: item.security === true,
      security_note: asString(item.security_note),
      summary: asString(item.summary),
      highlights: asStrings(item.highlights),
      before_upgrade: asStrings(item.before_upgrade),
      changelog_url: asString(item.changelog_url)
    }))

  return {
    current: asString(answer.current),
    latest: releases[0]?.version ?? asString(answer.latest),
    update_available: releases.length > 0,
    behind: releases.length,
    security: releases.some((release) => release.security),
    upgrade_url: asString(answer.upgrade_url),
    releases
  }
}

/** Only https links from the update service are rendered; anything else is dropped. */
export function safeHttpsUrl(url: string | undefined): string | null {
  if (!url) return null
  try {
    return new URL(url).protocol === 'https:' ? url : null
  } catch {
    return null
  }
}
