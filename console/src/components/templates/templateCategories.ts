import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'

/** The server's template categories, in the order the console lists them. */
export const TEMPLATE_CATEGORIES = [
  'marketing',
  'transactional',
  'welcome',
  'opt_in',
  'unsubscribe',
  'bounce',
  'blocklist',
  'other',
  'blog'
] as const

export type TemplateCategory = (typeof TEMPLATE_CATEGORIES)[number]

export const TEMPLATE_CATEGORY_LABELS: Record<TemplateCategory, MessageDescriptor> = {
  marketing: msg`Marketing`,
  transactional: msg`Transactional`,
  welcome: msg`Welcome`,
  opt_in: msg`Opt-in`,
  unsubscribe: msg`Unsubscribe`,
  bounce: msg`Bounce`,
  blocklist: msg`Blocklist`,
  other: msg`Other`,
  blog: msg`Blog`
}

export function isTemplateCategory(value: unknown): value is TemplateCategory {
  return typeof value === 'string' && (TEMPLATE_CATEGORIES as readonly string[]).includes(value)
}
