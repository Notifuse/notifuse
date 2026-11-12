import { describe, it, expect, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { useNumberFormat, numberFormat, relativeDate } from './format'

const NNBSP = ' ' // narrow no-break space: French thousands separator
const NBSP = ' ' // no-break space: before French % and compact units
const MINUS = '−'

const inLocale = (locale: string) => {
  i18n.load(locale, {})
  i18n.activate(locale)
  return renderHook(() => useNumberFormat()).result.current
}

describe('useNumberFormat', () => {
  afterEach(() => i18n.activate('en'))

  it('formats in French', () => {
    const f = inLocale('fr')
    expect(f.int(18342)).toBe(`18${NNBSP}342`)
    expect(f.percent(0.493)).toBe(`49,3${NBSP}%`)
    expect(f.points(2.1)).toBe('+2,1 pts')
    // French counts below 2 as singular.
    expect(f.points(-0.4)).toBe(`${MINUS}0,4 pt`)
    expect(f.compact(2500)).toBe(`2,5${NBSP}k`)
  })

  it('formats in English', () => {
    const f = inLocale('en')
    expect(f.int(18342)).toBe('18,342')
    expect(f.percent(0.493)).toBe('49.3%')
    expect(f.points(2.1)).toBe('+2.1 pts')
    expect(f.points(-0.4)).toBe(`${MINUS}0.4 pts`)
    expect(f.points(1)).toBe('+1.0 pt')
    expect(f.compact(2500)).toBe('2.5K')
  })

  it('takes the number of decimals of a percentage', () => {
    const f = inLocale('en')
    expect(f.percent(0.5, 0)).toBe('50%')
    expect(f.percent(0.12345, 2)).toBe('12.35%')
  })

  it('writes a zero difference without a sign', () => {
    expect(inLocale('en').points(0)).toBe('0.0 pts')
    expect(inLocale('en').points(-0.01)).toBe('0.0 pts')
  })

  it('follows the console locale it is called with', () => {
    expect(numberFormat('de', i18n).int(1234567)).toBe('1.234.567')
  })
})

describe('relativeDate', () => {
  const now = new Date('2026-11-12T12:00:00Z')

  it('reads a date under 7 days old relative to now, in the console language', () => {
    expect(relativeDate('en', new Date('2026-11-12T11:58:00Z'), now)).toBe('2 minutes ago')
    expect(relativeDate('en', new Date('2026-11-12T09:00:00Z'), now)).toBe('3 hours ago')
    expect(relativeDate('en', new Date('2026-11-11T09:00:00Z'), now)).toBe('yesterday')
    expect(relativeDate('en', new Date('2026-11-07T12:00:00Z'), now)).toBe('5 days ago')
    expect(relativeDate('fr', new Date('2026-11-12T09:00:00Z'), now)).toBe('il y a 3 heures')
  })

  it('says now for a date under a minute old', () => {
    expect(relativeDate('en', new Date('2026-11-12T11:59:40Z'), now)).toBe('now')
  })

  it('writes the date itself from 7 days on', () => {
    // Local noon: the date is written in the reader's time zone, whatever the machine's is.
    expect(relativeDate('en', new Date(2026, 10, 4, 12), now)).toBe('Nov 4, 2026')
    expect(relativeDate('fr', new Date(2026, 9, 1, 12), now)).toBe('1 oct. 2026')
  })

  it('reads a date to come relative to now as well', () => {
    expect(relativeDate('en', new Date('2026-11-14T12:00:00Z'), now)).toBe('in 2 days')
  })

  it('takes ISO strings', () => {
    expect(relativeDate('en', '2026-11-12T11:00:00Z', now)).toBe('1 hour ago')
  })
})
