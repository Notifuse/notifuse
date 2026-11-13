import type { ReactNode } from 'react'
import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { KpiStrip, useNumberFormat, type KpiItem } from '../../layout'
import type { MessageHistoryStatusSum } from '../../../services/api/messages_history'
import { isPartiallyTracked, trackedBase } from '../../../lib/emailTracking'
import { TrackedTooltipTitle } from '../../common/TrackedTooltipTitle'
import { formatDuration, formatValue } from '../../web_analytics/lib/format'
import type { BroadcastVisits } from '../broadcastAnalytics'
import { CouldntLoad } from '../CouldntLoad'

/** The log filter of a funnel cell: none for Sent. */
export type LogFilter = 'sent' | 'is_delivered' | 'is_opened' | 'is_clicked' | 'is_bounced' | 'is_complained' | 'is_unsubscribed' | 'is_failed'

export interface BroadcastFunnelProps {
  stats: MessageHistoryStatusSum
  smtp: boolean
  /** Not tracked: opens and clicks read "—". */
  untracked: boolean
  /** Shown with web analytics only. */
  web?: {
    visits?: BroadcastVisits
    loading: boolean
    error: boolean
    /** The reason of BroadcastAnalyticsLinks when the broadcast has no campaign. */
    noCampaign?: string
  }
  onOpenLogs: (filter: LogFilter) => void
}

/** F2 as a funnel, Sent to Conversions, with the incidents aside. */
export function BroadcastFunnel({ stats, smtp, untracked, web, onOpenLogs }: BroadcastFunnelProps) {
  const { t } = useLingui()
  const { int, percent } = useNumberFormat()
  const sent = stats.total_sent
  const share = (n: number, of: number) => (of > 0 ? percent(n / of) : '—')

  const opensBase = trackedBase(sent, stats.total_opens_tracked)
  const clicksBase = trackedBase(sent, stats.total_clicks_tracked)
  const smtpNote = t`SMTP provider doesn't support delivery webhooks, so delivery status cannot be tracked`
  const notTracked = t`Not tracked`

  const items: KpiItem[] = [
    { key: 'sent', label: t`Sent`, value: int(sent), sub: share(sent, sent), onClick: () => onOpenLogs('sent') },
    smtp
      ? { key: 'delivered', label: t`Delivered`, value: <Tooltip title={smtpNote}><span>—</span></Tooltip> }
      : {
          key: 'delivered',
          label: t`Delivered`,
          value: int(stats.total_delivered),
          sub: share(stats.total_delivered, sent),
          onClick: () => onOpenLogs('is_delivered')
        },
    untracked
      ? { key: 'opened', label: t`Opened`, value: '—', sub: notTracked }
      : {
          key: 'opened',
          label: t`Opened`,
          value: int(stats.total_opened),
          sub: share(stats.total_opened, opensBase),
          hint: isPartiallyTracked(sent, stats.total_opens_tracked) ? (
            <TrackedTooltipTitle title={t`Open rate`} tracked={stats.total_opens_tracked} />
          ) : undefined,
          onClick: () => onOpenLogs('is_opened')
        },
    untracked
      ? { key: 'clicked', label: t`Clicked`, value: '—', sub: notTracked }
      : {
          key: 'clicked',
          label: t`Clicked`,
          value: int(stats.total_clicked),
          sub: share(stats.total_clicked, clicksBase),
          hint: isPartiallyTracked(sent, stats.total_clicks_tracked) ? (
            <TrackedTooltipTitle title={t`Click rate`} tracked={stats.total_clicks_tracked} />
          ) : undefined,
          onClick: () => onOpenLogs('is_clicked')
        }
  ]

  if (web) {
    const unavailable: ReactNode = web.noCampaign ? (
      <Tooltip title={web.noCampaign}>
        <span>—</span>
      </Tooltip>
    ) : web.error ? (
      <CouldntLoad />
    ) : null
    const visits = web.visits
    const median = visits && visits.medianDuration > 0 ? formatDuration(visits.medianDuration) : '—'
    items.push(
      {
        key: 'visits',
        label: t`Website visits`,
        value: unavailable ?? int(visits?.sessions ?? 0),
        sub: unavailable ? undefined : t`${median} median time on site`
      },
      {
        key: 'conversions',
        label: t`Conversions`,
        value: unavailable ?? int(visits?.conversions ?? 0),
        sub: unavailable ? undefined : formatValue(visits?.value ?? 0, 'currency')
      }
    )
  }

  const incidents: { key: LogFilter; label: string; count: number; of: number; tone: 'warning' | 'danger'; smtpNote?: string }[] = [
    {
      key: 'is_bounced',
      label: t`Bounces`,
      count: stats.total_bounced,
      of: sent,
      tone: 'warning',
      smtpNote: smtp ? t`SMTP provider doesn't support bounce webhooks, so bounce status cannot be tracked` : undefined
    },
    {
      key: 'is_complained',
      label: t`Complaints`,
      count: stats.total_complained,
      of: sent,
      tone: 'danger',
      smtpNote: smtp ? t`SMTP provider doesn't support complaint webhooks, so spam complaints cannot be tracked` : undefined
    },
    { key: 'is_unsubscribed', label: t`Unsubscribed`, count: stats.total_unsubscribed, of: sent, tone: 'warning' },
    // A failed email was never sent: its share is over everything attempted.
    { key: 'is_failed', label: t`Failed`, count: stats.total_failed, of: sent + stats.total_failed, tone: 'danger' }
  ]

  const aside = (
    <ul data-unreset className="m-0 flex list-none flex-col gap-1 p-0 text-sm">
      {incidents.map((incident) => {
        const tone = incident.count > 0 ? (incident.tone === 'danger' ? 'text-danger-fg' : 'text-warning-fg') : 'text-ink'
        if (incident.smtpNote) {
          return (
            <li key={incident.key} className="flex items-center justify-between gap-6">
              <span className="text-ink-3">{incident.label}</span>
              <Tooltip title={incident.smtpNote}>
                <span className="text-ink-3">—</span>
              </Tooltip>
            </li>
          )
        }
        return (
          <li key={incident.key}>
            <button
              type="button"
              onClick={() => onOpenLogs(incident.key)}
              className="flex w-full cursor-pointer items-center justify-between gap-6 rounded-sm border-0 bg-transparent p-0 text-left hover:bg-fill"
            >
              <span className="text-ink-3">{incident.label}</span>
              <span className={`tabular-nums ${tone}`}>
                {int(incident.count)} <span className="text-ink-3">· {share(incident.count, incident.of)}</span>
              </span>
            </button>
          </li>
        )
      })}
    </ul>
  )

  return <KpiStrip items={items} connector="arrow" aside={aside} />
}
