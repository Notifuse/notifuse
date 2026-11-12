import type { ReactNode } from 'react'
import { Button, Dropdown, Table, Tooltip, type MenuProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { useRouter } from '@tanstack/react-router'
import { CalendarClock, Ellipsis, Pause, Send, Trophy } from 'lucide-react'
import { Pill, RateBar, useNumberFormat } from '../layout'
import type { Broadcast } from '../../services/api/broadcast'
import type { List } from '../../services/api/list'
import type { Segment } from '../../services/api/segment'
import type { Workspace } from '../../services/api/types'
import type { UserPermissions } from '../../services/api/workspace'
import { buildBroadcastAnalyticsLinks } from '../web_analytics/lib/broadcastLinks'
import { BroadcastStatusPill } from './BroadcastStatusPill'
import { availableActions, getRemainingTestTime } from './broadcastStatus'
import { SendingProgress } from './SendingProgress'
import { CouldntLoad } from './CouldntLoad'
import { workspaceTimezone } from './access'
import { broadcastCampaign } from './attribution'
import { scheduledInstant, useBroadcastDates } from './useBroadcastDates'
import type { PageStats } from './useBroadcastPageStats'
import type { ConfirmedAction } from './useBroadcastActions'

export const OPEN_RATE_SCALE = 0.7
export const CLICK_RATE_SCALE = 0.25
export const UNSUBSCRIBE_WARNING = 0.005
export const BOUNCE_WARNING = 0.02

export interface BroadcastsTableProps {
  workspace: Workspace
  rows: Broadcast[]
  loading: boolean
  permissions: UserPermissions | null
  canReadHistory: boolean
  canReadWeb: boolean
  pageStats: PageStats
  estimates: Map<string, { count?: number; error: boolean }>
  lists: List[]
  segments: Pick<Segment, 'id' | 'name' | 'color'>[]
  activeId?: string
  /** A click on a row: the detail, or the composer for a draft. */
  onOpen: (broadcast: Broadcast) => void
  onCompose: (broadcast: Broadcast) => void
  onAction: (action: ConfirmedAction, broadcast: Broadcast) => void
  /** "Retry failed…" of a row without a queue: the detail loads it and offers the action. */
  onOpenDetail: (broadcast: Broadcast) => void
  onDelete: (broadcast: Broadcast) => void
  footer?: ReactNode
}

const isStarted = (b: Broadcast) => b.status !== 'draft' && b.status !== 'scheduled'

/** The template shown for a row: the winner's once chosen, else the first variant's. */
export function shownVariation(broadcast: Broadcast) {
  const variations = broadcast.test_settings?.variations ?? []
  return (
    (broadcast.winning_template && variations.find((v) => v.template_id === broadcast.winning_template)) ||
    variations[0]
  )
}

export function BroadcastsTable(props: BroadcastsTableProps) {
  const {
    workspace,
    rows,
    loading,
    permissions,
    canReadHistory,
    canReadWeb,
    pageStats,
    estimates,
    lists,
    segments,
    activeId,
    onOpen,
    onCompose,
    onAction,
    onOpenDetail,
    onDelete,
    footer
  } = props
  const { t } = useLingui()
  const { int, percent } = useNumberFormat()
  const dates = useBroadcastDates()
  const router = useRouter()
  const timezone = workspaceTimezone(workspace)
  const canWrite = Boolean(permissions?.broadcasts?.write)
  const noWrite = t`You don't have write permission for broadcasts`

  const statCell = (b: Broadcast, render: (s: NonNullable<ReturnType<PageStats['stats']['get']>>) => ReactNode) => {
    if (!isStarted(b) || !canReadHistory) return <span className="text-ink-4">—</span>
    if (pageStats.statsError) return <CouldntLoad />
    const s = pageStats.stats.get(b.id)
    if (!s) return <span className="text-ink-4">—</span>
    return render(s)
  }

  const audience = (b: Broadcast) => {
    const listName = lists.find((l) => l.id === b.audience?.list)?.name ?? b.audience?.list
    const chosen = b.audience?.segments ?? []
    const names = chosen.map((id) => segments.find((s) => s.id === id)?.name ?? id)
    return (
      <span className="inline-flex min-w-0 items-center gap-1.5">
        <span className="truncate text-ink">{listName || '—'}</span>
        {chosen.length > 0 ? (
          <Tooltip title={names.join(', ')}>
            <span className="shrink-0 text-xs text-ink-3">+{int(chosen.length)}</span>
          </Tooltip>
        ) : null}
      </span>
    )
  }

  const sending = (b: Broadcast) => {
    if (b.status === 'draft') {
      const ago = dates.relative(b.updated_at)
      return <span className="text-ink-3">{t`Edited ${ago}`}</span>
    }
    if (b.status === 'scheduled') {
      const scheduled = scheduledInstant(b.schedule, timezone)
      if (!scheduled) return <span className="text-ink-4">—</span>
      const iso = scheduled.instant.toISOString()
      return (
        <span className="inline-flex items-center gap-1.5 text-ink">
          <CalendarClock size={13} aria-hidden="true" className="text-ink-3" />
          {dates.dateTime(iso, scheduled.timezone)} {dates.zone(iso, scheduled.timezone)}
        </span>
      )
    }
    if (b.status === 'testing') {
      const duration = getRemainingTestTime(b)
      if (duration) return <span className="text-ink">{t`Test ends in ${duration}`}</span>
    }
    const verdict = pageStats.verdicts.get(b.id)
    const progress = pageStats.progress.get(b.id)
    const enqueued = pageStats.tasks.get(b.id)?.state?.send_broadcast?.enqueued_count
    if (b.status === 'processed' && verdict?.kind === 'sending' && progress && enqueued) {
      return (
        <div className="w-[180px]">
          <SendingProgress
            enqueuedCount={enqueued}
            sentCount={progress.sentCount}
            failedCount={progress.failedCount}
            startedAt={b.started_at}
            remainingOverride={progress.inFlight}
          />
        </div>
      )
    }
    return b.started_at ? <span className="text-ink">{dates.dateTime(b.started_at, timezone)}</span> : <span className="text-ink-4">—</span>
  }

  const recipients = (b: Broadcast) => {
    if (isStarted(b)) return statCell(b, (s) => <span className="tabular-nums">{int(s.sent)}</span>)
    const estimate = estimates.get(b.id)
    if (estimate?.error) return <CouldntLoad />
    return estimate?.count === undefined ? (
      <span className="text-ink-4">—</span>
    ) : (
      <span className="tabular-nums text-ink-2">≈ {int(estimate.count)}</span>
    )
  }

  const rowMenu = (b: Broadcast): MenuProps['items'] => {
    const verdict = pageStats.verdicts.get(b.id)
    const actions = availableActions(b, verdict, pageStats.progress.get(b.id))
    const l2Failed = pageStats.stats.get(b.id)?.failed ?? 0
    const campaign = broadcastCampaign(b)
    const traffic =
      canReadWeb && b.started_at && campaign
        ? buildBroadcastAnalyticsLinks({
            workspaceId: workspace.id,
            campaign,
            startedAt: b.started_at,
            completedAt: b.completed_at,
            timezone
          }).traffic
        : null

    const items: NonNullable<MenuProps['items']> = []
    const guarded = (key: string, label: string, onClick: () => void, danger = false) =>
      items.push({ key, label: canWrite ? label : <Tooltip title={noWrite}>{label}</Tooltip>, disabled: !canWrite, danger, onClick })
    if (actions.edit) guarded('edit', t`Edit`, () => onCompose(b))
    if (actions.pause) guarded('pause', t`Pause`, () => onAction('pause', b))
    if (actions.resume) guarded('resume', t`Resume`, () => onAction('resume', b))
    if (actions.cancel) guarded('cancel', t`Cancel`, () => onAction('cancel', b))
    if (actions.retryFailed) guarded('retry', t`Retry failed`, () => onAction('retryFailed', b))
    else if ((b.status === 'processed' || b.status === 'failed') && !verdict && l2Failed > 0)
      items.push({ key: 'retry-detail', label: t`Retry failed…`, onClick: () => onOpenDetail(b) })
    if (actions.delete) guarded('delete', t`Delete`, () => onDelete(b), true)
    if (traffic) {
      items.push({ type: 'divider' })
      items.push({
        key: 'traffic',
        label: (
          <a href={router.buildLocation(traffic).href} target="_blank" rel="noopener noreferrer">
            {t`Website traffic`}
          </a>
        )
      })
    }
    return items
  }

  const columns: ColumnsType<Broadcast> = [
    {
      key: 'status',
      title: t`Status`,
      width: 120,
      render: (_, b) => (
        <BroadcastStatusPill
          broadcast={b}
          verdict={pageStats.verdicts.get(b.id)}
          l2Failed={pageStats.stats.get(b.id)?.failed}
        />
      )
    },
    {
      key: 'broadcast',
      title: t`Broadcast`,
      render: (_, b) => {
        const variation = shownVariation(b)
        const subject = variation?.template?.email?.subject
        const ab = b.test_settings?.enabled
        const winner = b.winning_template
          ? b.test_settings.variations.find((v) => v.template_id === b.winning_template)
          : undefined
        const count = b.test_settings?.variations?.length ?? 0
        // The A/B pill opens the second line, so the name keeps the column's width.
        const pill = ab ? (
          winner ? (
            <Pill tone="success" icon={<Trophy aria-hidden="true" />}>
              {`A/B · ${winner.template?.name || winner.variation_name}`}
            </Pill>
          ) : (
            <Pill tone="neutral" dot={false}>
              {t`${plural(count, { one: 'A/B · # variant', other: 'A/B · # variants' })}`}
            </Pill>
          )
        ) : null
        const second = subject ? (
          <span className="truncate text-xs text-ink-3">{subject}</span>
        ) : b.status === 'draft' && !variation?.template_id ? (
          <span className="text-xs text-ink-4">{t`No template yet`}</span>
        ) : variation?.template_id && !variation.template ? (
          // Without Templates read the list comes without templates: the ID says which.
          <span className="truncate text-xs text-ink-3">{variation.template_id}</span>
        ) : null
        return (
          <div className="flex min-w-0 flex-col">
            <span className="truncate font-medium text-ink" title={b.name}>
              {b.name}
            </span>
            {pill || second ? (
              <span className="flex min-w-0 items-center gap-1.5">
                {pill ? <span className="shrink-0">{pill}</span> : null}
                {second}
              </span>
            ) : null}
          </div>
        )
      }
    },
    { key: 'audience', title: t`Audience`, width: 150, render: (_, b) => audience(b) },
    { key: 'sending', title: t`Sending`, width: 190, render: (_, b) => sending(b) },
    { key: 'recipients', title: t`Recipients`, width: 100, align: 'right', render: (_, b) => recipients(b) },
    {
      key: 'opens',
      title: t`Opens`,
      width: 110,
      render: (_, b) =>
        statCell(b, (s) => (
          <RateBar
            rate={s.opensTracked > 0 ? s.opened / s.opensTracked : null}
            scale={OPEN_RATE_SCALE}
            tone={s.opensTracked < s.sent ? 'partial' : 'default'}
          />
        ))
    },
    {
      key: 'clicks',
      title: t`Clicks`,
      width: 110,
      render: (_, b) =>
        statCell(b, (s) => (
          <RateBar
            rate={s.clicksTracked > 0 ? s.clicked / s.clicksTracked : null}
            scale={CLICK_RATE_SCALE}
            tone={s.clicksTracked < s.sent ? 'partial' : 'default'}
          />
        ))
    },
    {
      key: 'unsub',
      title: t`Unsub.`,
      width: 80,
      align: 'right',
      render: (_, b) =>
        statCell(b, (s) => {
          if (s.sent === 0) return <span className="text-ink-4">—</span>
          const share = s.unsubscribed / s.sent
          return (
            <span className={`tabular-nums ${share >= UNSUBSCRIBE_WARNING ? 'text-warning-fg' : 'text-ink'}`}>
              {percent(share)}
            </span>
          )
        })
    },
    {
      key: 'bounces',
      title: t`Bounces`,
      width: 80,
      align: 'right',
      render: (_, b) =>
        statCell(b, (s) => {
          if (s.sent === 0) return <span className="text-ink-4">—</span>
          const share = s.bounced / s.sent
          return (
            <span className={`tabular-nums ${share >= BOUNCE_WARNING ? 'text-warning-fg' : 'text-ink'}`}>
              {percent(share)}
            </span>
          )
        })
    }
  ]

  if (canReadWeb) {
    columns.push({
      key: 'visits',
      title: (
        <Tooltip title={t`Sessions from the broadcast's UTM campaign, from the day it was sent to 7 days after it completed.`}>
          <span>{t`Website visits`}</span>
        </Tooltip>
      ),
      width: 110,
      align: 'right',
      render: (_, b) => {
        if (!b.started_at) return <span className="text-ink-4">—</span>
        if (!broadcastCampaign(b)) {
          return (
            <Tooltip title={t`This broadcast has no UTM campaign, so its website traffic can't be identified`}>
              <span className="text-ink-4">—</span>
            </Tooltip>
          )
        }
        if (pageStats.visitsError) return <CouldntLoad />
        const visits = pageStats.visits.get(b.id)
        return visits === null || visits === undefined ? (
          <span className="text-ink-4">—</span>
        ) : (
          <span className="tabular-nums">{int(visits)}</span>
        )
      }
    })
  }

  columns.push({
    key: 'actions',
    width: 76,
    align: 'right',
    render: (_, b) => {
      const verdict = pageStats.verdicts.get(b.id)
      const sendingNow = b.status === 'processing' || (b.status === 'processed' && verdict?.kind === 'sending')
      return (
        // The row opens on click: the actions keep their clicks.
        <div className="flex items-center justify-end gap-1" onClick={(e) => e.stopPropagation()}>
          {b.status === 'draft' ? (
            <Tooltip title={canWrite ? t`Send` : noWrite}>
              <Button
                type="text"
                size="small"
                aria-label={t`Send`}
                disabled={!canWrite}
                icon={<Send size={14} aria-hidden="true" />}
                onClick={() => onCompose(b)}
              />
            </Tooltip>
          ) : sendingNow ? (
            <Tooltip title={canWrite ? t`Pause` : noWrite}>
              <Button
                type="text"
                size="small"
                aria-label={t`Pause`}
                disabled={!canWrite}
                icon={<Pause size={14} aria-hidden="true" />}
                onClick={() => onAction('pause', b)}
              />
            </Tooltip>
          ) : null}
          <Dropdown menu={{ items: rowMenu(b) }} trigger={['click']} placement="bottomRight">
            <Button type="text" size="small" aria-label={t`More actions`} icon={<Ellipsis size={14} aria-hidden="true" />} />
          </Dropdown>
        </div>
      )
    }
  })

  return (
    <Table<Broadcast>
      rowKey="id"
      columns={columns}
      dataSource={rows}
      loading={loading && rows.length === 0 ? false : undefined}
      pagination={false}
      size="middle"
      tableLayout="fixed"
      // The Broadcast column keeps about 200px: below that the table scrolls.
      scroll={{ x: 1150 }}
      rowClassName={(b) => `cursor-pointer ${b.id === activeId ? 'is-hover' : ''}`}
      onRow={(b) => ({ onClick: () => onOpen(b) })}
      footer={footer ? () => footer : undefined}
    />
  )
}
