import { useMemo, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { useBrandName } from '../../../hooks/useBrandName'
import { plural } from '@lingui/core/macro'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, App, Button, Select, Skeleton, Table, Tag, Typography } from 'antd'
import { Download } from 'lucide-react'
import {
  dataRetentionApi,
  dataRetentionQueryKey,
  type DataRetentionPolicy,
  type DataRetentionView,
  type RetentionCategory,
  type RetentionCategorySpec,
  type RetentionRun
} from '../../../services/api/dataRetention'
import { listsApi } from '../../../services/api/list'
import { listSegments } from '../../../services/api/segment'
import { downloadBlob } from '../../../lib/download'
import { Panel } from '../../layout'
import { useLicense } from '../../../hooks/useLicense'
import { LicenceGateNotice } from '../../license/LicenceGateNotice'
import { useNumberFormat } from '../../layout/format'
import { SettingsSaveBar } from '../SettingsSaveBar'
import { SettingsSectionHeader } from '../SettingsSectionHeader'
import { CATEGORY_DESCRIPTIONS, CATEGORY_LABELS, SKIP_REASONS } from './categoryText'
import { fromDays } from './periods'
import { PeriodInput } from './PeriodInput'
import { RetentionPreviewModal } from './RetentionPreviewModal'

const { Text } = Typography

interface DataRetentionSettingsProps {
  workspaceId: string
  /** Only owners change the policy; every member reads it. */
  isOwner: boolean
}

/** The policy the form starts from: the change that waits, if any, else the one in force. */
function startingPolicy(view: DataRetentionView): DataRetentionPolicy {
  const source = view.pending?.policy ?? view.policy
  return {
    categories: { ...(source?.categories ?? {}) },
    exempt_list_ids: [...(source?.exempt_list_ids ?? [])],
    exempt_segment_ids: [...(source?.exempt_segment_ids ?? [])],
    engagement_event_names: [...(source?.engagement_event_names ?? [])]
  }
}

function normalized(policy: DataRetentionPolicy): string {
  const sorted = (values?: string[]) => [...(values ?? [])].sort()
  const categories = Object.fromEntries(Object.entries(policy.categories).sort(([a], [b]) => a.localeCompare(b)))
  return JSON.stringify({
    categories,
    exempt_list_ids: sorted(policy.exempt_list_ids),
    exempt_segment_ids: sorted(policy.exempt_segment_ids),
    engagement_event_names: sorted(policy.engagement_event_names)
  })
}

export function DataRetentionSettings({ workspaceId, isOwner }: DataRetentionSettingsProps) {
  const { t } = useLingui()
  const { data: view, isLoading, error } = useQuery({
    queryKey: dataRetentionQueryKey(workspaceId),
    queryFn: () => dataRetentionApi.get(workspaceId)
  })

  const header = (
    <SettingsSectionHeader
      title={t`Data retention`}
      description={t`How long this workspace keeps each kind of data.`}
    />
  )

  if (isLoading) {
    return (
      <>
        {header}
        <Skeleton active />
      </>
    )
  }
  if (error || !view) {
    return (
      <>
        {header}
        <Alert type="error" showIcon message={t`Could not load the data retention settings`} />
      </>
    )
  }
  // Keyed on what was loaded, so a save or a cancel starts the form again from the server's
  // answer instead of syncing state in an effect.
  return (
    <>
      {header}
      <DataRetentionForm
        key={`${view.pending?.effective_at ?? ''}|${normalized(startingPolicy(view))}`}
        workspaceId={workspaceId}
        isOwner={isOwner}
        view={view}
      />
    </>
  )
}

interface FormProps {
  workspaceId: string
  isOwner: boolean
  view: DataRetentionView
}

