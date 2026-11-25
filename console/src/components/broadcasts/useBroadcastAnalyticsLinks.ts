import { useLingui } from '@lingui/react/macro'
import { canReadWebAnalytics, workspaceTimezone } from './access'
import type { Broadcast } from '../../services/api/broadcast'
import type { UserPermissions, Workspace } from '../../services/api/workspace'
import { buildBroadcastAnalyticsLinks, type BroadcastAnalyticsTarget } from '../web_analytics/lib/broadcastLinks'

/** The broadcast, or the variation of one, whose web analytics reports are linked. */
export interface BroadcastAnalyticsScope {
  workspaceId: string
  workspace?: Workspace
  broadcast: Broadcast
  permissions?: UserPermissions | null
  /**
   * Narrows both reports to one A/B variation, by the utm_content a send stamps
   * with the variation's template id. Omit for the whole broadcast.
   */
  variationTemplateId?: string
}

/** One web analytics report of a broadcast: its link, or why it has none. */
export interface BroadcastAnalyticsLink {
  label: string
  target?: BroadcastAnalyticsTarget
  disabledReason: string
}

/**
 * The traffic and conversions reports of a broadcast, or of one of its variations, scoped to
 * its UTM campaign and send window. Null when the workspace does not use web analytics, when the
 * member cannot read it, or before the broadcast has sent anything. A report without a target
 * cannot be expressed as a filter, because filtering on an empty value matches every untagged
 * session rather than none: shown enabled, it would open a plausible and entirely wrong report.
 */
export function useBroadcastAnalyticsLinks({
  workspaceId,
  workspace,
  broadcast,
  permissions,
  variationTemplateId
}: BroadcastAnalyticsScope): {
  traffic: BroadcastAnalyticsLink
  conversions: BroadcastAnalyticsLink
} | null {
  const { t } = useLingui()

  if (!canReadWebAnalytics(workspace, permissions)) return null
  if (!broadcast.started_at) return null

  const isVariation = variationTemplateId !== undefined
  const content = variationTemplateId?.trim()
  // A variation with no template carries no utm_content of its own, so there is
  // nothing to link to.
  if (isVariation && !content) return null

  const campaign = broadcast.utm_parameters?.campaign?.trim()
  // A send only stamps the variation's template id when the broadcast leaves
  // utm_content empty. With a fixed value every variation ships the same one,
  // so a per-variation report would silently be the whole broadcast's.
  const fixedContent = broadcast.utm_parameters?.content?.trim()
  const variationNotSeparable = isVariation && Boolean(fixedContent)

  const links =
    campaign && !variationNotSeparable
      ? buildBroadcastAnalyticsLinks({
          workspaceId,
          campaign,
          content,
          startedAt: broadcast.started_at,
          completedAt: broadcast.completed_at,
          timezone: workspaceTimezone(workspace)
        })
      : null

  const disabledReason = !campaign
    ? t`This broadcast has no UTM campaign, so its website traffic can't be identified`
    : t`This broadcast sets a fixed UTM content, so its variations can't be told apart in web analytics`

  return {
    traffic: {
      label: isVariation ? t`Website traffic from this variation` : t`Website traffic from this broadcast`,
      target: links?.traffic,
      disabledReason
    },
    conversions: {
      label: isVariation ? t`Website conversions from this variation` : t`Website conversions from this broadcast`,
      target: links?.conversions,
      disabledReason
    }
  }
}
