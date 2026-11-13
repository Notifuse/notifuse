import { describe, it, expect } from 'vitest'
import { isFullEmail } from './email'

describe('isFullEmail', () => {
  it.each([
    'ann@example.com',
    'ann.lee+news@mail.example.co.uk',
    "o'neil@example.org",
    'a_b-c@x1.io'
  ])('accepts %s', (email) => expect(isFullEmail(email)).toBe(true))

  it.each([
    '',
    'ann',
    'ann@',
    '@example.com',
    'ann@example',
    'ann@example.',
    'ann@@example.com',
    'ann lee@example.com',
    'ann.@example.com',
    'ann@-example.com',
    'ann@example.c0m1'
  ])('refuses %s', (email) => expect(isFullEmail(email)).toBe(false))
})
