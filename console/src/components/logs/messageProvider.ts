import type { MessageHistory } from '../../services/api/messages_history'
import type { Workspace } from '../../services/api/workspace'
import { isSmtpMarketingProvider } from '../broadcasts/access'
import type { MessageOrigin } from './logsSearch'

/** The address the server writes on the messages and webhook events of an erased contact. */
export const ERASED_EMAIL = 'DELETED_EMAIL'

type OriginFields = Pick<
  MessageHistory,
  'broadcast_id' | 'automation_id' | 'transactional_notification_id'
>

/** What sent a message, read in the order newsletter, automation, notification. */
export function messageOrigin(message: Partial<OriginFields>): MessageOrigin {
  if (message.broadcast_id) return 'broadcast'
  if (message.automation_id) return 'automation'
  if (message.transactional_notification_id) return 'transactional'
  return 'other'
}

function providerIsSmtp(workspace: Workspace | undefined, id: string | undefined): boolean {
  if (!id) return false
  return workspace?.integrations?.find((i) => i.id === id)?.email_provider?.kind === 'smtp'
}

const marketingIsSmtp = (workspace?: Workspace) => isSmtpMarketingProvider(workspace)
const transactionalIsSmtp = (workspace?: Workspace) =>
  providerIsSmtp(workspace, workspace?.settings?.transactional_email_provider_id)

/**
 * Whether the message went through an SMTP provider, which reports no delivery. Newsletters and
 * automations send with the marketing provider, notifications with the transactional one; a
 * message of no origin (a template test, a double opt-in confirmation) does not say, so either.
 */
export function rowIsSmtp(message: Partial<OriginFields>, workspace?: Workspace): boolean {
  switch (messageOrigin(message)) {
    case 'broadcast':
    case 'automation':
      return marketingIsSmtp(workspace)
    case 'transactional':
      return transactionalIsSmtp(workspace)
    case 'other':
      return marketingIsSmtp(workspace) || transactionalIsSmtp(workspace)
  }
}

/**
 * Whether the Delivered count can mean anything for the messages of this origin filter. Without one,
 * a provider that reports deliveries must be set up: an SMTP marketing provider and no
 * transactional one count nothing.
 */
export function countsDeliveries(
  workspace: Workspace | undefined,
  origin?: MessageOrigin
): boolean {
  if (origin === 'broadcast' || origin === 'automation') return !marketingIsSmtp(workspace)
  if (origin === 'transactional') return !transactionalIsSmtp(workspace)
  const marketing = workspace?.settings?.marketing_email_provider_id
  const transactional = workspace?.settings?.transactional_email_provider_id
  if (!marketing && !transactional) return true
  return (
    (Boolean(marketing) && !marketingIsSmtp(workspace)) ||
    (Boolean(transactional) && !transactionalIsSmtp(workspace))
  )
}
