import { useState } from 'react'
import { Segmented } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { Segment } from '../../../services/api/segment'
import type { Workspace } from '../../../services/api/types'
import { ContactTimeline } from '../../timeline'
import { BlockError } from '../BlockError'
import { useContactTimeline, type ActivityFilter } from './useContactSheetQueries'

export interface ContactActivityTabProps {
  workspace: Workspace
  email: string
  /** The contact's timezone, else the workspace's. */
  timezone: string
  segments: Segment[]
  canReadMessages: boolean
  canReadVisits: boolean
}

/** The contact's activity, filtered by kind; each filter has its own pages. */
export function ContactActivityTab({
  workspace,
  email,
  timezone,
  segments,
  canReadMessages,
  canReadVisits
}: ContactActivityTabProps) {
  const { t } = useLingui()
  const [filter, setFilter] = useState<ActivityFilter>('all')
  const timeline = useContactTimeline(workspace.id, email, filter)
  const entries = timeline.data?.pages.flatMap((page) => page.timeline ?? []) ?? []

  const options: { value: ActivityFilter; label: string }[] = [
    { value: 'all', label: t`All` },
    ...(canReadMessages ? [{ value: 'emails' as const, label: t`Emails` }] : []),
    { value: 'lists', label: t`Lists & segments` },
    { value: 'profile', label: t`Profile` },
    { value: 'events', label: t`Events` },
    ...(canReadVisits ? [{ value: 'web' as const, label: t`Web` }] : [])
  ]

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Segmented<ActivityFilter> size="small" value={filter} onChange={setFilter} options={options} />
        <span className="text-xs text-ink-3">{t`Times in ${timezone}`}</span>
      </div>
      {timeline.isError && !timeline.data ? (
        <BlockError onRetry={() => timeline.refetch()} />
      ) : filter !== 'all' && timeline.isSuccess && entries.length === 0 ? (
        <div className="py-8 text-center text-sm text-ink-3">{t`No activity of this kind`}</div>
      ) : (
        <ContactTimeline
          entries={entries}
          loading={timeline.isLoading}
          timezone={timezone}
          workspace={workspace}
          segments={segments}
          onLoadMore={() => timeline.fetchNextPage()}
          hasMore={timeline.hasNextPage}
          isLoadingMore={timeline.isFetchingNextPage}
        />
      )}
    </div>
  )
}
