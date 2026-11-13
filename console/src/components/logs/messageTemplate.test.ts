import { describe, it, expect } from 'vitest'
import type { Template } from '../../services/api/template'
import { messageLanguage, resolveEmail } from './messageTemplate'

const template = {
  id: 'tpl',
  name: 'Welcome',
  email: { subject: 'Hello', sender_id: 's1', reply_to: 'help@acme.io' },
  translations: { fr: { email: { subject: 'Bonjour', sender_id: '', reply_to: '' } } }
} as unknown as Template

describe('messageLanguage', () => {
  it('reads the contact language of the message data', () => {
    expect(messageLanguage({ contact: { language: 'fr' } })).toBe('fr')
    expect(messageLanguage({ contact: { language: '' } })).toBeUndefined()
    expect(messageLanguage({ contact: 'x' })).toBeUndefined()
    expect(messageLanguage(null)).toBeUndefined()
  })
})

describe('resolveEmail', () => {
  it('takes the translation of another language, with the default sender and reply-to', () => {
    expect(resolveEmail(template, 'fr', 'en')).toMatchObject({
      subject: 'Bonjour',
      sender_id: 's1',
      reply_to: 'help@acme.io'
    })
  })

  it('keeps the default content for the default language, no language or no translation', () => {
    expect(resolveEmail(template, 'en', 'en')?.subject).toBe('Hello')
    expect(resolveEmail(template, undefined, 'en')?.subject).toBe('Hello')
    expect(resolveEmail(template, 'de', 'en')?.subject).toBe('Hello')
  })
})