function DataRetentionForm({ workspaceId, isOwner, view }: FormProps) {
  const { t, i18n } = useLingui()
  const brand = useBrandName()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { int: number } = useNumberFormat()
  const initial = useMemo(() => startingPolicy(view), [view])
  const [draft, setDraft] = useState<DataRetentionPolicy>(initial)
  const [previewOpen, setPreviewOpen] = useState(false)
  const dirty = normalized(draft) !== normalized(initial)
  // Without a licence the backend accepts only a save that deletes less of what is set up:
  // categories switched off, exemptions removed (G7). The controls follow the same rule.
  const { has } = useLicense()
  const locked = !has('data_retention')

  const lists = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId })
  })
  const segments = useQuery({
    queryKey: ['segments', workspaceId],
    queryFn: () => listSegments({ workspace_id: workspaceId })
  })
  const backfillTask = useQuery({
    queryKey: ['data-retention-backfill', workspaceId],
    queryFn: () => dataRetentionApi.backfillTask(workspaceId),
    enabled: !view.backfill_ready
  })

  const refresh = () => queryClient.invalidateQueries({ queryKey: dataRetentionQueryKey(workspaceId) })

  const cancelPending = useMutation({
    mutationFn: () => dataRetentionApi.cancelPending(workspaceId),
    onSuccess: async () => {
      message.success(t`The change was cancelled`)
      await refresh()
    },
    onError: (err: Error) => message.error(err.message)
  })
  const retryBackfill = useMutation({
    mutationFn: () => dataRetentionApi.retryBackfill(workspaceId),
    onSuccess: async () => {
      message.success(t`Reading past activity again`)
      await queryClient.invalidateQueries({ queryKey: ['data-retention-backfill', workspaceId] })
    },
    onError: (err: Error) => message.error(err.message)
  })

  const formatDate = (value: string) => new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium' }).format(new Date(value))
  const formatDateTime = (value: string) =>
    new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
  const formatPeriod = (days: number) => {
    if (days === 0) return t`Off`
    const { value, unit } = fromDays(days)
    if (unit === 'years') return plural(value, { one: '# year', other: '# years' })
    if (unit === 'months') return plural(value, { one: '# month', other: '# months' })
    return plural(value, { one: '# day', other: '# days' })
  }

  const effectiveDays = (category: RetentionCategory) => draft.categories[category] ?? view.defaults[category] ?? 0
  const setDays = (category: RetentionCategory, days: number | undefined) =>
    setDraft((current) => {
      const categories = { ...current.categories }
      if (days === undefined) delete categories[category]
      else categories[category] = days
      return { ...current, categories }
    })

  const tooShort = view.categories.filter((spec) => {
    const days = effectiveDays(spec.category)
    return days > 0 && days < spec.min_days
  })

  const periodsColumns = [
    {
      title: t`Data`,
      key: 'data',
      render: (_: unknown, spec: RetentionCategorySpec) => (
        <div className="max-w-md">
          <div className="font-medium">{i18n._(CATEGORY_LABELS[spec.category])}</div>
          <Text type="secondary" className="text-xs leading-snug">
            {i18n._(CATEGORY_DESCRIPTIONS[spec.category])}
          </Text>
        </div>
      )
    },
    {
      title: t`Keep for`,
      key: 'period',
      render: (_: unknown, spec: RetentionCategorySpec) => {
        const days = effectiveDays(spec.category)
        const explicit = draft.categories[spec.category] !== undefined
        const label = i18n._(CATEGORY_LABELS[spec.category])
        return (
          <div className="flex flex-col gap-1">
            <PeriodInput
              days={days}
              onDays={spec.suggested_days}
              label={label}
              disabled={!isOwner}
              offOnly={locked && spec.default_days === 0}
              onChange={(next) => setDays(spec.category, next)}
            />
            {!explicit ? (
              <Text type="secondary" className="text-xs">
                {t`Deployment default`}
              </Text>
            ) : isOwner && !locked && draft.categories[spec.category] !== view.defaults[spec.category] ? (
              <span className="self-start">
                <Button type="link" size="small" onClick={() => setDays(spec.category, undefined)}>
                  {t`Use the default (${formatPeriod(view.defaults[spec.category] ?? 0)})`}
                </Button>
              </span>
            ) : null}
            {days > 0 && days < spec.min_days ? (
              <Text type="danger" className="text-xs">
                {t`At least ${formatPeriod(spec.min_days)}`}
              </Text>
            ) : null}
          </div>
        )
      }
    },
    {
      title: t`Shortest`,
      key: 'min',
      render: (_: unknown, spec: RetentionCategorySpec) => <Text type="secondary">{formatPeriod(spec.min_days)}</Text>
    },
    {
      title: t`Suggested`,
      key: 'suggested',
      render: (_: unknown, spec: RetentionCategorySpec) => <Text type="secondary">{formatPeriod(spec.suggested_days)}</Text>
    }
  ]

  const runColumns = [
    {
      title: t`Run`,
      key: 'started',
      render: (_: unknown, run: RetentionRun) => formatDateTime(run.started_at)
    },
    {
      title: t`Status`,
      key: 'status',
      render: (_: unknown, run: RetentionRun) => {
        const tones: Record<RetentionRun['status'], { color: string; label: string }> = {
          running: { color: 'processing', label: t`Running` },
          completed: { color: 'success', label: t`Completed` },
          partial: { color: 'warning', label: t`Partial` },
          failed: { color: 'error', label: t`Failed` }
        }
        const tone = tones[run.status]
        return <Tag color={tone.color}>{tone.label}</Tag>
      }
    },
    {
      title: t`What it removed`,
      key: 'results',
      render: (_: unknown, run: RetentionRun) => {
        const lines = view.categories
          .map((spec) => ({ spec, result: run.results[spec.category] }))
          .filter(({ result }) => result && (result.affected > 0 || result.error || result.remaining || (result.skipped && result.skipped !== 'off')))
        if (lines.length === 0) return <Text type="secondary">{t`Nothing`}</Text>
        return (
          <ul className="m-0 list-none p-0 text-xs leading-relaxed">
            {lines.map(({ spec, result }) => (
              <li key={spec.category}>
                <span className="font-medium">{i18n._(CATEGORY_LABELS[spec.category])}</span>
                {': '}
                {number(result?.affected ?? 0)}
                {result?.opt_outs_kept ? ` · ${t`${number(result.opt_outs_kept)} unsubscribes kept as hashes`}` : ''}
                {result?.remaining ? ` · ${t`more at the next run`}` : ''}
                {result?.skipped && SKIP_REASONS[result.skipped] ? ` · ${i18n._(SKIP_REASONS[result.skipped])}` : ''}
                {result?.error ? <Text type="danger">{` · ${result.error}`}</Text> : null}
              </li>
            ))}
          </ul>
        )
      }
    }
  ]

  const exportRuns = async () => {
    try {
      downloadBlob(await dataRetentionApi.exportRuns(workspaceId), 'data-retention-runs.csv')
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`The export failed`)
    }
  }

  const listOptions = (lists.data?.lists ?? []).map((list) => ({ value: list.id, label: list.name }))
  const segmentOptions = (segments.data?.segments ?? []).map((segment) => ({ value: segment.id, label: segment.name }))

  // What waits: the workspace's own change, or a change of the deployment defaults it inherits.
  // Periods, and the exemptions and engagement events the change removes: each deletes more.
  const removed = (next?: string[], current?: string[]) => (current ?? []).filter((value) => !(next ?? []).includes(value))
  const listName = (id: string) => listOptions.find((option) => option.value === id)?.label ?? id
  const segmentName = (id: string) => segmentOptions.find((option) => option.value === id)?.label ?? id
  const waiting = view.pending_at
    ? [
        ...view.categories
          .filter((spec) => view.pending_days && view.pending_days[spec.category] !== view.in_force[spec.category])
          .map(
            (spec) =>
              `${i18n._(CATEGORY_LABELS[spec.category])}: ${formatPeriod(view.in_force[spec.category])} → ${formatPeriod(view.pending_days?.[spec.category] ?? 0)}`
          ),
        ...removed(view.pending?.policy.exempt_list_ids, view.policy?.exempt_list_ids).map(
          (id) => t`No longer exempt: the list ${listName(id)}`
        ),
        ...removed(view.pending?.policy.exempt_segment_ids, view.policy?.exempt_segment_ids).map(
          (id) => t`No longer exempt: the segment ${segmentName(id)}`
        ),
        ...removed(view.pending?.policy.engagement_event_names, view.policy?.engagement_event_names).map(
          (name) => t`No longer counts as activity: ${name}`
        )
      ]
    : []

  const task = backfillTask.data

  return (
    <div className="flex flex-col gap-5">
      <LicenceGateNotice feature="data_retention" workspaceId={workspaceId} className="" />
      {view.pending_at ? (
        <Alert
          type="warning"
          showIcon
          message={t`A change that deletes more data takes effect on ${formatDate(view.pending_at)}`}
          description={
            <div className="flex flex-col gap-1">
              <span>{waiting.join(' · ')}</span>
              {view.pending ? (
                <span>
                  {view.pending.requested_by
                    ? t`Requested by ${view.pending.requested_by}. The owners were told by email.`
                    : t`The owners were told by email.`}
                </span>
              ) : (
                <span>{t`This comes from the deployment defaults, set by the administrator of this installation.`}</span>
              )}
            </div>
          }
          action={
            view.pending && isOwner ? (
              <Button size="small" danger loading={cancelPending.isPending} onClick={() => cancelPending.mutate()}>
                {t`Cancel the change`}
              </Button>
            ) : undefined
          }
        />
      ) : null}

      <Panel title={t`Periods`}>
        <Text type="secondary" className="mb-3 block">
          {t`Every day at 05:00 UTC, ${brand} deletes or anonymizes what is older than its period. Off keeps everything. A change that deletes more waits 7 days, so it can still be cancelled. Suggested values follow the CNIL's guidance.`}
        </Text>
        <Table<RetentionCategorySpec>
          rowKey="category"
          size="small"
          pagination={false}
          columns={periodsColumns}
          dataSource={view.categories}
          scroll={{ x: true }}
        />
      </Panel>

      <Panel title={t`Inactive contacts`}>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <Text strong>{t`Never delete members of these lists`}</Text>
            <Select
              mode="multiple"
              disabled={!isOwner}
              aria-label={t`Exempt lists`}
              placeholder={t`No list`}
              value={draft.exempt_list_ids}
              options={locked ? listOptions.filter((o) => initial.exempt_list_ids?.includes(o.value)) : listOptions}
              onChange={(ids: string[]) => setDraft((current) => ({ ...current, exempt_list_ids: ids }))}
            />
          </div>
          <div className="flex flex-col gap-1">
            <Text strong>{t`Never delete members of these segments`}</Text>
            <Select
              mode="multiple"
              disabled={!isOwner}
              aria-label={t`Exempt segments`}
              placeholder={t`No segment`}
              value={draft.exempt_segment_ids}
              options={locked ? segmentOptions.filter((o) => initial.exempt_segment_ids?.includes(o.value)) : segmentOptions}
              onChange={(ids: string[]) => setDraft((current) => ({ ...current, exempt_segment_ids: ids }))}
            />
            <Text type="secondary" className="text-xs">
              {t`If one of them is deleted or being rebuilt, no contact is deleted until it is back.`}
            </Text>
          </div>
          <div className="flex flex-col gap-1">
            <Text strong>{t`Custom events that count as activity`}</Text>
            <Select
              mode="tags"
              disabled={!isOwner || locked}
              aria-label={t`Custom events that count as activity`}
              placeholder={t`None`}
              value={draft.engagement_event_names}
              options={view.event_names.map((name) => ({ value: name, label: name }))}
              onChange={(names: string[]) => setDraft((current) => ({ ...current, engagement_event_names: names }))}
            />
            <Text type="secondary" className="text-xs">
              {t`An open never counts. Clicks, replies, confirmations, a contact's own subscriptions and identified visits always do.`}
            </Text>
          </div>
          {view.backfill_ready ? (
            <Text type="success" className="text-xs">
              {t`Past activity has been read from history: contacts can be deleted as inactive.`}
            </Text>
          ) : task?.status === 'failed' ? (
            <Alert
              type="error"
              showIcon
              message={t`Reading past activity from history failed. Until it succeeds, no contact is deleted as inactive.`}
              description={task.error_message}
              action={
                isOwner ? (
                  <Button size="small" loading={retryBackfill.isPending} onClick={() => retryBackfill.mutate()}>
                    {t`Run it again`}
                  </Button>
                ) : undefined
              }
            />
          ) : (
            <Alert
              type="info"
              showIcon
              message={t`${brand} is reading past activity from history. Until it finishes, no contact is deleted as inactive.`}
            />
          )}
        </div>
      </Panel>

      <Panel
        title={t`Runs`}
        flush
        extra={
          <Button size="small" icon={<Download size={14} />} onClick={exportRuns} disabled={view.runs.length === 0}>
            {t`Export CSV`}
          </Button>
        }
      >
        <Table<RetentionRun>
          rowKey="id"
          size="small"
          pagination={false}
          columns={runColumns}
          dataSource={view.runs}
          locale={{ emptyText: t`No run yet. The first one starts at 05:00 UTC.` }}
          scroll={{ x: true }}
        />
      </Panel>

      {isOwner ? (
        <SettingsSaveBar
          dirty={dirty}
          saving={false}
          saveLabel={t`Preview and save`}
          saveDisabled={tooShort.length > 0}
          saveDisabledReason={t`A period is shorter than its category allows`}
          onSave={() => setPreviewOpen(true)}
          onDiscard={() => setDraft(initial)}
          leaveWarning={t`You have unsaved data retention changes. Leave without saving?`}
        />
      ) : null}

      {previewOpen ? (
        <RetentionPreviewModal
          workspaceId={workspaceId}
          policy={draft}
          inForce={view.in_force}
          currentPolicy={view.policy}
          categories={view.categories}
          formatPeriod={formatPeriod}
          onClose={() => setPreviewOpen(false)}
          onSaved={async () => {
            setPreviewOpen(false)
            await refresh()
          }}
        />
      ) : null}
    </div>
  )
}
