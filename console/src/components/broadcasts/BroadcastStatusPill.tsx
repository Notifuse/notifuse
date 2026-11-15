import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Pill, useNumberFormat } from '../layout'
import type { Broadcast } from '../../services/api/broadcast'
import { statusPill, type StatusPillContext } from './broadcastStatus'

export interface BroadcastStatusPillProps extends StatusPillContext {
  broadcast: Broadcast
  /** In a table column: a label too long ends with an ellipsis, and the tooltip starts with it. */
  truncate?: boolean
}

// The labels are in the 'newsletter status' context: French agrees them with « newsletter »
// (Envoyée, Programmée), where the same English words elsewhere qualify an email.

/** The status of a broadcast, as a pill with its explanatory tooltip (list and detail). */
export function BroadcastStatusPill({ broadcast, verdict, l2Failed, truncate }: BroadcastStatusPillProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const spec = statusPill(broadcast, { verdict, l2Failed })
  // The count of the state: still to go while sending, refused once done. `failed` names the
  // placeholder of the refused-count plurals.
  const failed = spec.count ?? 0
  const count = int(failed)

  let label: string
  let tooltip: string
  switch (spec.kind) {
    case 'draft':
      label = t({ message: 'Draft', context: 'newsletter status' })
      tooltip = t`This broadcast is a draft and has not been scheduled or sent yet.`
      break
    case 'scheduled':
      label = t({ message: 'Scheduled', context: 'newsletter status' })
      tooltip = t`This broadcast is scheduled and will start sending at the specified time.`
      break
    case 'testing':
      label = t({ message: 'Testing', context: 'newsletter status' })
      tooltip = t`A/B test is in progress. Emails are being sent to the test group.`
      break
    case 'pickWinner':
      label = t({ message: 'Pick a winner', context: 'newsletter status' })
      tooltip = t({
        message: 'A/B test has completed. Select a winner to send to the remaining recipients.',
        context: 'newsletter status'
      })
      break
    case 'preparing':
      label = t({ message: 'Sending', context: 'newsletter status' })
      tooltip = t`Preparing emails for delivery. Contacts are being added to the sending queue.`
      break
    case 'winnerSending':
      label = t({ message: 'Sending', context: 'newsletter status' })
      tooltip = t({
        message: 'A winner has been selected. Emails are being sent to the remaining recipients.',
        context: 'newsletter status'
      })
      break
    case 'paused':
      label = t({ message: 'Paused', context: 'newsletter status' })
      tooltip = broadcast.pause_reason || t`Sending has been paused. You can resume at any time.`
      break
    case 'sending':
      label = t({ message: 'Sending', context: 'newsletter status' })
      tooltip = t`Emails are being delivered. ${count} still to go.`
      break
    case 'sent':
      label = t({ message: 'Sent', context: 'newsletter status' })
      tooltip = t`All emails have been sent.`
      break
    case 'someFailed':
      label = t({ message: plural(failed, { one: '# failed', other: '# failed' }), context: 'newsletter status' })
      tooltip = t`${plural(failed, {
        one: 'The provider refused # recipient and every retry was used up. Open the logs to see why, then retry it.',
        other: 'The provider refused # recipients and every retry was used up. Open the logs to see why, then retry them.'
      })}`
      break
    case 'cancelled':
      label = t({ message: 'Cancelled', context: 'newsletter status' })
      tooltip = t`This broadcast was cancelled before completion.`
      break
    case 'failed':
      label = t({ message: 'Failed', context: 'newsletter status' })
      tooltip = t`This broadcast failed due to an error. Check the logs for details.`
      break
    default:
      label = broadcast.status
      tooltip = broadcast.status
  }

  return (
    <Tooltip
      title={
        truncate ? (
          <>
            <div className="font-medium">{label}</div>
            {tooltip}
          </>
        ) : (
          tooltip
        )
      }
    >
      <span className={truncate ? 'inline-flex max-w-full min-w-0' : 'inline-flex'}>
        <Pill tone={spec.tone} pulse={spec.pulse} truncate={truncate}>
          {label}
        </Pill>
      </span>
    </Tooltip>
  )
}
