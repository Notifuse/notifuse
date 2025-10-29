import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('Timezones', () => {
  beforeEach(() => {
    // Mock window.TIMEZONES with a sample list
    window.TIMEZONES = [
      'UTC',
      'GMT',
      'America/New_York',
      'America/Chicago',
      'America/Los_Angeles',
      'Europe/London',
      'Europe/Paris',
      'Asia/Tokyo',
      'Asia/Shanghai',
      'Australia/Sydney',
      'Africa/Cairo',
      'Pacific/Auckland',
      'US/Eastern',
      'Asia/Calcutta',
      'Asia/Kolkata'
    ]
    
    // Re-import the module to pick up the mocked window.TIMEZONES
    vi.resetModules()
  })

  describe('VALID_TIMEZONES', () => {
    it('should load timezones from window.TIMEZONES', async () => {
      const { VALID_TIMEZONES } = await import('./timezones')
      expect(VALID_TIMEZONES.length).toBeGreaterThan(0)
      expect(VALID_TIMEZONES).toContain('UTC')
    })

    it('should include major timezones', async () => {
      const { VALID_TIMEZONES } = await import('./timezones')
      const majorTimezones = [
        'America/New_York',
        'America/Chicago',
        'America/Los_Angeles',
        'Europe/London',
        'Europe/Paris',
        'Asia/Tokyo',
      ]

      majorTimezones.forEach(tz => {
        expect(VALID_TIMEZONES).toContain(tz)
      })
    })

    it('should include both canonical and alias zones', async () => {
      const { VALID_TIMEZONES } = await import('./timezones')
      
      // Canonical
      expect(VALID_TIMEZONES).toContain('America/New_York')
      expect(VALID_TIMEZONES).toContain('Asia/Kolkata')
      
      // Aliases
      expect(VALID_TIMEZONES).toContain('GMT')
      expect(VALID_TIMEZONES).toContain('US/Eastern')
      expect(VALID_TIMEZONES).toContain('Asia/Calcutta')
    })
  })

  describe('TIMEZONE_OPTIONS', () => {
    it('should have correct structure for Ant Design Select', async () => {
      const { TIMEZONE_OPTIONS } = await import('./timezones')
      
      expect(TIMEZONE_OPTIONS.length).toBeGreaterThan(0)
      
      TIMEZONE_OPTIONS.forEach(option => {
        expect(option).toHaveProperty('value')
        expect(option).toHaveProperty('label')
        expect(option.value).toBe(option.label)
      })
    })
  })

  describe('isValidTimezone', () => {
    it('should return true for valid timezones', async () => {
      const { isValidTimezone } = await import('./timezones')
      
      expect(isValidTimezone('UTC')).toBe(true)
      expect(isValidTimezone('America/New_York')).toBe(true)
      expect(isValidTimezone('Europe/London')).toBe(true)
      expect(isValidTimezone('Asia/Tokyo')).toBe(true)
    })

    it('should return false for invalid timezones', async () => {
      const { isValidTimezone } = await import('./timezones')
      
      expect(isValidTimezone('')).toBe(false)
      expect(isValidTimezone('Invalid/Timezone')).toBe(false)
      expect(isValidTimezone('NotReal/City')).toBe(false)
      expect(isValidTimezone('America/FakeCity')).toBe(false)
    })

    it('should be case sensitive', async () => {
      const { isValidTimezone } = await import('./timezones')
      
      expect(isValidTimezone('UTC')).toBe(true)
      expect(isValidTimezone('utc')).toBe(false)
      expect(isValidTimezone('america/new_york')).toBe(false)
    })

    it('should handle aliases', async () => {
      const { isValidTimezone } = await import('./timezones')
      
      expect(isValidTimezone('GMT')).toBe(true)
      expect(isValidTimezone('US/Eastern')).toBe(true)
      expect(isValidTimezone('Asia/Calcutta')).toBe(true)
    })
  })

  describe('areTimezonesLoaded', () => {
    it('should return true when timezones are loaded', async () => {
      const { areTimezonesLoaded, TIMEZONE_COUNT } = await import('./timezones')
      
      expect(areTimezonesLoaded()).toBe(true)
      expect(TIMEZONE_COUNT).toBeGreaterThan(0)
    })

    it('should return false when timezones are not loaded', async () => {
      window.TIMEZONES = []
      vi.resetModules()
      
      const { areTimezonesLoaded, TIMEZONE_COUNT } = await import('./timezones')
      
      expect(areTimezonesLoaded()).toBe(false)
      expect(TIMEZONE_COUNT).toBe(0)
    })

    it('should return false when window.TIMEZONES is undefined', async () => {
      window.TIMEZONES = undefined
      vi.resetModules()
      
      const { areTimezonesLoaded } = await import('./timezones')
      
      expect(areTimezonesLoaded()).toBe(false)
    })
  })

  describe('Backend synchronization', () => {
    it('should use timezones from backend config.js', async () => {
      const { VALID_TIMEZONES } = await import('./timezones')
      
      // These should match what the backend provides
      expect(VALID_TIMEZONES).toEqual(window.TIMEZONES)
    })

    it('should handle empty timezone list gracefully', async () => {
      window.TIMEZONES = []
      vi.resetModules()
      
      const { VALID_TIMEZONES, TIMEZONE_OPTIONS, isValidTimezone } = await import('./timezones')
      
      expect(VALID_TIMEZONES).toEqual([])
      expect(TIMEZONE_OPTIONS).toEqual([])
      expect(isValidTimezone('UTC')).toBe(false)
    })
  })
})
