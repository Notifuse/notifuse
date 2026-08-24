import { createContext, useContext, useState, useEffect, useCallback, useRef, ReactNode } from 'react'
import { i18n, loadLocale, getInitialLocale, Locale, locales, localeNames } from '../i18n'
import { AuthContext } from './AuthContext'

interface LocaleContextType {
  locale: Locale
  setLocale: (locale: Locale) => Promise<void>
  locales: Locale[]
  localeNames: Record<Locale, string>
  isLoading: boolean
}

const LocaleContext = createContext<LocaleContextType | null>(null)

interface LocaleProviderProps {
  children: ReactNode
}

export function LocaleProvider({ children }: LocaleProviderProps) {
  const [locale, setLocaleState] = useState<Locale>(getInitialLocale())
  const [isLoading, setIsLoading] = useState(true)

  // A locale bundle is fetched asynchronously and the provider can unmount while
  // that is in flight — a route change during the initial load, or a test file
  // ending. Every setState below an await is guarded by this: in the app such an
  // update is dropped with a warning, and under vitest's per-file jsdom teardown
  // it surfaces as an unhandled "window is not defined" that fails the run.
  //
  // Reset on mount, not just cleared on unmount, so a remount (StrictMode's
  // double-invoke, or a provider that is torn down and rebuilt) does not leave
  // the ref stuck false and silently suppress every later update.
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  // Load the bootstrap locale on mount. This deliberately does not persist:
  // the value came straight back out of localStorage, and rewriting it here is
  // what used to let a superseded load re-pin a locale the user never chose.
  //
  // It can also still be in flight when the users.language sync below starts a
  // second load. loadLocale's generation guard decides that — the newest
  // request wins, regardless of which catalog arrives first — and returns null
  // to the loser, so only the load that actually activated sets the locale.
  // Clearing isLoading is not guarded: whichever load finishes last clears it,
  // which can be the loser, but nothing in the app reads it and leaving it
  // guarded would strand it true when a load fails.
  useEffect(() => {
    const init = async () => {
      setIsLoading(true)
      const activated = await loadLocale(locale)
      if (!mountedRef.current) return
      if (activated) setLocaleState(activated)
      setIsLoading(false)
    }
    init()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const setLocale = useCallback(async (newLocale: Locale) => {
    if (newLocale === locale) return
    setIsLoading(true)
    const activated = await loadLocale(newLocale, { persist: true })
    if (!mountedRef.current) return
    if (activated) setLocaleState(activated)
    setIsLoading(false)
  }, [locale])

  // Once the authenticated user is loaded, sync the UI locale to their saved
  // language preference. The users.language column is the source of truth, so
  // it wins over the localStorage value used for the pre-login bootstrap.
  // Read the auth context directly (rather than useAuth) so LocaleProvider can
  // still mount without an AuthProvider above it.
  const userLanguage = useContext(AuthContext)?.user?.language
  useEffect(() => {
    if (!userLanguage || !locales.includes(userLanguage as Locale)) return
    void setLocale(userLanguage as Locale)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [userLanguage])

  return (
    <LocaleContext.Provider
      value={{
        locale,
        setLocale,
        locales,
        localeNames,
        isLoading,
      }}
    >
      {children}
    </LocaleContext.Provider>
  )
}

export function useLocale() {
  const context = useContext(LocaleContext)
  if (!context) {
    throw new Error('useLocale must be used within a LocaleProvider')
  }
  return context
}

export { i18n }
