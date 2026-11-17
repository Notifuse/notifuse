import { describe, expect, it } from 'vitest'
import { resolveGalleryLanguage } from './resolveGalleryLanguage'

describe('resolveGalleryLanguage', () => {
  it('keeps a language the gallery is written in', () => {
    expect(resolveGalleryLanguage('fr')).toBe('fr')
    expect(resolveGalleryLanguage('pt-BR')).toBe('pt-BR')
    expect(resolveGalleryLanguage('ja')).toBe('ja')
  })

  it('writes Portuguese in Brazilian Portuguese', () => {
    expect(resolveGalleryLanguage('pt')).toBe('pt-BR')
  })

  it('falls back to English for a language the gallery lacks, or none', () => {
    expect(resolveGalleryLanguage('nl')).toBe('en')
    expect(resolveGalleryLanguage('')).toBe('en')
    expect(resolveGalleryLanguage(undefined)).toBe('en')
  })
})
