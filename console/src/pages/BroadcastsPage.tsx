import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Empty, Input, Result, Skeleton, Tooltip } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Plus, Search } from 'lucide-react'
import { CountTabs, FullDrawer, PageHeader, Panel, TableToolbar, useNumberFormat, type CountTabOption } from '../components/layout'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { BROADCAST_STATUS_GROUPS, type Broadcast } from '../services/api/broadcast'
import { listsApi } from '../services/api/list'
import { listSegments } from '../services/api/segment'
import type { BroadcastsSearch } from '../router'
import { BroadcastKpis } from '../components/broadcasts/BroadcastKpis'
import { BroadcastsTable } from '../components/broadcasts/BroadcastsTable'
import { DeleteBroadcastModal } from '../components/broadcasts/DeleteBroadcastModal'
import { BroadcastDetail } from '../components/broadcasts/detail/BroadcastDetail'
import { BroadcastCompose } from '../components/broadcasts/compose/BroadcastCompose'
import { canReadWebAnalytics } from '../components/broadcasts/access'
import { useAudienceEstimates } from '../components/broadcasts/useAudienceEstimates'
import { useBroadcastActions } from '../components/broadcasts/useBroadcastActions'
import { useBroadcastPageStats } from '../components/broadcasts/useBroadcastPageStats'
import { PAGE_SIZE, groupCount, useBroadcastsList } from '../components/broadcasts/useBroadcastsList'

type StatusGroup = 'all' | keyof typeof BROADCAST_STATUS_GROUPS

// Valid status groups of the URL; anything else falls back to 'all'.
const STATUS_GROUP_VALUES: StatusGroup[] = ['all', 'draft', 'scheduled', 'sending', 'sent', 'failed']
const SEARCH_DELAY_MS = 300
const ROUTE = '/console/workspace/$workspaceId/broadcasts'

