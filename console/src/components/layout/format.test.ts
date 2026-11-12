import { describe, it, expect, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { useNumberFormat, numberFormat } from './format'

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
