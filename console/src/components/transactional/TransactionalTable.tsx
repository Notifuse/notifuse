import type { MouseEvent, ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { Button, Dropdown, Table, Tooltip, type MenuProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { MoreHorizontal, Send, SquareTerminal } from 'lucide-react'
import { Mono, RateBar, useNumberFormat } from '../layout'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'
import type { UserPermissions, Workspace } from '../../services/api/types'
import type { TemplateState } from './PreviewPanel'
import {
  DeletedTemplatePill,
  GatedStat,
  IntegrationTag,
  MenuEntryLabel,
  SmtpDash,
  type StatGate
} from './notificationParts'
import { templateIdOf, useActionRules } from './notificationRules'
import { statsOf, statsRates, BOUNCE_WARNING, type NotificationStats, type TrackedRate } from './transactionalStats'
import { formatFullDateTime, formatLastSent } from './transactionalTime'
import { LAST_SENT_DAYS } from './useTransactionalStats'

export interface TransactionalRowActions {
  onOpen: (notification: TransactionalNotification) => void
  onTest: (notification: TransactionalNotification) => void
  onApi: (notification: TransactionalNotification) => void
  onEdit: (notification: TransactionalNotification) => void
  onPreview: (notification: TransactionalNotification) => void
  onCopyId: (notification: TransactionalNotification) => void
  onLogs: (notification: TransactionalNotification) => void
  onDelete: (notification: TransactionalNotification) => void
}

interface TransactionalTableProps extends TransactionalRowActions {
  workspace: Workspace
  permissions: UserPermissions | null
  notifications: TransactionalNotification[]
  stats?: Map<string, NotificationStats>
  statsGate: StatGate
  lastSent?: Map<string, string | null>
  lastSentGate: StatGate
  templates: Map<string, TemplateState>
  smtp: boolean
  timezone: string
  now: Date
  footer: ReactNode
}

/** A tracked rate: its bar, « Not tracked », or the partial tone with how many emails were tracked. */
export function TrackedRateCell({ value, sent }: { value: TrackedRate; sent: number }) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  if (value.notTracked) return <span className="text-sm text-ink-3">{t`Not tracked`}</span>
  if (!value.partial) return <RateBar rate={value.rate} />
  const tracked = int(value.tracked)
  const total = int(sent)
  return (
    <Tooltip title={t`Tracked on ${tracked} of ${total} emails`}>
      <span>
        <RateBar rate={value.rate} tone="partial" />
      </span>
    </Tooltip>
  )
}

/** The template column: its name once read, its ID otherwise, flagged when deleted or missing. */
export function TemplateLabel({ state, templateId }: { state: TemplateState | undefined; templateId?: string }) {
  const { t } = useLingui()
  if (!templateId) return <span className="text-sm text-ink-4">—</span>
  if (state?.status === 'ok') {
    return (
      <span className="inline-flex min-w-0 items-center gap-2">
        <span className="truncate text-sm text-ink-3">{state.template.name}</span>
        {state.template.deleted_at ? <DeletedTemplatePill /> : null}
      </span>
    )
  }
  if (state?.status === 'notFound') {
    return (
      <Tooltip title={t`Template not found`}>
        <span className="min-w-0">
          <Mono>{templateId}</Mono>
        </span>
      </Tooltip>
    )
  }
  return <Mono>{templateId}</Mono>
}

/** Keeps a click on a button of the row from opening the sheet. */
const stop = (event: MouseEvent) => event.stopPropagation()

/**
 * A plain click on a notification's link: the row's own click opens the sheet, so the link does not
 * navigate as well (one history entry). A click with a modifier is the browser's (a new tab or
 * window), and does not open the sheet here.
 */
function onNameClick(event: MouseEvent<HTMLAnchorElement>) {
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) event.stopPropagation()
  else event.preventDefault()
}

