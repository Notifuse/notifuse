import { describe, it, expect } from 'vitest'
import { getGravatarUrl, initialsFor } from './gravatar'

describe('getGravatarUrl', () => {
  // md5('test@example.com'), as Gravatar's own documentation hashes it.
  const hash = '55502f40dc8b7c769880b10874abc9d0'

  it('hashes the email and asks for an identicon fallback', () => {
    expect(getGravatarUrl('test@example.com', 52)).toBe(
      `https://www.gravatar.com/avatar/${hash}?s=52&d=identicon`
    )
  })

  it('trims and lower-cases the email first', () => {
    expect(getGravatarUrl('  Test@Example.COM ', 52)).toBe(getGravatarUrl('test@example.com', 52))
  })

  it('returns an empty string without an email', () => {
    expect(getGravatarUrl(undefined)).toBe('')
    expect(getGravatarUrl('')).toBe('')
  })
})

describe('initialsFor', () => {
  it.each([
    [{ name: 'Pierre Bazoge', email: 'p@example.com' }, 'PB'],
    [{ name: 'ada lovelace byron' }, 'AL'],
    [{ name: 'Cher', email: 'cher@example.com' }, 'C'],
    [{ name: '   ', email: 'jane@example.com' }, 'JA'],
    [{ email: 'zoe@example.com' }, 'ZO'],
    [{}, '?'],
    [null, '?']
  ])('%o gives %s', (user, initials) => {
    expect(initialsFor(user)).toBe(initials)
  })
})
