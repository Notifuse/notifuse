import { describe, it, expect } from 'vitest'
import { normalizeTimezone, isDeprecatedTimezone, getBrowserTimezone } from './timezoneNormalizer'

describe('timezoneNormalizer', () => {
  describe('normalizeTimezone', () => {
    it('should normalize deprecated Asia/Calcutta to Asia/Kolkata', () => {
      expect(normalizeTimezone('Asia/Calcutta')).toBe('Asia/Kolkata')
    })

    it('should normalize deprecated Europe/Kiev to Europe/Kyiv', () => {
      expect(normalizeTimezone('Europe/Kiev')).toBe('Europe/Kyiv')
    })

    it('should normalize US timezone abbreviations', () => {
      expect(normalizeTimezone('US/Eastern')).toBe('America/New_York')
      expect(normalizeTimezone('US/Central')).toBe('America/Chicago')
      expect(normalizeTimezone('US/Mountain')).toBe('America/Denver')
      expect(normalizeTimezone('US/Pacific')).toBe('America/Los_Angeles')
    })

    it('should normalize deprecated Argentina zones', () => {
      expect(normalizeTimezone('America/Buenos_Aires')).toBe('America/Argentina/Buenos_Aires')
      expect(normalizeTimezone('America/Cordoba')).toBe('America/Argentina/Cordoba')
    })

    it('should normalize Canada timezone abbreviations', () => {
      expect(normalizeTimezone('Canada/Eastern')).toBe('America/Toronto')
      expect(normalizeTimezone('Canada/Pacific')).toBe('America/Vancouver')
    })

    it('should return canonical timezone unchanged', () => {
      expect(normalizeTimezone('Asia/Kolkata')).toBe('Asia/Kolkata')
      expect(normalizeTimezone('America/New_York')).toBe('America/New_York')
      expect(normalizeTimezone('Europe/London')).toBe('Europe/London')
      expect(normalizeTimezone('UTC')).toBe('UTC')
    })

    it('should return unknown timezone unchanged', () => {
      expect(normalizeTimezone('Invalid/Timezone')).toBe('Invalid/Timezone')
    })

    it('should normalize UTC variations', () => {
      expect(normalizeTimezone('GMT')).toBe('UTC')
      expect(normalizeTimezone('Etc/UTC')).toBe('UTC')
      expect(normalizeTimezone('Etc/GMT')).toBe('UTC')
    })

    it('should normalize country-based legacy zones', () => {
      expect(normalizeTimezone('Japan')).toBe('Asia/Tokyo')
      expect(normalizeTimezone('Egypt')).toBe('Africa/Cairo')
      expect(normalizeTimezone('Turkey')).toBe('Europe/Istanbul')
      expect(normalizeTimezone('Poland')).toBe('Europe/Warsaw')
    })
  })

  describe('isDeprecatedTimezone', () => {
    it('should identify deprecated timezones', () => {
      expect(isDeprecatedTimezone('Asia/Calcutta')).toBe(true)
      expect(isDeprecatedTimezone('Europe/Kiev')).toBe(true)
      expect(isDeprecatedTimezone('US/Eastern')).toBe(true)
    })

    it('should identify canonical timezones as not deprecated', () => {
      expect(isDeprecatedTimezone('Asia/Kolkata')).toBe(false)
      expect(isDeprecatedTimezone('America/New_York')).toBe(false)
      expect(isDeprecatedTimezone('Europe/London')).toBe(false)
    })

    it('should handle unknown timezones', () => {
      expect(isDeprecatedTimezone('Invalid/Timezone')).toBe(false)
    })
  })

  describe('getBrowserTimezone', () => {
    it('should return a normalized timezone string', () => {
      const timezone = getBrowserTimezone()
      expect(typeof timezone).toBe('string')
      expect(timezone.length).toBeGreaterThan(0)
      // Should not be a deprecated timezone
      expect(isDeprecatedTimezone(timezone)).toBe(false)
    })
  })
})
