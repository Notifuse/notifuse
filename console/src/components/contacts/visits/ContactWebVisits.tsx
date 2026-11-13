import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Segmented, Skeleton, Table, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Link } from '@tanstack/react-router'
import {
  Globe,
  Image,
  Mail,
  Monitor,
  MonitorSmartphone,
  MousePointer,
  Play,
  Search,
  Share2,
  Smartphone,
  Tablet,
  Target
} from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { I18n } from '@lingui/core'
import dayjs from '../../../lib/dayjs'
import {
  webAnalyticsService,
  type ContactVisitSession,
  type ContactVisitsFilter,
  type ContactVisitsSummary
} from '../../../services/api/web_analytics'
import type { Workspace } from '../../../services/api/types'
import { Mono, useNumberFormat } from '../../layout'
import { BlockError } from '../BlockError'
import { useContactVisits } from '../sheet/useContactSheetQueries'
import { formatDuration } from './duration'
import { deviceIcon, visitSource, type DeviceIcon, type VisitSourceIcon } from './webVisitSource'
import { webAnalyticsContactSearch } from './webAnalyticsContactSearch'

const SOURCE_ICONS: Record<VisitSourceIcon, typeof Mail> = {
  mail: Mail,
  search: Search,
  social: Share2,
  display: Image,
  video: Play,
  direct: MousePointer,
  other: Globe
}

const DEVICE_ICONS: Record<DeviceIcon, typeof Monitor> = {
  desktop: Monitor,
  mobile: Smartphone,
  tablet: Tablet,
  other: MonitorSmartphone
}

const IDENTIFYING_VISITORS_DOCS = 'https://docs.notifuse.com/web-analytics/identifying-visitors'

/** « Today », « Yesterday », then the date, and the start time, in the workspace timezone. */
function sessionDay(createdAt: string, timezone: string, labels: { today: string; yesterday: string }) {
  const start = dayjs(createdAt).tz(timezone)
  const today = dayjs().tz(timezone).startOf('day')
  const day = start.isSame(today, 'day')
    ? labels.today
    : start.isSame(today.subtract(1, 'day'), 'day')
      ? labels.yesterday
      : start.format('ll')
  return `${day} · ${start.format('HH:mm')}`
}

/** A goal's value, which may carry cents. */
const goalValue = (value: number, locale: string) =>
  new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(value)

function SourceCell({ session, i18n }: { session: ContactVisitSession; i18n: I18n }) {
  const source = visitSource(session, i18n)
  const Icon = SOURCE_ICONS[source.icon]
  return (
    <div className="flex min-w-0 flex-col">
      <span className="flex items-center gap-1.5 text-ink">
        <Icon
          size={13}
          aria-hidden="true"
          className={source.icon === 'mail' ? 'shrink-0 text-primary-500' : 'shrink-0 text-ink-3'}
        />
        <span className="truncate">{source.label}</span>
      </span>
      {source.detail ? <span className="truncate text-xs text-ink-3">{source.detail}</span> : null}
    </div>
  )
}

function SummaryCell({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1 px-4 py-3">
      <span className="text-overline text-ink-3 uppercase">{title}</span>
      {children}
    </div>
  )
}

function Summary({ summary, workspace }: { summary: ContactVisitsSummary; workspace: Workspace }) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()
  const timezone = workspace.settings?.timezone || 'UTC'
  const first = summary.first_session
  return (
    <div className="grid grid-cols-1 divide-y divide-line-soft rounded-lg border border-line bg-surface md:grid-cols-3 md:divide-x md:divide-y-0">
      <SummaryCell title={t`First identified visit`}>
        {first ? (
          <>
            <span className="text-sm text-ink">{dayjs(first.created_at).tz(timezone).format('ll')}</span>
            <SourceCell session={first} i18n={i18n} />
            <span className="text-xs text-ink-3">
              {[first.utm_campaign, first.device].filter(Boolean).join(' · ')}
            </span>
          </>
        ) : (
          <span className="text-sm text-ink-3">—</span>
        )}
      </SummaryCell>
      <SummaryCell title={t`Goals reached`}>
        {summary.goals.length === 0 ? (
          <span className="text-sm text-ink-3">{t`No goal yet`}</span>
        ) : (
          summary.goals.map((goal) => (
            <span key={goal.goal_name} className="flex items-center gap-1.5 text-sm text-ink">
              <Target size={13} aria-hidden="true" className="shrink-0 text-ink-3" />
              {goal.goal_name} · {int(goal.count)} · {goalValue(goal.value, i18n.locale)}
            </span>
          ))
        )}
      </SummaryCell>
      <SummaryCell title={t`Most viewed pages`}>
        {summary.top_pages.length === 0 ? (
          <span className="text-sm text-ink-3">—</span>
        ) : (
          summary.top_pages.map((page) => (
            <span key={page.path} className="flex items-center justify-between gap-2 text-sm">
              <Mono>{page.path}</Mono>
              <span className="text-ink-3 tabular-nums">{int(page.views)}</span>
            </span>
          ))
        )}
      </SummaryCell>
    </div>
  )
}

