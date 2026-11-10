import type { Broadcast, BroadcastVariation } from '../../services/api/broadcast'
import type { Sender, Workspace } from '../../services/api/types'

// A broadcast always sends through the workspace's marketing email integration, whatever the
// category of its templates, so a variation's sender is one of that integration's.
export function getMarketingSenders(workspace?: Workspace | null): Sender[] {
  const id = workspace?.settings?.marketing_email_provider_id
  if (!id) return []
  return workspace?.integrations?.find((i) => i.id === id)?.email_provider?.senders ?? []
}

export interface ResolvedVariationSender {
  // The sender the variation leaves from, when it is one of the integration's.
  sender?: Sender
  // The variation chooses its own sender rather than the template's.
  chosen: boolean
  // The chosen sender is no longer one of the integration's: the server refuses to send.
  missing: boolean
}

// resolveVariationSender mirrors the server: the variation's sender wins over the template's,
// a template without one sends from the integration's default sender.
export function resolveVariationSender(
  senders: Sender[],
  variationSenderId?: string,
  templateSenderId?: string
): ResolvedVariationSender {
  if (variationSenderId) {
    const sender = senders.find((s) => s.id === variationSenderId)
    return { sender, chosen: true, missing: !sender }
  }
  const sender =
    (templateSenderId ? senders.find((s) => s.id === templateSenderId) : undefined) ??
    senders.find((s) => s.is_default)
  return { sender, chosen: false, missing: false }
}

export const formatSender = (sender: Sender) => `${sender.name} <${sender.email}>`

// variationsWithMissingSender returns the variations whose chosen sender is no longer one of
// the marketing integration's. The server refuses to save, schedule or resume a broadcast
// while any of them is left, and pauses one that is sending.
export function variationsWithMissingSender(
  workspace: Workspace,
  broadcast: Broadcast
): BroadcastVariation[] {
  const senders = getMarketingSenders(workspace)
  return (broadcast.test_settings?.variations || []).filter(
    (variation) => resolveVariationSender(senders, variation.sender_id).missing
  )
}
