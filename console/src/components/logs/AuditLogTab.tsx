import { useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  App,
  Button,
  Dropdown,
  Empty,
  Input,
  Result,
  Select,
  Skeleton,
  Table,
  Tooltip,
  type MenuProps
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Link } from '@tanstack/react-router'
import { ChevronDown, ChevronRight, Download, RefreshCw } from 'lucide-react'
import dayjs from '../../lib/dayjs'
import { downloadBlob } from '../../lib/download'
import { useAuth } from '../../contexts/AuthContext'
import {
  auditLogApi,
  type AuditExportFormat,
  type AuditLog,
  type AuditScope
} from '../../services/api/audit_log'
import { membersQuery as workspaceMembersQuery } from '../../services/api/workspaceQueries'
import { LicenceGateNotice } from '../license/LicenceGateNotice'
import { Mono, Panel, Pill, TableToolbar, useNumberFormat } from '../layout'
import { AuditActor } from './AuditActor'
import { AuditEntryDetail } from './AuditEntryDetail'
import { AuditPhrase } from './AuditPhrase'
import { OUTCOME_TONES, useOutcomeLabel } from './auditDisplay'
import {
  auditListParams,
  auditScope,
  auditSearchKey,
  auditSearchShown,
  hasAuditFilters
} from './auditSearch'
import { FilterChip } from './FilterChip'
import { PeriodChip } from './PeriodChip'
import { AUDIT_CATEGORY_LABELS } from './labels'
import { AUDIT_ACTOR_TYPES, AUDIT_OUTCOMES, type AuditPeriod, type LogsSearch } from './logsSearch'
import { formatLogDateTime, formatLogTime } from './logTime'
import { defaultAuditPeriod } from './period'
import { useLogsUrl } from './useLogsUrl'
import { TextFilter } from './TextFilter'
import { useDebouncedUrlValue } from '../../lib/useDebouncedUrlValue'

const PAGE_SIZE = 50

type AddedFilter = 'actions' | 'outcome' | 'actor_email' | 'target_id' | 'ip_address'

export interface AuditLogTabProps {
  workspaceId: string
  timezone?: string
  // A root user also gets the deployment-wide view: every workspace plus the entries that belong
  // to none (sign-ins, the licence, the system settings).
  isRoot: boolean
}