/** The pages of one session in order, with its goals at their time; loaded when expanded. */
function SessionJourney({
  workspace,
  email,
  session
}: {
  workspace: Workspace
  email: string
  session: ContactVisitSession
}) {
  const { t, i18n } = useLingui()
  const timezone = workspace.settings?.timezone || 'UTC'
  const journey = useQuery({
    queryKey: ['contact-session-pages', workspace.id, email, session.id],
    queryFn: () =>
      webAnalyticsService.contactSessionPages({
        workspace_id: workspace.id,
        email,
        session_id: session.id,
        session_date: session.session_date
      }),
    staleTime: Infinity
  })
  if (journey.isError) return <BlockError onRetry={() => journey.refetch()} />
  if (!journey.data) return <Skeleton active title={false} paragraph={{ rows: 3 }} />

  const steps = [
    ...journey.data.pages.map((page) => ({ kind: 'page' as const, at: page.entered_at, page })),
    ...journey.data.goals.map((goal) => ({ kind: 'goal' as const, at: goal.goal_at, goal }))
  ].sort((a, b) => dayjs(a.at).valueOf() - dayjs(b.at).valueOf())

  return (
    <div data-session-journey className="flex flex-col gap-1.5 py-1 text-sm">
      {steps.map((step, index) => (
        <div key={index} className="flex items-center gap-3">
          <span className="w-12 shrink-0 text-xs text-ink-3 tabular-nums">
            {dayjs(step.at).tz(timezone).format('HH:mm')}
          </span>
          {step.kind === 'page' ? (
            <>
              <Mono>{step.page.path}</Mono>
              <span className="ml-auto text-xs text-ink-3">
                {formatDuration(step.page.duration_ms, i18n)}
              </span>
            </>
          ) : (
            <span data-journey-goal className="flex items-center gap-1.5 text-ink">
              <Target size={13} aria-hidden="true" className="text-success-fg" />
              {step.goal.goal_name}
              {step.goal.goal_value ? (
                <span className="text-ink-3">· {goalValue(step.goal.goal_value, i18n.locale)}</span>
              ) : null}
            </span>
          )}
        </div>
      ))}
      <div className="mt-1 flex items-center gap-2 border-t border-line-soft pt-2 text-xs text-ink-3">
        <span>{t`Exited on`}</span>
        <Mono>{session.exit_path}</Mono>
        <Link
          className="ml-auto"
          to="/console/workspace/$workspaceId/web-analytics/$tab"
          params={{ workspaceId: workspace.id, tab: 'dashboard' }}
          search={webAnalyticsContactSearch(email, timezone)}
        >
          {t`Open in web analytics`}
        </Link>
      </div>
    </div>
  )
}

export interface ContactWebVisitsProps {
  workspace: Workspace
  email: string
}

