import type { MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'

/** Audit categories, as `auditLogs.actions` names them. */
export const AUDIT_CATEGORY_LABELS: Record<string, MessageDescriptor> = {
  auth: msg`Sign-in`,
  licence: msg`Licence`,
  system: msg`System`,
  workspaces: msg`Workspace`,
  members: msg`Members`,
  api_keys: msg`API keys`,
  integrations: msg`Integrations`,
  webhooks: msg`Webhooks`,
  templates: msg`Templates`,
  broadcasts: msg`Newsletters`,
  automations: msg`Automations`,
  lists: msg`Lists`,
  segments: msg`Segments`,
  transactional: msg`Transactional`,
  blog: msg`Blog`,
  contacts: msg`Contacts`,
  audit: msg`Audit log`
}

export const AUDIT_OUTCOME_LABELS: Record<'success' | 'failure' | 'denied', MessageDescriptor> = {
  success: msg`Success`,
  failure: msg`Failed`,
  denied: msg`Denied`
}

/** Status of a webhook delivery; Retrying is a failed delivery with attempts left. */
export const DELIVERY_STATUS_LABELS: Record<
  'pending' | 'delivering' | 'delivered' | 'retrying' | 'failed',
  MessageDescriptor
> = {
  pending: msg`Pending`,
  delivering: msg`Delivering`,
  delivered: msg`Delivered`,
  retrying: msg`Retrying`,
  failed: msg`Failed`
}

/** Providers and services that send webhooks to Notifuse. */
export const WEBHOOK_SOURCE_LABELS: Record<
  'ses' | 'postmark' | 'mailgun' | 'sparkpost' | 'mailjet' | 'smtp' | 'sendgrid' | 'supabase',
  MessageDescriptor
> = {
  ses: msg`Amazon SES`,
  postmark: msg`Postmark`,
  mailgun: msg`Mailgun`,
  sparkpost: msg`SparkPost`,
  mailjet: msg`Mailjet`,
  smtp: msg`SMTP`,
  sendgrid: msg`SendGrid`,
  supabase: msg`Supabase`
}

/** Types of incoming webhook events. */
export const INBOUND_EVENT_LABELS: Record<
  | 'delivered'
  | 'bounce'
  | 'complaint'
  | 'unsubscribe'
  | 'reply'
  | 'auto_reply'
  | 'auth_email'
  | 'before_user_created',
  MessageDescriptor
> = {
  delivered: msg`Delivered`,
  bounce: msg`Bounce`,
  complaint: msg`Complaint`,
  unsubscribe: msg`Unsubscribe`,
  reply: msg`Reply`,
  auto_reply: msg`Auto-reply`,
  auth_email: msg`Auth email`,
  before_user_created: msg`User created`
}

/** Origins of a message, as the Origin filter and the Message column name them. */
export const MESSAGE_ORIGIN_LABELS: Record<
  'broadcast' | 'automation' | 'transactional' | 'other',
  MessageDescriptor
> = {
  broadcast: msg`Newsletter`,
  automation: msg`Automation`,
  transactional: msg`Transactional`,
  other: msg`Other`
}

/** The most advanced status of a message (lib/messageStatus). */
export const MESSAGE_STATUS_LABELS: Record<
  | 'failed'
  | 'complained'
  | 'bounced'
  | 'unsubscribed'
  | 'clicked'
  | 'opened'
  | 'delivered'
  | 'sent',
  MessageDescriptor
> = {
  failed: msg`Failed`,
  complained: msg`Complaint`,
  bounced: msg`Bounced`,
  unsubscribed: msg`Unsubscribed`,
  clicked: msg`Clicked`,
  opened: msg`Opened`,
  delivered: msg`Delivered`,
  sent: msg`Sent`
}
