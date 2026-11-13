import { getBrowserTimezone } from '../../lib/timezoneNormalizer'
import type { UserPermissions, Workspace } from '../../services/api/workspace'

/** Web analytics is used by the workspace and readable by the member (BroadcastAnalyticsLinks). */
export const canReadWebAnalytics = (workspace?: Workspace | null, permissions?: UserPermissions | null) =>
  Boolean(workspace?.settings?.web_analytics?.enabled && permissions?.web_analytics?.read)

/** The timezone analytics queries and dates of the broadcasts screens use. */
export const workspaceTimezone = (workspace?: Workspace | null) =>
  workspace?.settings?.timezone || getBrowserTimezone() || 'UTC'

/** The marketing provider sends through SMTP, which reports no delivery, bounce nor complaint. */
export function isSmtpMarketingProvider(workspace?: Workspace | null): boolean {
  const id = workspace?.settings?.marketing_email_provider_id
  if (!id) return false
  return workspace?.integrations?.find((i) => i.id === id)?.email_provider?.kind === 'smtp'
}
