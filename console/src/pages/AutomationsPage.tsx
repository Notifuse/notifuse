import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Empty, Input, Result, Skeleton, Tooltip } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Plus, Search } from 'lucide-react'
import { CountTabs, PageHeader, Panel, TableToolbar, type CountTabOption } from '../components/layout'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { automationApi, type Automation } from '../services/api/automation'
import { listsApi } from '../services/api/list'
import { listSegments } from '../services/api/segment'
import { templatesApi } from '../services/api/template'
import type { AutomationsSearch, AutomationStatusFilter } from '../router'
import { AutomationsTable } from '../components/automations/list/AutomationsTable'
import { automationsCountLabel } from '../components/automations/list/countLabel'
import { useAutomationActions } from '../components/automations/list/useAutomationActions'
import { useAutomationOpenRates } from '../components/automations/list/useAutomationOpenRates'
import { AutomationEditor } from '../components/automations/AutomationEditor'
import { describeTrigger, type TriggerNames } from '../components/automations/describeTrigger'

type StatusTab = 'all' | AutomationStatusFilter

const SEARCH_DELAY_MS = 300
const PAGE_STEP = 50
const ROUTE = '/console/workspace/$workspaceId/automations'

/** Case- and accent-insensitive: "Bienvenue" finds "bienvenué". */
const fold = (text: string) =>
  text
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .toLowerCase()

