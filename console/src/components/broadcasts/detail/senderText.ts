import { useLingui } from '@lingui/react/macro'
import type { BroadcastVariation } from '../../../services/api/broadcast'
import type { Workspace } from '../../../services/api/types'
import { formatSender, getMarketingSenders, resolveVariationSender } from '../variationSender'

/** The sender a variant leaves from, and whether the broadcast chose it. */
export function resolveSenderOf(workspace: Workspace | undefined, variation: BroadcastVariation) {
  const resolved = resolveVariationSender(
    getMarketingSenders(workspace),
    variation.sender_id,
    variation.template?.email?.sender_id
  )
  return { ...resolved, senderId: variation.sender_id }
}

/**
 * The key the header compares to tell "Several senders": the resolved sender's id, or the chosen
 * id when that sender is gone, or '' for the integration's default.
 */
export function senderKey(workspace: Workspace | undefined, variation: BroadcastVariation): string {
  const resolved = resolveSenderOf(workspace, variation)
  return resolved.sender?.id ?? resolved.senderId ?? ''
}

/** "Name <email>", "{id} (deleted)" or "Default sender". */
export function useSenderText() {
  const { t } = useLingui()
  return (workspace: Workspace | undefined, variation: BroadcastVariation) => {
    const resolved = resolveSenderOf(workspace, variation)
    if (resolved.sender) return formatSender(resolved.sender)
    const senderId = resolved.senderId
    if (resolved.missing) return t`${senderId} (deleted)`
    return t`Default sender`
  }
}
