import { describe, it, expect } from 'vitest'
import { validateOAuthAuthorizeSearch } from '../router'

describe('validateOAuthAuthorizeSearch', () => {
  it('keeps the request ID as text', () => {
    expect(validateOAuthAuthorizeSearch({ request: ' Xy_9-abc ' })).toEqual({ request: 'Xy_9-abc' })
  })

  // The router parses ?request=123 as a number.
  it('reads an ID the router parsed as a number back as text', () => {
    expect(validateOAuthAuthorizeSearch({ request: 123 })).toEqual({ request: '123' })
  })

  it('takes the first of a repeated parameter and drops anything else', () => {
    expect(validateOAuthAuthorizeSearch({ request: ['a', 'b'], other: 'x' })).toEqual({ request: 'a' })
    expect(validateOAuthAuthorizeSearch({ request: '' })).toEqual({})
    expect(validateOAuthAuthorizeSearch({ request: { id: 'a' } })).toEqual({})
  })
})
