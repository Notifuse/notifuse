import { Fragment, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { App, Button, Popconfirm, Tooltip } from 'antd'
import { Pencil, RefreshCw } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../lib/dayjs'
import { rebuildSegment, type Segment } from '../../services/api/segment'
import type { List } from '../../services/api/types'
import { DrawerSegment } from '../segment/button_upsert'
import { segmentSummary } from '../segment/segmentSummary'
import { SegmentBuildingIndicator } from './SegmentProgress'

export interface SegmentDefinedByProps {
  workspaceId: string
  segment: Segment
  lists: List[]
  customFieldLabels: Record<string, string>
  /** Segments write: Recompute and Edit segment. */
  canWrite: boolean
  /** Templates read, which the segment drawer needs to list the templates. */
  canReadTemplates: boolean
}

/** The first conditions of a segment, when it last changed, and with segments write Recompute and Edit. */
export function SegmentDefinedBy({
  workspaceId,
  segment,
  lists,
  customFieldLabels,
  canWrite,
  canReadTemplates
}: SegmentDefinedByProps) {
  const { t, i18n } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const invalidateSegments = () => queryClient.invalidateQueries({ queryKey: ['segments', workspaceId] })
  const rebuild = useMutation({
    mutationFn: () => rebuildSegment({ workspace_id: workspaceId, segment_id: segment.id }),
    onSuccess: (data) => {
      message.success(data.message || t`Segment rebuild started successfully`)
      invalidateSegments()
    },
    onError: (error: Error) => message.error(error?.message || t`Failed to rebuild segment`)
  })
  const summary = segmentSummary(segment.tree, i18n, {
    listName: (id) => lists.find((list) => list.id === id)?.name ?? id,
    customFieldLabels
  })
  const updated = dayjs(segment.db_updated_at).tz(segment.timezone || 'UTC')
  // db_updated_at also moves on a rename, a colour or a schedule change: "Updated", not
  // "Recomputed".
  const updatedAgo = updated.fromNow()

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line bg-surface px-3 py-2 text-sm">
      <span className="text-ink-3">{t`Defined by`}</span>
      {summary.conditions.map((condition, index) => (
        <Fragment key={index}>
          {index > 0 ? <span className="text-ink-3">{summary.joiner}</span> : null}
          <span className="rounded-md border border-line bg-subtle px-1.5 text-xs leading-5 text-ink-2">
            {condition}
          </span>
        </Fragment>
      ))}
      {summary.more > 0 ? <span className="text-xs text-ink-3">{t`+${summary.more}`}</span> : null}
      <span className="ml-auto flex items-center gap-2 text-xs text-ink-3">
        {segment.status === 'building' ? (
          <SegmentBuildingIndicator workspaceId={workspaceId} segment={segment} />
        ) : null}
        <Tooltip title={updated.format('LLL')}>
          <span>{t`Updated ${updatedAgo}`}</span>
        </Tooltip>
      </span>
      {canWrite ? (
        <span className="flex items-center gap-1">
          <Popconfirm
            title={t`Rebuild segment`}
            description={t`Are you sure you want to rebuild "${segment.name}"? This will recalculate segment membership.`}
            okText={t`Yes`}
            cancelText={t`No`}
            onConfirm={() => rebuild.mutate()}
          >
            <Button
              size="small"
              type="text"
              icon={<RefreshCw size={13} aria-hidden="true" />}
              loading={rebuild.isPending}
            >
              {t`Recompute`}
            </Button>
          </Popconfirm>
          <Button size="small" icon={<Pencil size={13} aria-hidden="true" />} onClick={() => setEditing(true)}>
            {t`Edit segment`}
          </Button>
        </span>
      ) : null}
      {editing ? (
        <DrawerSegment
          segment={segment}
          canReadTemplates={canReadTemplates}
          setDrawserVisible={setEditing}
          onSuccess={invalidateSegments}
        />
      ) : null}
    </div>
  )
}
