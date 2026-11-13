import { useQuery } from '@tanstack/react-query'
import { Progress, Tooltip } from 'antd'
import { Loader2 } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { Segment } from '../../services/api/segment'
import { taskApi } from '../../services/api/task'
import { useNumberFormat } from '../layout'

/** The build task of a segment being computed, read again every 15 seconds. */
export function useSegmentBuildTask(workspaceId: string, segment: Pick<Segment, 'id' | 'status'>) {
  const building = segment.status === 'building'
  return useQuery({
    queryKey: ['segment-task', workspaceId, segment.id],
    queryFn: () => taskApi.findBySegmentId(workspaceId, segment.id),
    enabled: building,
    refetchInterval: building ? 15000 : false
  })
}

/** Percentage, contacts processed, matched and total of a segment's computation. */
function SegmentProgressContent({ workspaceId, segment }: { workspaceId: string; segment: Segment }) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const { data: task } = useSegmentBuildTask(workspaceId, segment)
  const build = task?.state?.build_segment
  if (!build) return <span>{t`Processing contacts`}</span>
  return (
    <div className="flex min-w-[180px] flex-col gap-1 text-xs">
      <Progress percent={Math.round(task?.progress || 0)} size="small" />
      <div>
        {t`Processed`}: {int(build.processed_count)}
      </div>
      <div>
        {t`Matched`}: {int(build.matched_count)}
      </div>
      {build.total_contacts > 0 ? (
        <div>
          {t`Total`}: {int(build.total_contacts)}
        </div>
      ) : null}
    </div>
  )
}

/** A spinning icon whose tooltip tells how far the segment's computation is. */
export function SegmentBuildingIndicator({
  workspaceId,
  segment
}: {
  workspaceId: string
  segment: Segment
}) {
  const { t } = useLingui()
  return (
    <Tooltip
      title={
        <div className="flex flex-col gap-1">
          <span className="font-medium">{t`Computing this segment…`}</span>
          <SegmentProgressContent workspaceId={workspaceId} segment={segment} />
        </div>
      }
    >
      <span
        data-segment-building
        role="img"
        aria-label={t`Computing this segment…`}
        className="flex text-primary-500"
      >
        <Loader2 size={13} className="motion-safe:animate-spin" aria-hidden="true" />
      </span>
    </Tooltip>
  )
}
