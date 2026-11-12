import type { EmailTemplate, Template } from '../services/api/template'

/**
 * A Liquid output of one of the unsubscribe variables, filters allowed: `{{ unsubscribe_url }}`,
 * `{{- oneclick_unsubscribe_url | escape -}}`. The word boundary keeps `unsubscribe_url_text` out.
 */
const UNSUBSCRIBE_OUTPUT =
  /\{\{-?\s*(?:unsubscribe_url|oneclick_unsubscribe_url|notification_center_url)\b\s*(?:\|[^}]*)?-?\}\}/

/** The text of an email the check reads: the MJML source in code mode, the visual tree otherwise. */
function emailContent(email: EmailTemplate | undefined): string {
  if (!email) return ''
  if (email.editor_mode === 'code') return email.mjml_source ?? ''
  const tree: unknown = email.visual_editor_tree
  if (!tree) return ''
  return typeof tree === 'string' ? tree : JSON.stringify(tree)
}

export const hasUnsubscribeLink = (email: EmailTemplate | undefined) => UNSUBSCRIBE_OUTPUT.test(emailContent(email))

/**
 * The languages of a template whose email has no unsubscribe link: `default` for the default
 * content, then each translation that has an email. Empty when every language has one.
 */
export function templateUnsubscribeCheck(template: Pick<Template, 'email' | 'translations'>): string[] {
  const missing: string[] = []
  if (!hasUnsubscribeLink(template.email)) missing.push('default')
  for (const [language, translation] of Object.entries(template.translations ?? {})) {
    if (translation?.email && !hasUnsubscribeLink(translation.email)) missing.push(language)
  }
  return missing
}
