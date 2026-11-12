import { useQueries } from '@tanstack/react-query'
import type { Broadcast } from '../../../services/api/broadcast'
import { getBroadcastVariationStats, type MessageHistoryStatusSum } from '../../../services/api/messages_history'

/** F4: the message-history totals of each variant of an A/B test, by template id. */
export function useVariationStats(
  workspaceId: string,
  broadcast: Broadcast | undefined,
  options: { enabled: boolean; refetchInterval?: number | false }
): Record<string, MessageHistoryStatusSum> {
  const variations = broadcast?.test_settings?.enabled ? broadcast.test_settings.variations ?? [] : []
  const results = useQueries({
    queries: variations.map((v) => ({
      queryKey: ['broadcast-variation-stats', workspaceId, broadcast?.id, v.template_id],
      queryFn: () => getBroadcastVariationStats(workspaceId, broadcast!.id, v.template_id),
      enabled: options.enabled && !!broadcast && !!v.template_id,
      staleTime: 60_000,
      refetchInterval: options.refetchInterval
    }))
  })
  const byId: Record<string, MessageHistoryStatusSum> = {}
  variations.forEach((v, i) => {
    const stats = results[i]?.data?.stats
    if (v.template_id && stats) byId[v.template_id] = stats
  })
  return byId
}
