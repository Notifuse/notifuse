import type { Integration, Sender, Workspace } from '../../services/api/workspace'
import type { Template } from '../../services/api/template'

export interface SenderInfo {
  /** The workspace's transactional provider, if it is set and still exists. */
  provider: Integration | null
  /** The sender of the template's default language, or the provider's default; null when none. */
  sender: Sender | null
  /** The template was not read (no templates.read): the provider's default is shown, marked « (default) ». */
  providerDefaultOnly: boolean
  replyTo: string | null
}

export function transactionalProvider(workspace: Pick<Workspace, 'settings' | 'integrations'>): Integration | null {
  const id = workspace.settings?.transactional_email_provider_id
  if (!id) return null
  return workspace.integrations?.find((integration) => integration.id === id) ?? null
}

export function defaultSender(provider: Integration | null): Sender | null {
  const senders = provider?.email_provider?.senders ?? []
  // As the server's EmailProvider.GetSender: no sender flagged default means none, and the send fails.
  return senders.find((sender) => sender.is_default) ?? null
}

/**
 * The sender a send without email_options.sender_id uses, as EmailProvider.GetSender picks it:
 * the template's default-language sender_id among the provider's senders, else the provider's
 * default. A translation can carry its own sender and Reply-To; this is the default language's.
 */
export function resolveSenderInfo(
  workspace: Pick<Workspace, 'settings' | 'integrations'>,
  template: Pick<Template, 'email'> | undefined
): SenderInfo {
  const provider = transactionalProvider(workspace)
  const fallback = defaultSender(provider)
  if (!template) return { provider, sender: fallback, providerDefaultOnly: true, replyTo: null }
  const senderId = template.email?.sender_id
  const chosen = senderId ? provider?.email_provider?.senders?.find((sender) => sender.id === senderId) : undefined
  return {
    provider,
    sender: chosen ?? fallback,
    providerDefaultOnly: false,
    replyTo: template.email?.reply_to?.trim() || null
  }
}
