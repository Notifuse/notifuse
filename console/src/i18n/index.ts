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
 * Options for {@link loadLocale}.
 */
export interface LoadLocaleOptions {
  /**
   * Record the locale as the user's stored preference. Off by default: only a
   * deliberate choice — the language switcher, or the users.language sync that
   * is authoritative over it — should decide what the next boot reads back.
   */
  persist?: boolean
}

// Monotonic ticket handed to each loadLocale call.
//
// i18n.activate() and the localStorage write are process-global side effects
// that run *after* the catalog import resolves, so without a ticket the slower
// of two overlapping loads wins, whichever was asked for last. Nothing in the
// UI reveals that: @lingui/react's I18nProvider repaints on i18n's own "change"
// event, so a late activate() re-renders every translated string while React
// state — the language button, the Ant Design locale — still names the locale
// that was actually requested. LocaleProvider routinely has two loads in flight
// (the localStorage bootstrap and the users.language sync), so this is the
// normal path, not an edge case.
let loadGeneration = 0

/**
 * Load and activate a locale.
 *
 * Resolves with the locale that was activated, or null when a newer load
 * superseded this one — in which case it has left i18n and localStorage
 * untouched.
 */
export async function loadLocale(
  locale: Locale,
  { persist = false }: LoadLocaleOptions = {},
): Promise<Locale | null> {
  const generation = ++loadGeneration
  try {
    const { messages } = await import(`./locales/${locale}.po`)
    // Cache the catalog before deciding whether to activate it: the import
    // succeeded, and it is the only catalog the app has if the newer load is
    // about to fail.
    i18n.load(locale, messages)
    if (generation !== loadGeneration) {
      // A newer request owns the choice, so do not activate over it or record a
      // preference — except when nothing is active at all. The newer load can
      // fail (its chunk 404s after a deploy while an open tab still holds the
      // old index.html), and then nothing else ever activates: @lingui/react's
      // I18nProvider renders null for an empty locale, and App wraps the whole
      // router in it, so the console becomes a blank page with no error. The
      // superseded language beats nothing.
      if (!i18n.locale) i18n.activate(locale)
      return null
    }
    i18n.activate(locale)
    if (persist) storeLocale(locale)
    return locale
  } catch (error) {
    console.error(`Failed to load locale ${locale}:`, error)
    if (generation !== loadGeneration) return null
    // Fall back to English, and never persist it: the user did not choose it,
    // and overwriting their stored preference here would outlive the reload
    // that fixes a transient import failure.
    if (locale !== "en") {
      return loadLocale("en")
    }
    return null
  }
}

// Record a locale as the preference to boot with next time. Private on purpose:
// it is reached through loadLocale's `persist` option so a locale can only be
// stored once its catalog actually activated.
function storeLocale(locale: Locale): void {
  localStorage.setItem("locale", locale)
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
