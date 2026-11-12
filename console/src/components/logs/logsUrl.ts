// Keys of the Logs page tabs, as they appear in its `tab` URL parameter.
export const LOGS_TAB = {
  messages: 'messages',
  incomingWebhooks: 'incoming-webhooks',
  outgoingWebhooks: 'outgoing-webhooks'
} as const

export type LogsTabKey = (typeof LOGS_TAB)[keyof typeof LOGS_TAB]

export interface LogsUrlFilter {
  field: string
  value: string
}

// URL of a Logs tab with its filters. Each tab writes its own key rather than reading
// `tab` back from the location: the router writes the URL in a microtask, so a tab
// just switched to could read the previous one. Without the tab, a copied link of a
// webhooks tab reopened the messages.
export function buildLogsTabUrl(
  pathname: string,
  tab: LogsTabKey,
  filters: LogsUrlFilter[]
): string {
  const searchParams = new URLSearchParams()
  searchParams.set('tab', tab)
  filters.forEach((filter) => {
    searchParams.set(filter.field, filter.value)
  })
  return `${pathname}?${searchParams.toString()}`
}
