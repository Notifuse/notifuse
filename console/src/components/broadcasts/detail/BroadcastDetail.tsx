import { useState, type ReactElement } from 'react'
import { Alert, Button, Dropdown, Result, Skeleton, Tooltip, type MenuProps } from 'antd'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { Ellipsis, Pause, Play, RotateCcw, Send } from 'lucide-react'
import { PageHeader, Panel, Pill } from '../../layout'
import { ApiError } from '../../../services/api/client'
import { broadcastApi, testEmailListProps, type Broadcast } from '../../../services/api/broadcast'
import { listsApi } from '../../../services/api/list'
import { listSegments } from '../../../services/api/segment'
import { getBroadcastStats } from '../../../services/api/messages_history'
import { taskApi } from '../../../services/api/task'
import { canSendTestEmail } from '../../../services/api/permissions'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'
import type { UserPermissions } from '../../../services/api/workspace'
import SendTemplateModal from '../../templates/SendTemplateModal'
import { buildBroadcastAnalyticsLinks, broadcastAttributionWindow } from '../../web_analytics/lib/broadcastLinks'
import { BroadcastStatusPill } from '../BroadcastStatusPill'
import { ChangeVariationSenderModal } from '../ChangeVariationSenderModal'
import { DeleteBroadcastModal } from '../DeleteBroadcastModal'
import { SendingProgress } from '../SendingProgress'
import { canReadWebAnalytics, isSmtpMarketingProvider, workspaceTimezone } from '../access'
import { broadcastCampaign } from '../attribution'
import { broadcastVisitsQuery, readBroadcastVisits } from '../broadcastAnalytics'
import { runBroadcastQuery } from '../broadcastQueryClient'
import { availableActions, deliveryVerdict, progressStatsFrom } from '../broadcastStatus'
import { useBroadcastActions } from '../useBroadcastActions'
import { scheduledInstant, useBroadcastDates } from '../useBroadcastDates'
import { AbTestPanel } from './AbTestPanel'
import { asTemplate } from './asTemplate'
import { BroadcastAside } from './BroadcastAside'
import { previewVariationOf } from './previewVariation'
import { BroadcastFunnel, type LogFilter } from './BroadcastFunnel'
import { EngagementChart } from './EngagementChart'
import { LinkStatsPanel } from './LinkStatsPanel'
import { senderKey, useSenderText } from './senderText'
import { useVariationStats } from './useVariationStats'

/** Refetch period of a broadcast that is sending or testing. */
export const DETAIL_LIVE_REFETCH_MS = 30_000
const FIVE_MINUTES = 5 * 60_000
const TEN_MINUTES = 10 * 60_000

const TESTING_STATUSES: Broadcast['status'][] = ['testing', 'test_completed', 'winner_selected']

export interface BroadcastDetailProps {
  workspace: Workspace
  permissions?: UserPermissions | null
  broadcastId: string
  /** Opens the composer on a draft or scheduled broadcast. */
  onEdit: (id: string) => void
  /** Closes the drawer: back from "not found", and after a delete. */
  onClose: () => void
}

const isNotFound = (error: unknown) => error instanceof ApiError && error.status === 404