/** Logs → Audit log: who did what, as sentences, with its filters in the link. */
export function AuditLogTab({ workspaceId, timezone, isRoot }: AuditLogTabProps) {
  const { t, i18n } = useLingui()
  const { message } = App.useApp()
  const { int } = useNumberFormat()
  const queryClient = useQueryClient()
  const { user, workspaces } = useAuth()
  const outcomeLabel = useOutcomeLabel()
  const { search, update, clear } = useLogsUrl(workspaceId, 'audit')
  const scope = auditScope(search, isRoot)
  const period: AuditPeriod =
    (search.period as AuditPeriod | undefined) ?? defaultAuditPeriod(search)

  // The clock of the period: set when the filters change and on Refresh, never by a re-render,
  // so a minute passing does not change the query key and drop the loaded pages.
  const filtersKey = JSON.stringify({ ...search, id: undefined })
  const [clock, setClock] = useState(() => ({ key: filtersKey, now: Date.now() }))
  if (clock.key !== filtersKey) setClock({ key: filtersKey, now: Date.now() })
  const now = clock.key === filtersKey ? clock.now : Date.now()

  const params = useMemo(
    () => auditListParams(search, { scope, workspaceId, now }),
    // `search` is new on every render; its content is the key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [filtersKey, scope, workspaceId, now]
  )

  // A1: 50 entries at a time, by cursor.
  const listQuery = useInfiniteQuery({
    queryKey: ['audit-logs', scope, workspaceId, params],
    queryFn: ({ pageParam }) =>
      auditLogApi.list({ ...params, cursor: pageParam, limit: PAGE_SIZE }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => (last.has_more ? last.next_cursor : undefined),
    staleTime: 30_000,
    refetchOnWindowFocus: false
  })
  const rows = useMemo(
    () => listQuery.data?.pages.flatMap((page) => page.logs) ?? [],
    [listQuery.data]
  )

  // A3: the catalogue, for the Resource and Action choices.
  const catalogueQuery = useQuery({
    queryKey: ['audit-log-actions'],
    queryFn: () => auditLogApi.actions(),
    staleTime: Infinity
  })

  // A4: the members and API keys, for the Actor choices. A member's access already loads it under
  // this key; a root, whose access skips it, loads it here in workspace view.
  const membersQuery = useQuery({
    ...workspaceMembersQuery(workspaceId, user?.id),
    enabled: scope === 'workspace',
    staleTime: 60_000
  })
  const members = membersQuery.data?.members ?? []
  const people = members.filter((member) => member.type === 'user' && member.user_id)
  const apiKeys = members.filter((member) => member.type === 'api_key')
  const me = members.find((member) => member.user_id === user?.id)
  const isOwner = me?.role === 'owner'

  // Refresh moves the clock. When the window moves with it (a rolling period, minutes later), the
  // new key fetches on its own; resetting too would refetch the old key still on screen. When it
  // stays (all time, a custom range, the same minute), the key is the same and is reset.
  const refresh = () => {
    const next = Date.now()
    const sameWindow =
      JSON.stringify(auditListParams(search, { scope, workspaceId, now: next })) ===
      JSON.stringify(params)
    setClock({ key: filtersKey, now: next })
    // The list only: an open entry does not change, and its key is not this one
    if (sameWindow) void queryClient.resetQueries({ queryKey: ['audit-logs', scope, workspaceId] })
  }

  const exportAs = async (format: AuditExportFormat) => {
    try {
      const blob = await auditLogApi.export(params, format)
      downloadBlob(
        blob,
        `audit-logs-${scope === 'workspace' ? workspaceId : scope}-${dayjs().format('YYYYMMDD')}.${format}`
      )
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`The export failed`)
    }
  }

  // The search box fills one of its parameters; the others stay as "+ Filter" set them.
  const shownKey = auditSearchShown(search)
  const [searchText, setSearchText] = useDebouncedUrlValue(
    shownKey ? (search[shownKey] ?? '') : '',
    (value) => {
      const key = value ? auditSearchKey(value) : undefined
      update(
        {
          ...(shownKey ? { [shownKey]: undefined } : {}),
          ...(key ? { [key]: key === 'actor_email' ? value.toLowerCase() : value } : {})
        },
        { replace: true }
      )
    }
  )
  // The hint names the parameter the box shows, until what is typed there differs from it.
  const shownText = shownKey ? (search[shownKey] ?? '') : ''
  const hintKey =
    searchText === shownText ? shownKey : searchText.trim() ? auditSearchKey(searchText) : undefined
  const searchHints: Record<string, string> = {
    ip_address: t`Searching by IP address`,
    actor_email: t`Searching by actor email`,
    target_id: t`Searching by target ID`
  }

  // Actor
  const actorTypeLabels: Record<(typeof AUDIT_ACTOR_TYPES)[number], string> = {
    user: t`Any member`,
    api_key: t`Any API key`,
    system: t`System`,
    anonymous: t`Signed-out visitor`
  }
  const memberLabel = (member: (typeof members)[number]) =>
    member.user_id === user?.id
      ? t`Me`
      : member.name
        ? `${member.name} (${member.email})`
        : member.email
  const actorItems: MenuProps['items'] = [
    { key: 'all', label: t`All` },
    ...(scope === 'workspace'
      ? people.map((member) => ({ key: `id:${member.user_id}`, label: memberLabel(member) }))
      : []),
    { key: 'type:api_key', label: actorTypeLabels.api_key },
    ...(scope === 'workspace'
      ? apiKeys.map((key) => ({ key: `id:${key.user_id}`, label: key.name || key.email }))
      : []),
    { key: 'type:system', label: actorTypeLabels.system },
    { key: 'type:anonymous', label: actorTypeLabels.anonymous }
  ]
  const actorValue = search.actor_id
    ? (() => {
        const member = members.find((candidate) => candidate.user_id === search.actor_id)
        return member
          ? member.type === 'api_key'
            ? member.name || member.email
            : memberLabel(member)
          : search.actor_id
      })()
    : search.actor_type
      ? actorTypeLabels[search.actor_type]
      : t`All`
  const actorMenu: MenuProps = {
    items: actorItems,
    selectable: true,
    selectedKeys: [
      search.actor_id
        ? `id:${search.actor_id}`
        : search.actor_type
          ? `type:${search.actor_type}`
          : 'all'
    ],
    onClick: ({ key }) => {
      if (key.startsWith('id:')) update({ actor_id: key.slice(3), actor_type: undefined })
      else if (key.startsWith('type:'))
        update({ actor_type: key.slice(5) as LogsSearch['actor_type'], actor_id: undefined })
      else update({ actor_id: undefined, actor_type: undefined })
    }
  }

  // Resource: several categories at once.
  const categories = search.categories ? search.categories.split(',').filter(Boolean) : []
  const categoryLabel = (category: string) =>
    AUDIT_CATEGORY_LABELS[category] ? i18n._(AUDIT_CATEGORY_LABELS[category]) : category
  const setCategories = (next: string[]) =>
    update({ categories: next.length ? next.join(',') : undefined })
  const resourceMenu: MenuProps = {
    items: (catalogueQuery.data?.categories ?? []).map((category) => ({
      key: category,
      label: categoryLabel(category)
    })),
    selectable: true,
    multiple: true,
    selectedKeys: categories,
    onSelect: ({ selectedKeys }) => setCategories(selectedKeys),
    onDeselect: ({ selectedKeys }) => setCategories(selectedKeys)
  }
  const resourceValue =
    categories.length === 0
      ? t`All`
      : categories.length === 1
        ? categoryLabel(categories[0])
        : plural(categories.length, { one: '# resource', other: '# resources' })

  // + Filter
  const [adding, setAdding] = useState<AddedFilter>()
  const addedLabels: Record<AddedFilter, string> = {
    actions: t`Action`,
    outcome: t`Outcome`,
    actor_email: t`Actor email`,
    target_id: t`Target ID`,
    ip_address: t`IP address`
  }
  const added = (Object.keys(addedLabels) as AddedFilter[]).filter(
    (key) => search[key] !== undefined || adding === key
  )
  const addMenu: MenuProps = {
    items: (Object.keys(addedLabels) as AddedFilter[])
      .filter((key) => !added.includes(key))
      .map((key) => ({ key, label: addedLabels[key] })),
    onClick: ({ key }) => setAdding(key as AddedFilter)
  }
  const actionOptions = useMemo(() => {
    const byCategory = new Map<string, { label: string; value: string }[]>()
    for (const entry of catalogueQuery.data?.actions ?? []) {
      const options = byCategory.get(entry.category) ?? []
      options.push({ label: entry.action, value: entry.action })
      byCategory.set(entry.category, options)
    }
    return Array.from(byCategory.entries()).map(([category, options]) => ({
      label: AUDIT_CATEGORY_LABELS[category] ? i18n._(AUDIT_CATEGORY_LABELS[category]) : category,
      title: category,
      options
    }))
  }, [catalogueQuery.data, i18n])

  const addedChip = (key: AddedFilter) => {
    const value = search[key]
    const close = (open: boolean) => {
      if (!open && adding === key) setAdding(undefined)
    }
    const clearIt = () => {
      setAdding(undefined)
      update({ [key]: undefined })
    }
    if (key === 'outcome') {
      return (
        <FilterChip
          key={key}
          label={addedLabels[key]}
          value={search.outcome ? outcomeLabel(search.outcome) : t`Any`}
          defaultOpen={adding === key}
          onOpenChange={close}
          onClear={clearIt}
          menu={{
            items: AUDIT_OUTCOMES.map((outcome) => ({
              key: outcome,
              label: outcomeLabel(outcome)
            })),
            selectable: true,
            selectedKeys: search.outcome ? [search.outcome] : [],
            onClick: ({ key: outcome }) => {
              setAdding(undefined)
              update({ outcome: outcome as LogsSearch['outcome'] })
            }
          }}
        />
      )
    }
    if (key === 'actions') {
      const actions = search.actions ? search.actions.split(',').filter(Boolean) : []
      return (
        <FilterChip
          key={key}
          label={addedLabels[key]}
          value={
            actions.length === 0
              ? t`Any`
              : actions.length === 1
                ? actions[0]
                : plural(actions.length, { one: '# action', other: '# actions' })
          }
          defaultOpen={adding === key}
          onOpenChange={close}
          onClear={clearIt}
          content={
            <Select
              mode="multiple"
              allowClear
              showSearch
              style={{ width: 320 }}
              placeholder={t`Choose actions`}
              value={actions}
              options={actionOptions}
              onChange={(next: string[]) =>
                update({ actions: next.length ? next.join(',') : undefined })
              }
              maxTagCount="responsive"
            />
          }
        />
      )
    }
    return (
      <FilterChip
        key={key}
        label={addedLabels[key]}
        value={value ?? t`Any`}
        defaultOpen={adding === key}
        onOpenChange={close}
        onClear={clearIt}
        content={
          <TextFilter
            initial={value ?? ''}
            placeholder={key === 'actor_email' ? t`Part of an email` : t`Exact value`}
            onApply={(text) => {
              setAdding(undefined)
              update({ [key]: text || undefined })
            }}
          />
        }
      />
    )
  }

  const filtered = hasAuditFilters(search)
  const scopeLabels: Record<AuditScope, string> = {
    workspace: t`This workspace`,
    deployment: t`Whole deployment`
  }
  // The view (scope) and the period are not filters: clearing keeps them.
  const clearFilters = () => clear(['scope', 'period', 'from', 'to'])
  const toolbar = (
    <TableToolbar
      start={
        <>
          <div className="flex flex-col">
            <Tooltip
              title={t`An IP address and a target ID must match exactly; an actor email matches any part of it.`}
            >
              <Input
                allowClear
                aria-label={t`Search the audit log`}
                placeholder={t`IP address, actor email or target ID`}
                value={searchText}
                onChange={(event) => setSearchText(event.target.value)}
                style={{ width: 260 }}
              />
            </Tooltip>
          </div>
          {hintKey ? <span className="text-xs text-ink-3">{searchHints[hintKey]}</span> : null}
          {/* Actor, Resource, then Period, the mockup's order */}
          <FilterChip label={t`Actor`} value={actorValue} menu={actorMenu} />
          <FilterChip label={t`Resource`} value={resourceValue} menu={resourceMenu} />
          {added.map(addedChip)}
          {addMenu.items && addMenu.items.length > 0 ? (
            <FilterChip label={t`+ Filter`} dashed menu={addMenu} />
          ) : null}
          {filtered ? (
            <Button type="link" onClick={clearFilters}>
              {t`Clear all`}
            </Button>
          ) : null}
        </>
      }
      end={
        <>
          {/* The period sits on the right, with the icons: the filters keep the left. */}
          <PeriodChip period={period} search={search} timezone={timezone} onChange={update} />
          {isRoot ? (
            // A menu rather than two buttons, so the toolbar keeps to one line at 1440
            <FilterChip
              label={t({ message: 'View', context: 'audit log scope' })}
              value={scopeLabels[scope]}
              menu={{
                items: (Object.keys(scopeLabels) as AuditScope[]).map((key) => ({ key, label: scopeLabels[key] })),
                selectable: true,
                selectedKeys: [scope],
                onClick: ({ key }) => {
                  // The scope already shown: nothing changes, and the actor filter and open entry stay
                  if (key === scope) return
                  update({
                    scope: key === 'deployment' ? 'deployment' : undefined,
                    id: undefined,
                    actor_id: undefined
                  })
                }
              }}
            />
          ) : null}
          <Dropdown
            trigger={['click']}
            menu={{
              items: [
                { key: 'csv', label: 'CSV' },
                { key: 'ndjson', label: 'NDJSON' }
              ],
              onClick: ({ key }) => void exportAs(key as AuditExportFormat)
            }}
          >
            <Button type="text" icon={<Download size={14} />}>
              <span className="inline-flex items-center gap-1">
                {t`Export`}
                <ChevronDown size={12} aria-hidden="true" />
              </span>
            </Button>
          </Dropdown>
          <Tooltip title={t`Refresh`}>
            <Button
              type="text"
              aria-label={t`Refresh`}
              icon={<RefreshCw size={14} />}
              onClick={refresh}
            />
          </Tooltip>
        </>
      }
    />
  )

  const columns: ColumnsType<AuditLog> = [
    {
      title: t`Time`,
      key: 'time',
      width: 150,
      render: (_, entry) => (
        <Tooltip title={formatLogDateTime(entry.occurred_at, timezone)}>
          <span className="text-ink-3 tabular-nums">
            {formatLogTime(i18n, entry.occurred_at, timezone)}
          </span>
        </Tooltip>
      )
    },
    {
      title: t`Actor`,
      key: 'actor',
      width: 200,
      render: (_, entry) => <AuditActor entry={entry} />
    },
    {
      title: t`Action`,
      key: 'action',
      render: (_, entry) => (
        <span className="flex flex-wrap items-center gap-1.5 text-ink-2">
          <span>
            <AuditPhrase entry={entry} />
          </span>
          {entry.outcome !== 'success' ? (
            <Pill tone={OUTCOME_TONES[entry.outcome]}>{outcomeLabel(entry.outcome)}</Pill>
          ) : null}
        </span>
      )
    },
    {
      title: t`Resource`,
      key: 'resource',
      width: 130,
      render: (_, entry) => (
        <Pill tone="neutral" dot={false}>
          {categoryLabel(entry.category)}
        </Pill>
      )
    },
    {
      title: t`Origin`,
      key: 'origin',
      width: 140,
      render: (_, entry) =>
        entry.ip_address ? <Mono>{entry.ip_address}</Mono> : <span className="text-ink-4">—</span>
    },
    ...(scope === 'deployment'
      ? [
          {
            title: t`Workspace`,
            key: 'workspace',
            render: (_: unknown, entry: AuditLog) =>
              entry.workspace_id ? (
                <Mono>{entry.workspace_id}</Mono>
              ) : (
                <span className="text-ink-3">{t`Deployment`}</span>
              )
          }
        ]
      : []),
    {
      key: 'open',
      width: 32,
      render: () => <ChevronRight size={14} className="text-ink-4" aria-hidden="true" />
    }
  ]

  const openEntry = (id: string) => update({ id }, { replace: true })

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
        title={t`Couldn't load the audit log`}
        subTitle={listQuery.error instanceof Error ? listQuery.error.message : undefined}
        extra={<Button onClick={() => void listQuery.refetch()}>{t`Retry`}</Button>}
      />
    )
  } else if (rows.length === 0) {
    const showAll = (
      <Button
        onClick={() => update({ period: 'all', from: undefined, to: undefined })}
      >{t`Show all time`}</Button>
    )
    body = (
      <div className="py-10">
        {filtered ? (
          <Empty description={t`No results match`}>
            <Button onClick={clearFilters}>{t`Clear filters`}</Button>
          </Empty>
        ) : period === 'all' ? (
          <Empty description={t`No audit entries yet`} />
        ) : search.period === undefined ? (
          <Empty description={t`Nothing in the last 7 days`}>{showAll}</Empty>
        ) : (
          <Empty description={t`Nothing in this period`}>{showAll}</Empty>
        )}
      </div>
    )
  } else {
    body = (
      <Table<AuditLog>
        size="small"
        columns={columns}
        dataSource={rows}
        rowKey="id"
        pagination={false}
        scroll={{ x: 'max-content' }}
        rowClassName={(entry) =>
          entry.id === search.id ? 'cursor-pointer is-hover' : 'cursor-pointer'
        }
        onRow={(entry) => ({
          onClick: () => openEntry(entry.id),
          onKeyDown: (event) => {
            if (event.key === 'Enter') openEntry(entry.id)
          },
          tabIndex: 0
        })}
      />
    )
  }

  // Retention: the workspace's, which only the owner can change.
  const workspace = workspaces.find((candidate) => candidate.id === workspaceId)
  const retentionDays = workspace?.settings.audit_logs?.retention_days
  const retention =
    retentionDays === 0
      ? t`Kept forever`
      : typeof retentionDays === 'number'
        ? plural(retentionDays, { one: 'Kept # day', other: 'Kept # days' })
        : t`Kept for the deployment default`

  const shown = rows.length
  const footer =
    rows.length > 0 || scope === 'workspace' ? (
      <div className="flex w-full items-center justify-between gap-3">
        <span className="flex items-center gap-3">
          {rows.length > 0 ? (
            <span className="text-sm text-ink-3">
              {plural(shown, { one: `${int(shown)} entry`, other: `${int(shown)} entries` })}
            </span>
          ) : null}
          {listQuery.hasNextPage ? (
            <Button
              size="small"
              loading={listQuery.isFetchingNextPage}
              onClick={() => void listQuery.fetchNextPage()}
            >
              {t`Load more`}
            </Button>
          ) : null}
        </span>
        {scope === 'workspace' ? (
          <span className="text-sm text-ink-3">
            {retention}
            {isOwner ? (
              <>
                {' · '}
                <Link
                  to="/console/workspace/$workspaceId/settings/$section"
                  params={{ workspaceId, section: 'audit-logs' }}
                >
                  {t`Change in Settings`}
                </Link>
              </>
            ) : null}
          </span>
        ) : null}
      </div>
    ) : undefined

  const openIndex = rows.findIndex((entry) => entry.id === search.id)

  return (
    <div className="flex flex-col gap-3">
      <LicenceGateNotice feature="audit_logs" workspaceId={workspaceId} />
      <Panel flush footer={footer}>
        {toolbar}
        {body}
      </Panel>
      <AuditEntryDetail
        workspaceId={workspaceId}
        scope={scope}
        id={search.id}
        row={openIndex >= 0 ? rows[openIndex] : undefined}
        timezone={timezone}
        nav={
          openIndex >= 0
            ? {
                position: openIndex + 1,
                total: rows.length,
                onPrev: openIndex > 0 ? () => openEntry(rows[openIndex - 1].id) : undefined,
                onNext:
                  openIndex < rows.length - 1 ? () => openEntry(rows[openIndex + 1].id) : undefined
              }
            : { buttons: false }
        }
        onClose={() => update({ id: undefined }, { replace: true })}
      />
    </div>
  )
}
