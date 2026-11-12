import type { PillTone } from '../components/layout'
import type { MessageHistory } from '../services/api/messages_history'

export type MessageStatusKey =
  | 'failed'
  | 'complained'
  | 'bounced'
  | 'unsubscribed'
  | 'clicked'
  | 'opened'
  | 'delivered'
  | 'sent'

export interface MessageStatus {
  key: MessageStatusKey
  tone: PillTone
}

type MessageEvents = Pick<
  MessageHistory,
  'failed_at' | 'complained_at' | 'bounced_at' | 'unsubscribed_at' | 'clicked_at' | 'opened_at' | 'delivered_at'
>

// Most advanced first: the first event a message has is its status.
const ORDER: { key: Exclude<MessageStatusKey, 'sent'>; field: keyof MessageEvents; tone: PillTone }[] = [
  { key: 'failed', field: 'failed_at', tone: 'danger' },
  { key: 'complained', field: 'complained_at', tone: 'danger' },
  { key: 'bounced', field: 'bounced_at', tone: 'warning' },
  { key: 'unsubscribed', field: 'unsubscribed_at', tone: 'warning' },
  { key: 'clicked', field: 'clicked_at', tone: 'primary' },
  { key: 'opened', field: 'opened_at', tone: 'info' },
  { key: 'delivered', field: 'delivered_at', tone: 'success' }
]

/**
 * The most advanced status of a message. There is no SMTP case: without delivery webhooks, an
 * accepted message stays Sent until it is opened or clicked, which is all that is known of it.
 */
export function messageStatus(message: MessageEvents): MessageStatus {
  const found = ORDER.find(({ field }) => Boolean(message[field]))
  return found ? { key: found.key, tone: found.tone } : { key: 'sent', tone: 'neutral' }
}
