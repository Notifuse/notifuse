import dayjs from 'dayjs'
// Each import registers its locale with dayjs. They are small and imported up front, so
// switching languages adds no download that could fail beside the catalog's own.
import 'dayjs/locale/fr'
import 'dayjs/locale/es'
import 'dayjs/locale/de'
import 'dayjs/locale/ca'
import 'dayjs/locale/pt-br'
import 'dayjs/locale/ja'
import 'dayjs/locale/it'
import type { Locale } from '.'

// dayjs's name for each console language. Record<Locale, …> makes a language added to the
// console a type error here until it has its dayjs locale.
const dayjsLocaleCodes: Record<Locale, string> = {
  en: 'en',
  fr: 'fr',
  es: 'es',
  de: 'de',
  ca: 'ca',
  'pt-BR': 'pt-br',
  ja: 'ja',
  it: 'it',
}

/**
 * Make dayjs format in a console language: month and weekday names, relative times ("il y a
 * 3 jours"), localized formats such as `ll`, and the first day of the week. antd's DatePicker takes
 * its panels' language from ConfigProvider instead, which only needs the locale registered by the
 * imports above.
 */
export function activateDayjsLocale(locale: Locale): void {
  dayjs.locale(dayjsLocaleCodes[locale])
}
