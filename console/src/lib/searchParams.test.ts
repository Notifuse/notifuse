import { describe, it, expect } from 'vitest'
import { searchString, parseSettingsSearch } from './searchParams'

// The router's default search parser runs JSON.parse on each value: "?webhook=2024" arrives as the
// number 2024 and "?webhook=true" as a boolean.
describe('searchString', () => {
  it('keeps a string, trimmed', () => {
    expect(searchString(' abc ')).toBe('abc')
  })

  it('turns a number or a boolean the parser produced back into its text', () => {
    expect(searchString(2024)).toBe('2024')
    expect(searchString(true)).toBe('true')
  })

  it('drops an empty value, an object, null and undefined', () => {
    expect(searchString('')).toBeUndefined()
    expect(searchString('   ')).toBeUndefined()
    expect(searchString({ a: 1 })).toBeUndefined()
    expect(searchString(null)).toBeUndefined()
    expect(searchString(undefined)).toBeUndefined()
  })

  it('reads the first value of a repeated parameter', () => {
    expect(searchString(['a', 'b'])).toBe('a')
  })
})

describe('parseSettingsSearch', () => {
  it('keeps the webhook to open, as a string', () => {
    expect(parseSettingsSearch({ webhook: 'wh_1' })).toEqual({ webhook: 'wh_1' })
    expect(parseSettingsSearch({ webhook: 42 })).toEqual({ webhook: '42' })
  })

  it('leaves the webhook out when absent or empty', () => {
    expect(parseSettingsSearch({})).toEqual({})
    expect(parseSettingsSearch({ webhook: '' })).toEqual({})
  })
})
