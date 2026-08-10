import { App, Alert, Button, Popconfirm, Progress, Space, Typography } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PlayCircleOutlined, StopOutlined, SyncOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { webAnalyticsService } from '../../../services/api/web_analytics'

const { Text } = Typography

export interface BackfillStatusProps {
  workspaceId: string
  /** Version the server recomputed from the currently saved rules. */
  filtersVersion?: string
  /** The banner is pointless on a workspace that has no rules to apply. */
  hasRules: boolean
}

export const BACKFILL_QUERY_KEY = 'web-analytics-backfill'

/**
 * Rule edits only affect new traffic; historical sessions and goals are
 * rewritten by a backfill task. This surfaces the drift between the saved
 * rules and the last completed run, then drives the task from the same panel.
 */
export function BackfillStatus(props: BackfillStatusProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { workspaceId, filtersVersion, hasRules } = props

  const { data: status, isLoading } = useQuery({
    queryKey: [BACKFILL_QUERY_KEY, workspaceId],
    queryFn: () => webAnalyticsService.backfillStatus(workspaceId),
    refetchInterval: (query) => {
      const current = query.state.data?.status
      // A long backfill parks in "paused" between runtime slices, so it has to
      // keep polling or the progress bar freezes mid-run.
      return current === 'pending' || current === 'running' || current === 'paused' ? 2000 : false
    }
  })

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

  const isActive =
    status?.status === 'pending' || status?.status === 'running' || status?.status === 'paused'

  if (isActive) {
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
        </div>

        <Progress
          percent={Math.round(status.progress ?? 0)}
          status="active"
          strokeColor={{ from: '#1890ff', to: '#52c41a' }}
        />

        {state ? (
          <div className="mt-2 text-sm text-gray-500">
            <Space separator="·" wrap>
              {partitionTotal > 0 ? (
                <span>{t`Partition ${state.partition_index} of ${partitionTotal}`}</span>
              ) : null}
              <span>{t`${state.rows_updated.toLocaleString()} rows rewritten`}</span>
            </Space>
          </div>
        ) : null}
      </div>
    )
  }

  if (status?.status === 'failed') {
    return (
      <Alert
        type="error"
        showIcon
        className="!mb-4"
        title={t`The last backfill failed`}
        description={
          <div className="mt-2 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
            <Text type="secondary">{status.error_message || t`No error message was recorded.`}</Text>
            <Button
              type="primary"
              icon={<PlayCircleOutlined />}
              loading={startMutation.isPending}
              onClick={() => startMutation.mutate()}
            >
              {t`Run backfill again`}
            </Button>
          </div>
        }
      />
    )
  }

  // Only a completed run proves which rule set the history was written with.
  const lastCompletedVersion =
    status?.status === 'completed' ? status.state?.filters_version : undefined
  const neverRan = !status
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
              : t`Historical sessions and goals were written with a different rule set.`}
          </Text>
          <Button
            type="primary"
            size="small"
            icon={<PlayCircleOutlined />}
            loading={startMutation.isPending}
            onClick={() => startMutation.mutate()}
          >
            {t`Run backfill`}
          </Button>
        </div>
      }
    />
  )
}
