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

export interface VariationSenderProps {
  workspace?: Workspace
  variation: BroadcastVariation
}

/** The From of a variant, with "chosen on the broadcast" and its Reply-To. */
export function VariationSender({ workspace, variation }: VariationSenderProps) {
  const { t } = useLingui()
  const senderText = useSenderText()
  const resolved = resolveSenderOf(workspace, variation)
  const replyTo = variation.template?.email?.reply_to
  return (
    <div className="flex min-w-0 flex-col">
      <span className={`truncate text-sm ${resolved.missing ? 'text-danger-fg' : 'text-ink'}`}>
        {senderText(workspace, variation)}
      </span>
      {resolved.chosen ? <span className="text-xs text-ink-3">{t`chosen on the broadcast`}</span> : null}
      {replyTo ? <span className="truncate text-xs text-ink-3">{t`Reply-To: ${replyTo}`}</span> : null}
    </div>
  )
}
