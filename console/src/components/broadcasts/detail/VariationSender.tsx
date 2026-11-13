import { useLingui } from '@lingui/react/macro'
import type { BroadcastVariation } from '../../../services/api/broadcast'
import type { Workspace } from '../../../services/api/types'
import { resolveSenderOf, useSenderText } from './senderText'

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
