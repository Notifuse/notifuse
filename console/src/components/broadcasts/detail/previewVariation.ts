import type { Broadcast, BroadcastVariation } from '../../../services/api/broadcast'

/** The variant whose template the detail previews: the winner once there is one, else the first. */
export function previewVariationOf(
  broadcast: Broadcast,
  winningTemplate?: string
): BroadcastVariation | undefined {
  const variations = broadcast.test_settings?.variations ?? []
  const winner = winningTemplate || broadcast.winning_template
  return variations.find((v) => winner && v.template_id === winner) ?? variations[0]
}