export function AutomationsPage() {
  const { t, i18n } = useLingui()
  const { workspaceId } = useParams({ from: ROUTE })
  const navigate = useNavigate({ from: ROUTE })
  const search = useSearch({ from: ROUTE }) as AutomationsSearch
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const workspace = workspaces.find((w) => w.id === workspaceId)

  const canWrite = !!permissions?.automations?.write
  const canReadHistory = !!permissions?.message_history?.read
  const canReadTemplates = !!permissions?.templates?.read
  const noWrite = t`You don't have write permission for automations`

  const statusTab: StatusTab = search.status ?? 'all'
  const searchTerm = search.q ?? ''

  // A new automation needs write: the URL forgets it rather than opening an editor that cannot save.
  useEffect(() => {
    if (search.automation === 'new' && permissions && !canWrite) {
      navigate({ replace: true, search: (prev) => ({ ...prev, automation: undefined, stats: undefined }) })
    }
  }, [search.automation, permissions, canWrite, navigate])

  // Search box: typing stays responsive, the URL follows after a pause; the last value written lets
  // the URL-to-box sync react only to back/forward and links.
  const [searchInput, setSearchInput] = useState(searchTerm)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const [lastWrittenQ, setLastWrittenQ] = useState<string | undefined>(undefined)
  const [seenQ, setSeenQ] = useState(searchTerm)
  if (searchTerm !== seenQ) {
    setSeenQ(searchTerm)
    if (searchTerm !== lastWrittenQ) setSearchInput(searchTerm)
  }
  // A link or back/forward wins over a pending write.
  useEffect(() => {
    if (searchTerm !== lastWrittenQ) clearTimeout(debounceRef.current)
  }, [searchTerm, lastWrittenQ])
  useEffect(() => () => clearTimeout(debounceRef.current), [])

  const handleSearchChange = (value: string) => {
    setSearchInput(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      const trimmed = value.trim()
      setLastWrittenQ(trimmed)
      navigate({ replace: true, search: (prev) => ({ ...prev, q: trimmed || undefined }) })
    }, SEARCH_DELAY_MS)
  }
  const handleClearFilters = () => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    setLastWrittenQ('')
    setSearchInput('')
    navigate({ search: (prev) => ({ ...prev, status: undefined, q: undefined }) })
  }

  // A1: every automation with its exact journey counts.
  const list = useQuery({
    queryKey: ['automations', workspaceId],
    queryFn: () => automationApi.list({ workspace_id: workspaceId, with_journey_counts: true }),
    staleTime: 30_000
  })

  const listsQuery = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId })
  })
  const segmentsQuery = useQuery({
    queryKey: ['segments', workspaceId],
    queryFn: () => listSegments({ workspace_id: workspaceId })
  })
  // Every email template the step picker allows: the picker is category-agnostic, so a category
  // filter here would leave a non-marketing template (a welcome email) unnamed on the canvas.
  // Without Templates read it answers 403: the canvas then names templates by ID.
  const templatesQuery = useQuery({
    queryKey: ['templates', workspaceId, 'email'],
    queryFn: () => templatesApi.list({ workspace_id: workspaceId, channel: 'email' }),
    enabled: canReadTemplates
  })
  const lists = useMemo(() => listsQuery.data?.lists ?? [], [listsQuery.data])
  const segments = useMemo(() => segmentsQuery.data?.segments ?? [], [segmentsQuery.data])
  const templates = useMemo(() => templatesQuery.data?.templates ?? [], [templatesQuery.data])
  const names: TriggerNames = useMemo(() => ({ lists, segments, templates }), [lists, segments, templates])

  const all = useMemo(() => list.data?.automations ?? [], [list.data])
  const counts = list.data?.journey_counts

  // The search reads the name and the trigger's label; the tabs count what it keeps.
  const searched = useMemo(() => {
    const needle = fold(searchTerm.trim())
    if (!needle) return all
    return all.filter((a) => {
      const label = describeTrigger(a.trigger, names)
      const haystack = [a.name, label ? i18n._(label.message) : '', label?.mono ? String(label.message.values?.[label.mono] ?? '') : '']
      return haystack.some((text) => fold(text).includes(needle))
    })
  }, [all, searchTerm, names, i18n])
  const byStatus = (status: AutomationStatusFilter) => searched.filter((a) => a.status === status).length
  const tabs: CountTabOption<StatusTab>[] = [
    { value: 'all', label: t`All`, count: searched.length },
    { value: 'live', label: t`Live`, count: byStatus('live') },
    { value: 'paused', label: t`Paused`, count: byStatus('paused') },
    { value: 'draft', label: t`Drafts`, count: byStatus('draft') }
  ]
  const filtered = statusTab === 'all' ? searched : searched.filter((a) => a.status === statusTab)

  // 50 rows, then 50 more on demand; a new filter starts over.
  const [shownCount, setShownCount] = useState(PAGE_STEP)
  const filterKey = `${statusTab}|${searchTerm}`
  const [lastFilterKey, setLastFilterKey] = useState(filterKey)
  if (filterKey !== lastFilterKey) {
    setLastFilterKey(filterKey)
    setShownCount(PAGE_STEP)
  }
  const rows = filtered.slice(0, shownCount)
  const rest = filtered.length - rows.length

  const openRates = useAutomationOpenRates(workspace, all, canReadHistory)
  const actions = useAutomationActions(workspaceId)

  const open = (a: Automation, withStats: boolean) =>
    navigate({ search: (prev) => ({ ...prev, automation: a.id, stats: withStats ? 1 : undefined }) })
  const openNew = () => navigate({ search: (prev) => ({ ...prev, automation: 'new', stats: undefined }) })
  const viewMessages = (id: string) =>
    navigate({ to: '/console/workspace/$workspaceId/logs', params: { workspaceId }, search: { automation_id: id } })

  const newButton = (
    <Tooltip title={canWrite ? undefined : noWrite}>
      <span className="inline-flex">
        <Button type="primary" icon={<Plus size={14} />} disabled={!canWrite} onClick={openNew}>
          {t`New automation`}
        </Button>
      </span>
    </Tooltip>
  )

  // The editor keeps its session while `new` becomes the created id; any other change of
  // automation opens a new one.
  const [created, setCreated] = useState<string | null>(null)
  const [session, setSession] = useState({ id: search.automation, key: 0 })
  if (search.automation !== session.id) {
    const fromCreate = session.id === 'new' && search.automation === created
    setSession({ id: search.automation, key: fromCreate ? session.key : session.key + 1 })
  }

  if (!workspace) return null

  const workspaceEmpty = list.isSuccess && all.length === 0
  const footer =
    filtered.length > 0 ? (
      <>
        <span>{automationsCountLabel(filtered.length)}</span>
        {rest > 0 ? (
          <Button size="small" onClick={() => setShownCount((n) => n + PAGE_STEP)}>
            {plural(Math.min(rest, PAGE_STEP), { one: 'Show # more', other: 'Show # more' })}
          </Button>
        ) : null}
      </>
    ) : undefined

  const editorId = search.automation
  return (
    <div className="flex flex-col gap-3">
      <PageHeader title={t`Automations`} count={list.isSuccess ? all.length : undefined} actions={workspaceEmpty ? undefined : newButton} />

      {list.isError && !list.data ? (
        <Result
          status="error"
          title={t`Couldn't load the automations`}
          extra={<Button onClick={() => void list.refetch()}>{t`Retry`}</Button>}
        />
      ) : workspaceEmpty ? (
        <Panel>
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <span className="flex flex-col gap-1">
                <span className="text-base font-medium text-ink">{t`No automations yet`}</span>
                <span className="text-sm text-ink-3">{t`Send emails and update lists when contacts do something`}</span>
              </span>
            }
          >
            <Tooltip title={canWrite ? undefined : noWrite}>
              <span className="inline-flex">
                <Button type="primary" icon={<Plus size={14} />} disabled={!canWrite} onClick={openNew}>
                  {t`Create your first automation`}
                </Button>
              </span>
            </Tooltip>
          </Empty>
        </Panel>
      ) : (
        <Panel flush footer={footer}>
          <TableToolbar
            start={
              <CountTabs<StatusTab>
                options={tabs}
                value={statusTab}
                onChange={(value) => navigate({ search: (prev) => ({ ...prev, status: value === 'all' ? undefined : value }) })}
              />
            }
            end={
              <div className="flex items-center gap-3">
                <span className="hidden text-xs text-ink-3 xl:inline">{t`Journeys since creation · open rate last 30 days`}</span>
                <div className="w-64">
                  <Input
                    allowClear
                    size="small"
                    aria-label={t`Search by name or trigger`}
                    placeholder={t`Search by name or trigger`}
                    prefix={<Search size={14} aria-hidden="true" />}
                    value={searchInput}
                    onChange={(e) => handleSearchChange(e.target.value)}
                  />
                </div>
              </div>
            }
          />
          {list.isLoading ? (
            <div className="px-3.5 py-3" data-list-loading>
              <Skeleton active title={false} paragraph={{ rows: 6 }} />
            </div>
          ) : filtered.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={<span className="text-sm text-ink-3">{t`No automations match`}</span>}>
              <Button onClick={handleClearFilters}>{t`Clear filters`}</Button>
            </Empty>
          ) : (
            <AutomationsTable
              rows={rows}
              counts={counts}
              openRates={openRates}
              names={names}
              canWrite={canWrite}
              canReadHistory={canReadHistory}
              activeId={editorId}
              onOpen={open}
              onActivate={(a) => void actions.activate(a.id)}
              onPause={(a) => void actions.pause(a.id)}
              onDelete={(a) => void actions.remove(a.id)}
              onViewMessages={(a) => viewMessages(a.id)}
            />
          )}
        </Panel>
      )}

      {editorId && (editorId !== 'new' || canWrite) ? (
        <AutomationEditor
          key={session.key}
          workspace={workspace}
          automationParam={editorId}
          withStats={search.stats === 1}
          canWrite={canWrite}
          canReadHistory={canReadHistory}
          canReadTemplates={canReadTemplates}
          lists={lists}
          segments={segments}
          templates={templates}
          journeyTotal={editorId !== 'new' ? counts?.[editorId]?.total : undefined}
          // The editor has already asked: the route's blocker must not ask again.
          onClose={() => navigate({ ignoreBlocker: true, search: (prev) => ({ ...prev, automation: undefined, stats: undefined }) })}
          onCreated={(id) => {
            setCreated(id)
            navigate({ replace: true, ignoreBlocker: true, search: (prev) => ({ ...prev, automation: id }) })
          }}
          onToggleStats={(on) => navigate({ replace: true, search: (prev) => ({ ...prev, stats: on ? 1 : undefined }) })}
          onViewMessages={viewMessages}
        />
      ) : null}
    </div>
  )
}

export default AutomationsPage
