import { describe, expect, it } from 'vitest'
import { i18n } from '@lingui/core'
import dayjs from '../../../lib/dayjs'
import {
  formatXAxisLabel,
  formatDuration,
  formatNumber,
  formatCurrency,
  formatPercent,
  formatValue,
  formatAxisValue,
  formatRangeSeriesName,
  toNumber,
  changePercent
} from './format'

describe('formatXAxisLabel', () => {
  it('reads a bucket as wall clock, not as an instant to re-convert', () => {
    // The engine truncates in the query's timezone and serialises the result
    // with a "Z". Treating that as UTC and converting to the viewer's zone
    // would move a day boundary, so a browser west of the workspace would see
    // every bucket labelled one day early.
    expect(formatXAxisLabel('2026-08-09T00:00:00Z', 'day')).toBe('Aug 9')
    expect(formatXAxisLabel('2026-08-09T09:00:00Z', 'hour')).toBe('9:00 AM')
    expect(formatXAxisLabel('2026-08-01T00:00:00Z', 'month')).toBe('Aug 2026')
    expect(formatXAxisLabel('2026-01-01T00:00:00Z', 'year')).toBe('2026')
  })

  it('writes the ticks in the console language', () => {
    i18n.load('fr', {})
    i18n.activate('fr')
    try {
      expect(formatXAxisLabel('2026-08-09T00:00:00Z', 'day')).toBe('9 août')
      expect(formatXAxisLabel('2026-08-01T00:00:00Z', 'month')).toBe('août 2026')
      expect(formatRangeSeriesName({ start: dayjs.utc('2026-12-21'), end: dayjs.utc('2026-12-27') })).toBe(
        '21–27 déc.'
      )
    } finally {
      i18n.activate('en')
    }
  })

  it('names a range series by its days', () => {
    expect(formatRangeSeriesName({ start: dayjs.utc('2026-12-21'), end: dayjs.utc('2026-12-27') })).toBe(
      'Dec 21 – 27'
    )
    expect(formatRangeSeriesName({ start: dayjs.utc('2026-11-28'), end: dayjs.utc('2026-12-04') })).toBe(
      'Nov 28 – Dec 4'
    )
  })

  it('passes an unparseable bucket through untouched', () => {
    expect(formatXAxisLabel('not-a-date', 'day')).toBe('not-a-date')
  })
})

describe('measure parsing', () => {
  it('accepts the strings PostgreSQL numerics arrive as', () => {
    expect(toNumber('20.0')).toBe(20)
    expect(toNumber(null)).toBe(0)
    expect(toNumber(undefined)).toBe(0)
    expect(toNumber('')).toBe(0)
    expect(toNumber(42)).toBe(42)
  })

  it('treats a missing comparison as no change rather than infinite growth', () => {
    expect(changePercent(10, 0)).toBe(0)
    expect(changePercent(150, 100)).toBe(50)
    expect(changePercent(50, 100)).toBe(-50)
  })
})

describe('formatters', () => {
  it('abbreviates counts and spells out durations', () => {
    expect(formatNumber(999)).toBe('999')
    expect(formatNumber(1500)).toBe('1.5K')
    expect(formatNumber(2_400_000)).toBe('2.4M')
    // Rounded before it is abbreviated: never "1000.0K", and 999.6 reads like 1000.
    expect(formatNumber(999_999)).toBe('1M')
    expect(formatNumber(999.6)).toBe('1K')
    expect(formatDuration(45)).toBe('45 s')
    expect(formatDuration(312)).toBe('5 min 12 s')
    expect(formatDuration(300)).toBe('5 min')
    expect(formatDuration(9000)).toBe('2 h 30 min')
    expect(formatDuration(3900)).toBe('1 h 05 min')
    expect(formatDuration(7200)).toBe('2 h')
    expect(formatDuration(0)).toBe('0 s')
    // Seconds that round up to a minute carry over: never "4 min 60 s".
    expect(formatDuration(299.6)).toBe('5 min')
    expect(formatDuration(7199.9)).toBe('2 h')
    // From an hour, rounded to the minute from the value itself.
    expect(formatDuration(7170)).toBe('2 h')
    expect(formatDuration(3630)).toBe('1 h 01 min')
    expect(formatDuration(3629.5)).toBe('1 h')
  })

  it('writes percentages and axis values', () => {
    expect(formatPercent(12.5)).toBe('12.5%')
    expect(formatPercent(12.46, 0)).toBe('12%')
    expect(formatValue(12.5, 'percentage')).toBe('12.5%')
    expect(formatValue(312, 'duration')).toBe('5 min 12 s')
    expect(formatAxisValue(12.6, 'percentage')).toBe('13%')
    expect(formatAxisValue(45, 'duration')).toBe('45 s')
    expect(formatAxisValue(59.6, 'duration')).toBe('1 min')
    expect(formatAxisValue(150, 'duration')).toBe('2.5 min')
    expect(formatAxisValue(1500, 'number')).toBe('1.5K')
  })
})

describe('formatters in the console language', () => {
  // Intl separates some French parts with a narrow no-break space; compared as plain spaces.
  const plain = (text: string) => text.replace(/[\u202f\u00a0]/g, ' ')

  it('writes counts, percentages and durations with the separators of the active language', () => {
    i18n.load('fr', {})
    i18n.activate('fr')
    try {
      expect(plain(formatNumber(999))).toBe('999')
      expect(plain(formatNumber(1500))).toBe('1,5 k')
      expect(plain(formatNumber(2_400_000))).toBe('2,4 M')
      expect(plain(formatPercent(12.5))).toBe('12,5 %')
      expect(plain(formatValue(12.5, 'percentage'))).toBe('12,5 %')
      expect(plain(formatAxisValue(150, 'duration'))).toBe('2,5 min')
      expect(plain(formatAxisValue(12.6, 'percentage'))).toBe('13 %')
    } finally {
      i18n.activate('en')
    }
  })

  it('groups four-digit counts in languages that do not abbreviate them', () => {
    i18n.load('de', {})
    i18n.activate('de')
    try {
      expect(formatNumber(1234)).toBe('1.234')
      expect(plain(formatNumber(2_400_000))).toBe('2,4 Mio.')
    } finally {
      i18n.activate('en')
    }
  })
})

describe('formatCurrency in the console language', () => {
  // Intl separates French thousands with a narrow no-break space; compared as plain spaces.
  const plain = (text: string) => text.replace(/[\u202f\u00a0]/g, ' ')

  it('writes amounts with the separators of the active language', () => {
    i18n.load('fr', {})
    i18n.activate('fr')
    try {
      expect(plain(formatCurrency(1234.5))).toBe('1 235')
      expect(plain(formatCurrency(12.25))).toBe('12,25')
      expect(plain(formatCurrency(980, 'EUR'))).toBe('980 €')
      expect(plain(formatCurrency(12_500, 'EUR'))).toBe('12,5K €')
      // Past a billion the millions keep their grouping: "2 346M €", not "2M 346 €".
      expect(plain(formatCurrency(2_345_678_000, 'EUR'))).toBe('2 346M €')
    } finally {
      i18n.activate('en')
    }
    expect(formatCurrency(1234.5)).toBe('1,235')
    expect(formatCurrency(12_500, 'USD')).toBe('$12.5K')
    expect(formatCurrency(2_345_678_000, 'USD')).toBe('$2,346M')
  })
})

