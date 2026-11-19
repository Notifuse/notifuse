import { describe, it, expect, vi, afterEach } from 'vitest'
import { browserLocale, browserTimeZone, checkForUpdates, normalizeUpdateCheck, safeHttpsUrl, UPDATES_ENDPOINT } from './updates'
import type { Entitlements } from '../types/license'

const entitlements: Entitlements = {
  tier: 'enterprise',
  org: 'ACME SAS',
  sub: 'billing@acme.com',
  max_workspaces: -1,
  features: ['sso'],
  state: 'active',
  expires_at: '2027-03-01T00:00:00Z'
}

const okResponse = {
  current: '42.0',
  latest: '43.0',
  update_available: true,
  behind: 1,
  security: false,
  upgrade_url: 'https://docs.notifuse.com/self-hosting/installation',
  releases: [
    {
      version: '43.0',
      released_at: '2026-11-07',
      security: false,
      security_note: '',
      summary: 'Summary.',
      highlights: ['One'],
      before_upgrade: [],
      changelog_url: 'https://github.com/Notifuse/notifuse/blob/main/CHANGELOG.md'
    }
  ]
}

describe('checkForUpdates', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('posts the version, locale, time zone and entitlements, without credentials or a session token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(okResponse), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    localStorage.setItem('auth_token', 'session-token-that-must-stay-home')

    const result = await checkForUpdates({ version: '42.0', locale: 'fr', timezone: 'Europe/Paris', entitlements })

    expect(result).toEqual(okResponse)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe(UPDATES_ENDPOINT)
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('omit')
    expect(JSON.parse(init.body as string)).toEqual({
      version: '42.0',
      locale: 'fr',
      timezone: 'Europe/Paris',
      entitlements
    })
    const headers = new Headers(init.headers)
    expect(headers.get('Authorization')).toBeNull()
    expect(JSON.stringify(init)).not.toContain('session-token-that-must-stay-home')
    localStorage.removeItem('auth_token')
  })

  it('leaves entitlements and an unknown time zone out', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(okResponse), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await checkForUpdates({ version: '42.0', locale: 'en', timezone: '', entitlements: null })

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(JSON.parse(init.body as string)).toEqual({ version: '42.0', locale: 'en' })
  })

  it('rejects on an error status', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', { status: 500 })))
    await expect(checkForUpdates({ version: '42.0', locale: 'en', timezone: '', entitlements: null })).rejects.toThrow('500')
  })
})

describe('browserLocale', () => {
  it("takes the first of the browser's languages the console speaks", () => {
    expect(browserLocale(['de-AT', 'fr'])).toBe('de')
    expect(browserLocale(['zh-CN', 'ja-JP', 'en'])).toBe('ja')
  })

  it('matches pt-BR exactly and sends any other Portuguese to pt-BR', () => {
    expect(browserLocale(['pt-br'])).toBe('pt-BR')
    expect(browserLocale(['pt-PT'])).toBe('pt-BR')
  })

  it('falls back to English when the browser speaks nothing the console does', () => {
    expect(browserLocale(['zh-CN', 'ko'])).toBe('en')
    expect(browserLocale([])).toBe('en')
  })

  it("reads navigator.languages when given nothing", () => {
    vi.stubGlobal('navigator', { languages: ['es-MX', 'en'], language: 'es-MX' })
    expect(browserLocale()).toBe('es')
    vi.unstubAllGlobals()
  })
})

describe('browserTimeZone', () => {
  afterEach(() => vi.restoreAllMocks())

  it("gives the browser's IANA time zone", () => {
    vi.spyOn(Intl.DateTimeFormat.prototype, 'resolvedOptions').mockReturnValue({
      timeZone: 'America/New_York'
    } as Intl.ResolvedDateTimeFormatOptions)
    expect(browserTimeZone()).toBe('America/New_York')
  })

  it('gives an empty string when Intl cannot tell', () => {
    vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(() => {
      throw new Error('no Intl')
    })
    expect(browserTimeZone()).toBe('')
  })
})

describe('safeHttpsUrl', () => {
  it('keeps https links and drops anything else', () => {
    expect(safeHttpsUrl('https://github.com/Notifuse/notifuse')).toBe('https://github.com/Notifuse/notifuse')
    expect(safeHttpsUrl('javascript:alert(1)')).toBeNull()
    expect(safeHttpsUrl('http://example.com')).toBeNull()
    expect(safeHttpsUrl('not a url')).toBeNull()
    expect(safeHttpsUrl('')).toBeNull()
    expect(safeHttpsUrl(undefined)).toBeNull()
  })
})

describe('normalizeUpdateCheck', () => {
  it('refuses an answer without a list of releases', () => {
    expect(normalizeUpdateCheck(null)).toBeNull()
    expect(normalizeUpdateCheck('nope')).toBeNull()
    expect(normalizeUpdateCheck({ update_available: true, latest: '44.0', releases: null })).toBeNull()
  })

  it('turns missing or broken fields into empty ones and drops releases without a version', () => {
    const answer = normalizeUpdateCheck({
      current: '42.0',
      update_available: false,
      security: false,
      releases: [
        { version: '44.0', highlights: null, before_upgrade: ['Do this', 3, ''], security: 'yes' },
        { summary: 'no version' },
        null
      ]
    })
    expect(answer).not.toBeNull()
    expect(answer?.releases).toHaveLength(1)
    expect(answer?.releases[0]).toMatchObject({ version: '44.0', highlights: [], before_upgrade: ['Do this'], security: false })
  })

  it('derives the summary from the releases, so the card and the panel agree', () => {
    const answer = normalizeUpdateCheck({
      latest: '99.0',
      update_available: false,
      behind: 0,
      security: false,
      releases: [{ version: '44.0' }, { version: '43.1', security: true }]
    })
    expect(answer).toMatchObject({ latest: '44.0', update_available: true, behind: 2, security: true })

    const noSecurity = normalizeUpdateCheck({ security: true, releases: [{ version: '44.0' }] })
    expect(noSecurity?.security).toBe(false)
  })

  it('checkForUpdates rejects an answer it cannot read', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ releases: 'x' }), { status: 200 })))
    await expect(checkForUpdates({ version: '42.0', locale: 'en', timezone: '', entitlements: null })).rejects.toThrow()
    vi.unstubAllGlobals()
  })
})
