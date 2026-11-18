import { afterEach, describe, expect, it, vi } from 'vitest'
import { handOffSignInEmail, takeSignInEmail } from './signInHandoff'

describe('signInHandoff', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it('gives the handed email once', () => {
    handOffSignInEmail('admin@acme.com')
    expect(takeSignInEmail()).toBe('admin@acme.com')
    expect(takeSignInEmail()).toBeUndefined()
  })

  it('stores nothing for no email', () => {
    handOffSignInEmail(undefined)
    expect(takeSignInEmail()).toBeUndefined()
  })

  it('does without storage when the browser refuses it', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    expect(() => handOffSignInEmail('admin@acme.com')).not.toThrow()
    expect(takeSignInEmail()).toBeUndefined()
  })
})
