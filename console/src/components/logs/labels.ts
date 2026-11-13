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
