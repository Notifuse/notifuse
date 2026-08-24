import { describe, it, expect, beforeEach, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'

/**
 * The locale race.
 *
 * LocaleProvider starts two catalog loads whenever the localStorage bootstrap
 * value disagrees with users.language: the mount effect loads the stored one,
 * the auth effect loads the user's. Both end in i18n.activate() and, before the
 * fix, in a localStorage write — after an await, with nothing sequencing them.
 * The slower import therefore won, so the console could render in a language
 * neither React state nor the language button admitted to.
 *
 * i18n.test.tsx cannot reach this: its catalog mocks are already-resolved
 * module records, so every await settles in FIFO start order and the
 * first-started load can never finish last. Reproducing it needs a per-test
 * latency for each catalog, which is what withCatalogs sets up.
 */

// vi.doMock rather than vi.mock: a hoisted vi.mock factory runs once for the
// whole file and its module record survives resetModules, so every test after
// the first would silently run with no latency at all — and pass whatever the
// implementation does.
async function withCatalogs(
  delays: Record<string, number>,
  failures: Record<string, boolean> = {},
) {
  const catalog = (locale: string, messages: Record<string, string>) => async () => {
    await new Promise((resolve) => setTimeout(resolve, delays[locale] ?? 0))
    // A rejecting catalog is the deploy case: an open tab holds the old
    // index.html and asks for a hashed chunk that no longer exists.
    if (failures[locale]) throw new Error(`chunk for ${locale} is gone`)
    return { messages }
  }

  vi.doMock('../i18n/locales/en.po', catalog('en', { Hello: 'Hello' }))
  vi.doMock('../i18n/locales/es.po', catalog('es', { Hello: 'Hola' }))
  vi.doMock('../i18n/locales/fr.po', catalog('fr', { Hello: 'Bonjour' }))
  vi.doMock('../i18n/locales/ja.po', catalog('ja', { Hello: 'こんにちは' }))

  return await import('../i18n')
}

vi.mock('../services/api/auth', () => ({
  authService: {
    getCurrentUser: vi.fn(),
    updateLanguage: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn(),
  },
  isRootUser: () => false,
}))
vi.mock('../services/api/workspace', () => ({ workspaceService: {} }))

beforeEach(async () => {
  vi.resetModules()
  window.localStorage.clear()

  // @lingui/core's i18n is a singleton that vi.resetModules() does not reach,
  // and src/__tests__/setup.tsx activates 'en' on it at import time. Left alone
  // it leaks the previous test's locale into the next one, which silently makes
  // any "what is active now" assertion pass on stale state — and hides the
  // first-paint case entirely, since nothing is ever activated on a real boot
  // until a catalog lands. Clear it so each test starts where the browser does.
  const { i18n: singleton } = await import('@lingui/core')
  const internals = singleton as unknown as { _locale: string; _messages: Record<string, unknown> }
  internals._locale = ''
  internals._messages = {}
})

describe('loadLocale generation guard', () => {
  it('lets the newest request win when its catalog arrives first', async () => {
    const { loadLocale, i18n } = await withCatalogs({ es: 60, en: 5 })

    const bootstrap = loadLocale('es')
    const userSync = loadLocale('en', { persist: true })
    const [bootstrapResult, userResult] = await Promise.all([bootstrap, userSync])

    expect(i18n.locale).toBe('en')
    expect(window.localStorage.getItem('locale')).toBe('en')
    expect(userResult).toBe('en')
    expect(bootstrapResult).toBeNull()
  })

  it('lets the newest request win when its catalog arrives last', async () => {
    // Same intent, opposite latency: the superseded 'es' load resolves early
    // and must still decline to activate. Both directions are asserted because
    // only one of them fails when the guard is missing.
    const { loadLocale, i18n } = await withCatalogs({ es: 5, en: 60 })

    const bootstrap = loadLocale('es')
    const userSync = loadLocale('en', { persist: true })
    const [bootstrapResult, userResult] = await Promise.all([bootstrap, userSync])

    expect(i18n.locale).toBe('en')
    expect(window.localStorage.getItem('locale')).toBe('en')
    expect(userResult).toBe('en')
    expect(bootstrapResult).toBeNull()
  })

  it('leaves a superseded load unable to touch localStorage', async () => {
    const { loadLocale } = await withCatalogs({ es: 60, fr: 5 })

    const superseded = loadLocale('es', { persist: true })
    const winner = loadLocale('fr', { persist: true })
    await Promise.all([superseded, winner])

    expect(window.localStorage.getItem('locale')).toBe('fr')
  })
})

describe('LocaleProvider with a stale stored locale', () => {
  it('ends on users.language and clears the stale value, however the catalogs land', async () => {
    window.localStorage.setItem('locale', 'es')
    const { i18n } = await withCatalogs({ es: 60, en: 5 })

    const { LocaleProvider } = await import('../contexts/LocaleContext')
    const { AuthContext } = await import('../contexts/AuthContext')

    render(
      <AuthContext.Provider
        value={{
          user: { id: 'u1', email: 'test@example.com', language: 'en' },
          workspaces: [],
          isAuthenticated: true,
          signin: vi.fn(),
          signout: vi.fn(),
          loading: false,
          refreshWorkspaces: vi.fn(),
        }}
      >
        <LocaleProvider>
          <div>child</div>
        </LocaleProvider>
      </AuthContext.Provider>,
    )

    await waitFor(() => expect(i18n.locale).toBe('en'))

    // The stored 'es' catalog is still in flight here. Let it land: it must not
    // repaint the app or re-pin itself for the next boot.
    await new Promise((resolve) => setTimeout(resolve, 120))

    expect(i18n.locale).toBe('en')
    expect(window.localStorage.getItem('locale')).toBe('en')
  })

  it('keeps the stored locale when there is no authenticated user to correct it', async () => {
    window.localStorage.setItem('locale', 'es')
    vi.mocked(window.localStorage.setItem).mockClear()
    const { i18n } = await withCatalogs({ es: 5 })

    const { LocaleProvider } = await import('../contexts/LocaleContext')

    render(
      <LocaleProvider>
        <div>child</div>
      </LocaleProvider>,
    )

    // Pre-auth loads have no users.language to defer to, so the stored choice
    // stands. Reading it back proves nothing — it would read back the same if
    // the bootstrap rewrote it — so assert the write itself never happens.
    await waitFor(() => expect(i18n.locale).toBe('es'))
    expect(window.localStorage.setItem).not.toHaveBeenCalled()
    expect(window.localStorage.getItem('locale')).toBe('es')
  })
})

describe('a catalog that fails to load', () => {
  beforeEach(() => {
    // loadLocale reports failures through console.error; keep the run readable.
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  it('still renders in the superseded language when the winning catalog is gone', async () => {
    // The regression this guards: skipping i18n.load/activate for a superseded
    // load assumes the winner will activate something. It does not when the
    // winner's chunk 404s, and an i18n with no locale makes @lingui/react's
    // I18nProvider render null — a blank console rather than a wrong-language
    // one.
    const { loadLocale, i18n } = await withCatalogs({ es: 60, en: 5 }, { en: true })

    const bootstrap = loadLocale('es')
    const userSync = loadLocale('en', { persist: true })
    await Promise.all([bootstrap, userSync])

    expect(i18n.locale).toBe('es')
    expect(i18n.messages).toHaveProperty('Hello', 'Hola')
  })

  it('does not let a superseded failure drag the winner to English', async () => {
    // The failing load must check its ticket before running the English
    // fallback, or a slow 404 clobbers the locale that already won.
    const { loadLocale, i18n } = await withCatalogs({ es: 60, fr: 5 }, { es: true })

    const superseded = loadLocale('es', { persist: true })
    const winner = loadLocale('fr', { persist: true })
    const [supersededResult, winnerResult] = await Promise.all([superseded, winner])

    expect(i18n.locale).toBe('fr')
    expect(winnerResult).toBe('fr')
    expect(supersededResult).toBeNull()
    expect(window.localStorage.getItem('locale')).toBe('fr')
  })

  it('falls back to English without recording it as the preference', async () => {
    window.localStorage.setItem('locale', 'fr')
    const { loadLocale, i18n } = await withCatalogs({ ja: 5 }, { ja: true })

    // A deliberate switcher click, so persist is on — yet the fallback is not
    // what the user chose, and storing it would outlive the reload that fixes
    // the failed import.
    await expect(loadLocale('ja', { persist: true })).resolves.toBe('en')

    expect(i18n.locale).toBe('en')
    expect(window.localStorage.getItem('locale')).toBe('fr')
  })
})

describe('LocaleProvider state and i18n stay in step', () => {
  it('adopts the locale that won, not the one it asked for', async () => {
    // Two switcher clicks in quick succession. Without adopting loadLocale's
    // return value the provider records whichever request it made, so the
    // language button and the Ant Design locale name 'fr' while every
    // translated string renders in 'es'.
    const { i18n } = await withCatalogs({ fr: 80, es: 5 })
    const { LocaleProvider, useLocale } = await import('../contexts/LocaleContext')

    function Probe() {
      const { locale, setLocale } = useLocale()
      return (
        <div>
          <span data-testid="current-locale">{locale}</span>
          <button
            data-testid="switch-twice"
            onClick={() => {
              void setLocale('fr')
              void setLocale('es')
            }}
          >
            Switch twice
          </button>
        </div>
      )
    }

    render(
      <LocaleProvider>
        <Probe />
      </LocaleProvider>,
    )

    await act(async () => {
      fireEvent.click(screen.getByTestId('switch-twice'))
      await new Promise((resolve) => setTimeout(resolve, 200))
    })

    expect(i18n.locale).toBe('es')
    expect(screen.getByTestId('current-locale')).toHaveTextContent('es')
  })

  it('follows the fallback when the stored locale cannot be loaded', async () => {
    // The stored catalog is gone, so the bootstrap resolves as English. If the
    // provider kept the locale it asked for, the language button and the Ant
    // Design locale would say JA while every string rendered in English.
    vi.spyOn(console, 'error').mockImplementation(() => {})
    window.localStorage.setItem('locale', 'ja')

    const { i18n } = await withCatalogs({ ja: 5, en: 5 }, { ja: true })
    const { LocaleProvider, useLocale } = await import('../contexts/LocaleContext')

    function Probe() {
      const { locale } = useLocale()
      return <span data-testid="current-locale">{locale}</span>
    }

    render(
      <LocaleProvider>
        <Probe />
      </LocaleProvider>,
    )

    await waitFor(() => expect(i18n.locale).toBe('en'))
    await waitFor(() => expect(screen.getByTestId('current-locale')).toHaveTextContent('en'))
  })

  it('does not record a preference for the locale it merely booted with', async () => {
    window.localStorage.setItem('locale', 'fr')
    vi.mocked(window.localStorage.setItem).mockClear()

    const { i18n } = await withCatalogs({ fr: 5 })
    const { LocaleProvider } = await import('../contexts/LocaleContext')

    render(
      <LocaleProvider>
        <div>child</div>
      </LocaleProvider>,
    )

    await waitFor(() => expect(i18n.locale).toBe('fr'))
    expect(window.localStorage.setItem).not.toHaveBeenCalled()
  })
})
