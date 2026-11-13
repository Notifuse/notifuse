// Keys of the Logs page tabs, as they appear in its `tab` URL parameter.
export const LOGS_TAB = {
  messages: 'messages',
  incomingWebhooks: 'incoming-webhooks',
  outgoingWebhooks: 'outgoing-webhooks',
  audit: 'audit'
} as const

export type LogsTabKey = (typeof LOGS_TAB)[keyof typeof LOGS_TAB]
