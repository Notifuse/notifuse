import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import { KpiStrip, useNumberFormat, type KpiItem } from '../../layout'
import { formatDuration } from '../visits/duration'
import { activityLabel } from './activityLabel'
import { useContactMessageStats, useContactTimeline, useContactVisits } from './useContactSheetQueries'

export interface ContactKpisProps {
  workspaceId: string
  email: string
  canReadMessages: boolean
  /** Web analytics is enabled in the workspace and the member has web_analytics.read. */
  canReadVisits: boolean
}

/** Emails received, open and click rates, last activity and site visits of a contact. */
export function ContactKpis({ workspaceId, email, canReadMessages, canReadVisits }: ContactKpisProps) {
  const { t, i18n } = useLingui()
  const { int, percent } = useNumberFormat()
  const stats = useContactMessageStats(workspaceId, email, canReadMessages)
  const timeline = useContactTimeline(workspaceId, email, 'all')
  const visits = useContactVisits(workspaceId, email, 'all', canReadVisits)

  const rate = (count: number, tracked: number) => (tracked > 0 ? percent(count / tracked) : '—')
  const s = stats.data
  const last = timeline.data?.pages[0]?.timeline?.[0]
  const summary = visits.data?.pages[0]?.summary

  const items: KpiItem[] = []
  if (canReadMessages) {
    items.push(
      {
        key: 'received',
        label: t`Emails received`,
        value: s ? int(s.sent) : '—',
        sub: s ? t`Last 30 days: ${int(s.sentLast30Days)}` : undefined
      },
      {
        key: 'open',
        label: t`Open rate`,
        value: s ? rate(s.opened, s.opensTracked) : '—',
        sub: s ? t`${int(s.opened)} opened` : undefined
      },
      {
        key: 'click',
        label: t`Click rate`,
        value: s ? rate(s.clicked, s.clicksTracked) : '—',
        sub: s ? t`${int(s.clicked)} clicked` : undefined
      }
    )
  }
  items.push({
    key: 'last',
    label: t`Last activity`,
    value: last ? (
      <Tooltip title={dayjs(last.created_at).format('LLL')}>
        <span>{dayjs(last.created_at).fromNow()}</span>
      </Tooltip>
    ) : (
      '—'
    ),
    sub: last ? activityLabel(last, i18n) : undefined
  })
  if (canReadVisits) {
    items.push({
      key: 'visits',
      label: t`Site visits`,
      value: summary ? int(summary.sessions) : '—',
      sub:
        summary && summary.sessions > 0
          ? t`Median ${formatDuration(summary.median_duration_ms, i18n)}`
          : undefined
    })
  }

  const loading =
    (canReadMessages && stats.isLoading) || timeline.isLoading || (canReadVisits && visits.isLoading)
  return <KpiStrip items={items} loading={loading} />
}
