import { describe, expect, it } from 'vitest'
import { API_KEY_PREFIX_MAX_LENGTH, apiKeyDomain, finalApiKeyPrefix, toApiKeyPrefix } from './apiKeyPrefix'

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

describe('finalApiKeyPrefix', () => {
  it('drops the leading and trailing underscores and dashes, not the inner ones', () => {
    expect(finalApiKeyPrefix('website_')).toBe('website')
    expect(finalApiKeyPrefix('-_crm_sync-_')).toBe('crm_sync')
    expect(finalApiKeyPrefix('website-signup')).toBe('website-signup')
    expect(finalApiKeyPrefix('__')).toBe('')
  })
})

describe('apiKeyDomain', () => {
  it('takes the host of the endpoint without its port, as the server mints the address', () => {
    expect(apiKeyDomain('https://localapi.notifuse.com:4000')).toBe('localapi.notifuse.com')
    expect(apiKeyDomain('http://203.0.113.7:8080/api')).toBe('203.0.113.7')
    expect(apiKeyDomain('https://api.example.com/')).toBe('api.example.com')
    expect(apiKeyDomain('api.example.com')).toBe('api.example.com')
    expect(apiKeyDomain(undefined)).toBe('api.example.com')
  })

  it('drops the brackets of an IPv6 host, with or without a port, as the server does', () => {
    expect(apiKeyDomain('http://[::1]')).toBe('::1')
    expect(apiKeyDomain('http://[::1]:4000')).toBe('::1')
    expect(apiKeyDomain('http://[2001:db8::1]:8080/api')).toBe('2001:db8::1')
  })
})
