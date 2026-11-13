import type { EmailTemplate, Template } from '../../services/api/template'

/** The language a message was sent in: the contact's, as the send put it in the message data. */
export function messageLanguage(
  data: Record<string, unknown> | null | undefined
): string | undefined {
  const contact = data?.contact
  if (!contact || typeof contact !== 'object') return undefined
  const language = (contact as Record<string, unknown>).language
  return typeof language === 'string' && language ? language : undefined
}

/**
 * The email a template sends in a language, as the server picks it (Template.ResolveEmailContent):
 * the translation when the language is not the workspace default and the template has one, taking
 * the sender and reply-to of the default content when it leaves them empty; else the default.
 */
export function resolveEmail(
  template: Template,
  language: string | undefined,
  defaultLanguage: string
): EmailTemplate | undefined {
  if (!template.email || !language || language === defaultLanguage) return template.email
  const translated = template.translations?.[language]?.email
  if (!translated) return template.email
  return {
    ...translated,
    sender_id: translated.sender_id || template.email.sender_id,
    reply_to: translated.reply_to || template.email.reply_to
  }
}
