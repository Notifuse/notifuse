import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'
import type { PermissionResource } from '../../services/api/permissions'

/** The name of each permission resource, shared by the permissions matrix and the access summary. */
export const PERMISSION_RESOURCE_LABELS: Record<PermissionResource, MessageDescriptor> = {
  contacts: msg`Contacts`,
  lists: msg`Lists`,
  templates: msg`Templates`,
  broadcasts: msg`Broadcasts`,
  transactional: msg`Transactional`,
  workspace: msg`Workspace`,
  message_history: msg`Message History`,
  blog: msg`Blog`,
  automations: msg`Automations`,
  llm: msg`LLM`,
  web_analytics: msg`Web Analytics`,
  segments: msg`Segments`,
  webhook_subscriptions: msg`Webhook Subscriptions`,
  webhook_events: msg`Webhook Events`,
  audit_logs: msg`Audit Logs`
}
