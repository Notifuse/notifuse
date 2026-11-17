import { describe, expect, it } from 'vitest'
import { pageFooter, shareOf } from './breakdownRows'

describe('shareOf', () => {
  it('is a percentage of the row broken down', () => {
    expect(shareOf(412, 1612)).toBeCloseTo(25.56, 2)
  })

  it('is 0 when the row has no session', () => {
    expect(shareOf(12, 0)).toBe(0)
  })
})

describe('pageFooter', () => {
  const rows = Array.from({ length: 23 }, (_, index) => ({ sessions: 100 - index }))

  it('adds up the sessions of the page shown', () => {
    expect(pageFooter(rows, 1, 10, 2000)).toEqual({ onPage: 955, base: 2000, paginated: true })
    expect(pageFooter(rows, 3, 10, 2000).onPage).toBe(78 + 79 + 80)
  })

  it('says "on this page" only beyond one page', () => {
    expect(pageFooter(rows.slice(0, 10), 1, 10, 2000).paginated).toBe(false)
    expect(pageFooter(rows.slice(0, 11), 1, 10, 2000).paginated).toBe(true)
  })
})
