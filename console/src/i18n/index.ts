import { i18n } from "@lingui/core"

// Keep this list in sync with the backend's canonical set:
// domain.SupportedUILanguages (internal/domain/languages.go) and the
// pkg/mailer translation registry. The two backend lists are guarded by a
// test; this frontend list is not automatically guarded against them.
export type Locale = "en" | "fr" | "es" | "de" | "ca" | "pt-BR" | "ja" | "it"

export const locales: Locale[] = ["en", "fr", "es", "de", "ca", "pt-BR", "ja", "it"]

export const localeNames: Record<Locale, string> = {
  en: "English",
  fr: "Français",
  es: "Español",
  de: "Deutsch",
  ca: "Català",
  "pt-BR": "Português (Brasil)",
  ja: "日本語",
  it: "Italiano",
}

/**
 * Load and activate a locale
 */
export async function loadLocale(locale: Locale): Promise<void> {
  try {
    const { messages } = await import(`./locales/${locale}.po`)
    i18n.load(locale, messages)
    i18n.activate(locale)
    localStorage.setItem("locale", locale)
  } catch (error) {
    console.error(`Failed to load locale ${locale}:`, error)
    // Fallback to English
    if (locale !== "en") {
      await loadLocale("en")
    }
  }
}

/**
 * Get the initial locale from localStorage or default to English
 */
export function getInitialLocale(): Locale {
  const stored = localStorage.getItem("locale")
  if (stored && locales.includes(stored as Locale)) {
    return stored as Locale
  }
  return "en"
}

/**
 * Initialize i18n with the stored or default locale
 */
export async function initI18n(): Promise<void> {
  const locale = getInitialLocale()
  await loadLocale(locale)
}

export { i18n }
