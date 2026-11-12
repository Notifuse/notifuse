import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Pill, useNumberFormat } from '../layout'
import type { Broadcast } from '../../services/api/broadcast'
import { statusPill, type StatusPillContext } from './broadcastStatus'

export interface BroadcastStatusPillProps extends StatusPillContext {
  broadcast: Broadcast
}

/** The status of a broadcast, as a pill with its explanatory tooltip (list and detail). */
export function BroadcastStatusPill({ broadcast, verdict, l2Failed }: BroadcastStatusPillProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const spec = statusPill(broadcast, { verdict, l2Failed })
  const count = int(spec.count ?? 0)

  let label: string
  let tooltip: string
  switch (spec.kind) {
    case 'draft':
      label = t`Draft`
      tooltip = t`This broadcast is a draft and has not been scheduled or sent yet.`
      break
    case 'scheduled':
      label = t`Scheduled`
      tooltip = t`This broadcast is scheduled and will start sending at the specified time.`
      break
    case 'testing':
      label = t`Testing`
      tooltip = t`A/B test is in progress. Emails are being sent to the test group.`
      break
    case 'pickWinner':
      label = t`Pick a winner`
      tooltip = t`A/B test has completed. Select a winner to send to the remaining recipients.`
      break
    case 'preparing':
      label = t`Sending`
      tooltip = t`Preparing emails for delivery. Contacts are being added to the sending queue.`
      break
    case 'winnerSending':
      label = t`Sending`
      tooltip = t`A winner has been selected. Emails are being sent to the remaining recipients.`
      break
    case 'paused':
      label = t`Paused`
      tooltip = broadcast.pause_reason || t`Sending has been paused. You can resume at any time.`
      break
    case 'sending':
      label = t`Sending`
      tooltip = t`Emails are being delivered. ${count} still to go.`
      break
    case 'sent':
      label = t`Sent`
      tooltip = t`All emails have been sent.`
      break
    case 'someFailed':
      label = t`${plural(spec.count ?? 0, { one: '# failed', other: '# failed' })}`
      tooltip = t`${plural(spec.count ?? 0, {
        one: 'The provider refused # recipient and every retry was used up. Open the logs to see why, then retry it.',
        other: 'The provider refused # recipients and every retry was used up. Open the logs to see why, then retry them.'
      })}`
      break
    case 'cancelled':
      label = t`Cancelled`
      tooltip = t`This broadcast was cancelled before completion.`
      break
    case 'failed':
      label = t`Failed`
      tooltip = t`This broadcast failed due to an error. Check the logs for details.`
      break
    default:
      label = broadcast.status
      tooltip = broadcast.status
  }

  return (
    <Tooltip title={tooltip}>
      <span className="inline-flex">
        <Pill tone={spec.tone} pulse={spec.pulse}>
          {label}
        </Pill>
      </span>
    </Tooltip>
  )
}
