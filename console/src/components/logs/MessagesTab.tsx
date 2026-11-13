import { useMemo, useState, type ReactNode } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Empty, Result, Select, Skeleton, Table, Tooltip, type MenuProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { ChevronRight, CircleCheck, Eye, MousePointerClick, RefreshCw, Send } from 'lucide-react'
import { useAuth } from '../../contexts/AuthContext'
import type { UserPermissions } from '../../services/api/types'
import {
  countMessages,
  listMessages,
  type MessageCounts,
  type MessageFilterParams,
  type MessageHistory
} from '../../services/api/messages_history'
import { broadcastApi } from '../../services/api/broadcast'
import { automationApi } from '../../services/api/automation'
import { listsApi } from '../../services/api/list'
import { messageStatus } from '../../lib/messageStatus'
import { MessageColumnsSelector } from '../messages/MessageColumnsSelector'
import { Mono, Panel, Pill, TableToolbar, useNumberFormat, useRelativeDate } from '../layout'
import type { PillTone } from '../layout'
import { FilterChip } from './FilterChip'
import { LogsSearchBox } from './LogsSearchBox'
import { MessageDetail } from './MessageDetail'
import { TextFilter } from './TextFilter'
import { MESSAGE_ORIGIN_LABELS, MESSAGE_STATUS_LABELS } from './labels'
import {
  MESSAGE_CHANNELS,
  MESSAGE_FLAGS,
  MESSAGE_ORIGINS,
  type LivePeriod,
  type LogsSearch,
  type MessageChannel,
  type MessageFlag,
  type MessageOrigin
} from './logsSearch'
import { formatLogDateTime, formatLogTime } from './logTime'
import {
  MESSAGE_COLUMNS_KEY,
  loadMessageColumns,
  type MessageColumnKey,
  type MessageColumns
} from './messageColumns'
import { ERASED_EMAIL, countsDeliveries, messageOrigin, rowIsSmtp } from './messageProvider'
import { canCountMessages, defaultMessagesPeriod, periodBounds } from './period'
import { useLogsUrl } from './useLogsUrl'

const PAGE_SIZE = 50

const ORIGIN_TONES: Record<MessageOrigin, PillTone> = {
  broadcast: 'primary',
  automation: 'success',
  transactional: 'neutral',
  other: 'neutral'
}

type CounterFlag = Exclude<MessageFlag, 'is_unsubscribed'>
interface Counter {
  flag?: CounterFlag
  count: keyof MessageCounts
  dot: string
}
// Whole class names: Tailwind only generates the classes it finds written out.
const COUNTERS: Counter[] = [
  { count: 'total', dot: 'bg-ink-4' },
  { flag: 'is_delivered', count: 'delivered', dot: 'bg-success-fg' },
  { flag: 'is_opened', count: 'opened', dot: 'bg-info-fg' },
  { flag: 'is_clicked', count: 'clicked', dot: 'bg-primary' },
  { flag: 'is_bounced', count: 'bounced', dot: 'bg-warning-fg' },
  { flag: 'is_failed', count: 'failed', dot: 'bg-danger-fg' },
  { flag: 'is_complained', count: 'complained', dot: 'bg-danger-fg' }
]

// Filters of "+ Filter", besides the statuses.
const ENTITY_FILTERS = [
  'channel',
  'list_id',
  'template_id',
  'broadcast_id',
  'automation_id',
  'transactional_notification_id',
  'clicked_url'
] as const
type EntityFilter = (typeof ENTITY_FILTERS)[number]
type AddedFilter = MessageFlag | EntityFilter

const SEARCH_KEYS = ['contact_email', 'id', 'external_id'] as const
// What the list and the counts send from the URL; `message` is the open detail, not a filter.
const FILTER_KEYS = [...SEARCH_KEYS, 'origin', ...ENTITY_FILTERS] as const

/** The URL parameters that narrow the messages: everything but the tab, the period and the detail. */
function hasMessageFilters(search: LogsSearch): boolean {
  return (
    FILTER_KEYS.some((key) => search[key] !== undefined) ||
    MESSAGE_FLAGS.some((flag) => search[flag] !== undefined)
  )
}

