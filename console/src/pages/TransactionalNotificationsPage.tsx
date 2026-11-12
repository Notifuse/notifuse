import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { Alert, App, Button, Empty, Result, Segmented, Skeleton, Tooltip } from 'antd'
import { SquareTerminal, Zap } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { KpiStrip, PageHeader, Panel, TableToolbar, useNumberFormat, type KpiItem } from '../components/layout'
import {
  transactionalNotificationsApi,
  type TransactionalNotification
} from '../services/api/transactional_notifications'
import type { GetTemplateResponse, Template } from '../services/api/template'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { canCreateTransactionalNotification } from '../services/api/permissions'
import { copyToClipboard } from '../lib/copyToClipboard'
import UpsertTransactionalNotificationDrawer from '../components/transactional/UpsertTransactionalNotificationDrawer'
import SendTemplateModal from '../components/templates/SendTemplateModal'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import { ApiCommandModal } from '../components/transactional/ApiCommandModal'
import { TransactionalDetail } from '../components/transactional/TransactionalDetail'
import { TransactionalTable } from '../components/transactional/TransactionalTable'
import {
  transactionalPeriod,
  validateTransactionalSearch,
  type TransactionalPeriod
} from '../components/transactional/transactionalSearch'
import { periodDays, resolveTransactionalPeriod, workspaceToday } from '../components/transactional/period'
import { useMinuteTick } from '../components/transactional/transactionalClient'
import { useTemplateStates, useTransactionalStats } from '../components/transactional/useTransactionalStats'
import { defaultSender, transactionalProvider } from '../components/transactional/senderInfo'
import { deliveredChange, statsRates, sumStats } from '../components/transactional/transactionalStats'
import {
  logsSearch,
  templateIdOf,
  useDeleteNotification,
  useNotificationCount
} from '../components/transactional/notificationRules'

const DOCS_URL = 'https://docs.notifuse.com/features/transactional-api'
const SEARCH_DELAY_MS = 250

type Overlay = { kind: 'test' | 'api' | 'edit' | 'preview'; notification: TransactionalNotification } | null

const PERIODS: TransactionalPeriod[] = ['7d', '30d', '60d']

