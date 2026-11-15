import { describe, it, expect } from 'vitest'
import { validateAutomationsSearch } from '../router'

describe('validateAutomationsSearch', () => {
  it('keeps a known status, the search and the open automation, trimmed', () => {
    expect(validateAutomationsSearch({ status: ' live ', q: ['welcome', 'x'], automation: ' a1 ', stats: 1 })).toEqual({
      status: 'live',
      q: 'welcome',
      automation: 'a1',
      stats: 1,
      period: undefined
    })
  })

  it('drops an unknown status and empty values', () => {
    expect(validateAutomationsSearch({ status: 'sent', q: '  ', automation: '' })).toEqual({
      status: undefined,
      q: undefined,
      automation: undefined,
      stats: undefined,
      period: undefined
    })
  })

  it('keeps the period only for 7 days or all time, 30 days being its absence', () => {
    expect(validateAutomationsSearch({ period: '7d' }).period).toBe('7d')
    expect(validateAutomationsSearch({ period: ['all', '7d'] }).period).toBe('all')
    expect(validateAutomationsSearch({ period: '30d' }).period).toBeUndefined()
    expect(validateAutomationsSearch({ period: 7 }).period).toBeUndefined()
  })

  it('keeps stats only when it is 1, written as a number', () => {
    expect(validateAutomationsSearch({ stats: '1' }).stats).toBe(1)
    expect(validateAutomationsSearch({ stats: [1, 0] }).stats).toBe(1)
    expect(validateAutomationsSearch({ stats: 'yes' }).stats).toBeUndefined()
    expect(validateAutomationsSearch({ stats: 0 }).stats).toBeUndefined()
  })

  it('reads an automation id the router parsed as a number back as text', () => {
    expect(validateAutomationsSearch({ automation: 42 }).automation).toBe('42')
  })
})
