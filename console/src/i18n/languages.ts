import { locales, localeNames, type Locale } from './index'
import { authService } from '../services/api/auth'

/** The console languages, each named in itself, in the order the menus list them. */
export const LANGUAGES: ReadonlyArray<{ locale: Locale; name: string }> = locales.map((locale) => ({
  locale,
  name: localeNames[locale]
}))

/**
 * Switches the console to a language and saves it on the user's profile, so it drives both the
 * console and the system emails sent to them. The save is fire-and-forget: a failed save must
 * not undo or block the change on screen.
 */
export function changeLanguage(setLocale: (locale: Locale) => Promise<void>, locale: Locale): void {
  void setLocale(locale)
  authService.updateLanguage(locale).catch((err) => {
    console.error('Failed to persist language preference:', err)
  })
}
