import { describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useLanguageName } from './useLanguageName'

const locale = vi.hoisted(() => ({ value: 'fr' }))
vi.mock('@lingui/react/macro', () => ({ useLingui: () => ({ i18n: { locale: locale.value } }) }))

describe('useLanguageName', () => {
  it("names a language in the console's language, capitalised", () => {
    locale.value = 'fr'
    const { result } = renderHook(() => useLanguageName())
    expect(result.current('en')).toBe('Anglais')
    expect(result.current('fr')).toBe('Français')
  })

  it('falls back to the English name for a code Intl cannot name', () => {
    locale.value = 'en'
    const { result } = renderHook(() => useLanguageName())
    expect(result.current('pt-BR')).toMatch(/Portuguese/)
    expect(result.current('zz-invalid-code-')).toBe('zz-invalid-code-')
  })
})
