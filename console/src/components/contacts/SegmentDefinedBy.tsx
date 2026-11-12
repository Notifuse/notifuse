import { Fragment } from 'react'
import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../lib/dayjs'
import type { Segment } from '../../services/api/segment'
import type { List } from '../../services/api/types'
import { segmentSummary } from '../segment/segmentSummary'
import { SegmentBuildingIndicator } from './SegmentProgress'

export interface SegmentDefinedByProps {
  workspaceId: string
  segment: Segment
  lists: List[]
  customFieldLabels: Record<string, string>
}

/** The first conditions of a segment, and when it last changed. */
export function SegmentDefinedBy({ workspaceId, segment, lists, customFieldLabels }: SegmentDefinedByProps) {
  const { t, i18n } = useLingui()
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
    </div>
  )
}
