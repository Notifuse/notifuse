import { describe, it, expect } from 'vitest'
import { validateBroadcastsSearch } from '../router'

describe('validateBroadcastsSearch', () => {
  it('normalises broadcast and compose like status and q', () => {
    expect(
      validateBroadcastsSearch({ status: ' sent ', q: ['news', 'other'], broadcast: '  b1 ', compose: ['new', 'x'] })
    ).toEqual({ status: 'sent', q: 'news', broadcast: 'b1', compose: 'new' })
  })

  it('drops empty values', () => {
    expect(validateBroadcastsSearch({ status: '', q: '   ', broadcast: '', compose: [] })).toEqual({
      status: undefined,
      q: undefined,
      broadcast: undefined,
      compose: undefined
    })
  })

  it('reads an identifier the router parsed as a number back as text', () => {
    expect(validateBroadcastsSearch({ broadcast: 1234, compose: 99 })).toMatchObject({ broadcast: '1234', compose: '99' })
  })

  it('drops unknown keys', () => {
    expect(validateBroadcastsSearch({ status: 'draft', page: '2', foo: 'bar' })).toEqual({
      status: 'draft',
      q: undefined,
      broadcast: undefined,
      compose: undefined
    })
  })
})
