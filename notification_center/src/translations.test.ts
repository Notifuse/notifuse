import { describe, it, expect } from 'vitest'
import { translations, type Language } from './translations'

// Every language the widget offers in its language menu
const languages: Language[] = ['en', 'fr', 'es', 'de', 'zh', 'hi', 'ar', 'pt', 'ru', 'ja', 'pl']

describe('translations', () => {
  it.each(Object.keys(translations))('%s is translated in every language', (key) => {
    for (const language of languages) {
      const text = translations[key][language]
      expect(text, `${key} in ${language}`).toBeTruthy()
      expect(text!.trim(), `${key} in ${language}`).not.toBe('')
    }
  })
})
