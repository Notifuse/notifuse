import type { ReactNode } from 'react'
import { App, Button, Dropdown, Table, Tooltip, type MenuProps, type TableColumnsType } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Ellipsis, Pause, Play, Zap } from 'lucide-react'
import { Pill, RateBar, useNumberFormat, useRelativeDate } from '../../layout'
import type { Automation, JourneyCounts } from '../../../services/api/automation'
import { CouldntLoad } from '../../broadcasts/CouldntLoad'
import { AutomationStatusPill } from '../AutomationStatusPill'
import { TriggerText } from '../TriggerText'
import { describeTrigger, type TriggerNames } from '../describeTrigger'
import { completedRate, exitedRate, hasEmailStep, neverRan } from '../journeyRates'
import { NODE_TYPES } from '../nodeTypes'
import { orderedSteps, stepSummary } from '../stepIcons'
import { getNodeDescription } from '../nodes/constants'
import type { OpenRates } from './useAutomationOpenRates'

export interface AutomationsTableProps {
  rows: Automation[]
  counts: Record<string, JourneyCounts> | undefined
  openRates: OpenRates
  names: TriggerNames
  canWrite: boolean
  canReadHistory: boolean
  activeId?: string
  footer?: ReactNode
  onOpen: (automation: Automation, withStats: boolean) => void
  onActivate: (automation: Automation) => void
  onPause: (automation: Automation) => void
  onDelete: (automation: Automation) => void
  onViewMessages: (automation: Automation) => void
}

