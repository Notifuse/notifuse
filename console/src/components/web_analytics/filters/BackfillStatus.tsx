import type { ReactNode } from 'react'
import { App, Alert, Button, Popconfirm, Progress, Space, Tooltip, Typography } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { PlayCircleOutlined, StopOutlined, SyncOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { webAnalyticsService } from '../../../services/api/web_analytics'
import { useNumberFormat } from '../../layout'
import { describeBackfill } from '../lib/backfill'
import { formatPercent } from '../lib/format'
import { colors } from '../../../theme/tokens'
import { BACKFILL_QUERY_KEY, useBackfillStatus } from './useBackfillStatus'
import { useWebAnalyticsWriteReason } from './useWebAnalyticsWriteReason'

const { Text } = Typography

export interface BackfillStatusProps {
  workspaceId: string
  /** Version the server recomputed from the currently saved rules. */
  filtersVersion?: string
  /** The banner is pointless on a workspace that has no rules to apply. */
  hasRules: boolean
  /**
   * Starting and cancelling a run need the web analytics write permission: without it the buttons stay,
   * disabled and saying why, and the progress still shows.
   */
  readOnly?: boolean
}

/**
 * Rule edits only affect new traffic; historical sessions and goals are
 * rewritten by a backfill task. This surfaces the drift between the saved
 * rules and the last completed run, then drives the task from the same panel.
 */
export function BackfillStatus(props: BackfillStatusProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { workspaceId, filtersVersion, hasRules, readOnly = false } = props
  const noWrite = useWebAnalyticsWriteReason()

  /** A disabled button gets no pointer events: the span carries the reason. */
  const withoutWrite = (button: ReactNode) => (
    <Tooltip title={noWrite}>
      <span>{button}</span>
    </Tooltip>
  )

  const { data: status, isLoading } = useBackfillStatus(workspaceId)

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: [BACKFILL_QUERY_KEY, workspaceId] })

  const startMutation = useMutation({
    mutationFn: () => webAnalyticsService.backfillStart(workspaceId),
    onSuccess: async () => {
      await invalidate()
      message.success(t`Backfill started`)
    },
    onError: (error: Error) => message.error(error.message)
  })

  const cancelMutation = useMutation({
    mutationFn: () => webAnalyticsService.backfillCancel(workspaceId),
    onSuccess: async () => {
      await invalidate()
      message.info(t`Backfill cancelled`)
    },
    onError: (error: Error) => message.error(error.message)
  })

  if (isLoading) return null

  const backfill = describeBackfill(status)

  if (status && backfill.state === 'active') {
    const state = status.state
    const partitionTotal = state?.partitions?.length ?? 0
    return (
      <div className="mb-4 rounded-lg border border-blue-200 bg-blue-50 p-4">
        <div className="mb-2 flex items-center justify-between">
          <Space>
            <SyncOutlined spin className="text-blue-500" />
            <Text strong>
              {status.status === 'pending' ? t`Preparing backfill...` : t`Backfill in progress`}
            </Text>
          </Space>
          {readOnly ? (
            withoutWrite(
              <Button size="small" icon={<StopOutlined />} disabled>
                {t`Cancel`}
              </Button>
            )
          ) : (
            <Popconfirm
              title={t`Cancel backfill?`}
              description={t`Partitions already rewritten keep their new values.`}
              onConfirm={() => cancelMutation.mutate()}
              okText={t`Yes, cancel`}
              cancelText={t`No`}
            >
              <Button size="small" icon={<StopOutlined />} loading={cancelMutation.isPending}>
                {t`Cancel`}
              </Button>
            </Popconfirm>
          )}
        </div>

        <Progress
          percent={backfill.progress ?? 0}
          // antd writes "42%" in English otherwise
          format={(percent) => formatPercent(percent ?? 0, 0)}
          status="active"
          strokeColor={{ from: colors.status.info, to: colors.status.success }}
        />

        {state ? (
          <div className="mt-2 text-sm text-ink-3">
            <Space separator="·" wrap>
              {partitionTotal > 0 ? (
                <span>{t`Partition ${state.partition_index} of ${partitionTotal}`}</span>
              ) : null}
              <span>{t`${int(state.rows_updated)} rows rewritten`}</span>
            </Space>
          </div>
        ) : null}
      </div>
    )
  }

  // A cancelled run is recorded as failed, but nothing broke: it is offered again as a plain backfill.
  const cancelled = backfill.state === 'cancelled'

  if (status && backfill.state === 'failed') {
    return (
      <Alert
        type="error"
        showIcon
        className="!mb-4"
        title={t`The last backfill failed`}
        description={
          <div className="mt-2 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
            <Text type="secondary">{status.error_message || t`No error message was recorded.`}</Text>
            {readOnly ? (
              withoutWrite(
                <Button type="primary" icon={<PlayCircleOutlined />} disabled>
                  {t`Run backfill again`}
                </Button>
              )
            ) : (
              <Button
                type="primary"
                icon={<PlayCircleOutlined />}
                loading={startMutation.isPending}
                onClick={() => startMutation.mutate()}
              >
                {t`Run backfill again`}
              </Button>
            )}
          </div>
        }
      />
    )
  }

  // Only a completed run proves which rule set the history was written with.
  const lastCompletedVersion =
    backfill.state === 'completed' ? status?.state?.filters_version : undefined
  const neverRan = backfill.state === 'never'
  const needsBackfill = hasRules && (neverRan || lastCompletedVersion !== filtersVersion)

  if (!needsBackfill) return null

  return (
    <Alert
      type="info"
      className="!mb-4"
      title={t`Rule changes only apply to new traffic`}
      description={
        <div className="mt-2 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <Text type="secondary">
            {neverRan
              ? t`No backfill has run yet. Run one to apply these rules to historical sessions and goals.`
              : cancelled
                ? t`The last backfill was cancelled. Run one to apply these rules to all historical sessions and goals.`
                : t`Historical sessions and goals were written with a different rule set.`}
          </Text>
          {readOnly ? (
            withoutWrite(
              <Button type="primary" size="small" icon={<PlayCircleOutlined />} disabled>
                {t`Run backfill`}
              </Button>
            )
          ) : (
            <Button
              type="primary"
              size="small"
              icon={<PlayCircleOutlined />}
              loading={startMutation.isPending}
              onClick={() => startMutation.mutate()}
            >
              {t`Run backfill`}
            </Button>
          )}
        </div>
      }
    />
  )
}