/** The body of the broadcast detail drawer, loaded by id. */
export function BroadcastDetail({ workspace, permissions, broadcastId, onEdit, onClose }: BroadcastDetailProps) {
  const { t } = useLingui()
  const ws = workspace.id
  const timezone = workspaceTimezone(workspace)
  const canReadHistory = !!permissions?.message_history?.read
  const canReadWeb = canReadWebAnalytics(workspace, permissions)
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const router = useRouter()
  const dates = useBroadcastDates()
  const senderText = useSenderText()
  const actions = useBroadcastActions(ws)
  const [testTemplate, setTestTemplate] = useState<Template | null>(null)
  const [deleting, setDeleting] = useState<Broadcast | null>(null)
  // The refetch rhythm follows the last verdict read, which needs F2 itself.
  const [verdictSending, setVerdictSending] = useState(false)

  // F1
  const broadcastQuery = useQuery({
    queryKey: ['broadcast', ws, broadcastId],
    queryFn: async () => (await broadcastApi.get({ workspace_id: ws, id: broadcastId, with_templates: true })).broadcast,
    retry: (count, error) => !isNotFound(error) && count < 2,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'testing' || status === 'processing' || status === 'winner_selected' || verdictSending
        ? DETAIL_LIVE_REFETCH_MS
        : false
    }
  })
  const broadcast = broadcastQuery.data
  const status = broadcast?.status
  const started = !!broadcast && status !== 'draft' && status !== 'scheduled'
  const live = status === 'testing' || status === 'processing' || status === 'winner_selected' || verdictSending
  const refetchInterval = live ? DETAIL_LIVE_REFETCH_MS : (false as const)

  // F3: the sending task, among the workspace's last 100.
  const taskQuery = useQuery({
    queryKey: ['task', ws, broadcastId],
    queryFn: () => taskApi.findByBroadcastId(ws, broadcastId),
    enabled: !!broadcast && status !== 'draft',
    refetchInterval
  })
  const task = taskQuery.data

  // F2
  const statsQuery = useQuery({
    queryKey: ['broadcast-stats', ws, broadcastId],
    queryFn: () => getBroadcastStats(ws, broadcastId),
    enabled: canReadHistory && started,
    staleTime: live ? 0 : FIVE_MINUTES,
    refetchInterval
  })
  const enqueued = task?.state?.send_broadcast?.enqueued_count
  const progress = statsQuery.data ? progressStatsFrom(statsQuery.data, enqueued) : undefined
  const verdict = progress ? deliveryVerdict(progress) : undefined
  const sendingVerdict = status === 'processed' && verdict?.kind === 'sending'
  if (sendingVerdict !== verdictSending) setVerdictSending(sendingVerdict)

  const abEnabled = !!broadcast?.test_settings?.enabled
  const testResultsQuery = useQuery({
    queryKey: ['testResults', ws, broadcastId],
    queryFn: () => broadcastApi.getTestResults({ workspace_id: ws, id: broadcastId }),
    enabled: abEnabled && !!status && TESTING_STATUSES.includes(status),
    refetchInterval
  })
  const testResults = testResultsQuery.data

  // F4
  const variationStats = useVariationStats(ws, broadcast, { enabled: canReadHistory && started, refetchInterval })

  // F6: the website visits the broadcast brought, over its attribution window.
  const campaign = broadcast ? broadcastCampaign(broadcast) : null
  const attributionWindow = broadcast ? broadcastAttributionWindow(broadcast, timezone) : null
  const visitsQuery = useQuery({
    queryKey: ['broadcast-visits', ws, broadcastId, campaign],
    queryFn: async () =>
      readBroadcastVisits((await runBroadcastQuery(broadcastVisitsQuery({ campaign: campaign!, window: attributionWindow!, timezone }), ws)).data ?? []),
    enabled: canReadWeb && !!campaign && !!attributionWindow,
    staleTime: TEN_MINUTES
  })

  const listsQuery = useQuery({
    queryKey: ['lists', ws],
    queryFn: () => listsApi.list({ workspace_id: ws }),
    enabled: !!permissions?.lists?.read,
    staleTime: FIVE_MINUTES
  })
  const segmentsQuery = useQuery({
    queryKey: ['segments', ws],
    queryFn: () => listSegments({ workspace_id: ws, with_count: true }),
    enabled: !!permissions?.segments?.read,
    staleTime: FIVE_MINUTES
  })
  const lists = listsQuery.data?.lists ?? []
  const segments = segmentsQuery.data?.segments ?? []

  if (broadcastQuery.isLoading) {
    return <Skeleton active paragraph={{ rows: 8 }} />
  }
  if (broadcastQuery.isError || !broadcast) {
    return isNotFound(broadcastQuery.error) ? (
      <Result
        status="404"
        title={t`Broadcast not found`}
        extra={<Button onClick={onClose}>{t`Back to broadcasts`}</Button>}
      />
    ) : (
      <Result
        status="error"
        title={t`Couldn't load this broadcast`}
        extra={<Button onClick={() => broadcastQuery.refetch()}>{t`Retry`}</Button>}
      />
    )
  }

  const canWrite = !!permissions?.broadcasts?.write
  const noWrite = t`You don't have write permission for broadcasts`
  const set = availableActions(broadcast, verdict, progress)
  const variations = broadcast.test_settings?.variations ?? []
  const smtp = isSmtpMarketingProvider(workspace)
  const untracked = broadcast.tracking_mode === 'no_tracking'
  const stats = statsQuery.data?.stats

  const links =
    canReadWeb && broadcast.started_at && campaign
      ? buildBroadcastAnalyticsLinks({
          workspaceId: ws,
          campaign,
          startedAt: broadcast.started_at,
          completedAt: broadcast.completed_at,
          timezone
        })
      : null
  const noCampaign = t`This broadcast has no UTM campaign, so its website traffic can't be identified`

  // Meta line
  const statusMeta = (() => {
    switch (broadcast.status) {
      case 'draft':
        return t`Draft`
      case 'scheduled': {
        const at = scheduledInstant(broadcast.schedule, timezone)
        if (!at) return t`Scheduled`
        const iso = at.instant.toISOString()
        const date = dates.date(iso, at.timezone)
        const time = dates.time(iso, at.timezone)
        const zone = dates.zone(iso, at.timezone)
        return t`Scheduled for ${date} at ${time} ${zone}`
      }
      case 'paused': {
        const reason = broadcast.pause_reason
        return reason ? t`Paused: ${reason}` : t`Paused`
      }
      case 'processed':
        if (verdict?.kind !== 'sending' && (broadcast.completed_at || broadcast.started_at)) {
          const iso = (broadcast.completed_at || broadcast.started_at)!
          const date = dates.date(iso, timezone)
          const time = dates.time(iso, timezone)
          return t`Sent on ${date} at ${time}`
        }
        break
      case 'testing':
      case 'processing':
      case 'winner_selected':
        break
      default:
        return null
    }
    if (!broadcast.started_at) return null
    const since = dates.time(broadcast.started_at, timezone)
    return t`Sending since ${since}`
  })()
  const listId = broadcast.audience?.list
  const listName = listId ? (lists.find((l) => l.id === listId)?.name ?? listId) : null
  const audienceMeta = listName
    ? broadcast.audience.exclude_unsubscribed
      ? t`List ${listName}, unsubscribed excluded`
      : t`List ${listName}, unsubscribed included`
    : null
  const senderMeta = variations.length
    ? new Set(variations.map((v) => senderKey(workspace, v))).size > 1
      ? t`Several senders`
      : senderText(workspace, variations[0])
    : null

  const openLogs = (filter: LogFilter) => {
    const params = new URLSearchParams()
    params.set('broadcast_id', broadcast.id)
    if (filter !== 'sent') params.set(filter, 'true')
    navigate({ to: `/console/workspace/${ws}/logs?${params.toString()}` as string & {} })
  }

  const refreshStats = () => {
    for (const key of ['broadcast', 'broadcast-stats', 'task', 'testResults', 'broadcast-variation-stats', 'broadcast-link-stats']) {
      queryClient.invalidateQueries({ queryKey: [key, ws, broadcast.id] })
    }
  }

  const guard = (node: ReactElement) => (canWrite ? node : <Tooltip title={noWrite}>{node}</Tooltip>)
  const singleTemplate = !abEnabled ? (variations[0] ? asTemplate(variations[0]) : undefined) : undefined
  const testAllowed = canSendTestEmail(permissions)

  const menu: NonNullable<MenuProps['items']> = [
    { key: 'refresh', label: t`Refresh stats`, onClick: refreshStats }
  ]
  if (canReadHistory && started) menu.push({ key: 'logs', label: t`View sending logs`, onClick: () => openLogs('sent') })
  if (links) {
    menu.push({
      key: 'conversions',
      label: (
        <a href={router.buildLocation(links.conversions).href} target="_blank" rel="noopener noreferrer">
          {t`Website conversions`}
        </a>
      )
    })
  }
  if (set.delete) {
    menu.push({ type: 'divider' })
    menu.push({
      key: 'delete',
      danger: true,
      disabled: !canWrite,
      label: canWrite ? t`Delete` : <Tooltip title={noWrite}>{t`Delete`}</Tooltip>,
      onClick: () => setDeleting(broadcast)
    })
  }

  const headerActions = (
    <>
      {canReadWeb && broadcast.started_at ? (
        <Tooltip title={links ? undefined : noCampaign}>
          <span>
            <Button
              disabled={!links}
              {...(links ? { href: router.buildLocation(links.traffic).href, target: '_blank', rel: 'noopener noreferrer' } : {})}
            >
              {t`Website traffic`}
            </Button>
          </span>
        </Tooltip>
      ) : null}
      {singleTemplate ? (
        <Tooltip
          title={testAllowed ? undefined : t`You need Templates read and Transactional write permissions to send test emails`}
        >
          <Button icon={<Send size={14} />} disabled={!testAllowed} onClick={() => setTestTemplate(singleTemplate)}>
            {t`Send test`}
          </Button>
        </Tooltip>
      ) : null}
      {set.edit ? guard(<Button disabled={!canWrite} onClick={() => onEdit(broadcast.id)}>{t`Edit`}</Button>) : null}
      {set.pause
        ? guard(
            <Button icon={<Pause size={14} />} disabled={!canWrite} onClick={() => actions.confirm('pause', broadcast)}>
              {t`Pause`}
            </Button>
          )
        : null}
      {set.resume
        ? guard(
            <Button type="primary" icon={<Play size={14} />} disabled={!canWrite} onClick={() => actions.confirm('resume', broadcast)}>
              {t`Resume`}
            </Button>
          )
        : null}
      <ChangeVariationSenderModal
        workspace={workspace}
        broadcast={broadcast}
        disabled={!canWrite}
        canReadTemplates={!!permissions?.templates?.read}
      />
      {set.retryFailed
        ? guard(
            <Button icon={<RotateCcw size={14} />} disabled={!canWrite} onClick={() => actions.confirm('retryFailed', broadcast)}>
              {t`Retry failed`}
            </Button>
          )
        : null}
      {set.cancel
        ? guard(
            <Button danger disabled={!canWrite} onClick={() => actions.confirm('cancel', broadcast)}>
              {t`Cancel`}
            </Button>
          )
        : null}
      <Dropdown menu={{ items: menu }} trigger={['click']} placement="bottomRight">
        <Button aria-label={t`More actions`} icon={<Ellipsis size={16} />} />
      </Dropdown>
    </>
  )

  const variants = variations
    .map((v, index) => ({
      letter: String.fromCharCode(65 + index),
      templateId: v.template_id,
      clicksTracked: variationStats[v.template_id]?.total_clicks_tracked ?? variationStats[v.template_id]?.total_sent ?? 0
    }))
    .filter((v) => !!v.templateId)

  return (
    <>
      <PageHeader
        title={broadcast.name}
        status={
          <>
            <BroadcastStatusPill broadcast={broadcast} verdict={verdict} l2Failed={stats?.total_failed} />
            {abEnabled ? <Pill tone="primary" dot={false}>{t`A/B`}</Pill> : null}
          </>
        }
        meta={[statusMeta, audienceMeta, senderMeta]}
        actions={headerActions}
      />
      {task?.error_message ? <Alert type="error" showIcon title={task.error_message} /> : null}
      {broadcast.status === 'processed' && enqueued && progress && verdict?.kind === 'sending' ? (
        <SendingProgress
          enqueuedCount={enqueued}
          sentCount={progress.sentCount}
          failedCount={progress.failedCount}
          startedAt={broadcast.started_at}
          remainingOverride={progress.inFlight}
        />
      ) : null}

      <div className="flex flex-col gap-3 min-[1200px]:flex-row min-[1200px]:items-start">
        <div className="flex min-w-0 flex-1 flex-col gap-3">
          {!started ? (
            <Panel>
              <p data-unreset className="m-0 py-6 text-center text-sm text-ink-3">
                {t`Stats appear once the broadcast starts sending`}
              </p>
            </Panel>
          ) : canReadHistory ? (
            stats ? (
              <BroadcastFunnel
                stats={stats}
                smtp={smtp}
                untracked={untracked}
                web={
                  canReadWeb
                    ? {
                        visits: visitsQuery.data,
                        loading: visitsQuery.isLoading,
                        error: visitsQuery.isError,
                        noCampaign: campaign ? undefined : noCampaign
                      }
                    : undefined
                }
                onOpenLogs={openLogs}
              />
            ) : statsQuery.isError ? (
              <Alert type="warning" showIcon title={t`Couldn't load the statistics of this broadcast`} />
            ) : (
              <Skeleton active paragraph={{ rows: 2 }} />
            )
          ) : null}
          {started && canReadHistory && !untracked ? (
            <EngagementChart workspaceId={ws} broadcast={broadcast} timezone={timezone} live={live} />
          ) : null}
          <AbTestPanel
            workspace={workspace}
            broadcast={broadcast}
            testResults={testResults}
            variationStats={variationStats}
            permissions={permissions}
            onPickWinner={(templateId, name) => actions.selectWinner(broadcast, templateId, name)}
            onSendTest={setTestTemplate}
          />
          {started && canReadHistory ? (
            <LinkStatsPanel
              workspaceId={ws}
              broadcastId={broadcast.id}
              clicksTracked={stats?.total_clicks_tracked ?? stats?.total_sent ?? 0}
              variants={abEnabled ? variants : []}
              untracked={untracked}
            />
          ) : null}
        </div>
        <aside className="flex shrink-0 flex-col gap-3 min-[1200px]:w-aside">
          <BroadcastAside
            workspace={workspace}
            broadcast={broadcast}
            lists={lists}
            segments={segments}
            task={task}
            previewVariation={previewVariationOf(broadcast, testResults?.winning_template)}
            canReadTemplates={!!permissions?.templates?.read}
          />
        </aside>
      </div>

      <SendTemplateModal
        isOpen={!!testTemplate}
        onClose={() => setTestTemplate(null)}
        template={testTemplate}
        workspace={workspace}
        canReadLists={!!permissions?.lists?.read}
        canReadContacts={!!permissions?.contacts?.read}
        {...testEmailListProps(broadcast)}
      />
      <DeleteBroadcastModal
        broadcast={deleting}
        onClose={() => setDeleting(null)}
        onDelete={async (b) => {
          // Close first: the detail of a deleted broadcast must not refetch into a 404.
          onClose()
          await actions.remove(b)
        }}
      />
    </>
  )
}
