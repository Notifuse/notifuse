import type { I18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'
import type { EmailProviderKind, Integration } from '../../../services/api/types'

// Provider types that only support transactional emails, not marketing emails
export const transactionalEmailOnly: EmailProviderKind[] = ['mailjet']

/** One line of the body of an email provider card. Labels and values arrive translated. */
export interface DetailRow {
  key: string
  label: string
  value: string
  /** The value is an identifier or a host, set in the code typeface. */
  mono?: boolean
  /** A second line under the value. */
  hint?: string
  /** A state that needs the owner's attention. */
  tone?: 'warning'
}

// The labels of the credential hints the server returns (workspace_redact.go), by key.
const CREDENTIAL_LABELS: Record<string, MessageDescriptor> = {
  'ses.secret_key': msg`Secret key`,
  'smtp.password': msg`Password`,
  'smtp.oauth2_client_secret': msg`OAuth2 client secret`,
  'smtp.oauth2_refresh_token': msg`OAuth2 refresh token`,
  'sparkpost.api_key': msg`API key`,
  'postmark.server_token': msg`Server token`,
  'mailgun.api_key': msg`API key`,
  'mailjet.api_key': msg`API key`,
  'mailjet.secret_key': msg`Secret key`,
  'sendgrid.api_key': msg`API key`
}

/** The last characters of a secret, as the server reveals them. */
export const maskedHint = (hint: string) => `••••${hint}`

/**
 * The details an email provider card shows: what the integration is set to for its provider,
 * the marketing rate limit, then the end of each stored secret. No secret without a hint.
 */
export function providerDetails(integration: Integration, i18n: I18n): DetailRow[] {
  const provider = integration.email_provider
  if (!provider) return []
  const t = (descriptor: MessageDescriptor) => i18n._(descriptor)
  const number = new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 0 })
  const enabled = (on: boolean | undefined) => (on ? t(msg`Enabled`) : t(msg`Disabled`))
  const rows: DetailRow[] = []

  if (provider.kind === 'smtp' && provider.smtp) {
    const smtp = provider.smtp
    const oauthProvider = smtp.oauth2_provider === 'microsoft' ? 'Microsoft 365' : 'Google'
    rows.push(
      { key: 'host', label: t(msg`SMTP host`), value: `${smtp.host}:${smtp.port}`, mono: true },
      { key: 'username', label: t(msg`SMTP user`), value: smtp.username },
      { key: 'tls', label: t(msg`TLS`), value: smtp.use_tls ? t(msg`Yes`) : t(msg`No`) },
      {
        key: 'auth',
        label: t(msg`Authentication`),
        value:
          smtp.auth_type === 'oauth2'
            ? i18n._(msg`OAuth2 (${oauthProvider})`)
            : t(msg`Basic (username and password)`)
      }
    )
    if (smtp.ehlo_hostname) {
      rows.push({
        key: 'ehlo',
        label: t(msg`EHLO hostname`),
        value: smtp.ehlo_hostname,
        mono: true
      })
    }
  } else if (provider.kind === 'ses' && provider.ses) {
    const ses = provider.ses
    const configurationSet = ses.configuration_set_name || ses.managed_configuration_set
    const tenant = ses.tenant_name || ses.managed_tenant_name
    rows.push({ key: 'region', label: t(msg`AWS region`), value: ses.region, mono: true })
    rows.push(
      configurationSet
        ? {
            key: 'configuration_set',
            label: t(msg`Configuration set`),
            value: configurationSet,
            mono: true,
            hint: ses.configuration_set_name ? t(msg`custom`) : t(msg`managed`)
          }
        : {
            key: 'configuration_set',
            label: t(msg`Configuration set`),
            value: t(msg`not created yet — register webhooks`),
            tone: 'warning'
          }
    )
    if (tenant) {
      rows.push({
        key: 'reputation',
        label: t(msg`Reputation`),
        value: tenant,
        mono: true,
        hint: t(msg`isolated`)
      })
    } else if (ses.tenant_isolation_enabled) {
      // Intent recorded but nothing provisioned: the state that must never look fine.
      rows.push({
        key: 'reputation',
        label: t(msg`Reputation`),
        value: t(msg`isolation requested but not provisioned`),
        tone: 'warning'
      })
    } else {
      rows.push({
        key: 'reputation',
        label: t(msg`Reputation`),
        value: t(msg`shared with the rest of this AWS account`)
      })
    }
  } else if (provider.kind === 'sparkpost' && provider.sparkpost) {
    rows.push(
      {
        key: 'endpoint',
        label: t(msg`API endpoint`),
        value: provider.sparkpost.endpoint,
        mono: true
      },
      {
        key: 'sandbox',
        label: t(msg`Sandbox mode`),
        value: enabled(provider.sparkpost.sandbox_mode)
      }
    )
  } else if (provider.kind === 'mailgun' && provider.mailgun) {
    rows.push(
      { key: 'domain', label: t(msg`Domain`), value: provider.mailgun.domain, mono: true },
      { key: 'region', label: t(msg`Region`), value: provider.mailgun.region || 'US' }
    )
  } else if (provider.kind === 'postmark' && provider.postmark) {
    rows.push({
      key: 'message_stream',
      label: t(msg`Message stream`),
      value: provider.postmark.message_stream || 'outbound',
      mono: true
    })
  } else if (provider.kind === 'mailjet' && provider.mailjet) {
    rows.push({
      key: 'sandbox',
      label: t(msg`Sandbox mode`),
      value: enabled(provider.mailjet.sandbox_mode)
    })
  }

  const perMinute = provider.rate_limit_per_minute
  const perMinuteText = number.format(perMinute)
  const perHour = number.format(perMinute * 60)
  const perDay = number.format(perMinute * 60 * 24)
  rows.push({
    key: 'rate_limit',
    label: t(msg`Rate limit for marketing`),
    value: i18n._(msg`${perMinuteText} emails/min`),
    hint: i18n._(msg`≈ ${perHour} per hour · ≈ ${perDay} per day`)
  })

  const prefix = `${provider.kind}.`
  for (const [key, hint] of Object.entries(integration.credential_hints ?? {})) {
    const label = CREDENTIAL_LABELS[key]
    if (!key.startsWith(prefix) || !label || !hint) continue
    rows.push({ key: `hint:${key}`, label: t(label), value: maskedHint(hint), mono: true })
  }

  return rows
}
