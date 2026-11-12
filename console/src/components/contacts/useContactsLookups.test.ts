import { describe, it, expect } from 'vitest'
import { segmentsRefetchInterval } from './useContactsLookups'

describe('segmentsRefetchInterval', () => {
  it('reads the segments again every 15 seconds while one is being computed, and stops after', () => {
    expect(segmentsRefetchInterval([{ status: 'active' }, { status: 'building' }])).toBe(15000)
    expect(segmentsRefetchInterval([{ status: 'active' }])).toBe(false)
    expect(segmentsRefetchInterval(undefined)).toBe(false)
  })
})