/** The contact's identified web sessions: a summary, filters, and each session's journey. */
export function ContactWebVisits({ workspace, email }: ContactWebVisitsProps) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()
  const [filter, setFilter] = useState<ContactVisitsFilter>('all')
  const all = useContactVisits(workspace.id, email, 'all', true)
  const filtered = useContactVisits(workspace.id, email, filter, filter !== 'all')
  const current = filter === 'all' ? all : filtered
  const summary = all.data?.pages[0]?.summary
  const timezone = workspace.settings?.timezone || 'UTC'
  const dayLabels = { today: t`Today`, yesterday: t`Yesterday` }

  if (all.isError && !all.data) return <BlockError onRetry={() => all.refetch()} />
  if (all.isLoading) return <Skeleton active title={false} paragraph={{ rows: 6 }} />

  if (!summary || summary.sessions === 0) {
    return (
      <div className="flex flex-col items-center gap-1 py-10 text-center text-sm">
        <span className="text-ink-2">{t`No visits recorded for this contact`}</span>
        <a href={IDENTIFYING_VISITORS_DOCS} target="_blank" rel="noreferrer" className="text-xs">
          {t`Visits show up once the visitor is identified`}
        </a>
      </div>
    )
  }

  const sessions = current.data?.pages.flatMap((page) => page.sessions ?? []) ?? []

  const columns: ColumnsType<ContactVisitSession> = [
    {
      title: t`Session`,
      key: 'session',
      render: (_, session) => (
        <span className="whitespace-nowrap">{sessionDay(session.created_at, timezone, dayLabels)}</span>
      )
    },
    { title: t`Source`, key: 'source', render: (_, session) => <SourceCell session={session} i18n={i18n} /> },
    {
      title: t`Entry`,
      key: 'entry',
      render: (_, session) => (
        <span className="flex items-center gap-1.5">
          <Mono>{session.landing_path}</Mono>
          {session.pageview_count > 1 ? (
            <span className="text-xs text-ink-3">{t`+${session.pageview_count - 1}`}</span>
          ) : null}
        </span>
      )
    },
    {
      title: t`Duration`,
      key: 'duration',
      align: 'right',
      render: (_, session) => (
        <span className="tabular-nums">{formatDuration(session.duration_ms, i18n)}</span>
      )
    },
    {
      title: t`Device`,
      key: 'device',
      render: (_, session) => {
        const Icon = DEVICE_ICONS[deviceIcon(session.device)]
        return (
          <Tooltip title={[session.os, session.browser, session.country].filter(Boolean).join(' · ')}>
            <span data-device={deviceIcon(session.device)} className="flex text-ink-3">
              <Icon size={14} aria-label={session.device || t`Unknown device`} />
            </span>
          </Tooltip>
        )
      }
    },
    {
      title: t`Goal`,
      key: 'goal',
      render: (_, session) =>
        session.first_goal_name ? (
          <span className="flex items-center gap-1.5 whitespace-nowrap">
            <Target size={13} aria-hidden="true" className="text-success-fg" />
            {session.first_goal_name}
            {session.goal_count > 1 ? (
              <span className="text-xs text-ink-3">{t`+${session.goal_count - 1}`}</span>
            ) : null}
          </span>
        ) : null
    }
  ]

  return (
    <div className="flex flex-col gap-3">
      <Summary summary={summary} workspace={workspace} />
      <Segmented<ContactVisitsFilter>
        size="small"
        className="self-start"
        value={filter}
        onChange={setFilter}
        options={[
          { value: 'all', label: `${t`All`} ${int(summary.sessions)}` },
          { value: 'goal', label: `${t`With a goal`} ${int(summary.goal_sessions)}` },
          { value: 'email', label: `${t`From an email`} ${int(summary.email_sessions)}` }
        ]}
      />
      {current.isError && !current.data ? (
        <BlockError onRetry={() => current.refetch()} />
      ) : current.isSuccess && sessions.length === 0 ? (
        <div className="py-8 text-center text-sm text-ink-3">{t`No sessions match this filter`}</div>
      ) : (
        <>
          <Table<ContactVisitSession>
            size="small"
            rowKey="id"
            columns={columns}
            dataSource={sessions}
            loading={current.isLoading}
            pagination={false}
            expandable={{
              expandedRowRender: (session) => (
                <SessionJourney workspace={workspace} email={email} session={session} />
              )
            }}
          />
          {current.hasNextPage ? (
            <Button
              className="self-center"
              loading={current.isFetchingNextPage}
              onClick={() => current.fetchNextPage()}
            >
              {t`Load more`}
            </Button>
          ) : null}
        </>
      )}
    </div>
  )
}
