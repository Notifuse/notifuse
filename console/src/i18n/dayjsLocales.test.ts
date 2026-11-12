import { afterEach, describe, expect, it, vi } from 'vitest'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import { activateDayjsLocale } from './dayjsLocales'
import { locales, type Locale } from '.'
import { computeDateRange } from '../components/web_analytics/lib/dates'

dayjs.extend(relativeTime)

// The dayjs global outlives each test: put English back so no case leaks into the next.
afterEach(() => {
  dayjs.locale('en')
  vi.useRealTimers()
})

// March in each console language, as that language's dayjs locale spells it. A locale whose module
// was never imported is unknown to dayjs, so dayjs.locale() stays English and its March reads "March".
const march: Record<Locale, { code: string; month: string }> = {
  en: { code: 'en', month: 'March' },
  fr: { code: 'fr', month: 'mars' },
  es: { code: 'es', month: 'marzo' },
  de: { code: 'de', month: 'März' },
  ca: { code: 'ca', month: 'Març' },
  'pt-BR': { code: 'pt-br', month: 'março' },
  ja: { code: 'ja', month: '3月' },
  it: { code: 'it', month: 'marzo' },
}

describe('activateDayjsLocale', () => {
  it.each(locales)('makes dayjs speak %s', (locale) => {
    activateDayjsLocale(locale)

    expect(dayjs.locale()).toBe(march[locale].code)
    expect(dayjs('2026-03-05').format('MMMM')).toBe(march[locale].month)
  })

  it('writes relative times in the chosen language', () => {
    activateDayjsLocale('fr')

    expect(dayjs().subtract(3, 'day').fromNow()).toBe('il y a 3 jours')
  })

  it('goes back to English', () => {
    activateDayjsLocale('de')
    activateDayjsLocale('en')

    expect(dayjs.locale()).toBe('en')
    expect(dayjs('2026-03-05').format('MMMM')).toBe('March')
  })
})

// The web analytics week presets start on the language's first day of the week: Monday where the
// dayjs locale says so, Sunday otherwise. 2026-03-15 is a Sunday.
describe('week presets in the chosen language', () => {
  const sundayInParis = new Date('2026-03-15T10:00:00Z')

  it.each(['fr', 'es', 'de', 'ca', 'it'] as const)('start on Monday in %s', (locale) => {
    vi.useFakeTimers()
    vi.setSystemTime(sundayInParis)
    activateDayjsLocale(locale)

    const thisWeek = computeDateRange('this_week', 'Europe/Paris')
    const previousWeek = computeDateRange('previous_week', 'Europe/Paris')

    expect(thisWeek.start.format('YYYY-MM-DD HH:mm')).toBe('2026-03-09 00:00')
    expect(previousWeek.start.format('YYYY-MM-DD HH:mm')).toBe('2026-03-02 00:00')
    expect(previousWeek.end.format('YYYY-MM-DD HH:mm')).toBe('2026-03-08 23:59')
  })

  it.each(['en', 'pt-BR', 'ja'] as const)('start on Sunday in %s', (locale) => {
    vi.useFakeTimers()
    vi.setSystemTime(sundayInParis)
    activateDayjsLocale(locale)

    const thisWeek = computeDateRange('this_week', 'Europe/Paris')
    const previousWeek = computeDateRange('previous_week', 'Europe/Paris')

    expect(thisWeek.start.format('YYYY-MM-DD HH:mm')).toBe('2026-03-15 00:00')
    expect(previousWeek.start.format('YYYY-MM-DD HH:mm')).toBe('2026-03-08 00:00')
    expect(previousWeek.end.format('YYYY-MM-DD HH:mm')).toBe('2026-03-14 23:59')
  })
})
