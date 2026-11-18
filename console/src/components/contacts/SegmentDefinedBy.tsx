import { Fragment, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { App, Button, Popconfirm, Tooltip } from 'antd'
import { Pencil, RefreshCw, TriangleAlert } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import dayjs from '../../lib/dayjs'
import { rebuildSegment, type Segment } from '../../services/api/segment'
import type { List } from '../../services/api/types'
import { DrawerSegment } from '../segment/button_upsert'
import { segmentSummary } from '../segment/segmentSummary'
import { segmentLastErrorText } from '../segment/segmentValueError'
import { SegmentBuildingIndicator } from './SegmentProgress'

export interface SegmentDefinedByProps {
  workspaceId: string
  segment: Segment
  lists: List[]
  customFieldLabels: Record<string, string>
}

export interface SegmentActionsProps {
  workspaceId: string
  segment: Segment
  /** Templates read, which the segment drawer needs to list the templates. */
  canReadTemplates: boolean
  /** The workspace's contacts, for the share the drawer's count shows. */
  totalContacts?: number
}

/** Recompute (confirmed first) and Edit segment, for segments write: the view header shows them. */
export function SegmentActions({ workspaceId, segment, canReadTemplates, totalContacts }: SegmentActionsProps) {
  const { t } = useLingui()
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

  return (
    <>
      <Popconfirm
        title={t`Rebuild segment`}
        description={
          <div className="max-w-72">
            {t`Segments update automatically as contacts change. Recompute "${segment.name}" now? This recalculates all of its members.`}
          </div>
        }
        okText={t`Yes`}
        cancelText={t`No`}
        onConfirm={() => rebuild.mutate()}
      >
        <Button icon={<RefreshCw size={14} aria-hidden="true" />} loading={rebuild.isPending}>
          {t`Recompute`}
        </Button>
      </Popconfirm>
      <Button icon={<Pencil size={14} aria-hidden="true" />} onClick={() => setEditing(true)}>
        {t`Edit segment`}
      </Button>
      {editing ? (
        <DrawerSegment
          segment={segment}
          totalContacts={totalContacts}
          canReadTemplates={canReadTemplates}
          setDrawserVisible={setEditing}
          onSuccess={invalidateSegments}
        />
      ) : null}
    </>
  )
}

/** The first conditions of a segment and when it last changed. */
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
  // The same sentence as the table's « +N », for one string in the catalogue.
  const count = summary.more
  const joined = summary.hidden.join(', ')
  const lastError = segment.last_error
  const failedAgo = segment.last_error_at ? dayjs(segment.last_error_at).fromNow() : ''

  return (
    <div className="flex flex-col gap-2">
      {lastError ? (
        <div
          role="status"
          className="flex flex-wrap items-center gap-2 rounded-lg bg-warning-bg px-3 py-2 text-sm text-warning-fg"
          data-segment-last-error
        >
          <TriangleAlert size={14} aria-hidden="true" />
          <span>{segmentLastErrorText(lastError, i18n, customFieldLabels)}</span>
          <span>
            {/* The queue's error is about one contact: a recompute would meet the same value */}
            {lastError.source === 'contact_queue'
              ? t`Until the value is fixed, this contact keeps its current place in the segment.`
              : lastError.kind !== 'value_conversion'
                ? t`Membership may be incomplete: recompute the segment.`
                : t`Membership may be incomplete: fix the value, then recompute the segment.`}
          </span>
          {failedAgo ? <span className="text-xs">{t`Failed ${failedAgo}`}</span> : null}
        </div>
      ) : null}
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
        {summary.more > 0 ? (
          <Tooltip title={summary.hidden.join(` ${summary.joiner} `)}>
            <span className="text-xs text-ink-3">
              <span aria-hidden="true">{t`+${count}`}</span>
              <span className="sr-only">
                {plural(count, {
                  one: `and # more: ${joined}`,
                  other: `and # more: ${joined}`
                })}
              </span>
            </span>
          </Tooltip>
        ) : null}
        <span className="ml-auto flex items-center gap-2 text-xs text-ink-3">
          {segment.status === 'building' ? (
            <SegmentBuildingIndicator workspaceId={workspaceId} segment={segment} />
          ) : null}
          <Tooltip title={updated.format('LLL')}>
            <span>{t`Updated ${updatedAgo}`}</span>
          </Tooltip>
        </span>
      </div>
    </div>
  )
}
