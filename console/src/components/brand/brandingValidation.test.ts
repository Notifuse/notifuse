import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import {
  validateBrandingUrl,
  validateBrandName,
  MAX_BRAND_NAME_CODE_POINTS,
  MAX_BRANDING_URL_BYTES
} from './brandingValidation'

// The very fixtures pkg/branding/branding_test.go runs, read from disk in the test only: a URL
// the form accepts must be one the server accepts, and the other way round.
// Vitest runs from console/, the repository root is its parent.
const fixturesPath = resolve(process.cwd(), '..', 'pkg', 'branding', 'testdata', 'urls.json')
const fixtures = JSON.parse(readFileSync(fixturesPath, 'utf-8')) as {
  valid: string[]
  invalid: string[]
}

describe('validateBrandingUrl', () => {
  it('reads the shared fixtures', () => {
    expect(fixtures.valid.length).toBeGreaterThan(0)
    expect(fixtures.invalid.length).toBeGreaterThan(0)
  })

  it.each(fixtures.valid)('accepts %s like the server does', (url) => {
    expect(validateBrandingUrl(url)).toBeNull()
  })

  it.each(fixtures.invalid)('refuses %s like the server does', (url) => {
    expect(validateBrandingUrl(url)).not.toBeNull()
  })

  it('accepts an empty URL', () => {
    expect(validateBrandingUrl('')).toBeNull()
  })

  it('bounds the URL in UTF-8 bytes', () => {
    const prefix = 'https://cdn.example.com/'
    const atLimit = prefix + 'a'.repeat(MAX_BRANDING_URL_BYTES - prefix.length)
    expect(validateBrandingUrl(atLimit)).toBeNull()
    expect(validateBrandingUrl(atLimit + 'a')).toBe('too_long')
  })

  it('names the reason, for the error under the field', () => {
    expect(validateBrandingUrl('http://cdn.example.com/l.png')).toBe('not_https')
    expect(validateBrandingUrl('https://10.0.0.1/l.png')).toBe('private')
    expect(validateBrandingUrl('https://cdn.example.com:8443/l.png')).toBe('port')
    expect(validateBrandingUrl('https://bücher.example/l.png')).toBe('unicode_host')
    expect(validateBrandingUrl('https://localhost/l.png')).toBe('not_public')
    expect(validateBrandingUrl('https://0x7f000001/l.png')).toBe('ipv4')
  })
})

describe('validateBrandName', () => {
  it('accepts an empty name and plain text', () => {
    expect(validateBrandName('')).toBeNull()
    expect(validateBrandName('A & B <Labs>')).toBeNull()
  })

  it('counts code points, not UTF-16 units', () => {
    expect(validateBrandName('é'.repeat(MAX_BRAND_NAME_CODE_POINTS))).toBeNull()
    expect(validateBrandName('😀'.repeat(MAX_BRAND_NAME_CODE_POINTS))).toBeNull()
    expect(validateBrandName('a'.repeat(MAX_BRAND_NAME_CODE_POINTS + 1))).toBe('too_long')
  })

  it('refuses control and hidden characters, keeps the zero-width joiner', () => {
    expect(validateBrandName('Acme\nLabs')).toBe('control')
    expect(validateBrandName('Acme‮gnp.exe')).toBe('control')
    expect(validateBrandName('Ac​me')).toBe('control')
    expect(validateBrandName('👩‍💻 Labs')).toBeNull()
  })
})
