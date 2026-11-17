import type { EmailProvider, EmailProviderKind, Sender } from '../../../services/api/types'
import { getProviderName } from '../../integrations/EmailProviders'
import type { EmailIntegration } from './EmailProviderCard'

export interface EmailProviderFormValues {
  name?: string
  kind: EmailProviderKind
  ses?: EmailProvider['ses']
  smtp?: EmailProvider['smtp']
  sparkpost?: EmailProvider['sparkpost']
  postmark?: EmailProvider['postmark']
  mailgun?: EmailProvider['mailgun']
  mailjet?: EmailProvider['mailjet']
  sendgrid?: EmailProvider['sendgrid']
  senders: Sender[]
  rate_limit_per_minute: number
}

export const DEFAULT_RATE_LIMIT = 25

/**
 * What the form starts from: the integration being edited, or the defaults of a new one. Only the
 * fields the form renders are submitted (antd submits registered fields), so the server-owned SES
 * names and the encrypted credentials carried here never travel back; the server keeps them.
 */
export function emailProviderFormValues(kind: EmailProviderKind, integration?: EmailIntegration): EmailProviderFormValues {
  const stored = integration?.email_provider
  return {
    // A new integration is named as the catalogue names its provider ("Amazon SES").
    name: integration ? integration.name : getProviderName(kind),
    kind,
    senders: stored?.senders ?? [],
    rate_limit_per_minute: stored?.rate_limit_per_minute || DEFAULT_RATE_LIMIT,
    ses: stored?.ses,
    smtp:
      kind === 'smtp'
        ? ({
            ...stored?.smtp,
            use_tls: stored?.smtp?.use_tls ?? true,
            auth_type: stored?.smtp?.auth_type || 'basic'
          } as EmailProvider['smtp'])
        : stored?.smtp,
    sparkpost:
      kind === 'sparkpost'
        ? ({ ...stored?.sparkpost, sandbox_mode: stored?.sparkpost?.sandbox_mode ?? false } as EmailProvider['sparkpost'])
        : stored?.sparkpost,
    postmark:
      kind === 'postmark'
        ? { ...stored?.postmark, message_stream: stored?.postmark?.message_stream || 'outbound' }
        : stored?.postmark,
    mailgun:
      kind === 'mailgun'
        ? ({ ...stored?.mailgun, region: stored?.mailgun?.region ?? 'US' } as EmailProvider['mailgun'])
        : stored?.mailgun,
    mailjet:
      kind === 'mailjet'
        ? ({ ...stored?.mailjet, sandbox_mode: stored?.mailjet?.sandbox_mode ?? false } as EmailProvider['mailjet'])
        : stored?.mailjet,
    sendgrid: stored?.sendgrid
  }
}

/** The provider the form describes: its kind's settings only, with the senders and the rate. */
export function providerFromFormValues(values: EmailProviderFormValues): EmailProvider {
  const provider: EmailProvider = {
    kind: values.kind,
    senders: values.senders || [],
    rate_limit_per_minute: values.rate_limit_per_minute || DEFAULT_RATE_LIMIT
  }
  if (values.kind === 'ses' && values.ses) provider.ses = values.ses
  else if (values.kind === 'smtp' && values.smtp) provider.smtp = values.smtp
  else if (values.kind === 'sparkpost' && values.sparkpost) provider.sparkpost = values.sparkpost
  else if (values.kind === 'postmark' && values.postmark) provider.postmark = values.postmark
  else if (values.kind === 'mailgun' && values.mailgun) provider.mailgun = values.mailgun
  else if (values.kind === 'mailjet' && values.mailjet) provider.mailjet = values.mailjet
  else if (values.kind === 'sendgrid' && values.sendgrid) provider.sendgrid = values.sendgrid
  return provider
}

/**
 * A field's label as antd's required message reads it mid-sentence ("Please enter ${label}"): its
 * first letter lowered, "Server token" → "server token", but not an acronym ("API key") nor in
 * German, where nouns keep their capital.
 */
export function labelInSentence(label: string, locale: string): string {
  if (locale.toLowerCase().startsWith('de')) return label
  return /^\p{Lu}\p{Ll}/u.test(label) ? label.charAt(0).toLowerCase() + label.slice(1) : label
}
