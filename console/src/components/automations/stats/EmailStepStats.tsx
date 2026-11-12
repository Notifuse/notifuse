import type { ReactNode } from 'react'
import { Button, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { RateBar, useNumberFormat } from '../../layout'
import type { NodeEmailStats } from '../../../services/api/automation'

interface EmailStepStatsProps {
  email: NodeEmailStats | undefined
  canReadHistory: boolean
  /** SMTP reports no delivery: that row reads "—". */
  smtp: boolean
  /** The step's messages in Logs. */
  messagesHref: string
}

const ratio = (part: number, whole: number) => (whole > 0 ? part / whole : null)

/** An email step's results: sent, delivered, opened, clicked, then what went wrong. */
export function EmailStepStats({ email, canReadHistory, smtp, messagesHref }: EmailStepStatsProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()

  if (!canReadHistory) {
    return <p data-unreset className="m-0 text-sm text-ink-3">{t`You need access to the message history to see this step's emails.`}</p>
  }
  const e: NodeEmailStats = email ?? {
    executions: 0,
    queued: 0,
    not_sent: 0,
    sent: 0,
    delivered: 0,
    opened: 0,
    opens_tracked: 0,
    clicked: 0,
    clicks_tracked: 0,
    failed: 0,
    bounced: 0,
    complained: 0,
    unsubscribed: 0
  }

  const row = (key: string, label: string, value: ReactNode) => (
    <div key={key} data-email-row={key} className="flex items-center justify-between gap-3 text-sm">
      <span className="text-ink-2">{label}</span>
      {value}
    </div>
  )

  const problems = [
    plural(e.failed, { one: '# failed', other: '# failed' }),
    plural(e.unsubscribed, { one: '# unsubscribe', other: '# unsubscribes' }),
    plural(e.bounced, { one: '# bounce', other: '# bounces' })
  ].join(' · ')

  return (
    <div className="flex flex-col gap-2" data-email-stats>
      {row(
        'sent',
        t`Sent`,
        <span className="flex items-center gap-3">
          <span className="text-ink-3 tabular-nums">{int(e.sent)}</span>
          <RateBar rate={e.sent > 0 ? 1 : null} />
        </span>
      )}
      {row(
        'delivered',
        t`Delivered`,
        smtp ? (
          <Tooltip title={t`SMTP does not report deliveries`}>
            <span className="text-sm text-ink-3">—</span>
          </Tooltip>
        ) : (
          <RateBar rate={ratio(e.delivered, e.sent)} />
        )
      )}
      {row('opened', t`Opened`, <RateBar rate={ratio(e.opened, e.opens_tracked)} />)}
      {row('clicked', t`Clicked`, <RateBar rate={ratio(e.clicked, e.clicks_tracked)} />)}
      <div className="text-xs text-ink-3 tabular-nums">{problems}</div>
      {e.queued > 0 ? <div className="text-xs text-ink-3">{plural(e.queued, { one: '# queued', other: '# queued' })}</div> : null}
      {e.not_sent > 0 ? (
        <div className="text-xs text-ink-3">{plural(e.not_sent, { one: '# not sent', other: '# not sent' })}</div>
      ) : null}
      <div>
        <Button size="small" href={messagesHref}>
          {t`View messages`}
        </Button>
      </div>
    </div>
  )
}
