import { describe, expect, it } from 'vitest'
import { formatDimensionValue } from './dimensions'

const en = { emptyLabel: '(empty)', locale: 'en' }

describe('formatDimensionValue', () => {
  it('labels the three ways a dimension can be absent', () => {
    // Dimensions are stored NOT NULL DEFAULT '', and the one that is not is
    // exposed through COALESCE, so all three shapes reach the console.
    expect(formatDimensionValue('country', null, en)).toBe('(empty)')
    expect(formatDimensionValue('country', undefined, en)).toBe('(empty)')
    expect(formatDimensionValue('country', '', en)).toBe('(empty)')
  })

  it('keeps falsy values that are real', () => {
    // The guard has to compare strictly. A truthiness check would report a
    // numeric dimension of 0 and a boolean dimension of false as missing,
    // which reads as "no data" for two perfectly ordinary values.
    expect(formatDimensionValue('pageview_count', 0, en)).toBe('0')
    expect(formatDimensionValue('is_weekend', false, en)).toBe('false')
  })

  it('names weekdays in the active locale', () => {
    expect(formatDimensionValue('day_of_week', 1, en)).toBe('Monday')
    expect(formatDimensionValue('day_of_week', 7, en)).toBe('Sunday')
    expect(formatDimensionValue('day_of_week', 1, { ...en, locale: 'fr' })).toBe('lundi')
  })

  it('shows the raw value when a weekday is out of range', () => {
    // Better a visible 0 than a confidently wrong "Sunday": the value is not
    // one this dimension should produce, and hiding that helps nobody.
    expect(formatDimensionValue('day_of_week', 0, en)).toBe('0')
    expect(formatDimensionValue('day_of_week', 9, en)).toBe('9')
  })

  it('only treats day_of_week specially when it is actually a number', () => {
    // The engine returns text for most dimensions; a string here means the
    // caller is looking at something else and it must pass through untouched.
    expect(formatDimensionValue('day_of_week', 'Monday', en)).toBe('Monday')
    expect(formatDimensionValue('country', 1, en)).toBe('1')
  })

  it('stringifies every other dimension as-is', () => {
    expect(formatDimensionValue('country', 'FR', en)).toBe('FR')
    expect(formatDimensionValue('landing_path', '/pricing', en)).toBe('/pricing')
  })
})
