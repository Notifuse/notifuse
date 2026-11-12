import { useMemo, type ReactNode } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Button, Dropdown, Result, Skeleton, Tooltip, type MenuProps } from 'antd'
import { MoreHorizontal, Send } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { FullDrawer, KeyValueList, KpiStrip, Mono, PageHeader, Panel, useNumberFormat, type KpiItem } from '../layout'
import { ApiError } from '../../services/api/client'
import {
  transactionalNotificationsApi,
  type TransactionalNotification
} from '../../services/api/transactional_notifications'
import type { UserPermissions, Workspace } from '../../services/api/types'
import { getEmailTrackingPolicy } from '../../lib/emailTracking'
import { LOGS_TAB } from '../logs/logsUrl'
import { ApiCallPanel } from './ApiCallPanel'
import { LoadError } from './LoadError'
import { PreviewPanel } from './PreviewPanel'
import { RecentSends } from './RecentSends'
import { SendsPerDayChart } from './SendsPerDayChart'
import { resolveSenderInfo } from './senderInfo'
import type { TransactionalPeriod } from './transactionalSearch'
import type { TransactionalPeriods } from './period'
import { transactionalAnalytics } from './transactionalClient'
import { formatFullDateTime, formatSeriesDay, todayKey } from './transactionalTime'
import {
  BOUNCE_WARNING,
  parseDaily,
  peakFailureDay,
  statsOf,
  statsRates,
  type NotificationStats,
  type TrackedRate
} from './transactionalStats'
import { DeletedTemplatePill, IntegrationTag, MenuEntryLabel, type StatGate } from './notificationParts'
import { templateIdOf, useActionRules } from './notificationRules'
import type { TransactionalRowActions } from './TransactionalTable'
import { useCurrentStats, useTemplateStates } from './useTransactionalStats'

export interface TransactionalDetailProps extends Omit<TransactionalRowActions, 'onOpen' | 'onApi'> {
  notificationId: string | undefined
  workspace: Workspace
  permissions: UserPermissions | null
  permissionsLoading: boolean
  /** The list's row, shown while the notification loads. */
  placeholder?: TransactionalNotification
  /** IDs of the rows shown, search included, for J and K. */
  order: string[]
  onNavigate: (id: string) => void
  onClose: () => void
  /** Opens the API command modal, from « All languages ». */
  onApi: (notification: TransactionalNotification) => void
  period: TransactionalPeriod
  periods: TransactionalPeriods
  onPeriodChange: (period: TransactionalPeriod) => void
  stats?: Map<string, NotificationStats>
  /** `stats` covers this notification; when it does not, the sheet reads its own. */
  listed: boolean
  statsGate: StatGate
  smtp: boolean
  timezone: string
  now: Date
}

/** Screen 92: one notification in a full drawer, opened by `?notification={id}`. */
export function TransactionalDetail(props: TransactionalDetailProps) {
  const { t } = useLingui()
  const { notificationId, order, onNavigate } = props
  const index = notificationId ? order.indexOf(notificationId) : -1
  const inList = index >= 0
  const nav = {
    position: inList ? index + 1 : undefined,
    total: inList ? order.length : undefined,
    onPrev: inList && index > 0 ? () => onNavigate(order[index - 1]) : undefined,
    onNext: inList && index < order.length - 1 ? () => onNavigate(order[index + 1]) : undefined
  }

  return (
    <FullDrawer open={Boolean(notificationId)} onClose={props.onClose} backLabel={t`Transactional`} nav={nav}>
      {notificationId ? <DetailBody key={notificationId} {...props} notificationId={notificationId} /> : null}
    </FullDrawer>
  )
}

function DetailBody(props: TransactionalDetailProps & { notificationId: string }) {
  const { t } = useLingui()
  const { notificationId, workspace, placeholder } = props

  const query = useQuery({
    queryKey: ['transactional-notification', workspace.id, notificationId],
    queryFn: () => transactionalNotificationsApi.get({ workspace_id: workspace.id, id: notificationId }),
    placeholderData: placeholder?.id === notificationId ? { notification: placeholder } : undefined,
    retry: (count, error) => !(error instanceof ApiError && error.status === 404) && count < 2
  })

  const notification = query.data?.notification
  if (!notification) {
    if (query.error instanceof ApiError && query.error.status === 404) {
      return (
        <Result
          status="404"
          title={t`Notification not found`}
          extra={<Button onClick={props.onClose}>{t`Back to Transactional`}</Button>}
        />
      )
    }
    if (query.isError) return <LoadError error={query.error} onRetry={() => void query.refetch()} />
    return <Skeleton active paragraph={{ rows: 8 }} />
  }
  return <DetailContent {...props} notification={notification} />
}