/** The counter whose flag is the only status of the URL; `all` with none. */
function activeCounter(search: LogsSearch): Counter | undefined {
  const set = MESSAGE_FLAGS.filter((flag) => search[flag] !== undefined)
  if (set.length === 0) return COUNTERS[0]
  if (set.length > 1 || search[set[0]] !== true) return undefined
  return COUNTERS.find((counter) => counter.flag === set[0])
}

export interface MessagesTabProps {
  workspaceId: string
  /** The member's rights, as the Logs page loaded them. */
  permissions: UserPermissions | null
}

/** Logs → Messages: every message the workspace sent, with counts by status. */
export function MessagesTab({ workspaceId, permissions }: MessagesTabProps) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()
  const relative = useRelativeDate()
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const workspace = workspaces.find((candidate) => candidate.id === workspaceId)
  const timezone = workspace?.settings.timezone
  const { search, update, clear } = useLogsUrl(workspaceId, 'messages')
  const period: LivePeriod =
    (search.period as LivePeriod | undefined) ?? defaultMessagesPeriod(search)

  // The clock of the period: set when the filters change and on Refresh, never by a re-render.
  // Neither the open message nor a counter (the statuses) moves it: opening a row keeps the loaded
  // pages, and every counter compares the same window, under the same count.
  const withoutFlags: Record<string, unknown> = { ...search, message: undefined }
  for (const flag of MESSAGE_FLAGS) delete withoutFlags[flag]
  const filtersKey = JSON.stringify(withoutFlags)
  const flagsKey = JSON.stringify(MESSAGE_FLAGS.map((flag) => search[flag]))
  const [clock, setClock] = useState(() => ({ key: filtersKey, now: Date.now() }))
  if (clock.key !== filtersKey) setClock({ key: filtersKey, now: Date.now() })
  const now = clock.key === filtersKey ? clock.now : Date.now()

  const filters = useMemo(() => {
    const out: MessageFilterParams = {}
    for (const key of FILTER_KEYS) {
      const value = search[key]
      if (value !== undefined) Object.assign(out, { [key]: value })
    }
    const after = periodBounds(period, now).from
    if (after) out.created_after = after
    return out
    // `search` is new on every render; its content is the key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filtersKey, period, now])
  const flags = useMemo(() => {
    const out: Partial<Record<MessageFlag, boolean>> = {}
    for (const flag of MESSAGE_FLAGS) if (search[flag] !== undefined) out[flag] = search[flag]
    return out
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [flagsKey])

  // M1: 50 messages at a time, by cursor.
  const listQuery = useInfiniteQuery({
    queryKey: ['logs', 'messages', workspaceId, { ...filters, ...flags }],
    queryFn: ({ pageParam }) =>
      listMessages(workspaceId, { ...filters, ...flags, cursor: pageParam, limit: PAGE_SIZE }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => (last.has_more ? last.next_cursor : undefined),
    staleTime: 30_000
  })
  const rows = useMemo(
    () => listQuery.data?.pages.flatMap((page) => page.messages) ?? [],
    [listQuery.data]
  )

  // M2: the counts, without the statuses, so each counter shows what a click on it lists.
  const countable = canCountMessages(search, period)
  const countQuery = useQuery({
    queryKey: ['logs', 'messages-count', workspaceId, filters],
    queryFn: () => countMessages(workspaceId, filters),
    enabled: countable,
    staleTime: 30_000
  })

  // N1 to N3: the names of what sent the messages.
  const canReadBroadcasts = Boolean(permissions?.broadcasts?.read)
  const canReadAutomations = Boolean(permissions?.automations?.read)
  const canReadLists = Boolean(permissions?.lists?.read)
  const broadcastsQuery = useQuery({
    queryKey: ['broadcasts', workspaceId],
    queryFn: () => broadcastApi.list({ workspace_id: workspaceId, limit: 1000 }),
    enabled: canReadBroadcasts,
    staleTime: 5 * 60 * 1000
  })
  const automationsQuery = useQuery({
    queryKey: ['automations', workspaceId],
    queryFn: () => automationApi.list({ workspace_id: workspaceId, with_journey_counts: true }),
    enabled: canReadAutomations,
    staleTime: 5 * 60 * 1000
  })
  const listsQuery = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: canReadLists,
    staleTime: 5 * 60 * 1000
  })
  const broadcasts = useMemo(
    () => new Map((broadcastsQuery.data?.broadcasts ?? []).map((item) => [item.id, item])),
    [broadcastsQuery.data]
  )
  const automations = useMemo(
    () => new Map((automationsQuery.data?.automations ?? []).map((item) => [item.id, item])),
    [automationsQuery.data]
  )
  const lists = useMemo(
    () => new Map((listsQuery.data?.lists ?? []).map((item) => [item.id, item])),
    [listsQuery.data]
  )
  const listName = (id: string) => lists.get(id)?.name
  const broadcastName = (id: string) => broadcasts.get(id)?.name
  const automationName = (id: string) => automations.get(id)?.name

  const [columns, setColumns] = useState<MessageColumns>(loadMessageColumns)

  const refresh = () => {
    setClock({ key: filtersKey, now: Date.now() })
    void queryClient.resetQueries({ queryKey: ['logs', 'messages', workspaceId] })
    void queryClient.resetQueries({ queryKey: ['logs', 'messages-count', workspaceId] })
  }

  // Labels
  const periodLabels: Record<LivePeriod, string> = {
    '1h': t`Last hour`,
    '24h': t`Last 24 hours`,
    '7d': t`Last 7 days`,
    '30d': t`Last 30 days`,
    all: t`All time`
  }
  const originFilterLabels: Record<MessageOrigin, string> = {
    broadcast: t`Newsletters`,
    automation: t`Automations`,
    transactional: t`Transactional`,
    other: t`Other`
  }
  const flagLabels: Record<MessageFlag, string> = {
    is_delivered: t`Delivered`,
    is_failed: t`Failed`,
    is_opened: t`Opened`,
    is_clicked: t`Clicked`,
    is_bounced: t`Bounced`,
    is_complained: t`Complained`,
    is_unsubscribed: t`Unsubscribed`
  }
  const counterLabels: Record<keyof MessageCounts, string> = {
    total: t`All`,
    delivered: t`Delivered`,
    opened: t`Opened`,
    clicked: t`Clicked`,
    bounced: t`Bounced`,
    failed: t`Failed`,
    complained: t`Complaints`
  }
  const entityLabels: Record<EntityFilter, string> = {
    channel: t`Channel`,
    list_id: t`List`,
    template_id: t`Template ID`,
    broadcast_id: t`Newsletter`,
    automation_id: t`Automation`,
    transactional_notification_id: t`Transactional notification`,
    clicked_url: t`Clicked link`
  }
  const channelLabels: Record<MessageChannel, string> = {
    email: t`Email`,
    sms: t`SMS`,
    push: t`Push`
  }

  // Toolbar
  const periodMenu: MenuProps = {
    items: (['1h', '24h', '7d', '30d', 'all'] as const).map((key) => ({
      key,
      label: periodLabels[key]
    })),
    selectable: true,
    selectedKeys: [period],
    onClick: ({ key }) => update({ period: key as LivePeriod })
  }
  const originMenu: MenuProps = {
    items: [
      { key: '', label: t`All` },
      ...MESSAGE_ORIGINS.map((origin) => ({ key: origin, label: originFilterLabels[origin] }))
    ],
    selectable: true,
    selectedKeys: [search.origin ?? ''],
    onClick: ({ key }) => update({ origin: (key || undefined) as MessageOrigin | undefined })
  }

  // + Filter: the statuses a counter does not show, then the entities.
  const counter = activeCounter(search)
  const [adding, setAdding] = useState<AddedFilter>()
  const shownFlags = MESSAGE_FLAGS.filter(
    (flag) => (search[flag] !== undefined && !(counter?.flag === flag)) || adding === flag
  )
  const shownEntities = ENTITY_FILTERS.filter(
    (key) =>
      (search[key] !== undefined || adding === key) &&
      (key !== 'clicked_url' || search.broadcast_id !== undefined)
  )
  const addable: AddedFilter[] = [
    ...MESSAGE_FLAGS.filter((flag) => search[flag] === undefined && adding !== flag),
    ...ENTITY_FILTERS.filter(
      (key) =>
        search[key] === undefined &&
        adding !== key &&
        (key !== 'clicked_url' || search.broadcast_id !== undefined)
    )
  ]
  const addedLabel = (key: AddedFilter) =>
    (MESSAGE_FLAGS as readonly string[]).includes(key)
      ? flagLabels[key as MessageFlag]
      : entityLabels[key as EntityFilter]
  const addMenu: MenuProps = {
    items: addable.map((key) => ({ key, label: addedLabel(key) })),
    onClick: ({ key }) => setAdding(key as AddedFilter)
  }

  const chipProps = (key: AddedFilter) => ({
    label: addedLabel(key),
    defaultOpen: adding === key,
    onOpenChange: (open: boolean) => {
      if (!open && adding === key) setAdding(undefined)
    },
    onClear: () => {
      setAdding(undefined)
      // The clicked link needs its newsletter: it goes with it.
      update(
        key === 'broadcast_id'
          ? { broadcast_id: undefined, clicked_url: undefined }
          : { [key]: undefined }
      )
    }
  })
  const apply = (patch: Partial<LogsSearch>) => {
    setAdding(undefined)
    update(patch)
  }

  const flagChip = (flag: MessageFlag) => (
    <FilterChip
      key={flag}
      {...chipProps(flag)}
      value={search[flag] === undefined ? t`Any` : search[flag] ? t`Yes` : t`No`}
      menu={{
        items: [
          { key: 'yes', label: t`Yes` },
          { key: 'no', label: t`No` }
        ],
        selectable: true,
        selectedKeys: search[flag] === undefined ? [] : [search[flag] ? 'yes' : 'no'],
        onClick: ({ key }) => apply({ [flag]: key === 'yes' })
      }}
    />
  )

  const namedChip = (
    key: 'list_id' | 'broadcast_id' | 'automation_id',
    names: Map<string, { id: string; name: string }>,
    canRead: boolean
  ) => {
    const value = search[key]
    const name = value ? names.get(value)?.name : undefined
    return (
      <FilterChip
        key={key}
        {...chipProps(key)}
        value={value ? (name ?? value) : t`Any`}
        content={
          canRead ? (
            <Select
              autoFocus
              showSearch
              size="small"
              style={{ width: 280 }}
              placeholder={t`Choose`}
              value={value}
              optionFilterProp="label"
              options={Array.from(names.values()).map((item) => ({
                value: item.id,
                label: item.name
              }))}
              onChange={(next: string) => apply({ [key]: next })}
            />
          ) : (
            <TextFilter
              initial={value ?? ''}
              placeholder={t`Exact ID`}
              onApply={(text) => apply({ [key]: text || undefined })}
            />
          )
        }
      />
    )
  }

  const entityChip = (key: EntityFilter) => {
    if (key === 'channel') {
      return (
        <FilterChip
          key={key}
          {...chipProps(key)}
          value={search.channel ? channelLabels[search.channel] : t`Any`}
          menu={{
            items: MESSAGE_CHANNELS.map((channel) => ({
              key: channel,
              label: channelLabels[channel]
            })),
            selectable: true,
            selectedKeys: search.channel ? [search.channel] : [],
            onClick: ({ key: channel }) => apply({ channel: channel as MessageChannel })
          }}
        />
      )
    }
    if (key === 'list_id') return namedChip(key, lists, canReadLists)
    if (key === 'broadcast_id') return namedChip(key, broadcasts, canReadBroadcasts)
    if (key === 'automation_id') return namedChip(key, automations, canReadAutomations)
    const value = search[key]
    return (
      <FilterChip
        key={key}
        {...chipProps(key)}
        value={value ?? t`Any`}
        content={
          <TextFilter
            initial={value ?? ''}
            placeholder={key === 'clicked_url' ? t`Exact URL` : t`Exact ID`}
            onApply={(text) => apply({ [key]: text || undefined })}
          />
        }
      />
    )
  }

  const filterCount =
    FILTER_KEYS.filter((key) => search[key] !== undefined).length +
    MESSAGE_FLAGS.filter((flag) => search[flag] !== undefined).length
  // The period is not a filter: clearing keeps it.
  const clearFilters = () => clear(['period'])

  const columnTitles: Record<MessageColumnKey, string> = {
    recipient: t`Recipient`,
    error: t`Error`,
    id: t`Message ID`,
    external_id: t`External ID`,
    metadata: t`Metadata`,
    template: t`Template`,
    list: t`List`,
    created: t`Created`
  }

  const toolbar = (
    <TableToolbar
      start={
        <>
          <LogsSearchBox
            keys={[
              { key: 'contact_email', label: t`Email`, email: true },
              { key: 'id', label: t`Message ID` },
              { key: 'external_id', label: t`External ID` }
            ]}
            values={search}
            onWrite={(patch) => update(patch, { replace: true })}
            ariaLabel={t`Search the messages`}
            placeholder={t`Exact value`}
          />
          <FilterChip label={t`Period`} value={periodLabels[period]} menu={periodMenu} />
          <FilterChip
            label={t`Origin`}
            value={search.origin ? originFilterLabels[search.origin] : t`All`}
            menu={originMenu}
          />
          {shownFlags.map(flagChip)}
          {shownEntities.map(entityChip)}
          {addable.length > 0 ? <FilterChip label={t`+ Filter`} dashed menu={addMenu} /> : null}
          {filterCount >= 2 ? (
            <Button type="link" size="small" onClick={clearFilters}>
              {t`Clear all`}
            </Button>
          ) : null}
        </>
      }
      end={
        <>
          <MessageColumnsSelector
            storageKey={MESSAGE_COLUMNS_KEY}
            columns={(Object.keys(columnTitles) as MessageColumnKey[]).map((key) => ({
              key,
              title: columnTitles[key],
              visible: columns[key]
            }))}
            onColumnVisibilityChange={(key, visible) =>
              setColumns((current) => ({ ...current, [key]: visible }))
            }
          />
          <Tooltip title={t`Refresh`}>
            <Button
              type="text"
              size="small"
              aria-label={t`Refresh`}
              icon={<RefreshCw size={14} />}
              onClick={refresh}
            />
          </Tooltip>
        </>
      }
    />
  )

  // Counters
  const deliveriesReported = countsDeliveries(workspace, search.origin)
  const counterNumber = (item: Counter): ReactNode => {
    if (!countable) return null
    if (item.count === 'delivered' && !deliveriesReported) {
      return (
        <Tooltip title={t`Delivery isn't reported by SMTP providers`}>
          <span className="text-ink-3">—</span>
        </Tooltip>
      )
    }
    if (countQuery.isPending) {
      return (
        <span
          data-testid="count-loading"
          className="inline-block h-3 w-6 animate-pulse rounded-sm bg-fill"
        />
      )
    }
    if (countQuery.isError) {
      return (
        <Tooltip title={t`Couldn't load`}>
          <span className="text-ink-3">—</span>
        </Tooltip>
      )
    }
    return <span className="tabular-nums">{int(countQuery.data[item.count])}</span>
  }
  const setCounter = (item: Counter) => {
    const patch: Partial<LogsSearch> = {}
    for (const flag of MESSAGE_FLAGS) patch[flag] = undefined
    if (item.flag) patch[item.flag] = true
    update(patch)
  }
  const counters = (
    <div className="flex flex-wrap items-center gap-1 border-b border-line-soft px-2.5 py-1.5">
      {COUNTERS.map((item) => {
        const active = counter === item
        const button = (
          <button
            key={item.count}
            type="button"
            aria-pressed={active}
            onClick={() => setCounter(item)}
            className={`inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-sm ${
              active ? 'bg-primary-50 text-primary-700' : 'text-ink-2 hover:bg-fill'
            }`}
          >
            <span aria-hidden="true" className={`size-1.5 rounded-full ${item.dot}`} />
            <span>{counterLabels[item.count]}</span>
            {counterNumber(item)}
          </button>
        )
        return countable ? (
          button
        ) : (
          <Tooltip key={item.count} title={t`Pick a period or a filter to count`}>
            {button}
          </Tooltip>
        )
      })}
      <span className="ml-auto text-xs text-ink-3">
        {periodLabels[period]}
        {timezone ? ` · ${timezone}` : ''}
      </span>
    </div>
  )

  // Table
  const originLabel = (origin: MessageOrigin) => i18n._(MESSAGE_ORIGIN_LABELS[origin])
  const messageName = (message: MessageHistory): ReactNode => {
    const origin = messageOrigin(message)
    if (origin === 'broadcast' && message.broadcast_id) {
      const broadcast = broadcasts.get(message.broadcast_id)
      if (!broadcast) return <Mono>{message.broadcast_id}</Mono>
      const audience = broadcast.audience?.list
      const audienceName = audience ? (listName(audience) ?? audience) : undefined
      return audienceName ? (
        <Tooltip title={t`Sent to ${audienceName}`}>
          <span className="truncate">{broadcast.name}</span>
        </Tooltip>
      ) : (
        <span className="truncate">{broadcast.name}</span>
      )
    }
    if (origin === 'automation' && message.automation_id) {
      const name = automationName(message.automation_id)
      return name ? <span className="truncate">{name}</span> : <Mono>{message.automation_id}</Mono>
    }
    if (origin === 'transactional') return <Mono>{message.transactional_notification_id}</Mono>
    return <Mono>{message.template_id}</Mono>
  }

  const tableColumns: ColumnsType<MessageHistory> = [
    {
      title: t`Time`,
      key: 'time',
      width: 140,
      render: (_, message) => (
        <Tooltip title={formatLogDateTime(message.created_at, timezone)}>
          <span className="text-ink-3 tabular-nums">
            {formatLogTime(i18n, message.created_at, timezone)}
          </span>
        </Tooltip>
      )
    },
    {
      title: columnTitles.recipient,
      key: 'recipient',
      hidden: !columns.recipient,
      render: (_, message) =>
        message.contact_email === ERASED_EMAIL ? (
          <span className="text-ink-3">{t`Contact erased`}</span>
        ) : (
          <span className="truncate">{message.contact_email}</span>
        )
    },
    {
      title: t`Message`,
      key: 'message',
      render: (_, message) => {
        const origin = messageOrigin(message)
        return (
          <span className="flex min-w-0 items-center gap-2">
            <Pill tone={ORIGIN_TONES[origin]} dot={false}>
              {originLabel(origin)}
            </Pill>
            {messageName(message)}
          </span>
        )
      }
    },
    {
      title: t`Status`,
      key: 'status',
      render: (_, message) => {
        const status = messageStatus(message)
        return <Pill tone={status.tone}>{i18n._(MESSAGE_STATUS_LABELS[status.key])}</Pill>
      }
    },
    {
      title: t`Journey`,
      key: 'journey',
      render: (_, message) => (
        <Journey message={message} smtp={rowIsSmtp(message, workspace)} timezone={timezone} />
      )
    },
    {
      title: columnTitles.error,
      key: 'error',
      hidden: !columns.error,
      render: (_, message) =>
        message.status_info ? (
          <Tooltip title={message.status_info}>
            <span className="block max-w-[240px] truncate text-warning-fg">
              {message.status_info}
            </span>
          </Tooltip>
        ) : message.complained_at ? (
          <span className="text-warning-fg">{t`Marked as spam`}</span>
        ) : (
          <span className="text-ink-4">—</span>
        )
    },
    {
      title: columnTitles.id,
      key: 'id',
      hidden: !columns.id,
      render: (_, message) => <Mono copy>{message.id}</Mono>
    },
    {
      title: columnTitles.external_id,
      key: 'external_id',
      hidden: !columns.external_id,
      render: (_, message) =>
        message.external_id ? (
          <Mono>{message.external_id}</Mono>
        ) : (
          <span className="text-ink-4">—</span>
        )
    },
    {
      title: columnTitles.metadata,
      key: 'metadata',
      hidden: !columns.metadata,
      render: (_, message) => <MetadataCell metadata={message.message_data?.metadata} />
    },
    {
      title: columnTitles.template,
      key: 'template',
      hidden: !columns.template,
      render: (_, message) => (
        <span className="flex items-center gap-1">
          <Mono>{message.template_id}</Mono>
          <span className="text-xs text-ink-3">v{message.template_version}</span>
        </span>
      )
    },
    {
      title: columnTitles.list,
      key: 'list',
      hidden: !columns.list,
      render: (_, message) =>
        message.list_id ? (
          (listName(message.list_id) ?? <Mono>{message.list_id}</Mono>)
        ) : (
          <span className="text-ink-4">—</span>
        )
    },
    {
      title: columnTitles.created,
      key: 'created',
      hidden: !columns.created,
      render: (_, message) => (
        <span className="text-ink-3">{relative(message.created_at, new Date(now))}</span>
      )
    },
    {
      key: 'open',
      width: 32,
      render: () => <ChevronRight size={14} className="text-ink-4" aria-hidden="true" />
    }
  ]

  const openMessage = (id: string) => update({ message: id }, { replace: true })

  let body
  if (listQuery.isPending) {
    body = (
      <div className="p-3.5">
        <Skeleton active title={false} paragraph={{ rows: 8 }} />
      </div>
    )
  } else if (listQuery.isError) {
    body = (
      <Result
        status="error"
        title={t`Couldn't load the messages`}
        subTitle={listQuery.error instanceof Error ? listQuery.error.message : undefined}
        extra={<Button onClick={() => void listQuery.refetch()}>{t`Retry`}</Button>}
      />
    )
  } else if (rows.length === 0) {
    const showAll = <Button onClick={() => update({ period: 'all' })}>{t`Show all time`}</Button>
    body = (
      <div className="py-10">
        {hasMessageFilters(search) ? (
          <Empty description={t`No results match`}>
            <Button onClick={clearFilters}>{t`Clear filters`}</Button>
          </Empty>
        ) : period === 'all' ? (
          <Empty description={t`No messages sent yet`} />
        ) : search.period === undefined ? (
          <Empty description={t`Nothing in the last 24 hours`}>{showAll}</Empty>
        ) : (
          <Empty description={t`Nothing in this period`}>{showAll}</Empty>
        )}
      </div>
    )
  } else {
    body = (
      <Table<MessageHistory>
        size="small"
        columns={tableColumns}
        dataSource={rows}
        rowKey="id"
        pagination={false}
        scroll={{ x: 'max-content' }}
        rowClassName={(message) =>
          message.id === search.message ? 'cursor-pointer is-hover' : 'cursor-pointer'
        }
        onRow={(message) => ({
          onClick: () => openMessage(message.id),
          onKeyDown: (event) => {
            if (event.key === 'Enter') openMessage(message.id)
          },
          tabIndex: 0
        })}
      />
    )
  }

  // The total the list will reach: the count of the counter shown active, when there is one.
  const total =
    countable &&
    counter &&
    countQuery.data &&
    !(counter.count === 'delivered' && !deliveriesReported)
      ? countQuery.data[counter.count]
      : undefined
  const shown = int(rows.length)
  const footer =
    rows.length > 0 ? (
      <div className="flex items-center justify-between">
        <span className="text-sm text-ink-3">
          {total !== undefined
            ? plural(total, { one: `${shown} of # message`, other: `${shown} of # messages` })
            : plural(rows.length, { one: `${shown} message`, other: `${shown} messages` })}
        </span>
        {listQuery.hasNextPage ? (
          <Button
            size="small"
            loading={listQuery.isFetchingNextPage}
            onClick={() => void listQuery.fetchNextPage()}
          >
            {t`Load more`}
          </Button>
        ) : null}
      </div>
    ) : undefined

  const openIndex = rows.findIndex((message) => message.id === search.message)

  return (
    <>
      <Panel flush footer={footer}>
        {toolbar}
        {counters}
        {body}
      </Panel>
      <MessageDetail
        workspaceId={workspaceId}
        permissions={permissions}
        id={search.message}
        row={openIndex >= 0 ? rows[openIndex] : undefined}
        names={{ broadcast: broadcastName, automation: automationName, list: listName }}
        nav={
          openIndex >= 0
            ? {
                position: openIndex + 1,
                total: rows.length,
                onPrev: openIndex > 0 ? () => openMessage(rows[openIndex - 1].id) : undefined,
                onNext:
                  openIndex < rows.length - 1
                    ? () => openMessage(rows[openIndex + 1].id)
                    : undefined
              }
            : { buttons: false }
        }
        onClose={() => update({ message: undefined }, { replace: true })}
      />
    </>
  )
}

interface JourneyProps {
  message: MessageHistory
  smtp: boolean
  timezone?: string
}

/** Sent, delivered, opened, clicked: lit when it happened, toned when it went wrong. */
function Journey({ message, smtp, timezone }: JourneyProps) {
  const { t } = useLingui()
  const when = (value?: string) => (value ? formatLogDateTime(value, timezone) : undefined)
  const steps: {
    key: string
    icon: typeof Send
    label: string
    at?: string
    state: 'on' | 'off' | 'danger' | 'warning'
    off: string
  }[] = [
    {
      key: 'sent',
      icon: Send,
      label: message.failed_at ? t`Failed` : t`Sent`,
      at: message.failed_at ?? message.sent_at,
      state: message.failed_at ? 'danger' : 'on',
      off: t`Not sent`
    },
    {
      key: 'delivered',
      icon: CircleCheck,
      label: message.bounced_at ? t`Bounced` : t`Delivered`,
      at: message.bounced_at ?? message.delivered_at,
      state: message.bounced_at ? 'warning' : message.delivered_at ? 'on' : 'off',
      off: smtp ? t`Delivery isn't reported by SMTP providers` : t`Not delivered`
    },
    {
      key: 'opened',
      icon: Eye,
      label: message.complained_at ? t`Marked as spam` : t`Opened`,
      at: message.complained_at ?? message.opened_at,
      state: message.complained_at ? 'warning' : message.opened_at ? 'on' : 'off',
      off: t`Not opened`
    },
    {
      key: 'clicked',
      icon: MousePointerClick,
      label: t`Clicked`,
      at: message.clicked_at,
      state: message.clicked_at ? 'on' : 'off',
      off: t`Not clicked`
    }
  ]
  const TONES = {
    on: 'text-primary',
    off: 'text-ink-4',
    danger: 'text-danger-fg',
    warning: 'text-warning-fg'
  }
  return (
    <span className="flex items-center gap-1.5">
      {steps.map((step) => {
        const Icon = step.icon
        const title = step.state === 'off' ? step.off : `${step.label} · ${when(step.at) ?? ''}`
        return (
          <Tooltip key={step.key} title={title}>
            <span data-step={step.key} data-state={step.state} aria-label={title} role="img">
              <Icon size={18} className={TONES[step.state]} aria-hidden="true" />
            </span>
          </Tooltip>
        )
      })}
    </span>
  )
}

const METADATA_SHOWN = 2

// Metadata is any JSON map the send API received: a value that is not text is serialised.
const metadataText = (value: unknown): string => {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return JSON.stringify(value)
}

function MetadataCell({ metadata }: { metadata?: Record<string, unknown> }) {
  const entries = Object.entries(metadata ?? {})
  if (entries.length === 0) return <span className="text-ink-4">—</span>
  const shown = entries.slice(0, METADATA_SHOWN)
  const rest = entries.length - shown.length
  return (
    <Tooltip
      title={
        <div>
          {entries.map(([key, value]) => (
            <div key={key} className="break-all">
              {key}: {metadataText(value)}
            </div>
          ))}
        </div>
      }
    >
      <span className="flex items-center gap-1">
        {shown.map(([key, value]) => (
          <span
            key={key}
            className="max-w-[160px] truncate rounded-sm bg-fill px-1.5 text-xs text-ink-2"
          >
            {key}: {metadataText(value)}
          </span>
        ))}
        {rest > 0 ? (
          <span className="rounded-sm bg-fill px-1.5 text-xs text-ink-2">+{rest}</span>
        ) : null}
      </span>
    </Tooltip>
  )
}
