import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'
import type { RetentionCategory } from '../../../services/api/dataRetention'

export const CATEGORY_LABELS: Record<RetentionCategory, MessageDescriptor> = {
  inactive_contacts: msg`Inactive contacts`,
  message_history: msg`Message history`,
  contact_timeline: msg`Contact timeline`,
  custom_events: msg`Custom events`,
  automation_history: msg`Automation history`,
  web_analytics: msg`Web analytics`,
  inbound_webhook_events: msg`Provider events`,
  webhook_deliveries: msg`Webhook deliveries`,
  failed_emails: msg`Failed emails`
}

export const CATEGORY_DESCRIPTIONS: Record<RetentionCategory, MessageDescriptor> = {
  inactive_contacts: msg`Contacts who have not clicked, replied, confirmed or visited for this long. They are deleted with their history; their unsubscribes are kept as hashes for 3 years.`,
  message_history: msg`Sent emails older than this are anonymized. The address and the content go; the counts in broadcast reports stay.`,
  contact_timeline: msg`Timeline entries older than this are deleted.`,
  custom_events: msg`Events older than this are deleted. An event that counts as activity is copied to the contact's last activity first.`,
  automation_history: msg`Finished journeys older than this are deleted. Contacts are never enrolled twice in a one-time automation.`,
  web_analytics: msg`Whole months older than this are deleted.`,
  inbound_webhook_events: msg`Deliveries, bounces and replies reported by your email provider.`,
  webhook_deliveries: msg`Outgoing webhook calls and their responses.`,
  failed_emails: msg`Emails your provider refused and that will not be retried.`
}

/** Why a run skipped a category, as the report says it. */
export const SKIP_REASONS: Record<string, MessageDescriptor> = {
  off: msg`Off`,
  backfill_pending: msg`Waiting for past activity to be read`,
  exemption_unavailable: msg`An exempt list or segment is missing or being rebuilt`
}