/** The list of screen 91: one row per notification, in the server's order. */
export function TransactionalTable(props: TransactionalTableProps) {
  const {
    workspace,
    permissions,
    notifications,
    stats,
    statsGate,
    lastSent,
    lastSentGate,
    templates,
    smtp,
    timezone,
    now,
    footer
  } = props
  const { t, i18n } = useLingui()
  const { int, percent } = useNumberFormat()
  const rulesOf = useActionRules()

  const statCell = (render: (stats: NotificationStats) => ReactNode) => {
    const Cell = (_: unknown, notification: TransactionalNotification) => (
      <GatedStat gate={statsGate}>{() => render(statsOf(stats, notification.id))}</GatedStat>
    )
    return Cell
  }

  const columns: ColumnsType<TransactionalNotification> = [
    {
      title: t`Notification`,
      key: 'notification',
      render: (_, notification) => (
        <div className="flex min-w-0 flex-col gap-0.5">
          <div className="flex min-w-0 items-center gap-2">
            <Tooltip title={notification.description || undefined}>
              {/* The row's link, for the keyboard and screen readers; a plain click is left to the row. */}
              <Link
                to="."
                search={(prev: Record<string, unknown>) => ({ ...prev, notification: notification.id })}
                onClick={onNameClick}
                className="row-title-link truncate text-base font-medium"
              >
                {notification.name}
              </Link>
            </Tooltip>
            <IntegrationTag notification={notification} workspace={workspace} />
          </div>
          <span className="min-w-0">
            <Mono>{notification.id}</Mono>
          </span>
        </div>
      )
    },
    {
      title: t`Template`,
      key: 'template',
      render: (_, notification) => {
        const templateId = templateIdOf(notification)
        return <TemplateLabel templateId={templateId} state={templateId ? templates.get(templateId) : undefined} />
      }
    },
    {
      title: t`Sent`,
      key: 'sent',
      align: 'right',
      render: statCell((row) => <span className="text-sm tabular-nums text-ink">{int(row.sent)}</span>)
    },
    {
      title: t`Delivered`,
      key: 'delivered',
      render: statCell((row) => {
        if (smtp) return <SmtpDash kind="delivery" />
        const rates = statsRates(row)
        return <RateBar rate={rates.delivered} tone={rates.deliveredTone === 'warning' ? 'warning' : 'default'} />
      })
    },
    {
      title: t`Opens`,
      key: 'opens',
      render: statCell((row) => <TrackedRateCell value={statsRates(row).opens} sent={row.sent} />)
    },
    {
      title: t`Clicks`,
      key: 'clicks',
      render: statCell((row) => <TrackedRateCell value={statsRates(row).clicks} sent={row.sent} />)
    },
    {
      title: t`Failed`,
      key: 'failed',
      align: 'right',
      render: statCell((row) => (
        <span className={`text-sm tabular-nums ${row.failed ? 'text-ink' : 'text-ink-4'}`}>{int(row.failed)}</span>
      ))
    },
    {
      title: t`Bounces`,
      key: 'bounces',
      align: 'right',
      render: statCell((row) => {
        if (smtp) return <SmtpDash kind="bounce" />
        const rate = statsRates(row).bounced
        if (rate === null) return <span className="text-sm text-ink-3">—</span>
        return (
          <span className={`text-sm tabular-nums ${rate >= BOUNCE_WARNING ? 'text-warning-fg' : 'text-ink'}`}>
            {percent(rate)}
          </span>
        )
      })
    },
    {
      title: t`Last sent`,
      key: 'lastSent',
      render: (_, notification) => (
        <GatedStat gate={lastSentGate}>
          {() => {
            const at = lastSent?.get(notification.id)
            if (!at)
              return (
                <Tooltip
                  title={t`${plural(LAST_SENT_DAYS, {
                    one: 'No successful send in the last # day',
                    other: 'No successful send in the last # days'
                  })}`}
                >
                  <span className="text-sm text-ink-3">—</span>
                </Tooltip>
              )
            return (
              <Tooltip title={formatFullDateTime(at, timezone, i18n.locale)}>
                <span className="text-sm whitespace-nowrap text-ink-2">
                  {formatLastSent(at, now, timezone, i18n.locale, (time) => t`Yesterday, ${time}`)}
                </span>
              </Tooltip>
            )
          }}
        </GatedStat>
      )
    },
    {
      title: <span className="sr-only">{t`Actions`}</span>,
      key: 'actions',
      align: 'right',
      width: 116,
      render: (_, notification) => {
        const templateId = templateIdOf(notification)
        const rules = rulesOf(notification, permissions, templateId ? templates.get(templateId) : undefined, workspace)
        const items: MenuProps['items'] = [
          {
            key: 'edit',
            label: <MenuEntryLabel label={t`Edit`} reason={rules.edit} />,
            disabled: rules.edit !== undefined
          },
          {
            key: 'preview',
            label: <MenuEntryLabel label={t`Preview template`} reason={rules.preview} />,
            disabled: rules.preview !== undefined
          },
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
        const onMenu: MenuProps['onClick'] = ({ key, domEvent }) => {
          domEvent.stopPropagation()
          if (key === 'edit') props.onEdit(notification)
          else if (key === 'preview') props.onPreview(notification)
          else if (key === 'copy') props.onCopyId(notification)
          else if (key === 'logs') props.onLogs(notification)
          else if (key === 'delete') props.onDelete(notification)
        }
        return (
          <div
            className="flex items-center justify-end gap-1 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100"
            onClick={stop}
          >
            <Tooltip title={rules.test ?? t`Send test`}>
              {/* A span: a disabled button receives no hover, and its tooltip would never show. */}
              <span>
                <Button
                  type="text"
                  size="small"
                  aria-label={t`Send test`}
                  icon={<Send size={14} aria-hidden="true" />}
                  disabled={rules.test !== undefined}
                  onClick={() => props.onTest(notification)}
                />
              </span>
            </Tooltip>
            <Tooltip title={t`API command`}>
              <Button
                type="text"
                size="small"
                aria-label={t`API command`}
                icon={<SquareTerminal size={14} aria-hidden="true" />}
                onClick={() => props.onApi(notification)}
              />
            </Tooltip>
            <Dropdown trigger={['click']} menu={{ items, onClick: onMenu }}>
              <Button
                type="text"
                size="small"
                aria-label={t`More actions`}
                icon={<MoreHorizontal size={14} aria-hidden="true" />}
              />
            </Dropdown>
          </div>
        )
      }
    }
  ]

  return (
    <Table<TransactionalNotification>
      rowKey="id"
      size="small"
      columns={columns}
      dataSource={notifications}
      pagination={false}
      rowClassName="group cursor-pointer"
      onRow={(notification) => ({ onClick: () => props.onOpen(notification) })}
      footer={() => footer}
    />
  )
}
