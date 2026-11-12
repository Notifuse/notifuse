import type { QueryClient } from '@tanstack/react-query'

/**
 * The query keys of the webhooks settings. The list key is the one the Outgoing webhooks tab of
 * Logs uses for the same call, so both read one cache entry, which holds the raw response.
 */
export const webhookQueryKeys = {
  /** W1: webhookSubscriptions.list, `{ subscriptions }`. */
  list: (workspaceId: string) => ['webhook-subscriptions', workspaceId] as const,
  /** W2: the 7-day success rate of every subscription, from a time fixed when the section opens. */
  successRates: (workspaceId: string, from: string) => ['webhook-success-7d', workspaceId, from] as const,
  /** W3: the deliveries of one subscription over 24 hours or 7 days. */
  stats: (workspaceId: string, id: string, range: string) => ['webhook-stats', workspaceId, id, range] as const,
  /** W4: webhookSubscriptions.get, `{ subscription }`. */
  subscription: (workspaceId: string, id: string) => ['webhook-subscription', workspaceId, id] as const
}

/** After any write on a subscription: the list, the rates and every open subscription. */
export function invalidateWebhookQueries(queryClient: QueryClient, workspaceId: string) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: webhookQueryKeys.list(workspaceId) }),
    queryClient.invalidateQueries({ queryKey: ['webhook-success-7d', workspaceId] }),
    queryClient.invalidateQueries({ queryKey: ['webhook-subscription', workspaceId] })
  ])
}
