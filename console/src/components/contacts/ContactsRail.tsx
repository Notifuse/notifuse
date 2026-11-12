import { Skeleton } from 'antd'
import { Plus, Square, Users } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { Segment } from '../../services/api/segment'
import type { UserPermissions } from '../../services/api/types'
import { RailItem, RailSection } from '../layout'
import type { ContactsView } from './contactsView'
import { SegmentBuildingIndicator } from './SegmentProgress'
import { useListsWithCountsQuery } from './useContactsLookups'

export interface ContactsRailProps {
  workspaceId: string
  view: ContactsView
  permissions: UserPermissions | null
  /** contacts.count of the workspace. */
  contactsCount?: number
  segments?: Segment[]
  segmentsLoading: boolean
  onSelect: (view: ContactsView) => void
  onCreateList: () => void
  onCreateSegment: () => void
}

const byName = <T extends { name: string }>(a: T, b: T) => a.name.localeCompare(b.name)

function SkeletonRows() {
  return (
    <>
      {[0, 1, 2].map((key) => (
        <div key={key} data-rail-skeleton className="flex h-7 items-center px-2">
          <Skeleton.Input active size="small" block />
        </div>
      ))}
    </>
  )
}

/** The left column of the Contacts page: all contacts, the lists, the segments. */
export function ContactsRail({
  workspaceId,
  view,
  permissions,
  contactsCount,
  segments,
  segmentsLoading,
  onSelect,
  onCreateList,
  onCreateSegment
}: ContactsRailProps) {
  const { t } = useLingui()
  const canReadContacts = !!permissions?.contacts?.read
  const canReadLists = !!permissions?.lists?.read
  const canReadSegments = !!permissions?.segments?.read
  const { data: listsData, isLoading: listsLoading } = useListsWithCountsQuery(workspaceId, canReadLists)
  const lists = [...(listsData?.lists ?? [])].sort(byName)
  const sortedSegments = [...(segments ?? [])].sort(byName)

  return (
    <>
      {canReadContacts ? (
        <RailSection>
          <RailItem
            icon={<Users size={15} aria-hidden="true" />}
            label={t`All contacts`}
            count={contactsCount}
            active={view.kind === 'all'}
            onClick={() => onSelect({ kind: 'all' })}
          />
        </RailSection>
      ) : null}

      {canReadLists ? (
        <RailSection
          title={t`Lists`}
          action={
            permissions?.lists?.write
              ? { label: t`New list`, icon: <Plus size={14} aria-hidden="true" />, onClick: onCreateList }
              : undefined
          }
        >
          {listsLoading ? (
            <SkeletonRows />
          ) : lists.length === 0 ? (
            permissions?.lists?.write ? (
              <RailItem
                icon={<Plus size={14} aria-hidden="true" />}
                label={t`Create your first list`}
                onClick={onCreateList}
              />
            ) : (
              <div className="px-2 py-1 text-sm text-ink-3">{t`No lists yet`}</div>
            )
          ) : (
            lists.map((list) => (
              <RailItem
                key={list.id}
                icon={<Square size={13} aria-hidden="true" />}
                label={list.name}
                // An empty cell while the count loads, so the row does not shift.
                count={list.active_count}
                active={view.kind === 'list' && view.listId === list.id}
                onClick={() => onSelect({ kind: 'list', listId: list.id })}
              />
            ))
          )}
        </RailSection>
      ) : null}

      {canReadSegments ? (
        <RailSection
          title={t`Segments`}
          action={
            permissions?.segments?.write
              ? {
                  label: t`New segment`,
                  icon: <Plus size={14} aria-hidden="true" />,
                  onClick: onCreateSegment
                }
              : undefined
          }
        >
          {segmentsLoading ? (
            <SkeletonRows />
          ) : sortedSegments.length === 0 ? (
            <div className="px-2 py-1 text-sm text-ink-3">{t`No segments yet`}</div>
          ) : (
            sortedSegments.map((segment) => (
              <RailItem
                key={segment.id}
                icon={
                  <span
                    className="size-2 rounded-full"
                    // The segment's own colour, chosen by the user: not a theme token.
                    style={{ backgroundColor: segment.color }}
                  />
                }
                label={segment.name}
                count={segment.status === 'building' ? undefined : segment.users_count}
                active={view.kind === 'segment' && view.segmentId === segment.id}
                onClick={() => onSelect({ kind: 'segment', segmentId: segment.id })}
                trailing={
                  segment.status === 'building' ? (
                    <SegmentBuildingIndicator workspaceId={workspaceId} segment={segment} />
                  ) : undefined
                }
              />
            ))
          )}
        </RailSection>
      ) : null}
    </>
  )
}