export function AutomationsTable({
  rows,
  counts,
  openRates,
  names,
  canWrite,
  canReadHistory,
  activeId,
  footer,
  onOpen,
  onActivate,
  onPause,
  onDelete,
  onViewMessages
}: AutomationsTableProps) {
  const { t, i18n } = useLingui()
  const { modal } = App.useApp()
  const { int } = useNumberFormat()
  const relative = useRelativeDate()
  const noWrite = t`You don't have write permission for automations`

  const dash = <span className="text-ink-3">—</span>
  const journeyCell = (a: Automation, read: (c: JourneyCounts) => ReactNode) => {
    const c = counts?.[a.id]
    if (!c || neverRan(a, c)) return dash
    return read(c)
  }

  const confirm = (kind: 'activate' | 'pause', a: Automation) => {
    modal.confirm(
      kind === 'activate'
        ? {
            title: t`Activate automation?`,
            content: t`The automation will start processing contacts that match the trigger.`,
            okText: t`Yes, activate`,
            cancelText: t`Cancel`,
            onOk: () => onActivate(a)
          }
        : {
            title: t`Pause automation?`,
            content: t`The automation will stop processing new contacts.`,
            okText: t`Yes, pause`,
            cancelText: t`Cancel`,
            onOk: () => onPause(a)
          }
    )
  }

  const confirmDelete = (a: Automation) =>
    modal.confirm({
      title: t`Delete automation?`,
      content: t`This action cannot be undone.`,
      okText: t`Yes, delete`,
      cancelText: t`Cancel`,
      okButtonProps: { danger: true },
      onOk: () => onDelete(a)
    })

  const menu = (a: Automation): MenuProps['items'] => {
    const editable = a.status !== 'live'
    const items: MenuProps['items'] = []
    if (editable) items.push({ key: 'edit', label: t`Edit Automation`, disabled: !canWrite, onClick: () => onOpen(a, false) })
    if (!neverRan(a, counts?.[a.id])) items.push({ key: 'stats', label: t`View Flow Stats`, onClick: () => onOpen(a, true) })
    if (canReadHistory && hasEmailStep(a)) items.push({ key: 'messages', label: t`View messages`, onClick: () => onViewMessages(a) })
    items.push({ type: 'divider' })
    items.push({
      key: 'delete',
      danger: true,
      disabled: !canWrite || !editable,
      label: (
        <Tooltip title={!canWrite ? noWrite : !editable ? t`Pause the automation first` : undefined} placement="left">
          <span>{t`Delete Automation`}</span>
        </Tooltip>
      ),
      onClick: () => confirmDelete(a)
    })
    return items
  }

  const columns: TableColumnsType<Automation> = [
    {
      title: t`Status`,
      key: 'status',
      width: 100,
      render: (_, a) => <AutomationStatusPill status={a.status} />
    },
    {
      title: t`Automation`,
      key: 'name',
      render: (_, a) => {
        const lastActivity = relative(a.updated_at)
        return (
          <div className="flex min-w-0 flex-col gap-0.5">
            <div className="flex min-w-0 items-center gap-2">
              <Tooltip title={t`ID ${a.id} · Last activity ${lastActivity}`}>
                <span className="truncate font-medium text-ink">{a.name}</span>
              </Tooltip>
              {a.trigger ? (
                <Pill tone="neutral" dot={false}>
                  {a.trigger.frequency === 'every_time' ? t`Every time` : t`Once`}
                </Pill>
              ) : null}
            </div>
            <div className="flex min-w-0 items-center gap-1 text-xs text-ink-3">
              <Zap size={12} aria-hidden="true" className="shrink-0 text-warning-fg" />
              <span className="truncate">
                <TriggerText label={describeTrigger(a.trigger, names)} />
              </span>
            </div>
          </div>
        )
      }
    },
    {
      title: t`Steps`,
      key: 'steps',
      width: 210,
      render: (_, a) => {
        const steps = orderedSteps(a)
        if (steps.length === 0) return dash
        const { shown, rest } = stepSummary(steps)
        const tooltip = (
          <ol className="m-0 flex list-decimal flex-col gap-0.5 pl-4">
            {steps.map((step) => (
              <li key={step.id}>{getNodeDescription(step.config) ?? i18n._(NODE_TYPES[step.type].label)}</li>
            ))}
          </ol>
        )
        return (
          <Tooltip title={tooltip}>
            <span className="inline-flex items-center gap-1" data-steps>
              {shown.map((step) => {
                const meta = NODE_TYPES[step.type]
                const Icon = meta.icon
                return (
                  <span key={step.id} data-step-type={step.type} className={`flex size-5.5 items-center justify-center rounded-md ${meta.chip}`}>
                    <Icon size={12} aria-hidden="true" />
                  </span>
                )
              })}
              {rest > 0 ? <span className="text-xs text-ink-3">{`+${int(rest)}`}</span> : null}
            </span>
          </Tooltip>
        )
      }
    },
    {
      title: t`List`,
      key: 'list',
      width: 140,
      render: (_, a) =>
        a.list_id ? (
          <Pill tone="neutral" dot={false}>
            {names.lists?.find((l) => l.id === a.list_id)?.name ?? a.list_id}
          </Pill>
        ) : (
          <span className="text-sm text-ink-3">{t`No list`}</span>
        )
    },
    {
      title: t`In progress`,
      key: 'active',
      width: 96,
      align: 'right',
      render: (_, a) => journeyCell(a, (c) => <span className="tabular-nums">{int(c.active)}</span>)
    },
    {
      title: t`Entered`,
      key: 'entered',
      width: 86,
      align: 'right',
      render: (_, a) => journeyCell(a, (c) => <span className="tabular-nums">{int(c.total)}</span>)
    },
    {
      title: t`Completed`,
      key: 'completed',
      width: 110,
      render: (_, a) =>
        journeyCell(a, (c) => {
          const completed = int(c.completed)
          const exited = int(c.exited)
          const failed = int(c.failed)
          return (
            <Tooltip title={t`${completed} completed · ${exited} exited · ${failed} failed`}>
              <span>
                <RateBar rate={completedRate(c)} />
              </span>
            </Tooltip>
          )
        })
    },
    {
      title: t`Exited`,
      key: 'exited',
      width: 76,
      align: 'right',
      render: (_, a) =>
        journeyCell(a, (c) => {
          const rate = exitedRate(c)
          return rate === null ? dash : <PercentText rate={rate} />
        })
    },
    {
      title: t`Failed`,
      key: 'failed',
      width: 70,
      align: 'right',
      render: (_, a) =>
        journeyCell(a, (c) => (
          <span className={`tabular-nums ${c.failed > 0 ? 'text-danger-fg' : ''}`}>{int(c.failed)}</span>
        ))
    },
    {
      title: t`Open rate`,
      key: 'open',
      width: 90,
      align: 'right',
      render: (_, a) => {
        if (!canReadHistory || !hasEmailStep(a)) return dash
        if (openRates.error) return <CouldntLoad />
        const rate = openRates.rates.get(a.id)
        return rate === null || rate === undefined ? dash : <PercentText rate={rate} />
      }
    },
    {
      key: 'actions',
      width: 84,
      align: 'right',
      render: (_, a) => (
        <div className="flex items-center justify-end gap-0.5" onClick={(e) => e.stopPropagation()}>
          {a.status === 'live' ? (
            <Tooltip title={canWrite ? t`Pause Automation` : noWrite}>
              <Button
                type="text"
                size="small"
                aria-label={t`Pause Automation`}
                disabled={!canWrite}
                icon={<Pause size={14} aria-hidden="true" />}
                onClick={() => confirm('pause', a)}
              />
            </Tooltip>
          ) : (
            <Tooltip title={canWrite ? t`Activate Automation` : noWrite}>
              <Button
                type="text"
                size="small"
                aria-label={t`Activate Automation`}
                disabled={!canWrite}
                icon={<Play size={14} aria-hidden="true" />}
                onClick={() => confirm('activate', a)}
              />
            </Tooltip>
          )}
          <Dropdown menu={{ items: menu(a) }} trigger={['click']} placement="bottomRight">
            <Button type="text" size="small" aria-label={t`More actions`} icon={<Ellipsis size={14} aria-hidden="true" />} />
          </Dropdown>
        </div>
      )
    }
  ]

  return (
    <Table<Automation>
      rowKey="id"
      columns={columns}
      dataSource={rows}
      pagination={false}
      size="middle"
      tableLayout="fixed"
      scroll={{ x: 1250 }}
      rowClassName={(a) => `cursor-pointer ${a.id === activeId ? 'is-hover' : ''}`}
      onRow={(a) => ({ onClick: () => onOpen(a, a.status !== 'draft') })}
      footer={footer ? () => footer : undefined}
    />
  )
}

function PercentText({ rate }: { rate: number }) {
  const { percent } = useNumberFormat()
  return <span className="text-sm tabular-nums">{percent(rate)}</span>
}