export function BroadcastsPage() {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const { workspaceId } = useParams({ from: ROUTE })
  const navigate = useNavigate({ from: ROUTE })
  const search = useSearch({ from: ROUTE }) as BroadcastsSearch
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const workspace = workspaces.find((w) => w.id === workspaceId)

  const canWrite = !!permissions?.broadcasts?.write
  const canReadHistory = !!permissions?.message_history?.read
  const canReadWeb = canReadWebAnalytics(workspace, permissions)
  const noWrite = t`You don't have write permission for broadcasts`

  const statusGroup: StatusGroup =
    search.status && STATUS_GROUP_VALUES.includes(search.status as StatusGroup) ? (search.status as StatusGroup) : 'all'
  const searchTerm = search.q ?? ''
  const hasActiveFilter = statusGroup !== 'all' || searchTerm !== ''

  // An unknown status lingers in shared links while the screen reads it as All: strip it.
  useEffect(() => {
    if (search.status && !STATUS_GROUP_VALUES.includes(search.status as StatusGroup)) {
      navigate({ replace: true, search: (prev) => ({ ...prev, status: undefined }) })
    }
  }, [search.status, navigate])

  // The composer wins over the detail.
  useEffect(() => {
    if (search.compose && search.broadcast) {
      navigate({ replace: true, search: (prev) => ({ ...prev, broadcast: undefined }) })
    }
  }, [search.compose, search.broadcast, navigate])

  // Search box: typing stays responsive, the URL (and the query) follows after a pause. The last
  // value written lets the URL-to-box sync ignore the echo of its own writes and react only to
  // back/forward and links.
  const [searchInput, setSearchInput] = useState(searchTerm)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const lastNavigatedQRef = useRef<string | undefined>(undefined)
  useEffect(() => {
    if (searchTerm !== lastNavigatedQRef.current) {
      if (debounceRef.current) clearTimeout(debounceRef.current)
      setSearchInput(searchTerm)
      lastNavigatedQRef.current = searchTerm
    }
  }, [searchTerm])
  useEffect(() => () => clearTimeout(debounceRef.current), [])

  const handleSearchChange = (value: string) => {
    setSearchInput(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      const trimmed = value.trim()
      lastNavigatedQRef.current = trimmed
      navigate({ replace: true, search: (prev) => ({ ...prev, q: trimmed || undefined }) })
    }, SEARCH_DELAY_MS)
  }
  const handleClearFilters = () => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    lastNavigatedQRef.current = ''
    setSearchInput('')
    navigate({ search: (prev) => ({ ...prev, status: undefined, q: undefined }) })
  }

  // L1, then the stats of the loaded rows. A followed row sending by its verdict makes the list
  // refetch as a live status does.
  const [verdictLive, setVerdictLive] = useState(false)
  const list = useBroadcastsList({ workspaceId, statusGroup, q: searchTerm, verdictLive })
  const rows = list.rows
  const pageStats = useBroadcastPageStats({ workspace, rows, canReadHistory, canReadWeb })
  useEffect(() => setVerdictLive(pageStats.verdictLive), [pageStats.verdictLive])
  const estimates = useAudienceEstimates(workspaceId, rows, permissions)

  const listsQuery = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: !!permissions?.lists?.read
  })
  const segmentsQuery = useQuery({
    queryKey: ['segments', workspaceId],
    queryFn: () => listSegments({ workspace_id: workspaceId, with_count: true }),
    enabled: !!permissions?.segments?.read
  })

  const actions = useBroadcastActions(workspaceId)
  const [deleting, setDeleting] = useState<Broadcast | null>(null)

  // Drawers driven by the URL.
  const openDetail = (id: string, replace = false) =>
    navigate({ replace, search: (prev) => ({ ...prev, broadcast: id, compose: undefined }) })
  const openCompose = (id: string) => navigate({ search: (prev) => ({ ...prev, compose: id, broadcast: undefined }) })
  const closeDetail = () => navigate({ search: (prev) => ({ ...prev, broadcast: undefined }) })

  // The detail keeps its last broadcast while it closes, so the drawer does not empty itself.
  const lastDetailId = useRef<string | undefined>(search.broadcast)
  if (search.broadcast) lastDetailId.current = search.broadcast
  const detailId = search.broadcast ?? lastDetailId.current

  // The composer's body key is fixed when it opens: `new` becoming an id after the create does
  // not remount the form being typed in.
  const [composeKey, setComposeKey] = useState<{ key: number; id: string } | null>(
    search.compose ? { key: 0, id: search.compose } : null
  )
  useEffect(() => {
    if (search.compose && !composeKey) setComposeKey({ key: Date.now(), id: search.compose })
    if (!search.compose && composeKey) setComposeKey(null)
  }, [search.compose, composeKey])

  // J and K walk the loaded rows; at the last one J loads the next page, then moves on.
  const index = search.broadcast ? rows.findIndex((b) => b.id === search.broadcast) : -1
  const pendingNext = useRef<string | null>(null)
  // A pending J is dropped once the drawer leaves its row (K, close, another row) or its page fails.
  useEffect(() => {
    if (pendingNext.current && search.broadcast !== pendingNext.current) pendingNext.current = null
  }, [search.broadcast])
  useEffect(() => {
    if (!pendingNext.current || search.broadcast !== pendingNext.current) return
    const from = rows.findIndex((b) => b.id === pendingNext.current)
    if (from >= 0 && from < rows.length - 1) {
      pendingNext.current = null
      openDetail(rows[from + 1].id, true)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows])
  const nav =
    index >= 0
      ? {
          position: index + 1,
          total: list.total,
          onPrev: index > 0 ? () => openDetail(rows[index - 1].id, true) : undefined,
          onNext:
            index < rows.length - 1
              ? () => openDetail(rows[index + 1].id, true)
              : list.hasNextPage
                ? () => {
                    const from = rows[index].id
                    pendingNext.current = from
                    void list.fetchNextPage().then((result) => {
                      if (result.isError && pendingNext.current === from) pendingNext.current = null
                    })
                  }
                : undefined
        }
      : undefined

  const counts = list.statusCounts
  const allCount = groupCount(counts, undefined)
  const tabs: CountTabOption<StatusGroup>[] = [
    { value: 'all', label: t`All`, count: allCount },
    { value: 'draft', label: t`Drafts`, count: groupCount(counts, BROADCAST_STATUS_GROUPS.draft) },
    { value: 'scheduled', label: t`Scheduled`, count: groupCount(counts, BROADCAST_STATUS_GROUPS.scheduled) },
    { value: 'sending', label: t`Sending`, count: groupCount(counts, BROADCAST_STATUS_GROUPS.sending) },
    { value: 'sent', label: t`Sent`, count: groupCount(counts, BROADCAST_STATUS_GROUPS.sent) }
  ]
  const failedCount = groupCount(counts, BROADCAST_STATUS_GROUPS.failed) ?? 0
  if (failedCount > 0 || statusGroup === 'failed') tabs.push({ value: 'failed', label: t`Failed`, count: failedCount })

  const hasMarketingProvider = !!workspace?.settings?.marketing_email_provider_id
  // Nothing at all in the workspace: no search, and the counts of every status are zero.
  const workspaceEmpty = list.isSuccess && !searchTerm && (allCount ?? list.total) === 0

  const newButton = (
    <Tooltip title={canWrite ? undefined : noWrite}>
      <span className="inline-flex">
        <Button type="primary" icon={<Plus size={14} />} disabled={!canWrite} onClick={() => openCompose('new')}>
          {t`New broadcast`}
        </Button>
      </span>
    </Tooltip>
  )

  const shown = rows.length
  const total = list.total
  const footer =
    shown > 0 ? (
      <>
        <span>{t`${int(shown)} of ${plural(total, { one: '# broadcast', other: '# broadcasts' })}`}</span>
        {list.hasNextPage ? (
          <Button size="small" loading={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>
            {t`Load ${PAGE_SIZE} more`}
          </Button>
        ) : null}
      </>
    ) : undefined

  const body = useMemo(() => {
    if (list.isError && !list.data) {
      return (
        <Result
          status="error"
          title={t`Couldn't load the broadcasts`}
          extra={<Button onClick={() => void list.refetch()}>{t`Retry`}</Button>}
        />
      )
    }
    return null
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [list.isError, list.data])

  if (!workspace) return null

  return (
    <div className="flex flex-col gap-3">
      <PageHeader title={t`Broadcasts`} count={allCount} actions={workspaceEmpty ? undefined : newButton} />

      {!hasMarketingProvider ? (
        <Alert
          type="warning"
          showIcon
          title={t`Email Provider Required`}
          description={t`You don't have a marketing email provider configured. Please set up an email provider in your workspace settings to send broadcasts.`}
          action={
            <Button size="small" href={`/console/workspace/${workspaceId}/settings/integrations`}>
              {t`Configure Provider`}
            </Button>
          }
        />
      ) : null}

      {body ?? (
        workspaceEmpty ? (
          <Panel>
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={
                <span className="flex flex-col gap-1">
                  <span className="text-base font-medium text-ink">{t`No broadcasts yet`}</span>
                  <span className="text-sm text-ink-3">{t`Send your first newsletter to a list`}</span>
                </span>
              }
            >
              {newButton}
            </Empty>
          </Panel>
        ) : (
          <>
            <BroadcastKpis workspace={workspace} permissions={permissions} />
            <Panel flush footer={footer}>
              <TableToolbar
                start={
                  <CountTabs<StatusGroup>
                    options={tabs}
                    value={statusGroup}
                    onChange={(value) => navigate({ search: (prev) => ({ ...prev, status: value === 'all' ? undefined : value }) })}
                  />
                }
                end={
                  <div className="w-64">
                    <Input
                      allowClear
                      size="small"
                      aria-label={t`Search by name`}
                      placeholder={t`Search by name`}
                      prefix={<Search size={14} aria-hidden="true" />}
                      value={searchInput}
                      onChange={(e) => handleSearchChange(e.target.value)}
                      onBlur={() => {
                        const trimmed = searchInput.trim()
                        if (trimmed !== searchInput) setSearchInput(trimmed)
                      }}
                    />
                  </div>
                }
              />
              {list.isLoading ? (
                <div className="px-3.5 py-3">
                  <Skeleton active title={false} paragraph={{ rows: 8 }} />
                </div>
              ) : rows.length === 0 && hasActiveFilter ? (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={<span className="text-sm text-ink-3">{t`No broadcasts match`}</span>}
                >
                  <Button onClick={handleClearFilters}>{t`Clear filters`}</Button>
                </Empty>
              ) : (
                <BroadcastsTable
                  workspace={workspace}
                  rows={rows}
                  loading={list.isFetching && !list.isFetchingNextPage && !list.isRefetching}
                  permissions={permissions}
                  canReadHistory={canReadHistory}
                  canReadWeb={canReadWeb}
                  pageStats={pageStats}
                  estimates={estimates}
                  lists={listsQuery.data?.lists ?? []}
                  segments={segmentsQuery.data?.segments ?? []}
                  activeId={search.broadcast}
                  onOpen={(b) => (b.status === 'draft' && canWrite ? openCompose(b.id) : openDetail(b.id))}
                  onCompose={(b) => openCompose(b.id)}
                  onAction={(action, b) => actions.confirm(action, b)}
                  onOpenDetail={(b) => openDetail(b.id)}
                  onDelete={setDeleting}
                />
              )}
            </Panel>
          </>
        )
      )}

      <FullDrawer open={!!search.broadcast && !search.compose} onClose={closeDetail} backLabel={t`Broadcasts`} nav={nav}>
        {detailId ? (
          <BroadcastDetail
            key={detailId}
            workspace={workspace}
            permissions={permissions}
            broadcastId={detailId}
            onEdit={(id) => openCompose(id)}
            onClose={closeDetail}
          />
        ) : null}
      </FullDrawer>

      {search.compose && composeKey ? (
        <BroadcastCompose
          key={composeKey.key}
          workspace={workspace}
          permissions={permissions}
          composeId={composeKey.id}
          // The composer has already saved or asked: the route's blocker must not ask again.
          onClose={() => navigate({ ignoreBlocker: true, search: (prev) => ({ ...prev, compose: undefined }) })}
          onCreated={(id) => navigate({ replace: true, ignoreBlocker: true, search: (prev) => ({ ...prev, compose: id }) })}
          onOpenDetail={(id) =>
            navigate({ replace: true, ignoreBlocker: true, search: (prev) => ({ ...prev, compose: undefined, broadcast: id }) })
          }
        />
      ) : null}

      <DeleteBroadcastModal broadcast={deleting} onClose={() => setDeleting(null)} onDelete={actions.remove} />
    </div>
  )
}

export default BroadcastsPage
