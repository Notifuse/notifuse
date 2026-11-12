import type { MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'

/**
 * One sentence per audited action, starting with the verb: the actor is in its own column.
 * `<0>{target}</0>` is the target's name (or id), drawn bold. One entry per line, in this exact form:
 * internal/domain/audit_actions_test.go reads this block and checks it against the server's catalogue.
 */
export const AUDIT_PHRASES: Record<string, MessageDescriptor> = {
  'annotations.create': msg({ message: 'Added the annotation <0>{target}</0>' }),
  'annotations.delete': msg({ message: 'Deleted the annotation <0>{target}</0>' }),
  'annotations.update': msg({ message: 'Updated the annotation <0>{target}</0>' }),
  'audit.purged': msg({ message: 'Deleted entries past the retention period' }),
  'auditLogs.export': msg({ message: 'Exported the audit log' }),
  'automations.activate': msg({ message: 'Activated the automation <0>{target}</0>' }),
  'automations.create': msg({ message: 'Created the automation <0>{target}</0>' }),
  'automations.delete': msg({ message: 'Deleted the automation <0>{target}</0>' }),
  'automations.pause': msg({ message: 'Paused the automation <0>{target}</0>' }),
  'automations.update': msg({ message: 'Updated the automation <0>{target}</0>' }),
  'blogCategories.create': msg({ message: 'Created the blog category <0>{target}</0>' }),
  'blogCategories.delete': msg({ message: 'Deleted the blog category <0>{target}</0>' }),
  'blogCategories.update': msg({ message: 'Updated the blog category <0>{target}</0>' }),
  'blogPosts.create': msg({ message: 'Created the blog post <0>{target}</0>' }),
  'blogPosts.delete': msg({ message: 'Deleted the blog post <0>{target}</0>' }),
  'blogPosts.publish': msg({ message: 'Published the blog post <0>{target}</0>' }),
  'blogPosts.unpublish': msg({ message: 'Unpublished the blog post <0>{target}</0>' }),
  'blogPosts.update': msg({ message: 'Updated the blog post <0>{target}</0>' }),
  'blogThemes.create': msg({ message: 'Created the blog theme <0>{target}</0>' }),
  'blogThemes.publish': msg({ message: 'Published the blog theme <0>{target}</0>' }),
  'blogThemes.update': msg({ message: 'Updated the blog theme <0>{target}</0>' }),
  'broadcasts.cancel': msg({ message: 'Cancelled the newsletter <0>{target}</0>' }),
  'broadcasts.create': msg({ message: 'Created the newsletter <0>{target}</0>' }),
  'broadcasts.delete': msg({ message: 'Deleted the newsletter <0>{target}</0>' }),
  'broadcasts.pause': msg({ message: 'Paused the newsletter <0>{target}</0>' }),
  'broadcasts.resume': msg({ message: 'Resumed the newsletter <0>{target}</0>' }),
  'broadcasts.retryFailed': msg({
    message: 'Retried the failed sends of the newsletter <0>{target}</0>'
  }),
  'broadcasts.schedule': msg({ message: 'Scheduled the newsletter <0>{target}</0>' }),
  'broadcasts.selectWinner': msg({
    message: 'Picked the winning version of the newsletter <0>{target}</0>'
  }),
  'broadcasts.sendToIndividual': msg({
    message: 'Sent the newsletter <0>{target}</0> to one recipient'
  }),
  'broadcasts.update': msg({ message: 'Updated the newsletter <0>{target}</0>' }),
  'contacts.delete': msg({ message: 'Deleted the contact <0>{target}</0>' }),
  'contacts.export': msg({ message: 'Exported contacts' }),
  'contacts.import': msg({ message: 'Imported contacts' }),
  'customEvents.import': msg({ message: 'Imported custom events' }),
  'licence.recordingResumed': msg({ message: 'Recording resumed' }),
  'licence.recordingStopped': msg({ message: 'Recording stopped: no valid licence' }),
  'licence.set': msg({ message: 'Set the licence <0>{target}</0>' }),
  'lists.create': msg({ message: 'Created the list <0>{target}</0>' }),
  'lists.delete': msg({ message: 'Deleted the list <0>{target}</0>' }),
  'lists.update': msg({ message: 'Updated the list <0>{target}</0>' }),
  'segments.create': msg({ message: 'Created the segment <0>{target}</0>' }),
  'segments.delete': msg({ message: 'Deleted the segment <0>{target}</0>' }),
  'segments.rebuild': msg({ message: 'Rebuilt the segment <0>{target}</0>' }),
  'segments.update': msg({ message: 'Updated the segment <0>{target}</0>' }),
  'ses.enableTenantIsolation': msg({
    message: 'Turned on Amazon SES tenant isolation for <0>{target}</0>'
  }),
  'settings.update': msg({ message: 'Updated the system settings' }),
  'setup.initialize': msg({ message: 'Set up this Notifuse installation' }),
  'templateBlocks.create': msg({ message: 'Saved the block <0>{target}</0>' }),
  'templateBlocks.delete': msg({ message: 'Deleted the saved block <0>{target}</0>' }),
  'templateBlocks.update': msg({ message: 'Updated the saved block <0>{target}</0>' }),
  'templates.create': msg({ message: 'Created the template <0>{target}</0>' }),
  'templates.delete': msg({ message: 'Deleted the template <0>{target}</0>' }),
  'templates.update': msg({ message: 'Updated the template <0>{target}</0>' }),
  'transactional.create': msg({
    message: 'Created the transactional notification <0>{target}</0>'
  }),
  'transactional.delete': msg({
    message: 'Deleted the transactional notification <0>{target}</0>'
  }),
  'transactional.update': msg({
    message: 'Updated the transactional notification <0>{target}</0>'
  }),
  'user.logout': msg({ message: 'Signed out <0>{target}</0>' }),
  'user.oidc.callback': msg({ message: 'Signed in with single sign-on' }),
  'user.oidc.exchange': msg({ message: 'Signed in with single sign-on as <0>{target}</0>' }),
  'user.rootSignin': msg({ message: 'Signed in as root <0>{target}</0>' }),
  'user.signin': msg({ message: 'Signed in as <0>{target}</0>' }),
  'user.verify': msg({ message: 'Verified the sign-in code of <0>{target}</0>' }),
  'webAnalytics.backfillCancel': msg({ message: 'Cancelled the web analytics backfill' }),
  'webAnalytics.backfillStart': msg({ message: 'Started the web analytics backfill' }),
  'webhookSubscriptions.create': msg({ message: 'Created the webhook <0>{target}</0>' }),
  'webhookSubscriptions.delete': msg({ message: 'Deleted the webhook <0>{target}</0>' }),
  'webhookSubscriptions.regenerateSecret': msg({
    message: 'Regenerated the secret of the webhook <0>{target}</0>'
  }),
  'webhookSubscriptions.toggle': msg({ message: 'Turned the webhook <0>{target}</0> on or off' }),
  'webhookSubscriptions.update': msg({ message: 'Updated the webhook <0>{target}</0>' }),
  'webhooks.register': msg({ message: 'Registered the provider webhooks of <0>{target}</0>' }),
  'workspaces.acceptInvitation': msg({ message: 'Accepted the invitation <0>{target}</0>' }),
  'workspaces.connectZapier': msg({ message: 'Connected Zapier with the API key <0>{target}</0>' }),
  'workspaces.create': msg({ message: 'Created the workspace <0>{target}</0>' }),
  'workspaces.createAPIKey': msg({ message: 'Created the API key <0>{target}</0>' }),
  'workspaces.createIntegration': msg({ message: 'Added the integration <0>{target}</0>' }),
  'workspaces.delete': msg({ message: 'Deleted the workspace <0>{target}</0>' }),
  'workspaces.deleteIntegration': msg({ message: 'Removed the integration <0>{target}</0>' }),
  'workspaces.deleteInvitation': msg({ message: 'Cancelled the invitation <0>{target}</0>' }),
  'workspaces.inviteMember': msg({ message: 'Invited <0>{target}</0>' }),
  'workspaces.joinInvitation': msg({ message: 'Joined with the invitation <0>{target}</0>' }),
  'workspaces.removeMember': msg({ message: 'Removed the member <0>{target}</0>' }),
  'workspaces.setAuditLogSettings': msg({
    message: 'Changed the audit log retention of <0>{target}</0>'
  }),
  'workspaces.setBlogSettings': msg({ message: 'Changed the blog settings of <0>{target}</0>' }),
  'workspaces.setCustomFieldLabels': msg({
    message: 'Renamed the custom fields of <0>{target}</0>'
  }),
  'workspaces.setUserPermissions': msg({ message: 'Changed the permissions of <0>{target}</0>' }),
  'workspaces.setWebAnalyticsSettings': msg({
    message: 'Changed the web analytics settings of <0>{target}</0>'
  }),
  'workspaces.update': msg({ message: 'Updated the workspace settings of <0>{target}</0>' }),
  'workspaces.updateIntegration': msg({ message: 'Updated the integration <0>{target}</0>' })
}
