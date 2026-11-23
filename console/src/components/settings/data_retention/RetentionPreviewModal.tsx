import { useLingui } from '@lingui/react/macro'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Alert, App, Button, Modal, Skeleton, Table, Typography } from 'antd'
import {
  dataRetentionApi,
  type DataRetentionPolicy,
  type RetentionCategory,
  type RetentionCategoryResult,
  type RetentionCategorySpec
} from '../../../services/api/dataRetention'
import { useNumberFormat } from '../../layout/format'
import { CATEGORY_LABELS, SKIP_REASONS } from './categoryText'

const { Text } = Typography

interface RetentionPreviewModalProps {
  workspaceId: string
  policy: DataRetentionPolicy
  inForce: Record<RetentionCategory, number>
  /** The workspace's policy in force: what the change's exemptions and events are compared with. */
  currentPolicy?: DataRetentionPolicy
  categories: RetentionCategorySpec[]
  formatPeriod: (days: number) => string
  onClose: () => void
  onSaved: () => void | Promise<void>
}

interface PreviewRow {
  category: RetentionCategory
  result: RetentionCategoryResult
}

// What a policy removes, before it is saved: the counts of its first run, and when it starts.
export function RetentionPreviewModal({
  workspaceId,
  policy,
  inForce,
  currentPolicy,
  categories,
  formatPeriod,
  onClose,
  onSaved
}: RetentionPreviewModalProps) {
  const { t, i18n } = useLingui()
  const { message } = App.useApp()
  const { int: number } = useNumberFormat()
  const preview = useQuery({
    queryKey: ['data-retention-preview', workspaceId, JSON.stringify(policy)],
    queryFn: () => dataRetentionApi.preview(workspaceId, policy),
    gcTime: 0
  })
  const save = useMutation({
    mutationFn: () => dataRetentionApi.set(workspaceId, policy),
    onSuccess: async (response) => {
      message.success(
        response.data_retention.pending
          ? t`Saved. The change that deletes more takes effect in 7 days.`
          : t`Data retention policy saved`
      )
      await onSaved()
    },
    onError: (err: Error) => message.error(err.message)
  })

  const startsLabel = (category: RetentionCategory, days: number) => {
    const pendingAt = preview.data?.pending_at
    const lost = (next?: string[], current?: string[]) => (current ?? []).some((value) => !(next ?? []).includes(value))
    const contactsLoseProtection =
      category === 'inactive_contacts' &&
      (lost(policy.exempt_list_ids, currentPolicy?.exempt_list_ids) ||
        lost(policy.exempt_segment_ids, currentPolicy?.exempt_segment_ids) ||
        lost(policy.engagement_event_names, currentPolicy?.engagement_event_names))
    const waits =
      preview.data?.pending &&
      pendingAt &&
      days > 0 &&
      (inForce[category] === 0 || days < inForce[category] || contactsLoseProtection)
    return waits ? new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium' }).format(new Date(pendingAt)) : t`At the next run`
  }

  // Categories that are on, or that change: one left off and unchanged says nothing.
  const rows: PreviewRow[] = categories
    .map((spec) => ({ category: spec.category, result: preview.data?.categories[spec.category] }))
    .filter((row): row is PreviewRow => !!row.result && (row.result.days > 0 || inForce[row.category] > 0))

  const columns = [
    {
      title: t`Data`,
      key: 'data',
      render: (_: unknown, row: PreviewRow) => (
        <div>
          <div className="font-medium">{i18n._(CATEGORY_LABELS[row.category])}</div>
          <Text type="secondary" className="text-xs">
            {inForce[row.category] === row.result.days
              ? formatPeriod(row.result.days)
              : `${formatPeriod(inForce[row.category])} → ${formatPeriod(row.result.days)}`}
          </Text>
        </div>
      )
    },
    {
      title: t`First run removes`,
      key: 'removes',
      render: (_: unknown, row: PreviewRow) => {
        if (row.result.skipped) return <Text type="secondary">{SKIP_REASONS[row.result.skipped] ? i18n._(SKIP_REASONS[row.result.skipped]) : row.result.skipped}</Text>
        return (
          <div>
            <div>{number(row.result.affected)}</div>
            {row.result.opt_outs_kept ? (
              <Text type="secondary" className="text-xs">
                {t`${number(row.result.opt_outs_kept)} unsubscribes kept as hashes`}
              </Text>
            ) : null}
          </div>
        )
      }
    },
    {
      title: t`Starts`,
      key: 'starts',
      render: (_: unknown, row: PreviewRow) => (row.result.days > 0 ? startsLabel(row.category, row.result.days) : t`At once`)
    }
  ]

  return (
    <Modal
      open
      title={t`What this policy removes`}
      onCancel={onClose}
      width={640}
      footer={[
        <Button key="back" onClick={onClose}>
          {t`Back to the settings`}
        </Button>,
        <Button key="save" type="primary" loading={save.isPending} disabled={!preview.data} onClick={() => save.mutate()}>
          {t`Save the policy`}
        </Button>
      ]}
    >
      {preview.isLoading ? (
        <Skeleton active />
      ) : preview.error ? (
        <Alert type="error" showIcon message={(preview.error as Error).message} />
      ) : (
        <div className="flex flex-col gap-3">
          <Text type="secondary">{t`Counts are for today; they grow as data ages.`}</Text>
          <Table<PreviewRow> rowKey="category" size="small" pagination={false} columns={columns} dataSource={rows} />
          {preview.data?.pending && preview.data.pending_at ? (
            <Alert
              type="warning"
              showIcon
              message={t`This deletes more than the policy in force, so that part starts on ${new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium' }).format(new Date(preview.data.pending_at))}. The owners get an email and can cancel it until then. The rest applies at the next run.`}
            />
          ) : null}
          <Text type="secondary" className="text-xs">
            {t`Deleted data stays in your database backups until they expire.`}
          </Text>
        </div>
      )}
    </Modal>
  )
}