function DetailContent(props: TransactionalDetailProps & { notification: TransactionalNotification }) {
  const { t, i18n } = useLingui()
  const { int, percent } = useNumberFormat()
  const { notification, workspace, permissions, permissionsLoading, smtp, timezone, now, period, periods } = props
  const ready = !permissionsLoading
  const canReadMessages = ready && Boolean(permissions?.message_history?.read)
  const canReadTemplates = Boolean(permissions?.templates?.read)

  const templateId = templateIdOf(notification)
  const templates = useTemplateStates({
    workspaceId: workspace.id,
    templateIds: templateId ? [templateId] : [],
    ready,
    canRead: canReadTemplates
  })
  const template = templateId ? templates.get(templateId) : undefined
  const loadedTemplate = template?.status === 'ok' ? template.template : undefined
  const rules = useActionRules()(notification, permissions, template, workspace)

  const daily = useQuery({
    queryKey: ['transactional-daily', workspace.id, notification.id, period, periods.current.endDay],
    queryFn: async () => {
      const response = await transactionalAnalytics.query(
        {
          schema: 'message_history',
          measures: ['count_sent', 'count_delivered', 'count_bounced', 'count_failed'],
          dimensions: [],
          timezone,
          timeDimensions: [
            {
              dimension: 'created_at',
              granularity: 'day',
              dateRange: [periods.current.startDay, periods.current.endDay]
            }
          ],
          filters: [{ member: 'transactional_notification_id', operator: 'equals', values: [notification.id] }]
        },
        workspace.id
      )
      return parseDaily(response.data)
    },
    enabled: canReadMessages,
    staleTime: 60_000,
    refetchOnWindowFocus: true,
    placeholderData: keepPreviousData
  })

  // A notification the list's statistics do not cover (created since, or opened by link) reads its own.
  const ids = useMemo(() => [notification.id], [notification.id])
  const own = useCurrentStats({
    workspaceId: workspace.id,
    ids,
    period,
    periods,
    timezone,
    enabled: canReadMessages && !props.listed
  })
  const stats = statsOf(props.listed ? props.stats : own.data?.stats, notification.id)
  const rates = statsRates(stats)
  const peak = peakFailureDay(daily.data ?? [])
  const daysLabel = t`${plural(periods.days, { one: 'Last # day', other: 'Last # days' })}`
  const emails = (n: number) => t`${plural(n, { one: `${int(n)} email`, other: `${int(n)} emails` })}`

  /** A KPI value that opens the message log with the cell's filter. */
  const logLink = (label: string, value: string, filter: Record<string, boolean>): ReactNode =>
    canReadMessages ? (
      <Link
        to="/console/workspace/$workspaceId/logs"
        params={{ workspaceId: workspace.id }}
        search={{ tab: LOGS_TAB.messages, transactional_notification_id: notification.id, ...filter }}
        // Named by what it counts, not by its number alone; the number stays in the name.
        aria-label={t`${label}: ${value}, view in the message log`}
        className="hover:underline"
      >
        {value}
      </Link>
    ) : (
      value
    )

  const smtpTooltip = (kind: 'delivery' | 'bounce') =>
    kind === 'delivery'
      ? t`SMTP provider doesn't support delivery webhooks, so delivery status cannot be tracked`
      : t`SMTP provider doesn't support bounce webhooks, so bounce status cannot be tracked`

  const trackedSub = (rate: TrackedRate, count: number) => (rate.notTracked ? t`Not tracked` : emails(count))
  const rateValue = (rate: number | null) => (rate === null ? '—' : percent(rate))
  const gate: StatGate = props.listed
    ? props.statsGate
    : {
        forbidden: props.statsGate.forbidden,
        // A period switch waits for the new figures, as the list's do.
        loading: !ready || own.isLoading || own.isPlaceholderData,
        error: own.isError && own.data === undefined
      }

  const labels = {
    sent: t`Sent`,
    delivered: t`Delivered`,
    opened: t`Opened`,
    clicked: t`Clicked`,
    bounced: t`Bounced`,
    failed: t`Failed`
  }
  const kpis: KpiItem[] = [
    {
      key: 'sent',
      label: labels.sent,
      value: logLink(labels.sent, int(stats.sent), { is_failed: false }),
      sub: daysLabel
    },
    smtp
      ? { key: 'delivered', label: labels.delivered, value: '—', hint: smtpTooltip('delivery') }
      : {
          key: 'delivered',
          label: labels.delivered,
          value: logLink(labels.delivered, rateValue(rates.delivered), { is_delivered: true }),
          sub: emails(stats.delivered)
        },
    {
      key: 'opened',
      label: labels.opened,
      value: logLink(labels.opened, rateValue(rates.opens.rate), { is_opened: true }),
      sub: trackedSub(rates.opens, stats.opened)
    },
    {
      key: 'clicked',
      label: labels.clicked,
      value: logLink(labels.clicked, rateValue(rates.clicks.rate), { is_clicked: true }),
      sub: trackedSub(rates.clicks, stats.clicked)
    },
    smtp
      ? { key: 'bounced', label: labels.bounced, value: '—', hint: smtpTooltip('bounce') }
      : {
          key: 'bounced',
          label: labels.bounced,
          value: logLink(labels.bounced, rateValue(rates.bounced), { is_bounced: true }),
          sub: emails(stats.bounced),
          tone: rates.bounced !== null && rates.bounced >= BOUNCE_WARNING ? 'warning' : undefined
        },
    {
      key: 'failed',
      label: labels.failed,
      value: logLink(labels.failed, int(stats.failed), { is_failed: true }),
      sub: peak ? t`Peak on ${formatSeriesDay(peak, i18n.locale)}` : undefined,
      tone: peak ? 'warning' : undefined,
      hint: t`Sends refused by the provider. Requests rejected before sending are not logged.`
    }
  ]
  const kpiItems = gate.error ? kpis.map((item) => ({ ...item, value: '—', sub: t`Couldn't load` })) : kpis

  // Header: template, sender, last update.
  const sender = resolveSenderInfo(workspace, loadedTemplate)
  // Without the template read (no right, missing, failed, or none set), its own sender is unknown:
  // the provider's default is shown, and marked as such.
  const senderText = sender.sender
    ? `${sender.sender.name ? `${sender.sender.name} <${sender.sender.email}>` : sender.sender.email}${
        sender.providerDefaultOnly ? ` ${t`(default)`}` : ''
      }`
    : null
  // Why the provider's default stands in for the template's sender.
  const defaultReason = !templateId
    ? t`The provider's default sender: this notification has no email template.`
    : template?.status === 'forbidden'
      ? t`The provider's default sender: you need access to templates to see the template's own.`
      : template?.status === 'notFound'
        ? t`The provider's default sender: the template was not found.`
        : template?.status === 'error'
          ? t`The provider's default sender: the template, which can choose another one, couldn't be loaded.`
          : undefined
  const meta: ReactNode[] = []
  if (templateId) {
    const name = loadedTemplate?.name
    meta.push(
      <span key="template" className="inline-flex items-center gap-1">
        {t`Template`} {name ? <span className="text-ink-2">{name}</span> : <Mono>{templateId}</Mono>}
        {loadedTemplate?.deleted_at ? <DeletedTemplatePill /> : null}
      </span>
    )
  }
  if (ready && template?.status !== 'loading') {
    meta.push(
      <Tooltip
        key="sender"
        title={
          (sender.sender && sender.providerDefaultOnly ? defaultReason : undefined) ??
          t`An API call can choose another sender with email_options.sender_id. A translation can use another sender.`
        }
      >
        {senderText ? <span>{senderText}</span> : <span className="text-warning-fg">{t`No sender`}</span>}
      </Tooltip>
    )
  }
  meta.push(
    <span key="updated">{t`Updated ${formatFullDateTime(notification.updated_at, timezone, i18n.locale)}`}</span>
  )

  const menuItems: MenuProps['items'] = [
    { key: 'copy', label: t`Copy ID` },
    {
      key: 'logs',
      label: <MenuEntryLabel label={t`View in logs`} reason={rules.logs} />,
      disabled: rules.logs !== undefined
    },
    { type: 'divider' },
    {
      key: 'delete',
      label: <MenuEntryLabel label={t`Delete`} reason={rules.remove} />,
      danger: rules.remove === undefined,
      disabled: rules.remove !== undefined
    }
  ]
  const onMenu: MenuProps['onClick'] = ({ key }) => {
    if (key === 'copy') props.onCopyId(notification)
    else if (key === 'logs') props.onLogs(notification)
    else if (key === 'delete') props.onDelete(notification)
  }

  const actions = ready ? (
    <div className="flex items-center gap-2">
      {/* Spans: a disabled button receives no hover, and its tooltip would never show. */}
      <Tooltip title={rules.test}>
        <span>
          <Button
            icon={<Send size={14} aria-hidden="true" />}
            disabled={rules.test !== undefined}
            onClick={() => props.onTest(notification)}
          >
            {t`Send test`}
          </Button>
        </span>
      </Tooltip>
      <Tooltip title={rules.edit}>
        <span>
          <Button disabled={rules.edit !== undefined} onClick={() => props.onEdit(notification)}>
            {t`Edit`}
          </Button>
        </span>
      </Tooltip>
      <Dropdown trigger={['click']} menu={{ items: menuItems, onClick: onMenu }}>
        <Button aria-label={t`More actions`} icon={<MoreHorizontal size={14} aria-hidden="true" />} />
      </Dropdown>
    </div>
  ) : (
    <Skeleton.Button active />
  )

  // Settings.
  const mode = notification.tracking_settings?.tracking_mode || 'inherit'
  const policy = getEmailTrackingPolicy(workspace.settings)
  const consentHint = t`Contacts' tracking consent still applies`
  const onOff = (on: boolean) => (on ? t`On` : t`Off`)
  const trackingLabel =
    mode === 'no_tracking'
      ? t`No open or click tracking, keep UTM parameters`
      : mode === 'disabled'
        ? t`Disabled for this notification`
        : t`Follow workspace setting`
  const settings = notification.tracking_settings
  const utm =
    mode === 'disabled' ? (
      t`None`
    ) : (
      <div className="flex flex-col items-start gap-1">
        {(['utm_source', 'utm_medium', 'utm_campaign'] as const).map((field) =>
          settings?.[field] ? (
            <span key={field} className="min-w-0">
              <Mono>{`${field}=${settings[field]}`}</Mono>
            </span>
          ) : null
        )}
        {settings?.utm_content ? (
          <Mono>{`utm_content=${settings.utm_content}`}</Mono>
        ) : templateId ? (
          <Tooltip title={t`Defaults to the template ID`}>
            <span className="min-w-0">
              <Mono>{`utm_content=${templateId}`}</Mono>
            </span>
          </Tooltip>
        ) : null}
      </div>
    )
  const integration = notification.integration_id
    ? workspace.integrations?.find((item) => item.id === notification.integration_id)
    : undefined
  const settingsItems = [
    { label: t`Channel`, value: t`Email` },
    { label: t`Tracking`, value: trackingLabel },
    {
      label: t`Opens`,
      value: mode === 'inherit' ? <Tooltip title={consentHint}>{onOff(policy.opens_enabled)}</Tooltip> : t`Off`
    },
    {
      label: t`Clicks`,
      value: mode === 'inherit' ? <Tooltip title={consentHint}>{onOff(policy.clicks_enabled)}</Tooltip> : t`Off`
    },
    { label: t`UTM`, value: utm },
    {
      label: t`Reply-To`,
      value: (
        <Tooltip title={t`An API call can set email_options.reply_to. A translation can use another one.`}>
          {/* A template not read may set one: unknown, not none. */}
          <span>{sender.replyTo ?? (templateId && defaultReason ? t`Unknown` : '—')}</span>
        </Tooltip>
      )
    },
    ...(notification.integration_id
      ? [{ label: t`Managed by`, value: integration ? integration.name : t`an integration` }]
      : []),
    { label: t`Created`, value: formatFullDateTime(notification.created_at, timezone, i18n.locale) }
  ]

  return (
    <>
      <PageHeader
        title={notification.name}
        status={
          <span className="flex items-center gap-2">
            <IntegrationTag notification={notification} workspace={workspace} />
            <Mono copy>{notification.id}</Mono>
          </span>
        }
        meta={meta}
        description={notification.description || undefined}
        actions={actions}
      />
      <div className="grid grid-cols-1 gap-3 min-[1200px]:grid-cols-[minmax(0,1fr)_380px]">
        <div className="flex min-w-0 flex-col gap-3">
          {canReadMessages || !ready ? (
            <>
              <KpiStrip items={kpiItems} loading={!ready || gate.loading} />
              <SendsPerDayChart
                points={daily.data}
                // A period switch keeps the old days in cache only: the chart waits rather than show them.
                loading={!ready || daily.isLoading || daily.isPlaceholderData}
                error={daily.isError && !daily.data ? daily.error : undefined}
                onRetry={() => void daily.refetch()}
                smtp={smtp}
                today={todayKey(timezone, now)}
                period={period}
                onPeriodChange={props.onPeriodChange}
              />
            </>
          ) : null}
          {ready ? (
            <RecentSends
              workspaceId={workspace.id}
              notificationId={notification.id}
              timezone={timezone}
              canReadMessages={canReadMessages}
              now={now}
            />
          ) : null}
        </div>
        <div className="flex min-w-0 flex-col gap-3">
          <PreviewPanel
            workspace={workspace}
            template={!templateId ? { status: 'notFound' } : (template ?? { status: 'loading' })}
            onOpen={() => props.onPreview(notification)}
          />
          <Panel title={t`Settings`}>
            <KeyValueList items={settingsItems} labelWidth={96} />
          </Panel>
          <ApiCallPanel
            notification={notification}
            workspaceId={workspace.id}
            onOpenAll={() => props.onApi(notification)}
          />
        </div>
      </div>
    </>
  )
}
