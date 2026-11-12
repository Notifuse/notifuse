import { describe, expect, it } from 'vitest'
import { i18n } from '@lingui/core'
import dayjs from '../../../lib/dayjs'
import {
  formatXAxisLabel,
  formatDuration,
  formatNumber,
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
    expect(formatDuration(45)).toBe('45s')
    expect(formatDuration(312)).toBe('5m 12s')
    expect(formatDuration(300)).toBe('5m')
    expect(formatDuration(9000)).toBe('2h 30m')
    expect(formatDuration(0)).toBe('0s')
  })
})
