import { describe, it, expect } from 'vitest'
import { emailDomain } from './dkim'

describe('emailDomain', () => {
  it('gives the domain as the server stores its key', () => {
    expect(emailDomain('News@ACME.io')).toBe('acme.io')
    expect(emailDomain('Acme <news@acme.io.>')).toBe('acme.io')
    expect(emailDomain('info@bücher.de')).toBe('xn--bcher-kva.de')
  })

  it('is empty without a domain', () => {
    expect(emailDomain('no-at-sign')).toBe('')
    expect(emailDomain('x@')).toBe('')
    expect(emailDomain('x@bad domain')).toBe('')
  })
})
