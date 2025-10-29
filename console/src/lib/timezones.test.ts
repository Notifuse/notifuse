import { describe, it, expect } from 'vitest'
import { 
  VALID_TIMEZONES, 
  TIMEZONE_OPTIONS, 
  isValidTimezone, 
  TIMEZONE_COUNT,
  type TimezoneIdentifier 
} from './timezones'

describe('Timezones', () => {
  describe('VALID_TIMEZONES', () => {
    it('should have 594 timezones', () => {
      expect(VALID_TIMEZONES).toHaveLength(594)
      expect(TIMEZONE_COUNT).toBe(594)
    })

    it('should include UTC', () => {
      expect(VALID_TIMEZONES).toContain('UTC')
    })

    it('should include major timezones', () => {
      const majorTimezones = [
        'America/New_York',
        'America/Chicago',
        'America/Los_Angeles',
        'Europe/London',
        'Europe/Paris',
        'Asia/Tokyo',
        'Asia/Shanghai',
        'Australia/Sydney',
        'Africa/Cairo',
        'Pacific/Auckland'
      ]

      majorTimezones.forEach(tz => {
        expect(VALID_TIMEZONES).toContain(tz)
      })
    })

    it('should include both canonical and alias zones', () => {
      // Canonical
      expect(VALID_TIMEZONES).toContain('America/New_York')
      expect(VALID_TIMEZONES).toContain('Asia/Kolkata')
      
      // Aliases
      expect(VALID_TIMEZONES).toContain('GMT')
      expect(VALID_TIMEZONES).toContain('US/Eastern')
      expect(VALID_TIMEZONES).toContain('Asia/Calcutta')
    })

    it('should not have duplicates', () => {
      const uniqueTimezones = new Set(VALID_TIMEZONES)
      expect(uniqueTimezones.size).toBe(VALID_TIMEZONES.length)
    })

    it('should have all non-empty strings', () => {
      VALID_TIMEZONES.forEach(tz => {
        expect(tz).toBeTruthy()
        expect(typeof tz).toBe('string')
        expect(tz.length).toBeGreaterThan(0)
      })
    })
  })

  describe('TIMEZONE_OPTIONS', () => {
    it('should have same length as VALID_TIMEZONES', () => {
      expect(TIMEZONE_OPTIONS).toHaveLength(VALID_TIMEZONES.length)
    })

    it('should have correct structure for Ant Design Select', () => {
      TIMEZONE_OPTIONS.forEach(option => {
        expect(option).toHaveProperty('value')
        expect(option).toHaveProperty('label')
        expect(option.value).toBe(option.label)
      })
    })

    it('should match VALID_TIMEZONES values', () => {
      const optionValues = TIMEZONE_OPTIONS.map(opt => opt.value)
      expect(optionValues).toEqual([...VALID_TIMEZONES])
    })
  })

  describe('isValidTimezone', () => {
    it('should return true for valid timezones', () => {
      expect(isValidTimezone('UTC')).toBe(true)
      expect(isValidTimezone('America/New_York')).toBe(true)
      expect(isValidTimezone('Europe/London')).toBe(true)
      expect(isValidTimezone('Asia/Tokyo')).toBe(true)
    })

    it('should return false for invalid timezones', () => {
      expect(isValidTimezone('')).toBe(false)
      expect(isValidTimezone('Invalid/Timezone')).toBe(false)
      expect(isValidTimezone('NotReal/City')).toBe(false)
      expect(isValidTimezone('America/FakeCity')).toBe(false)
    })

    it('should be case sensitive', () => {
      expect(isValidTimezone('UTC')).toBe(true)
      expect(isValidTimezone('utc')).toBe(false)
      expect(isValidTimezone('america/new_york')).toBe(false)
    })

    it('should handle aliases', () => {
      expect(isValidTimezone('GMT')).toBe(true)
      expect(isValidTimezone('US/Eastern')).toBe(true)
      expect(isValidTimezone('Asia/Calcutta')).toBe(true)
    })

    it('should work as type guard', () => {
      const timezone: string = 'America/New_York'
      
      if (isValidTimezone(timezone)) {
        // TypeScript should know timezone is TimezoneIdentifier here
        const typed: TimezoneIdentifier = timezone
        expect(typed).toBe('America/New_York')
      }
    })
  })

  describe('TimezoneIdentifier type', () => {
    it('should accept valid timezone strings', () => {
      const tz1: TimezoneIdentifier = 'UTC'
      const tz2: TimezoneIdentifier = 'America/New_York'
      const tz3: TimezoneIdentifier = 'Europe/London'
      
      expect(tz1).toBe('UTC')
      expect(tz2).toBe('America/New_York')
      expect(tz3).toBe('Europe/London')
    })
  })

  describe('Backend synchronization', () => {
    it('should match the backend timezone count', () => {
      // Backend has 594 timezones
      expect(TIMEZONE_COUNT).toBe(594)
    })

    it('should include zones from all major regions', () => {
      const regions = ['Africa', 'America', 'Antarctica', 'Asia', 'Atlantic', 'Australia', 'Europe', 'Indian', 'Pacific']
      
      regions.forEach(region => {
        const hasZone = VALID_TIMEZONES.some(tz => tz.startsWith(`${region}/`))
        expect(hasZone).toBe(true)
      })
    })
  })
})
