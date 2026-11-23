import type { MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { Contact } from '../../services/api/contacts'
import type { PillTone } from '../layout'

type Named = Pick<Contact, 'email' | 'full_name' | 'first_name' | 'last_name'>

/** Full name, else first and last name, else an empty string. */
export function contactDisplayName(contact: Named): string {
  return contact.full_name?.trim() || [contact.first_name, contact.last_name].filter(Boolean).join(' ').trim()
}

const initial = (word: string | undefined) => (word ? Array.from(word)[0].toUpperCase() : '')

/** First letters of the first and last words of the name, else of the email's local part. */
export function contactInitials(contact: Named): string {
  const name = contactDisplayName(contact)
  const words = (name || contact.email.split('@')[0].replace(/[._+-]+/g, ' ')).split(/\s+/).filter(Boolean)
  if (words.length === 0) return initial(contact.email)
  if (words.length === 1) return initial(words[0])
  return initial(words[0]) + initial(words[words.length - 1])
}

/** The tone of a list subscription status. */
export const LIST_STATUS_TONE: Record<string, PillTone> = {
  active: 'success',
  pending: 'warning',
  unsubscribed: 'neutral',
  bounced: 'danger',
  complained: 'danger'
}

/** The name of a list subscription status. */
export const LIST_STATUS_LABEL: Record<string, MessageDescriptor> = {
  active: msg`Active`,
  pending: msg`Pending`,
  unsubscribed: msg`Unsubscribed`,
  bounced: msg`Bounced`,
  complained: msg`Complained`
}

const LIST_STATUSES = ['active', 'pending', 'unsubscribed', 'bounced', 'complained'] as const

const OPT_OUTS: readonly string[] = ['unsubscribed', 'bounced', 'complained']

/**
 * The statuses "Change status" offers from the current one, as the server allows them: a
 * contact who opted out only moves to a stronger opt-out (unsubscribed, then bounced, then
 * complained). Bringing them back is "Re-subscribe", which says what it overrides.
 */
export function contactListStatusOptions(current: string): string[] {
  const rank = OPT_OUTS.indexOf(current)
  if (rank < 0) return [...LIST_STATUSES]
  return OPT_OUTS.slice(rank)
}

/** Whether "Re-subscribe" applies: an unsubscribe, or a bounce once the address works again. */
export function canResubscribe(status: string): boolean {
  return status === 'unsubscribed' || status === 'bounced'
}
