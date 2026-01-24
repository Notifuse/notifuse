import { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react'
import { i18n, loadLocale, getInitialLocale, Locale, locales, localeNames } from '../i18n'

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
