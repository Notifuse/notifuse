import { describe, it, expect } from 'vitest'
import { completedRate, exitedRate, finishedOf, hasEmailStep, neverRan } from './journeyRates'
import type { JourneyCounts } from '../../services/api/automation'

const counts = (over: Partial<JourneyCounts> = {}): JourneyCounts => ({ active: 0, completed: 0, exited: 0, failed: 0, total: 0, ...over })

describe('journey rates', () => {
  it('are over finished journeys, not those in progress', () => {
    const c = counts({ active: 100, completed: 6, exited: 3, failed: 1, total: 110 })
    expect(finishedOf(c)).toBe(10)
    expect(completedRate(c)).toBe(0.6)
    expect(exitedRate(c)).toBe(0.3)
  })

  it('have no value without a finished journey', () => {
    expect(completedRate(counts({ active: 4, total: 4 }))).toBeNull()
    expect(exitedRate(counts())).toBeNull()
  })

  it('tell a draft that never ran', () => {
    expect(neverRan({ status: 'draft' }, undefined)).toBe(true)
    expect(neverRan({ status: 'draft' }, counts())).toBe(true)
    expect(neverRan({ status: 'draft' }, counts({ completed: 1, total: 1 }))).toBe(false)
    expect(neverRan({ status: 'paused' }, counts())).toBe(false)
  })

  it('tell an automation with an email step', () => {
    expect(hasEmailStep({ nodes: [] })).toBe(false)
    expect(hasEmailStep({ nodes: [{ type: 'email' } as never] })).toBe(true)
  })
})
