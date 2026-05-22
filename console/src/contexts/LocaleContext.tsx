import { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react'
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

  // Load initial locale on mount
  useEffect(() => {
    const init = async () => {
      setIsLoading(true)
      await loadLocale(locale)
      setIsLoading(false)
    }
    init()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const setLocale = useCallback(async (newLocale: Locale) => {
    if (newLocale === locale) return
    setIsLoading(true)
    await loadLocale(newLocale)
    setLocaleState(newLocale)
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
