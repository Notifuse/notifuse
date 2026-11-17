import { describe, expect, it } from 'vitest'
import { DEFAULT_MIN_SESSIONS, minSessionsParam, parseMinSessions } from './minSessions'

describe('parseMinSessions', () => {
  it('reads a floor of 1 and above, as a number or as a string', () => {
    expect(parseMinSessions(1)).toBe(1)
    expect(parseMinSessions('1')).toBe(1)
    expect(parseMinSessions(25)).toBe(25)
    expect(parseMinSessions(' 25 ')).toBe(25)
    expect(parseMinSessions(DEFAULT_MIN_SESSIONS)).toBe(DEFAULT_MIN_SESSIONS)
  })

  it('rejects zero, negatives, fractions and anything that is not a number', () => {
    for (const value of [0, '0', -3, '-3', 2.5, '2.5', 'abc', '1abc', '', ' ', true, null, undefined, {}, [], NaN, Infinity]) {
      expect(parseMinSessions(value)).toBeUndefined()
    }
  })
})

describe('minSessionsParam', () => {
  it('keeps every floor other than the default in the URL, 1 included', () => {
    expect(minSessionsParam(1)).toBe(1)
    expect(minSessionsParam(2)).toBe(2)
    expect(minSessionsParam(500)).toBe(500)
  })

  it('drops the default, and anything below 1', () => {
    expect(minSessionsParam(10)).toBeUndefined()
    expect(minSessionsParam(0)).toBeUndefined()
    expect(minSessionsParam(-4)).toBeUndefined()
    expect(minSessionsParam(NaN)).toBeUndefined()
  })
})
