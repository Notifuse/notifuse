import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import { formatDuration } from './duration'

describe('formatDuration', () => {
  it('writes seconds, minutes and seconds, or hours and minutes', () => {
    expect(formatDuration(48_000, i18n)).toBe('48 s')
    expect(formatDuration(168_000, i18n)).toBe('2 min 48 s')
    expect(formatDuration(3_900_000, i18n)).toBe('1 h 05 min')
    expect(formatDuration(-5, i18n)).toBe('0 s')
  })
})