/** Screens 91 and 92: the transactional notifications, and the sheet of one. */
export function TransactionalNotificationsPage() {
  const { t } = useLingui()
  const { int, percent, points } = useNumberFormat()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { workspaceId: rawWorkspaceId } = useParams({ strict: false })
  const workspaceId = rawWorkspaceId as string
  const search = validateTransactionalSearch(useSearch({ strict: false }) as Record<string, unknown>)
  const period = transactionalPeriod(search)
  const { workspaces } = useAuth()
  const workspace = workspaces.find((item) => item.id === workspaceId)
  const { permissions, loading: permissionsLoading } = useWorkspacePermissions(workspaceId)
  const ready = !permissionsLoading
  const canReadMessages = ready && Boolean(permissions?.message_history?.read)
  const timezone = workspace?.settings?.timezone || 'UTC'
  const tick = useMinuteTick()
  // Read again each minute, so relative times stay true on an open page.
  const now = useMemo(() => new Date(Math.max(Date.now(), tick * 60_000)), [tick])
  // Resolved again when the workspace's day changes, so an open page follows midnight.
  const today = workspaceToday(timezone, now)
  const periods = useMemo(() => resolveTransactionalPeriod(period, timezone, today), [period, timezone, today])
  const countLabel = useNotificationCount()

  const setSearch = (change: Record<string, unknown>, replace = false) =>
    void navigate({
      to: '.',
      replace,
      search: (prev: Record<string, unknown>) => ({ ...prev, ...change })
    })

  // The search box mirrors what is typed and writes the trimmed term 250 ms later; the echo of its
  // own write is ignored, so trailing spaces are not eaten while typing.
  const searchQ = search.q ?? ''
  const [typed, setTyped] = useState(searchQ)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const lastWritten = useRef<string | undefined>(undefined)
  const onSearch = (value: string) => {
    setTyped(value)
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      const trimmed = value.trim()
      lastWritten.current = trimmed
      setSearch({ q: trimmed || undefined }, true)
    }, SEARCH_DELAY_MS)
  }
  useEffect(() => {
    if (searchQ !== lastWritten.current) {
      if (timer.current) clearTimeout(timer.current)
      setTyped(searchQ)
      lastWritten.current = searchQ
    }
  }, [searchQ])
  useEffect(() => () => clearTimeout(timer.current), [])

  const list = useQuery({
    queryKey: ['transactional-notifications', workspaceId],
    queryFn: () => transactionalNotificationsApi.list({ workspace_id: workspaceId }),
    enabled: Boolean(workspaceId)
  })
  const notifications = useMemo(() => list.data?.notifications ?? [], [list.data])
  const ids = useMemo(() => notifications.map((item) => item.id).sort(), [notifications])
  const templateIds = useMemo(
    () => [...new Set(notifications.map(templateIdOf).filter((id): id is string => Boolean(id)))].sort(),
    [notifications]
  )

  const provider = workspace ? transactionalProvider(workspace) : null
  const smtp = provider?.email_provider?.kind === 'smtp'
  const stats = useTransactionalStats({
    workspaceId,
    ids,
    period,
    periods,
    timezone,
    enabled: canReadMessages,
    smtp
  })
  const templates = useTemplateStates({
    workspaceId,
    templateIds,
    ready,
    canRead: Boolean(permissions?.templates?.read)
  })

  const term = typed.trim().toLowerCase()
  const shown = term
    ? notifications.filter((item) => item.name.toLowerCase().includes(term) || item.id.toLowerCase().includes(term))
    : notifications

  const [overlay, setOverlay] = useState<Overlay>(null)
  const closeOverlay = () => setOverlay(null)
  const openSheet = (id: string, replace = false) => setSearch({ notification: id }, replace)
  const closeSheet = () => setSearch({ notification: undefined })
  const remove = useDeleteNotification(workspaceId, (id) => {
    if (search.notification === id) closeSheet()
  })

  const actions = {
    onOpen: (notification: TransactionalNotification) => openSheet(notification.id),
    onTest: (notification: TransactionalNotification) => setOverlay({ kind: 'test', notification }),
    onApi: (notification: TransactionalNotification) => setOverlay({ kind: 'api', notification }),
    onEdit: (notification: TransactionalNotification) => setOverlay({ kind: 'edit', notification }),
    onPreview: (notification: TransactionalNotification) => setOverlay({ kind: 'preview', notification }),
    onCopyId: async (notification: TransactionalNotification) => {
      if (await copyToClipboard(notification.id)) message.success(t`ID copied`)
      else message.error(t`Failed to copy to clipboard`)
    },
    onLogs: (notification: TransactionalNotification) =>
      void navigate({
        to: '/console/workspace/$workspaceId/logs',
        params: { workspaceId },
        search: logsSearch(notification.id)
      }),
    onDelete: remove
  }

  if (list.isError) {
    return (
      <div className="flex flex-col gap-3">
        <PageHeader title={t`Transactional`} />
        <Result
          status="error"
          title={t`Couldn't load the notifications`}
          subTitle={(list.error as Error)?.message}
          extra={<Button onClick={() => void list.refetch()}>{t`Retry`}</Button>}
        />
      </div>
    )
  }

  if (!workspace) return <Skeleton active />

  const hasNotifications = notifications.length > 0
  const canCreate = canCreateTransactionalNotification(permissions)
  const createButton = ready ? (
    <Tooltip
      title={
        canCreate ? undefined : t`You need Transactional write and Templates read permissions to create notifications`
      }
    >
      <span>
        <UpsertTransactionalNotificationDrawer
          workspace={workspace}
          canReadTemplates={Boolean(permissions?.templates?.read)}
          buttonContent={t`New notification`}
          buttonProps={{ type: 'primary', disabled: !canCreate }}
        />
      </span>
    </Tooltip>
  ) : (
    <Skeleton.Button active />
  )

  const header = (
    <PageHeader
      title={t`Transactional`}
      count={hasNotifications ? notifications.length : undefined}
      actions={
        <div className="flex items-center gap-2">
          <Button
            href={DOCS_URL}
            target="_blank"
            rel="noopener noreferrer"
            icon={<SquareTerminal size={14} aria-hidden="true" />}
          >
            {t`API documentation`}
          </Button>
          {hasNotifications ? createButton : null}
        </div>
      }
    />
  )

  const providerAlert = provider ? null : (
    <Alert
      type="warning"
      showIcon
      title={t`No transactional email provider. Notifications can't be sent until you choose one.`}
      action={
        <Link to="/console/workspace/$workspaceId/settings/$section" params={{ workspaceId, section: 'integrations' }}>
          {t`Open integrations`}
        </Link>
      }
    />
  )

  // KPIs: the sum of the rows of every notification, the search filters the table only.
  const gate = {
    forbidden: ready && !canReadMessages,
    // No figures yet and no failure: the list or the rights are still on their way.
    loading: !ready || (canReadMessages && !stats.current.data && !stats.current.isError),
    error: stats.current.isError
  }
  // The sheet's chart reloads on a period switch: its figures wait with it rather than show the old period.
  const sheetGate = { ...gate, loading: gate.loading || stats.current.isPlaceholderData }
  const total = sumStats(stats.current.data, ids)
  const previous = stats.previous.data ? sumStats(stats.previous.data, ids) : undefined
  const rates = statsRates(total)
  // While the old period stays on screen, it is not compared with the new previous period.
  const change = stats.current.isPlaceholderData ? undefined : deliveredChange(total, previous)
  // The figures shown are labelled with their own period, the old one while a switch loads.
  const days = periodDays(stats.current.period ?? period)
  const lastDays = t`${plural(days, { one: 'Last # day', other: 'Last # days' })}`
  const dash = '—'
  const rate = (value: number | null) => (value === null ? dash : percent(value))
  const tracked = (value: { notTracked: boolean }) => (value.notTracked ? t`Not tracked` : t`On tracked emails`)
  const kpis: KpiItem[] = [
    { key: 'sent', label: t`Emails sent`, value: int(total.sent), sub: lastDays },
    smtp
      ? {
          key: 'delivered',
          label: t`Delivered`,
          value: dash,
          hint: t`SMTP provider doesn't support delivery webhooks, so delivery status cannot be tracked`
        }
      : {
          key: 'delivered',
          label: t`Delivered`,
          value: rate(rates.delivered),
          sub: change
            ? t`${plural(days, {
                one: `${points(change.points)} vs previous # day`,
                other: `${points(change.points)} vs previous # days`
              })}`
            : undefined,
          tone: change?.tone
        },
    { key: 'opens', label: t`Open rate`, value: rate(rates.opens.rate), sub: tracked(rates.opens) },
    { key: 'clicks', label: t`Click rate`, value: rate(rates.clicks.rate), sub: tracked(rates.clicks) },
    {
      key: 'failed',
      label: t`Failed`,
      value: int(total.failed),
      sub: rates.failedShare === null ? undefined : percent(rates.failedShare, 2),
      hint: t`Sends refused by the provider. Requests rejected before sending are not logged.`
    },
    smtp
      ? {
          key: 'bounced',
          label: t`Bounced`,
          value: dash,
          hint: t`SMTP provider doesn't support bounce webhooks, so bounce status cannot be tracked`
        }
      : {
          key: 'bounced',
          label: t`Bounced`,
          value: rate(rates.bounced),
          sub: t`${plural(total.bounced, { one: `${int(total.bounced)} email`, other: `${int(total.bounced)} emails` })}`,
          tone: rates.bouncedTone
        }
  ]
  const kpiItems = gate.error ? kpis.map((item) => ({ ...item, value: dash, sub: t`Couldn't load` })) : kpis

  const sender = defaultSender(provider)
  const providerLine = provider ? (
    <span className="flex min-w-0 items-center gap-1.5 text-sm text-ink-3">
      <Zap size={14} aria-hidden="true" className="shrink-0" />
      <span className="truncate">
        {sender ? t`Sent via ${provider.name} · ${sender.email}` : t`Sent via ${provider.name}`}
      </span>
    </span>
  ) : null
  const periodSwitch = (
    <Segmented<TransactionalPeriod>
      size="small"
      value={period}
      options={PERIODS.map((value) => ({
        value,
        label: value === '7d' ? t`7d` : value === '30d' ? t`30d` : t`60d`
      }))}
      onChange={(value) => setSearch({ period: value === '30d' ? undefined : value })}
    />
  )

  let body
  if (list.isLoading) {
    body = (
      <>
        <KpiStrip items={kpis} loading />
        <Panel flush>
          <div className="px-3.5 py-3">
            <Skeleton active title={false} paragraph={{ rows: 5 }} />
          </div>
        </Panel>
      </>
    )
  } else if (!hasNotifications) {
    body = (
      <Panel>
        <Empty
          className="py-8"
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={
            <div className="flex flex-col items-center gap-1">
              <span className="text-base font-medium text-ink">{t`No transactional notifications found`}</span>
              <span className="text-sm text-ink-3">{t`Create your first notification to get started`}</span>
            </div>
          }
        >
          {createButton}
        </Empty>
      </Panel>
    )
  } else {
    body = (
      <>
        {gate.forbidden ? null : <KpiStrip items={kpiItems} loading={gate.loading} />}
        <Panel flush>
          <TableToolbar
            start={<TableToolbar.Search value={typed} onChange={onSearch} placeholder={t`Name or ID`} />}
            end={
              <>
                {providerLine}
                {periodSwitch}
              </>
            }
          />
          {shown.length === 0 ? (
            <Empty className="py-6" image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No notifications match`}>
              <Button onClick={() => onSearch('')}>{t`Clear search`}</Button>
            </Empty>
          ) : (
            <TransactionalTable
              {...actions}
              workspace={workspace}
              permissions={permissions}
              notifications={shown}
              stats={stats.current.data}
              statsGate={gate}
              lastSent={stats.lastSent.data}
              lastSentGate={{
                forbidden: gate.forbidden,
                loading: !ready || stats.lastSent.isLoading,
                error: stats.lastSent.isError
              }}
              templates={templates}
              smtp={smtp}
              timezone={timezone}
              now={now}
              footer={<span className="text-sm text-ink-3">{countLabel(shown.length, notifications.length)}</span>}
            />
          )}
        </Panel>
      </>
    )
  }

  const previewTemplate = (notification: TransactionalNotification): Template | undefined => {
    const templateId = templateIdOf(notification)
    if (!templateId) return undefined
    const state = templates.get(templateId)
    if (state?.status === 'ok') return state.template
    // The sheet of a notification outside the list reads its template under the same key.
    return queryClient.getQueryData<GetTemplateResponse>(['template', workspaceId, templateId])?.template
  }
  const previewed = overlay?.kind === 'preview' ? previewTemplate(overlay.notification) : undefined
  const testTemplateId = overlay?.kind === 'test' ? templateIdOf(overlay.notification) : undefined

  return (
    <div className="flex flex-col gap-3">
      {header}
      {providerAlert}
      {body}

      <TransactionalDetail
        {...actions}
        notificationId={search.notification}
        workspace={workspace}
        permissions={permissions}
        permissionsLoading={permissionsLoading}
        placeholder={notifications.find((item) => item.id === search.notification)}
        order={shown.map((item) => item.id)}
        onNavigate={(id) => openSheet(id, true)}
        onClose={closeSheet}
        period={period}
        periods={periods}
        onPeriodChange={(value) => setSearch({ period: value === '30d' ? undefined : value })}
        stats={stats.current.data}
        statsGate={sheetGate}
        smtp={smtp}
        timezone={timezone}
        now={now}
      />

      {/* Mounted only while open: each opening lies after the sheet in the page, so above it. */}
      {overlay?.kind === 'api' ? (
        <ApiCommandModal open onClose={closeOverlay} notification={overlay.notification} workspaceId={workspaceId} />
      ) : null}
      {overlay?.kind === 'edit' ? (
        <UpsertTransactionalNotificationDrawer
          open
          workspace={workspace}
          notification={overlay.notification}
          canReadTemplates={Boolean(permissions?.templates?.read)}
          onClose={closeOverlay}
        />
      ) : null}
      {overlay?.kind === 'preview' && previewed ? (
        <TemplatePreviewDrawer open record={previewed} workspace={workspace} onClose={closeOverlay} />
      ) : null}
      {overlay?.kind === 'test' && testTemplateId ? (
        <SendTemplateModal
          isOpen
          onClose={closeOverlay}
          template={{
            id: testTemplateId,
            name: overlay.notification.name,
            version: 0,
            category: 'transactional',
            channel: 'email',
            created_at: '',
            updated_at: ''
          }}
          workspace={workspace}
          withCCAndBCC
          canReadContacts={Boolean(permissions?.contacts?.read)}
        />
      ) : null}
    </div>
  )
}
