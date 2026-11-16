import type { Integration, Sender, Workspace } from '../../services/api/workspace'
import type { Template } from '../../services/api/template'

/**
 * The provider a send path uses, as the server picks it (Workspace.GetEmailProviderWithIntegrationID):
 * the workspace's marketing one (broadcasts, double opt-in, an automation's Email node), its
 * transactional one (notifications), or the integration an automation's Email node names.
 */
export type SendingProvider = 'marketing' | 'transactional' | { integrationId: string }

export interface SenderInfo {
  /** The provider the send uses, if it is set and still exists. */
  provider: Integration | null
  /** The sender of the template's default language, or the provider's default; null when none. */
  sender: Sender | null
  /**
   * The template was not read (no templates.read, missing, failed, or none set): the provider's
   * default is shown, marked « (default) ».
   */
  providerDefaultOnly: boolean
  replyTo: string | null
}

export function sendingProvider(
  workspace: Pick<Workspace, 'settings' | 'integrations'>,
  sendsWith: SendingProvider
): Integration | null {
  const id =
    sendsWith === 'marketing'
      ? workspace.settings?.marketing_email_provider_id
      : sendsWith === 'transactional'
        ? workspace.settings?.transactional_email_provider_id
        : sendsWith.integrationId
  if (!id) return null
  return workspace.integrations?.find((integration) => integration.id === id) ?? null
}

export function transactionalProvider(workspace: Pick<Workspace, 'settings' | 'integrations'>): Integration | null {
  return sendingProvider(workspace, 'transactional')
}

export function defaultSender(provider: Integration | null): Sender | null {
  const senders = provider?.email_provider?.senders ?? []
  // As the server's EmailProvider.GetSender: no sender flagged default means none, and the send fails.
  return senders.find((sender) => sender.is_default) ?? null
}

/**
 * The sender a send without email_options.sender_id uses, as EmailProvider.GetSender picks it:
 * the template's default-language sender_id among the sending provider's senders (another
 * provider's sender is not looked at), else that provider's default. A translation can carry its
 * own sender and Reply-To; this is the default language's. The transactional provider unless told.
 */
export function resolveSenderInfo(
  workspace: Pick<Workspace, 'settings' | 'integrations'>,
  template: Pick<Template, 'email'> | undefined,
  sendsWith: SendingProvider = 'transactional',
  /** A sender the send path sets before the template's own, as a broadcast variant's (ResolveSender). */
  senderOverrideId?: string
): SenderInfo {
  const provider = sendingProvider(workspace, sendsWith)
  const fallback = defaultSender(provider)
  if (!template) return { provider, sender: fallback, providerDefaultOnly: true, replyTo: null }
  // As ResolveSender: a sender the send path chose is that one or none — the send fails rather
  // than falling back to the template's or the default.
  if (senderOverrideId) {
    const chosen = provider?.email_provider?.senders?.find((sender) => sender.id === senderOverrideId) ?? null
    return { provider, sender: chosen, providerDefaultOnly: false, replyTo: template.email?.reply_to?.trim() || null }
  }
  const senderId = template.email?.sender_id
  const chosen = senderId ? provider?.email_provider?.senders?.find((sender) => sender.id === senderId) : undefined
  return {
    provider,
    sender: chosen ?? fallback,
    providerDefaultOnly: false,
    replyTo: template.email?.reply_to?.trim() || null
  }
}
