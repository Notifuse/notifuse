import { Button, Divider, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faBullseye, faChartLine } from '@fortawesome/free-solid-svg-icons'
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import { useRouter } from '@tanstack/react-router'
import {
  useBroadcastAnalyticsLinks,
  type BroadcastAnalyticsLink,
  type BroadcastAnalyticsScope
} from './useBroadcastAnalyticsLinks'

interface BroadcastAnalyticsLinksProps extends BroadcastAnalyticsScope {
  /** Separates the reports from the action buttons that follow them. */
  withDivider?: boolean
  /** Shows the traffic report alone, where the conversions report sits in a menu. */
  only?: 'traffic'
}

/**
 * Opens the web analytics reports for one broadcast, or for one of its variations: hidden or
 * disabled as useBroadcastAnalyticsLinks says.
 */
export function BroadcastAnalyticsLinks({ withDivider = false, only, ...props }: BroadcastAnalyticsLinksProps) {
  const router = useRouter()
  const reports = useBroadcastAnalyticsLinks(props)
  if (!reports) return null

  const renderLink = ({ target, label, disabledReason }: BroadcastAnalyticsLink, icon: IconDefinition) => (
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
      {renderLink(reports.traffic, faChartLine)}
      {only === 'traffic' ? null : renderLink(reports.conversions, faBullseye)}
      {/* Rendered here rather than by the caller, so hiding the links does not
          leave an orphan divider behind. */}
      {withDivider && <Divider orientation="vertical" className="!mx-1" />}
    </>
  )
}
