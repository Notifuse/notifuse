import { describe, expect, it } from 'vitest'
import { countryName } from './isoCountries'

describe('countryName', () => {
  it('names a country in English from the table, by either code', () => {
    expect(countryName('US')).toBe('United States')
    expect(countryName('fra')).toBe('France')
    expect(countryName('DE', 'en-GB')).toBe('Germany')
  })

  it('names a country in the active language', () => {
    expect(countryName('DE', 'fr')).toBe('Allemagne')
    expect(countryName('DEU', 'es')).toBe('Alemania')
  })

  it('returns an unknown code unchanged and an empty one empty', () => {
    expect(countryName('ZZ', 'fr')).toBe('ZZ')
    expect(countryName('')).toBe('')
  })
})
