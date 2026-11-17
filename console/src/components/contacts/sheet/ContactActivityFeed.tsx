import { useState } from 'react'
import { Button, Popover, Skeleton, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import type { ContactTimelineEntry, MessageHistoryEntityData } from '../../../services/api/contact_timeline'
import type { Segment } from '../../../services/api/segment'
import type { Workspace } from '../../../services/api/types'
import TemplatePreviewDrawer from '../../templates/TemplatePreviewDrawer'
import { WebPageDrawer } from '../../timeline/WebPageDrawer'
import { useActivityDescriber, type ActivityTone } from './describeActivity'
import { useDayLabel } from './sheetDates'

// Whole class names: Tailwind only generates the classes it finds written out in the source.
const TONES: Record<ActivityTone, string> = {
  primary: 'bg-primary-100 text-primary-700',
  green: 'bg-success-bg text-success-fg',
  amber: 'bg-warning-bg text-warning-fg',
  gray: 'bg-fill text-ink-2',
  red: 'bg-danger-bg text-danger-fg',
  blue: 'bg-info-bg text-info-fg'
}

// Where the thread runs in a row: from the first icon's centre down, through, or down to the last
// icon's centre (the rows' padding is 1.5, the icon's centre 3 from the cell's top).
const THREAD = {
  first: 'top-3 -bottom-1.5',
  middle: '-top-1.5 -bottom-1.5',
  last: '-top-1.5 h-4.5'
}

export interface ContactActivityFeedProps {
  entries: ContactTimelineEntry[]
  loading: boolean
  /** The timezone the days and times are read in. */
  timezone: string
  workspace: Workspace
  segments: Segment[]
  hasMore: boolean
  isLoadingMore: boolean
  onLoadMore: () => void
}

interface Day {
  key: string
  label: string
  entries: ContactTimelineEntry[]
}

type PreviewRecord = Parameters<typeof TemplatePreviewDrawer>[0]['record']

/** The template record the preview drawer takes, from the template an email entry joined. */
function previewRecord(data: MessageHistoryEntityData): PreviewRecord {
  return {
    id: data.template_id,
    name: data.template_name || data.template_id,
    version: data.template_version,
    category: data.template_category || 'transactional',
    channel: data.channel as 'email' | 'web',
    email: data.template_email as unknown as PreviewRecord['email'],
    test_data: data.message_data?.data ?? {},
    created_at: '',
    updated_at: ''
  }
}

/**
 * The contact's activity as a feed: entries grouped by day, each with its time, a toned icon, one
 * sentence, an optional detail and where it came from.
 */
export function ContactActivityFeed({
  entries,
  loading,
  timezone,
  workspace,
  segments,
  hasMore,
  isLoadingMore,
  onLoadMore
}: ContactActivityFeedProps) {
  const { t } = useLingui()
  const dayLabel = useDayLabel(timezone)
  const [email, setEmail] = useState<MessageHistoryEntityData | null>(null)
  const [page, setPage] = useState<string | null>(null)
  const describe = useActivityDescriber({
    workspace,
    segments,
    timezone,
    onPreviewEmail: setEmail,
    onPreviewPage: setPage
  })

  if (loading && entries.length === 0) return <Skeleton active paragraph={{ rows: 6 }} />
  if (entries.length === 0) {
    return <div className="py-8 text-center text-sm text-ink-3">{t`No activity yet`}</div>
  }

  const days: Day[] = []
  // The timeline query can return a row once per webhook event of its message: each entry once.
  const seen = new Set<string>()
  for (const entry of entries) {
    if (seen.has(entry.id)) continue
    seen.add(entry.id)
    const at = dayjs(entry.created_at).tz(timezone)
    const key = at.format('YYYY-MM-DD')
    const last = days[days.length - 1]
    if (last && last.key === key) last.entries.push(entry)
    else days.push({ key, label: dayLabel(entry.created_at), entries: [entry] })
  }

  return (
    <div className="flex flex-col">
      {days.map((day) => (
        <section key={day.key} data-activity-day aria-label={day.label}>
          <h4 data-unreset className="m-0 pt-2.5 pb-1 text-overline font-semibold tracking-wide text-ink-3 uppercase">
            {day.label}
          </h4>
          <ol className="m-0 list-none p-0">
            {day.entries.map((entry, index) => {
              const item = describe(entry)
              const Icon = item.icon
              const at = dayjs(entry.created_at).tz(timezone)
              const first = index === 0
              const last = index === day.entries.length - 1
              return (
                <li
                  key={entry.id}
                  data-activity-entry
                  className="grid grid-cols-[4rem_1.375rem_minmax(0,1fr)_auto] items-start gap-x-2.5 py-1.5"
                >
                  <Tooltip title={t`${at.format('LLLL')} (${timezone})`}>
                    {/* One line: « 11:43 AM » broke after the minutes. */}
                    <time
                      dateTime={entry.created_at}
                      className="text-xs leading-5.5 whitespace-nowrap text-ink-3 tabular-nums"
                    >
                      {at.format('LT')}
                    </time>
                  </Tooltip>
                  <span className="relative flex justify-center self-stretch">
                    {/* The thread that links the day's icons, from the first icon to the last,
                        across the rows' padding. */}
                    {first && last ? null : (
                      <span
                        aria-hidden="true"
                        className={`absolute w-px bg-line ${THREAD[first ? 'first' : last ? 'last' : 'middle']}`}
                      />
                    )}
                    <Popover
                      title={t`Raw Entry Data`}
                      placement="right"
                      content={
                        <pre className="m-0 max-h-96 max-w-lg overflow-auto rounded-sm bg-subtle p-2 text-xs">
                          {JSON.stringify(entry, null, 2)}
                        </pre>
                      }
                    >
                      <span
                        data-tone={item.tone}
                        className={`relative z-1 flex size-5.5 items-center justify-center rounded-md ${TONES[item.tone]}`}
                      >
                        <Icon size={13} aria-hidden="true" />
                      </span>
                    </Popover>
                  </span>
                  <div className="min-w-0 pt-0.5">
                    <div className="text-base leading-4.5 text-ink">{item.title}</div>
                    {item.meta ? <div className="mt-0.5 text-xs text-ink-3">{item.meta}</div> : null}
                  </div>
                  <span className="mt-0.5 rounded-sm bg-canvas px-1.5 text-2xs leading-4.5 whitespace-nowrap text-ink-3">
                    {item.source}
                  </span>
                </li>
              )
            })}
          </ol>
        </section>
      ))}
      {hasMore ? (
        <div className="flex justify-center pt-2.5">
          <Button size="small" onClick={onLoadMore} loading={isLoadingMore}>
            {t`Load more activity`}
          </Button>
        </div>
      ) : null}

      {email ? (
        <TemplatePreviewDrawer
          open
          onClose={() => setEmail(null)}
          record={previewRecord(email)}
          workspace={workspace}
          templateData={email.message_data?.data ?? {}}
        />
      ) : null}
      <WebPageDrawer open={page !== null} url={page} onClose={() => setPage(null)} />
    </div>
  )
}
