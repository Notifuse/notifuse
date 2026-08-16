import { Button, Divider, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faBullseye, faChartLine } from '@fortawesome/free-solid-svg-icons'
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import { useLingui } from '@lingui/react/macro'
import { useRouter } from '@tanstack/react-router'
import { getBrowserTimezone } from '../../lib/timezoneNormalizer'
import { Broadcast } from '../../services/api/broadcast'
import { UserPermissions, Workspace } from '../../services/api/workspace'
import {
  BroadcastAnalyticsTarget,
  buildBroadcastAnalyticsLinks
} from '../web_analytics/lib/broadcastLinks'

interface BroadcastAnalyticsLinksProps {
  workspaceId: string
  workspace?: Workspace
  broadcast: Broadcast
  permissions?: UserPermissions | null
  /**
   * Narrows both reports to one A/B variation, by the utm_content a send stamps
   * with the variation's template id. Omit for the whole broadcast.
   */
  variationTemplateId?: string
  /** Separates the reports from the action buttons that follow them. */
  withDivider?: boolean
}

/**
 * Opens the web analytics reports for one broadcast — or for one of its
 * variations — scoped to its UTM campaign and send window.
 *
 * Hidden when the workspace does not use web analytics, when the member cannot
 * read it, or before the broadcast has sent anything. Shown but disabled when
 * the scope cannot be expressed as a filter, because filtering on an empty
 * value matches every untagged session rather than none: an enabled link would
 * open a plausible and entirely wrong report.
 */
export function BroadcastAnalyticsLinks({
  workspaceId,
  workspace,
  broadcast,
  permissions,
  variationTemplateId,
  withDivider = false
}: BroadcastAnalyticsLinksProps) {
  const { t } = useLingui()
  const router = useRouter()

  if (!workspace?.settings?.web_analytics?.enabled) return null
  if (!permissions?.web_analytics?.read) return null
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
          timezone: workspace.settings?.timezone || getBrowserTimezone() || 'UTC'
        })
      : null

  const disabledReason = !campaign
    ? t`This broadcast has no UTM campaign, so its website traffic can't be identified`
    : t`This broadcast sets a fixed UTM content, so its variations can't be told apart in web analytics`

  const trafficLabel = isVariation
    ? t`Website traffic from this variation`
    : t`Website traffic from this broadcast`
  const conversionsLabel = isVariation
    ? t`Website conversions from this variation`
    : t`Website conversions from this broadcast`

  const renderLink = (
    target: BroadcastAnalyticsTarget | undefined,
    icon: IconDefinition,
    label: string
  ) => (
    <Tooltip title={target ? label : disabledReason}>
      {/* A disabled button fires no mouse events, so the tooltip needs a
          wrapper that does — otherwise the explanation never appears, which is
          the whole point of showing the link disabled rather than hiding it. */}
      <span>
        <Button
          type="text"
          size="small"
          aria-label={label}
          disabled={!target}
          className="opacity-70 hover:opacity-100"
          {...(target
            ? {
                // Let the router build the href: it double-encodes the JSON
                // filters param, which is what the console reads back.
                href: router.buildLocation(target).href,
                target: '_blank',
                rel: 'noopener noreferrer'
              }
            : {})}
        >
          <FontAwesomeIcon icon={icon} />
        </Button>
      </span>
    </Tooltip>
  )

  return (
    <>
      {renderLink(links?.traffic, faChartLine, trafficLabel)}
      {renderLink(links?.conversions, faBullseye, conversionsLabel)}
      {/* Rendered here rather than by the caller, so hiding the links does not
          leave an orphan divider behind. */}
      {withDivider && <Divider orientation="vertical" className="!mx-1" />}
    </>
  )
}
