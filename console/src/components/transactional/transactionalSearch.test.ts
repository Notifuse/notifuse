import { describe, expect, it } from 'vitest'
import { transactionalPeriod, validateTransactionalSearch } from './transactionalSearch'

describe('validateTransactionalSearch', () => {
  it('keeps 7 and 60 days and drops 30 days, the default, and unknown periods', () => {
    expect(validateTransactionalSearch({ period: '7d' }).period).toBe('7d')
    expect(validateTransactionalSearch({ period: '60d' }).period).toBe('60d')
    expect(validateTransactionalSearch({ period: '30d' }).period).toBeUndefined()
    expect(validateTransactionalSearch({ period: '90d' }).period).toBeUndefined()
    expect(validateTransactionalSearch({ period: ['7d'] }).period).toBeUndefined()
  })

  it('trims the search and drops it when empty', () => {
    expect(validateTransactionalSearch({ q: '  reset ' }).q).toBe('reset')
    expect(validateTransactionalSearch({ q: '   ' }).q).toBeUndefined()
    expect(validateTransactionalSearch({}).q).toBeUndefined()
  })

  it('reads numbers the router parsed back as strings, and drops arrays and objects', () => {
    expect(validateTransactionalSearch({ q: 42, notification: 7 })).toEqual({ q: '42', period: undefined, notification: '7' })
    expect(validateTransactionalSearch({ q: ['a'], notification: { id: 'x' } })).toEqual({
      q: undefined,
      period: undefined,
      notification: undefined
    })
  })

  it('keeps the open notification', () => {
    expect(validateTransactionalSearch({ notification: 'password_reset' }).notification).toBe('password_reset')
    expect(validateTransactionalSearch({ notification: '' }).notification).toBeUndefined()
  })

  it('defaults to 30 days', () => {
    expect(transactionalPeriod({})).toBe('30d')
    expect(transactionalPeriod({ period: '60d' })).toBe('60d')
  })
})
