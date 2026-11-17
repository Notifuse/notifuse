import { describe, it, expect } from 'vitest'
import { emailProviderFormValues, labelInSentence, providerFromFormValues } from './emailProviderForm'
import type { EmailIntegration } from './EmailProviderCard'

const stored = (provider: Record<string, unknown>) =>
  ({
    id: 'int_1',
    name: 'Office relay',
    type: 'email',
    email_provider: {
      senders: [{ id: 's1', email: 'a@acme.com', name: 'Acme', is_default: true }],
      rate_limit_per_minute: 40,
      ...provider
    },
    created_at: '',
    updated_at: ''
  }) as unknown as EmailIntegration

describe('emailProviderFormValues', () => {
  it('names a new integration after its provider, as the catalogue does', () => {
    expect(emailProviderFormValues('ses').name).toBe('Amazon SES')
    expect(emailProviderFormValues('sparkpost').name).toBe('SparkPost')
    expect(emailProviderFormValues('sendgrid').name).toBe('SendGrid')
    expect(emailProviderFormValues('smtp').name).toBe('SMTP')
  })

  it('starts a new integration with no sender and 25 emails a minute', () => {
    expect(emailProviderFormValues('postmark')).toMatchObject({ senders: [], rate_limit_per_minute: 25 })
  })

  it('gives each kind the defaults its form had', () => {
    expect(emailProviderFormValues('smtp').smtp).toEqual({ use_tls: true, auth_type: 'basic' })
    expect(emailProviderFormValues('sparkpost').sparkpost).toEqual({ sandbox_mode: false })
    expect(emailProviderFormValues('postmark').postmark).toEqual({ message_stream: 'outbound' })
    expect(emailProviderFormValues('mailgun').mailgun).toEqual({ region: 'US' })
    expect(emailProviderFormValues('mailjet').mailjet).toEqual({ sandbox_mode: false })
    expect(emailProviderFormValues('ses').ses).toBeUndefined()
  })

  it('keeps what is stored, the defaults filling only what is missing', () => {
    const values = emailProviderFormValues(
      'smtp',
      stored({ kind: 'smtp', smtp: { host: 'h', port: 25, use_tls: false, username: 'u' } })
    )
    expect(values).toMatchObject({ name: 'Office relay', rate_limit_per_minute: 40 })
    expect(values.smtp).toEqual({ host: 'h', port: 25, use_tls: false, username: 'u', auth_type: 'basic' })
    expect(emailProviderFormValues('postmark', stored({ kind: 'postmark', postmark: {} })).postmark).toEqual({
      message_stream: 'outbound'
    })
  })
})

describe('providerFromFormValues', () => {
  it('sends only the settings of the provider kind', () => {
    const provider = providerFromFormValues({
      kind: 'mailgun',
      senders: [],
      rate_limit_per_minute: 0,
      mailgun: { domain: 'mg.acme.com', region: 'EU' },
      ses: { region: 'eu-west-1', access_key: 'stale' }
    })
    expect(provider).toEqual({
      kind: 'mailgun',
      senders: [],
      // A rate the form left empty falls back to the default, as before.
      rate_limit_per_minute: 25,
      mailgun: { domain: 'mg.acme.com', region: 'EU' }
    })
  })
})

describe('labelInSentence', () => {
  it('lowers the first letter of a label read mid-sentence, not an acronym nor in German', () => {
    // antd wrote "Please enter Server token".
    expect(labelInSentence('Server token', 'en')).toBe('server token')
    expect(labelInSentence('Secret access key', 'fr')).toBe('secret access key')
    expect(labelInSentence('API key', 'en')).toBe('API key')
    expect(labelInSentence('Domain', 'pt-BR')).toBe('domain')
    expect(labelInSentence('Server-Token', 'de')).toBe('Server-Token')
  })
})
