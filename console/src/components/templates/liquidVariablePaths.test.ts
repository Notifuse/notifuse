import { describe, expect, it } from 'vitest'
import { liquidVariablePaths, ownVariablePaths } from './liquidVariablePaths'

describe('liquidVariablePaths', () => {
  it('lists nested values as dotted paths, in order', () => {
    expect(
      liquidVariablePaths({
        contact: { first_name: 'John', email: 'john@example.com' },
        unsubscribe_url: 'https://x'
      })
    ).toEqual(['contact.first_name', 'contact.email', 'unsubscribe_url'])
  })

  it('stops at a list, which is looped over rather than printed', () => {
    expect(liquidVariablePaths({ order: { items: [{ name: 'Shirt' }], total: '$10' } })).toEqual([
      'order.items',
      'order.total'
    ])
  })

  it('does not go deeper than three levels', () => {
    expect(liquidVariablePaths({ a: { b: { c: { d: 1 } } } })).toEqual(['a.b.c'])
  })

  it('skips keys Liquid cannot name with a dot', () => {
    expect(liquidVariablePaths({ 'first-name': 'x', ok: 1, '2fa': true })).toEqual(['ok'])
  })

  it('returns nothing without data', () => {
    expect(liquidVariablePaths(undefined)).toEqual([])
    expect(liquidVariablePaths(null)).toEqual([])
    expect(liquidVariablePaths({})).toEqual([])
  })
})

describe('ownVariablePaths', () => {
  it('leaves out the variables every send fills in, keeping those a send has to provide', () => {
    expect(
      ownVariablePaths({
        contact: { first_name: 'Jane' },
        list: { id: 'news' },
        unsubscribe_url: 'https://x',
        reset_url: 'https://reset',
        order: { number: '42' },
        contactless: true
      })
    ).toEqual(['reset_url', 'order.number', 'contactless'])
  })
})
