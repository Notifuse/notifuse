import { describe, expect, it } from 'vitest'
import { API_KEY_PREFIX_MAX_LENGTH, toApiKeyPrefix } from './apiKeyPrefix'

// The server takes ^[a-z0-9_-]{1,64}$ as the part of the key's address before "@".
describe('toApiKeyPrefix', () => {
  it('keeps hyphens and underscores', () => {
    expect(toApiKeyPrefix('website-signup')).toBe('website-signup')
    expect(toApiKeyPrefix('crm_sync')).toBe('crm_sync')
  })

  it('lowercases and turns spaces into underscores', () => {
    expect(toApiKeyPrefix('Website Signup')).toBe('website_signup')
    expect(toApiKeyPrefix('Website-Signup')).toBe('website-signup')
  })

  it('drops every other character', () => {
    expect(toApiKeyPrefix('zap!er.prod@é')).toBe('zaperprod')
  })

  it('keeps a trailing space as an underscore while the name is typed', () => {
    expect(toApiKeyPrefix('website ')).toBe('website_')
  })

  it('stops at the length the server accepts', () => {
    expect(API_KEY_PREFIX_MAX_LENGTH).toBe(64)
    expect(toApiKeyPrefix('a'.repeat(70))).toBe('a'.repeat(64))
  })
})
