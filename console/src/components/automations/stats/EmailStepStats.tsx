import { forwardRef, type ReactNode } from 'react'
import { Button, Tooltip, type ButtonProps } from 'antd'
import { createLink } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { FillBar, useNumberFormat } from '../../layout'
import type { NodeEmailStats } from '../../../services/api/automation'

interface EmailStepStatsProps {
  email: NodeEmailStats | undefined
  canReadHistory: boolean
  /** SMTP reports no delivery: that row reads "—". */
  smtp: boolean
  /** The step's messages in Logs: a router link, so the console does not load again. */
  messages: { workspaceId: string; automationId: string; templateId?: string }
}

// An antd button that navigates through the router (antd renders an <a> for an href).
const ButtonLink = createLink(
  forwardRef<HTMLAnchorElement, ButtonProps>((props, ref) => <Button ref={ref as never} {...props} />)
)

const ratio = (part: number, whole: number) => (whole > 0 ? part / whole : null)

/** An email step's results: sent, delivered, opened, clicked, then what went wrong. */
export function EmailStepStats({ email, canReadHistory, smtp, messages }: EmailStepStatsProps) {
  const { t } = useLingui()
  const { int, percent } = useNumberFormat()

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

  // A result per row, as the mockup draws it: its name, a bar across the room left, its value.
  // `fill` is the bar's ratio, null for none.
  const row = (key: string, label: string, value: ReactNode, fill: number | null) => (
    <div key={key} data-email-row={key} className="flex items-center gap-3 text-sm">
      <span className="w-20 shrink-0 truncate text-ink-2">{label}</span>
      {/* No ratio: an empty track. */}
      <FillBar ratio={fill ?? 0} className="min-w-0 flex-1" />
      {value}
    </div>
  )
  const valueClass = 'w-14 shrink-0 text-right font-semibold text-ink tabular-nums'
  const rate = (key: string, label: string, r: number | null) =>
    row(key, label, <span className={valueClass}>{r === null || Number.isNaN(r) ? '—' : percent(r)}</span>, r)

  const problems = [
    // Emails, beside the step's failed journeys: each count says what it counts.
    plural(e.failed, { one: '# email failed', other: '# emails failed' }),
    plural(e.unsubscribed, { one: '# unsubscribe', other: '# unsubscribes' }),
    plural(e.bounced, { one: '# bounce', other: '# bounces' })
  ].join(' · ')

  return (
    <div className="flex flex-col gap-2" data-email-stats>
      {row('sent', t`Sent`, <span className={valueClass}>{int(e.sent)}</span>, e.sent > 0 ? 1 : 0)}
      {smtp
        ? row(
            'delivered',
            t`Delivered`,
            <Tooltip title={t`SMTP does not report deliveries`}>
              <span className={valueClass}>—</span>
            </Tooltip>,
            null
          )
        : rate('delivered', t`Delivered`, ratio(e.delivered, e.sent))}
      {rate('opened', t`Opened`, ratio(e.opened, e.opens_tracked))}
      {rate('clicked', t`Clicked`, ratio(e.clicked, e.clicks_tracked))}
      <div className="text-xs text-ink-3 tabular-nums">{problems}</div>
      {e.queued > 0 ? <div className="text-xs text-ink-3">{plural(e.queued, { one: '# queued', other: '# queued' })}</div> : null}
      {e.not_sent > 0 ? (
        <div className="text-xs text-ink-3">{plural(e.not_sent, { one: '# not sent', other: '# not sent' })}</div>
      ) : null}
      <div>
        <ButtonLink
          size="small"
          to="/console/workspace/$workspaceId/logs"
          params={{ workspaceId: messages.workspaceId }}
          search={{
            automation_id: messages.automationId,
            ...(messages.templateId ? { template_id: messages.templateId } : {})
          }}
        >
          {t`View messages`}
        </ButtonLink>
      </div>
    </div>
  )
}
