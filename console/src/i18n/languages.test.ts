import { describe, it, expect, vi, beforeEach } from 'vitest'
import { LANGUAGES, changeLanguage } from './languages'
import { authService } from '../services/api/auth'

vi.mock('../services/api/auth', () => ({
  authService: { updateLanguage: vi.fn() }
}))

describe('LANGUAGES', () => {
  it('lists every console locale with its own name', () => {
    expect(LANGUAGES.map((language) => language.locale)).toEqual(['en', 'fr', 'es', 'de', 'ca', 'pt-BR', 'ja', 'it'])
    expect(LANGUAGES.find((language) => language.locale === 'fr')?.name).toBe('Français')
  })
})

describe('changeLanguage', () => {
  beforeEach(() => vi.clearAllMocks())

  it('switches the console and saves the choice on the profile', async () => {
    vi.mocked(authService.updateLanguage).mockResolvedValue(undefined as never)
    const setLocale = vi.fn().mockResolvedValue(undefined)

    changeLanguage(setLocale, 'de')

    expect(setLocale).toHaveBeenCalledWith('de')
    expect(authService.updateLanguage).toHaveBeenCalledWith('de')
  })

  it('keeps the new locale when the profile cannot be saved', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(authService.updateLanguage).mockRejectedValue(new Error('offline'))
    const setLocale = vi.fn().mockResolvedValue(undefined)

    expect(() => changeLanguage(setLocale, 'ja')).not.toThrow()
    await vi.waitFor(() => expect(error).toHaveBeenCalled())
    expect(setLocale).toHaveBeenCalledWith('ja')
    error.mockRestore()
  })
})
