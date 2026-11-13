import { Button, Tag, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { KeyValueList, Mono, Panel, Pill, useNumberFormat, type KeyValueItem, type PillTone } from '../../layout'
import type { Broadcast, BroadcastVariation } from '../../../services/api/broadcast'
import type { List } from '../../../services/api/list'
import type { Segment } from '../../../services/api/segment'
import type { Task, TaskStatus } from '../../../services/api/task'
import type { Workspace } from '../../../services/api/types'
import TemplatePreviewDrawer from '../../templates/TemplatePreviewDrawer'
import { workspaceTimezone } from '../access'
import { useBroadcastDates } from '../useBroadcastDates'
import { asTemplate } from './asTemplate'
import { resolveSenderOf, useSenderText } from './senderText'
import { TemplatePreviewFrame } from './TemplatePreviewFrame'

export interface BroadcastAsideProps {
  workspace: Workspace
  broadcast: Broadcast
  lists: Pick<List, 'id' | 'name'>[]
  segments: Pick<Segment, 'id' | 'name' | 'color'>[]
  /** The broadcast's sending task, when one of the workspace's last 100 is it. */
  task?: Task | null
  /** The variant whose template the preview shows: the winner, else the first. */
  previewVariation?: BroadcastVariation
  /** Without Templates read the broadcast comes without its templates. */
  canReadTemplates?: boolean
}

/** The side column of the broadcast detail: preview, audience, tracking, task and history. */
export function BroadcastAside({
  workspace,
  broadcast,
  lists,
  segments,
  task,
  previewVariation,
  canReadTemplates = true
}: BroadcastAsideProps) {
  return (
    <div className="flex flex-col gap-3">
      <PreviewBlock workspace={workspace} broadcast={broadcast} variation={previewVariation} canReadTemplates={canReadTemplates} />
      <AudienceBlock workspace={workspace} broadcast={broadcast} lists={lists} segments={segments} />
      <TrackingBlock broadcast={broadcast} />
      {task ? <TaskBlock task={task} timezone={workspaceTimezone(workspace)} /> : null}
      <HistoryBlock broadcast={broadcast} timezone={workspaceTimezone(workspace)} />
    </div>
  )
}

function PreviewBlock({
  workspace,
  broadcast,
  variation,
  canReadTemplates
}: {
  workspace: Workspace
  broadcast: Broadcast
  variation?: BroadcastVariation
  canReadTemplates: boolean
}) {
  const { t } = useLingui()
  const senderText = useSenderText()
  const template = variation ? asTemplate(variation) : undefined
  const email = variation?.template?.email
  const single = !broadcast.test_settings?.enabled

  return (
    <Panel
      title={t`Preview`}
      extra={
        template ? (
          <TemplatePreviewDrawer record={template} workspace={workspace}>
            <Button size="small">{t`Open`}</Button>
          </TemplatePreviewDrawer>
        ) : null
      }
    >
      {variation?.template_id && !template ? (
        // Without Templates read the broadcast comes without its templates; with it, a template
        // missing from the join was deleted.
        <div className="flex flex-col gap-1">
          <Mono>{variation.template_id}</Mono>
          <span className="text-xs text-ink-3">
            {canReadTemplates
              ? t`This template no longer exists`
              : t`You need Templates read permission to see this template's name and preview`}
          </span>
        </div>
      ) : !variation || !template ? (
        <p data-unreset className="m-0 text-sm text-ink-3">{t`No template yet`}</p>
      ) : (
        <div className="flex flex-col gap-3">
          {single ? (
            <KeyValueList
              labelWidth={84}
              items={[
                {
                  label: t`From`,
                  value: (
                    <span className={resolveSenderOf(workspace, variation).missing ? 'text-danger-fg' : undefined}>
                      {senderText(workspace, variation)}
                    </span>
                  ),
                  hint: resolveSenderOf(workspace, variation).chosen ? t`chosen on the broadcast` : undefined
                },
                { label: t`Reply-To`, value: email?.reply_to },
                { label: t`Subject`, value: email?.subject },
                { label: t`Preview text`, value: email?.subject_preview }
              ]}
            />
          ) : null}
          <TemplatePreviewFrame workspace={workspace} template={template} />
        </div>
      )}
    </Panel>
  )
}

function AudienceBlock({
  workspace,
  broadcast,
  lists,
  segments
}: {
  workspace: Workspace
  broadcast: Broadcast
  lists: Pick<List, 'id' | 'name'>[]
  segments: Pick<Segment, 'id' | 'name' | 'color'>[]
}) {
  const { t } = useLingui()
  const listId = broadcast.audience?.list
  const list = listId ? lists.find((l) => l.id === listId) : undefined
  const chosen = broadcast.audience?.segments ?? []
  const schedule = broadcast.schedule
  return (
    <Panel title={t`Audience`}>
      <KeyValueList
        labelWidth={120}
        items={[
          { label: t`List`, value: listId ? (list?.name ?? t`Unknown list (${listId})`) : null },
          {
            label: t`Segments`,
            value: chosen.length ? (
              <span className="flex flex-wrap gap-1">
                {chosen.map((id) => {
                  const segment = segments.find((s) => s.id === id)
                  return (
                    <Tag key={id} variant="filled" color={segment?.color}>
                      {segment?.name ?? t`Unknown segment (${id})`}
                    </Tag>
                  )
                })}
              </span>
            ) : (
              <span className="text-ink-3">{t`None`}</span>
            )
          },
          {
            label: t`Unsubscribed`,
            value: broadcast.audience?.exclude_unsubscribed ? t`Excluded` : t`Included`
          },
          {
            label: t`Schedule timezone`,
            value: schedule?.use_recipient_timezone
              ? t`Each recipient's timezone`
              : schedule?.timezone || workspaceTimezone(workspace)
          }
        ]}
      />
    </Panel>
  )
}

function TrackingBlock({ broadcast }: { broadcast: Broadcast }) {
  const { t } = useLingui()
  const utm = broadcast.utm_parameters ?? {}
  const mono = (value?: string) => (value ? <Mono>{value}</Mono> : null)
  return (
    <Panel title={t`Web tracking`}>
      <KeyValueList
        labelWidth={120}
        items={[
          { label: 'utm_source', value: mono(utm.source) },
          { label: 'utm_medium', value: mono(utm.medium) },
          { label: 'utm_campaign', value: mono(utm.campaign) },
          { label: 'utm_term', value: mono(utm.term) },
          {
            label: 'utm_content',
            // The send stamps each variant's template id when the broadcast leaves it empty.
            value: utm.content ? <Mono>{utm.content}</Mono> : <span className="text-ink-3">{t`Template ID of each variant`}</span>
          },
          {
            label: t`Opens and clicks`,
            value: broadcast.tracking_mode === 'no_tracking' ? t`Not tracked` : t`Per contact consent`
          }
        ]}
      />
    </Panel>
  )
}

const TASK_TONE: Record<TaskStatus, PillTone> = {
  pending: 'info',
  running: 'primary',
  completed: 'success',
  failed: 'danger',
  cancelled: 'warning',
  paused: 'warning'
}

function TaskBlock({ task, timezone }: { task: Task; timezone: string }) {
  const { t } = useLingui()
  const { int, percent } = useNumberFormat()
  const dates = useBroadcastDates()
  const labels: Record<TaskStatus, string> = {
    pending: t`Pending`,
    running: t`Running`,
    completed: t`Completed`,
    failed: t`Failed`,
    cancelled: t`Cancelled`,
    paused: t`Paused`
  }
  const send = task.state?.send_broadcast
  const progress = send
    ? send.total_recipients > 0
      ? send.enqueued_count / send.total_recipients
      : null
    : task.progress > 0
      ? task.progress
      : null
  const failed = send?.failed_count ?? 0
  const items: KeyValueItem[] = [
    {
      label: t`Status`,
      value: (
        <Pill tone={TASK_TONE[task.status] ?? 'neutral'} pulse={task.status === 'running'}>
          {labels[task.status] ?? task.status}
        </Pill>
      )
    },
    {
      label: t`Next run`,
      value:
        task.next_run_after && task.status !== 'completed' ? (
          <Tooltip title={dates.dateTime(task.next_run_after, timezone)}>
            <span className="cursor-help">{dates.relative(task.next_run_after)}</span>
          </Tooltip>
        ) : null
    },
    { label: t`Progress`, value: progress === null ? null : percent(Math.min(1, progress)) },
    { label: t`Message`, value: task.state?.message },
    { label: t`Failed`, value: send ? <span className={failed > 0 ? 'text-danger-fg' : undefined}>{int(failed)}</span> : null },
    { label: t`Error`, value: task.error_message ? <span className="text-danger-fg">{task.error_message}</span> : null },
    { label: t`Task type`, value: task.type ? <Mono>{task.type}</Mono> : null }
  ]
  return (
    <Panel title={t`Sending task`}>
      <KeyValueList labelWidth={96} items={items} />
    </Panel>
  )
}

function HistoryBlock({ broadcast, timezone }: { broadcast: Broadcast; timezone: string }) {
  const { t } = useLingui()
  const dates = useBroadcastDates()
  const row = (label: string, iso?: string, hint?: string): KeyValueItem | null =>
    iso ? { label, value: dates.dateTime(iso, timezone), hint } : null
  const items = [
    row(t`Created`, broadcast.created_at),
    row(t`Started`, broadcast.started_at),
    row(t`Test sent`, broadcast.test_sent_at),
    row(t`Winner sent`, broadcast.winner_sent_at),
    row(t`Completed`, broadcast.completed_at),
    row(t`Paused`, broadcast.paused_at, broadcast.pause_reason),
    row(t`Cancelled`, broadcast.cancelled_at)
  ].filter((item): item is KeyValueItem => item !== null)
  return (
    <Panel title={t`History`}>
      <KeyValueList labelWidth={96} items={items} />
    </Panel>
  )
}
