import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import { providerDetails } from './providerDetails'
import type { EmailProvider, Integration } from '../../../services/api/types'

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const integrationWith = (
  provider: Partial<EmailProvider> & Pick<EmailProvider, 'kind'>,
  hints?: Record<string, string>
): Integration =>
  ({
    id: 'int-1',
    name: 'Main',
    type: 'email',
    email_provider: { senders: [], rate_limit_per_minute: 25, ...provider },
    credential_hints: hints,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z'
  }) as Integration

const byKey = (rows: ReturnType<typeof providerDetails>) =>
  Object.fromEntries(rows.map((row) => [row.key, row]))

describe('providerDetails', () => {
  it('gives the marketing rate limit per minute, hour and day for every provider', () => {
    const rows = byKey(providerDetails(integrationWith({ kind: 'sendgrid', sendgrid: {} }), i18n))

    expect(rows.rate_limit.label).toBe('Rate limit for marketing')
    expect(rows.rate_limit.value).toBe('25 emails/min')
    expect(rows.rate_limit.hint).toBe('≈ 1,500 per hour · ≈ 36,000 per day')
  })

  it('describes an SMTP server with basic authentication', () => {
    const rows = byKey(
      providerDetails(
        integrationWith({
          kind: 'smtp',
          smtp: {
            host: 'smtp.acme.io',
            port: 587,
            username: 'mailer',
            use_tls: true,
            ehlo_hostname: 'mx.acme.io'
          }
        }),
        i18n
      )
    )

    expect(rows.host).toMatchObject({ value: 'smtp.acme.io:587', mono: true })
    expect(rows.username.value).toBe('mailer')
    expect(rows.tls.value).toBe('Yes')
    expect(rows.auth.value).toBe('Basic (username and password)')
    expect(rows.ehlo.value).toBe('mx.acme.io')
  })

  it('names the OAuth2 provider of an SMTP server', () => {
    const microsoft = byKey(
      providerDetails(
        integrationWith({
          kind: 'smtp',
          smtp: {
            host: 'h',
            port: 25,
            username: 'u',
            use_tls: false,
            auth_type: 'oauth2',
            oauth2_provider: 'microsoft'
          }
        }),
        i18n
      )
    )
    const google = byKey(
      providerDetails(
        integrationWith({
          kind: 'smtp',
          smtp: {
            host: 'h',
            port: 25,
            username: 'u',
            use_tls: false,
            auth_type: 'oauth2',
            oauth2_provider: 'google'
          }
        }),
        i18n
      )
    )

    expect(microsoft.auth.value).toBe('OAuth2 (Microsoft 365)')
    expect(microsoft.tls.value).toBe('No')
    expect(microsoft.ehlo).toBeUndefined()
    expect(google.auth.value).toBe('OAuth2 (Google)')
  })

  it('tells a managed SES configuration set and an isolated reputation', () => {
    const rows = byKey(
      providerDetails(
        integrationWith({
          kind: 'ses',
          ses: {
            region: 'eu-west-1',
            access_key: 'AKIA',
            managed_configuration_set: 'notifuse-ws1',
            managed_tenant_name: 'notifuse-tenant-ws1'
          }
        }),
        i18n
      )
    )

    expect(rows.region.value).toBe('eu-west-1')
    expect(rows.configuration_set).toMatchObject({ value: 'notifuse-ws1', hint: 'managed' })
    expect(rows.reputation).toMatchObject({ value: 'notifuse-tenant-ws1', hint: 'isolated' })
  })

  it('warns about an SES integration whose set and tenant are not provisioned', () => {
    const rows = byKey(
      providerDetails(
        integrationWith({
          kind: 'ses',
          ses: { region: 'us-east-1', access_key: 'AKIA', tenant_isolation_enabled: true }
        }),
        i18n
      )
    )

    expect(rows.configuration_set).toMatchObject({
      value: 'not created yet — register webhooks',
      tone: 'warning'
    })
    expect(rows.reputation).toMatchObject({
      value: 'isolation requested but not provisioned',
      tone: 'warning'
    })
  })

  it('says a custom SES configuration set is the owner’s, and a shared reputation plainly', () => {
    const rows = byKey(
      providerDetails(
        integrationWith({
          kind: 'ses',
          ses: { region: 'us-east-1', access_key: 'AKIA', configuration_set_name: 'mine' }
        }),
        i18n
      )
    )

    expect(rows.configuration_set).toMatchObject({ value: 'mine', hint: 'custom' })
    expect(rows.reputation.value).toBe('shared with the rest of this AWS account')
    expect(rows.reputation.tone).toBeUndefined()
  })

  it('describes SparkPost, Mailgun, Postmark and Mailjet', () => {
    const sparkpost = byKey(
      providerDetails(
        integrationWith({
          kind: 'sparkpost',
          sparkpost: { endpoint: 'https://api.sparkpost.com', sandbox_mode: true }
        }),
        i18n
      )
    )
    const mailgun = byKey(
      providerDetails(integrationWith({ kind: 'mailgun', mailgun: { domain: 'mg.acme.io' } }), i18n)
    )
    const postmark = byKey(
      providerDetails(integrationWith({ kind: 'postmark', postmark: {} }), i18n)
    )
    const mailjet = byKey(
      providerDetails(integrationWith({ kind: 'mailjet', mailjet: { sandbox_mode: false } }), i18n)
    )

    expect(sparkpost.endpoint.value).toBe('https://api.sparkpost.com')
    expect(sparkpost.sandbox.value).toBe('Enabled')
    expect(mailgun.domain.value).toBe('mg.acme.io')
    expect(mailgun.region.value).toBe('US')
    expect(postmark.message_stream.value).toBe('outbound')
    expect(mailjet.sandbox.value).toBe('Disabled')
  })

  it('shows the end of each secret of the provider the server gave a hint for', () => {
    const rows = byKey(
      providerDetails(
        integrationWith(
          { kind: 'mailjet', mailjet: { sandbox_mode: false } },
          { 'mailjet.api_key': 'a91c', 'mailjet.secret_key': '', 'postmark.server_token': 'zzzz' }
        ),
        i18n
      )
    )

    expect(rows['hint:mailjet.api_key']).toMatchObject({
      label: 'API key',
      value: '••••a91c',
      mono: true
    })
    // An empty hint is no hint, and a hint of another provider is not this card's.
    expect(rows['hint:mailjet.secret_key']).toBeUndefined()
    expect(rows['hint:postmark.server_token']).toBeUndefined()
  })

  it('formats the numbers in the console language', () => {
    i18n.loadAndActivate({ locale: 'fr', messages: {} })
    try {
      const rows = byKey(providerDetails(integrationWith({ kind: 'sendgrid', sendgrid: {} }), i18n))
      expect(rows.rate_limit.hint?.replace(/\s/g, ' ')).toBe('≈ 1 500 per hour · ≈ 36 000 per day')
    } finally {
      i18n.loadAndActivate({ locale: 'en', messages: {} })
    }
  })
})
