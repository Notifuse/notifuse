import type { I18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'

/**
 * Number formatting for the layout components and the screens built with them, in the console's
 * language. Counters (`count` props) are formatted by the components with `int`; every other value
 * (a KPI, a rate) reaches them already formatted by the screen with these same functions.
 *
 * The web analytics screens keep their own components/web_analytics/lib/format.ts.
 */
export interface NumberFormat {
  /** 18342 → "18 342" (fr), "18,342" (en). */
  int: (n: number) => string
  /** A ratio from 0 to 1: 0.493 → "49,3 %" (fr), "49.3%" (en). */
  percent: (ratio: number, digits?: number) => string
  /** A difference of percentage points: 2.1 → "+2,1 pts", −0.4 → "−0,4 pt" (fr). */
  points: (delta: number) => string
  /** 2500 → "2,5 k" (fr), "2.5K" (en). */
  compact: (n: number) => string
}

export function numberFormat(locale: string, i18n: I18n): NumberFormat {
  const integer = new Intl.NumberFormat(locale, { maximumFractionDigits: 0 })
  const compact = new Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 })
  const delta = new Intl.NumberFormat(locale, {
    signDisplay: 'exceptZero',
    minimumFractionDigits: 1,
    maximumFractionDigits: 1
  })
  const plural = new Intl.PluralRules(locale)

  return {
    int: (n) => integer.format(n),
    percent: (ratio, digits = 1) =>
      new Intl.NumberFormat(locale, {
        style: 'percent',
        minimumFractionDigits: digits,
        maximumFractionDigits: digits
      }).format(ratio),
    points: (n) => {
      const rounded = Math.round(n * 10) / 10
      // Rounded first, so −0.01 reads "0.0" and not "−0.0"; a typographic minus, as in print.
      const value = delta.format(rounded === 0 ? 0 : rounded).replace('-', '−')
      return plural.select(Math.abs(rounded)) === 'one'
        ? i18n._(msg`${value} pt`)
        : i18n._(msg`${value} pts`)
    },
    compact: (n) => compact.format(n)
  }
}

/** Formatters in the console's current language. */
export function useNumberFormat(): NumberFormat {
  const { i18n } = useLingui()
  return numberFormat(i18n.locale, i18n)
}
