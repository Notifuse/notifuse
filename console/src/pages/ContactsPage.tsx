import React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Skeleton, Tooltip } from 'antd'
import { plural } from '@lingui/core/macro'
import { RefreshCw } from 'lucide-react'
import { useParams, useSearch, useNavigate, useRouter } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { workspaceContactsRoute, type ContactsSearch } from '../router'
import { contactsApi, type Contact } from '../services/api/contacts'
import { contactListApi } from '../services/api/contact_list'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { Panel, RailCard, TableToolbar, useNumberFormat } from '../components/layout'
import { ContactsRail } from '../components/contacts/ContactsRail'
import { ContactsTable, CONTACT_COLUMNS_STORAGE_KEY } from '../components/contacts/ContactsTable'
import { DEFAULT_VISIBLE_COLUMNS, useSelectableColumns } from '../components/contacts/contactColumns'
import { ContactsViewHeader } from '../components/contacts/ContactsViewHeader'
import { ContactsFilters } from '../components/contacts/ContactsFilters'
import { useContactFilterFields } from '../components/contacts/contactFilterFields'
import { ContactColumnsSelector } from '../components/contacts/ContactColumnsSelector'
import { ContactDetailsDrawer } from '../components/contacts/ContactDetailsDrawer'
import { ContactUpsertDrawer } from '../components/contacts/ContactUpsertDrawer'
import { ExportContactsModal } from '../components/contacts/ExportContactsModal'
import { BulkActionsBar } from '../components/contacts/BulkActionsBar'
import { BulkActionProgressModal } from '../components/contacts/BulkActionProgressModal'
import { BlockError } from '../components/contacts/BlockError'
import { SegmentDefinedBy } from '../components/contacts/SegmentDefinedBy'
import { useContactsCsvUpload } from '../components/contacts/ContactsCsvUploadProvider'
import { useContactsTableQuery, type ContactsTableData } from '../components/contacts/useContactsTableQuery'
import { useListsQuery, useSegmentsQuery } from '../components/contacts/useContactsLookups'
import {
  CONTACTS_PAGE_SIZE,
  MAX_CONTACT_ROWS,
  hasRowFilters,
  toTableSearch,
  viewOf,
  type ContactsView
} from '../components/contacts/contactsView'
import { exactViewTotal } from '../components/contacts/contactNavigation'
import { ListHeader } from '../components/lists/ListHeader'
import { LIST_STATUSES, LIST_STAT_KEYS, useListStats } from '../components/lists/listStats'
import { CreateListDrawer } from '../components/lists/ListDrawer'
import { DrawerSegment } from '../components/segment/button_upsert'
import { filtersToSegmentTree } from '../components/contacts/filtersToSegmentTree'
import type { TreeNode } from '../services/api/segment'
import { useBulkContactAction, SkippedAction, type BulkActionResult } from '../hooks/useBulkContactAction'

const SEARCH_DEBOUNCE_MS = 300
const CONTACTS_SEARCH_ID = 'contacts-search'

/** The search parameters that filter rows on top of the view; « Clear filters » empties them. */
const ROW_FILTER_RESET: Partial<ContactsSearch> = {
  search: undefined,
  email: undefined,
  external_id: undefined,
  first_name: undefined,
  last_name: undefined,
  full_name: undefined,
  phone: undefined,
  country: undefined,
  language: undefined,
  tracking_opens_consent: undefined,
  tracking_clicks_consent: undefined,
  segments: undefined,
  contact_list_status: undefined
}

function readStoredColumns(): Record<string, boolean> {
  try {
    const saved = localStorage.getItem(CONTACT_COLUMNS_STORAGE_KEY)
    return saved ? { ...DEFAULT_VISIBLE_COLUMNS, ...JSON.parse(saved) } : DEFAULT_VISIBLE_COLUMNS
  } catch {
    return DEFAULT_VISIBLE_COLUMNS
  }
}

export function ContactsPage() {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const { message } = App.useApp()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/contacts' })
  const search = useSearch({ from: workspaceContactsRoute.id })
  const navigate = useNavigate()
  const router = useRouter()
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const { permissions, role, loading: permissionsLoading } = useWorkspacePermissions(workspaceId)
  const { openDrawer: openImportDrawer, openDrawerWithSelectedList } = useContactsCsvUpload()
  const workspace = workspaces.find((w) => w.id === workspaceId)

  const canReadContacts = !!permissions?.contacts?.read
  const canWriteContacts = !!permissions?.contacts?.write
  const canReadLists = !!permissions?.lists?.read
  const canWriteLists = !!permissions?.lists?.write
  const canReadSegments = !!permissions?.segments?.read
  const canWriteSegments = !!permissions?.segments?.write

  const listsQuery = useListsQuery(workspaceId, canReadLists)
  const segmentsQuery = useSegmentsQuery(workspaceId, canReadSegments)
  const totalQuery = useQuery({
    queryKey: ['total-contacts', workspaceId],
    queryFn: () => contactsApi.getTotalContacts({ workspace_id: workspaceId }),
    enabled: canReadContacts
  })
  const lists = React.useMemo(() => listsQuery.data?.lists ?? [], [listsQuery.data])
  const segments = React.useMemo(() => segmentsQuery.data?.segments ?? [], [segmentsQuery.data])
  const firstList = React.useMemo(() => [...lists].sort((a, b) => a.name.localeCompare(b.name))[0], [lists])

  // The view the member may see, derived while rendering (never navigated, so a URL keeps its
  // list_id while the rights load): a list without lists.read and a segment without
  // segments.read fall back to All; a member with lists.read only opens the first list.
  const urlView = viewOf(search)
  let view: ContactsView = urlView
  if (view.kind === 'list' && !canReadLists) view = { kind: 'all' }
  if (view.kind === 'segment' && !canReadSegments) view = { kind: 'all' }
  if (view.kind === 'all' && !canReadContacts && firstList) view = { kind: 'list', listId: firstList.id }
  const effectiveSearch: ContactsSearch = {
    ...search,
    list_id: view.kind === 'list' ? view.listId : undefined,
    contact_list_status: view.kind === 'list' ? search.contact_list_status : undefined,
    segment_id: view.kind === 'segment' ? view.segmentId : undefined
  }
  const tableSearch = toTableSearch(effectiveSearch)
  const table = useContactsTableQuery(workspaceId, tableSearch, {
    enabled: canReadContacts && !permissionsLoading
  })

  const list = view.kind === 'list' ? lists.find((l) => l.id === view.listId) : undefined
  const segment = view.kind === 'segment' ? segments.find((s) => s.id === view.segmentId) : undefined
  const listGone = view.kind === 'list' && listsQuery.isSuccess && !list
  const segmentGone = view.kind === 'segment' && segmentsQuery.isSuccess && !segment
  const listStats = useListStats(workspaceId, view.kind === 'list' ? view.listId : undefined, !!list)

  // The page's own updates (more rows, a filter, the sheet) keep the scroll where it is: the
  // router scrolls to the top on every navigation unless told not to. A new view starts at the top.
  const go = React.useCallback(
    (update: (prev: ContactsSearch) => ContactsSearch, replace = false, resetScroll = false) =>
      navigate({ to: workspaceContactsRoute.to, params: { workspaceId }, search: update, replace, resetScroll }),
    [navigate, workspaceId]
  )

  const selectView = (next: ContactsView) =>
    go(
      (prev) => ({
        ...prev,
        list_id: next.kind === 'list' ? next.listId : undefined,
        segment_id: next.kind === 'segment' ? next.segmentId : undefined,
        contact_list_status: undefined,
        segments: undefined,
        contact: undefined,
        limit: undefined
      }),
      false,
      true
    )
  const backToAll = () => {
    selectView({ kind: 'all' })
  }

  // ---- Unified search: local while typing, written to the URL after 300 ms (replace).
  const [searchInput, setSearchInput] = React.useState(search.search ?? '')
  const sentSearch = React.useRef(search.search)
  React.useEffect(() => {
    // The URL changed from elsewhere (Clear, Back): show it. Our own write is skipped, so a key
    // typed while it lands is not lost, and a trailing space survives the trim.
    if (search.search === sentSearch.current) return
    sentSearch.current = search.search
    setSearchInput(search.search ?? '')
  }, [search.search])
  const searchTimer = React.useRef<number | undefined>(undefined)
  const commitSearch = React.useCallback(
    (next: string | undefined) => {
      sentSearch.current = next
      go((prev) => ({ ...prev, search: next, limit: undefined }), true)
    },
    [go]
  )
  React.useEffect(() => {
    const next = searchInput.trim() || undefined
    if (next === sentSearch.current) return
    searchTimer.current = window.setTimeout(() => commitSearch(next), SEARCH_DEBOUNCE_MS)
    return () => window.clearTimeout(searchTimer.current)
  }, [searchInput, commitSearch])

  // ---- Columns
  const [visibleColumns, setVisibleColumns] = React.useState<Record<string, boolean>>(readStoredColumns)
  const selectableColumns = useSelectableColumns(workspace)
  const columnChoices = selectableColumns.map((column) => ({ ...column, visible: !!visibleColumns[column.key] }))
  const setColumnVisible = (key: string, visible: boolean) =>
    setVisibleColumns((prev) => {
      const next = { ...prev, [key]: visible }
      try {
        localStorage.setItem(CONTACT_COLUMNS_STORAGE_KEY, JSON.stringify(next))
      } catch {
        // Storage refused (private window): the choice lasts for this visit.
      }
      return next
    })

  // ---- Selection and bulk actions
  const [selectedEmails, setSelectedEmails] = React.useState<string[]>([])
  const [bulkActionLabel, setBulkActionLabel] = React.useState('')
  const [showProgressModal, setShowProgressModal] = React.useState(false)
  const bulk = useBulkContactAction()
  const bulkRunningRef = React.useRef(false)
  React.useEffect(() => {
    bulkRunningRef.current = bulk.running
  })
  // The selection belongs to the rows of one view and filter set: it is cleared when they change,
  // not when more rows load (limit) nor when the sheet opens (contact is not in tableSearch).
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- limit is not a filter
  const { limit: _limit, ...filters } = tableSearch
  const filtersKey = JSON.stringify(filters)
  React.useEffect(() => {
    if (bulkRunningRef.current) return
    setSelectedEmails([])
  }, [filtersKey])
  React.useEffect(() => {
    if (!bulk.running) return
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = 'A bulk action is in progress. Leaving now will abort it.'
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [bulk.running])

  // A segment being computed has no rows yet: read the table again once it is done.
  const segmentStatus = segment?.status
  const previousSegmentStatus = React.useRef(segmentStatus)
  React.useEffect(() => {
    if (previousSegmentStatus.current === 'building' && segmentStatus === 'active') {
      queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })
    }
    previousSegmentStatus.current = segmentStatus
  }, [segmentStatus, queryClient, workspaceId])

  const invalidateAfterContactChange = () => {
    queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })
    queryClient.invalidateQueries({ queryKey: ['total-contacts', workspaceId] })
    queryClient.invalidateQueries({ queryKey: ['list-stats', workspaceId] })
  }

  const handleBulkDelete = async () => {
    const emails = [...selectedEmails]
    setBulkActionLabel(t`Delete contacts`)
    setShowProgressModal(true)
    await bulk.run(emails, async (email) => {
      await contactsApi.delete({ workspace_id: workspaceId, email })
    })
    invalidateAfterContactChange()
  }

  const handleBulkAddToList = async (listId: string) => {
    const emails = [...selectedEmails]
    setBulkActionLabel(t`Add to list`)
    setShowProgressModal(true)
    await bulk.runBatch(emails, async (batch) => {
      const response = await contactsApi.batchImport({
        workspace_id: workspaceId,
        contacts: batch.map((email) => ({ email })),
        subscribe_to_lists: [listId]
      })
      if (response.error) {
        return batch.map((email) => ({ email, success: false, error: response.error }))
      }
      const results = new Map<string, BulkActionResult>()
      for (const op of response.operations || []) {
        if (!op.email) continue
        results.set(op.email, {
          email: op.email,
          success: op.action === 'create' || op.action === 'update',
          error: op.action === 'error' ? op.error : undefined
        })
      }
      // An email the server left out of operations[] is a failure, not an assumed success.
      return batch.map((email) => results.get(email) || { email, success: false, error: 'Not acknowledged by server' })
    })
    invalidateAfterContactChange()
    queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
  }

  const handleBulkRemoveFromList = async (listId: string) => {
    const emails = [...selectedEmails]
    setBulkActionLabel(t`Remove from list`)
    setShowProgressModal(true)
    await bulk.run(emails, async (email) => {
      try {
        await contactListApi.removeContact({ workspace_id: workspaceId, email, list_id: listId })
      } catch (err) {
        if (err instanceof Error && err.message.includes('contact list not found')) {
          throw new SkippedAction('Not in list')
        }
        throw err
      }
    })
    invalidateAfterContactChange()
  }

  const handleBulkUnsubscribe = async (listId: string) => {
    const emails = [...selectedEmails]
    setBulkActionLabel(t`Unsubscribe from list`)
    setShowProgressModal(true)
    await bulk.run(emails, async (email) => {
      const response = await contactListApi.updateStatus({
        workspace_id: workspaceId,
        email,
        list_id: listId,
        status: 'unsubscribed'
      })
      // The server does nothing when the contact is not in the list: « Skipped », not « Success ».
      if (response.found === false) throw new SkippedAction('Not in list')
    })
    invalidateAfterContactChange()
  }

  // The bar goes with the selection: focus goes back to the search box rather than to the page.
  const clearSelection = () => {
    setSelectedEmails([])
    window.setTimeout(() => document.getElementById(CONTACTS_SEARCH_ID)?.focus(), 0)
  }

  const handleProgressModalClose = () => {
    if (bulk.running) bulk.cancel()
    setShowProgressModal(false)
    clearSelection()
    bulk.reset()
  }

  // ---- The contact sheet: opening adds a history entry, so browser Back closes it.
  const sheetOpenedHere = React.useRef(false)
  React.useEffect(() => {
    // Closed by Back, or by any other way: the next close no longer owns a history entry.
    if (!search.contact) sheetOpenedHere.current = false
  }, [search.contact])
  const openContact = (email: string) => {
    // A search still waiting for its timer goes into the URL with the sheet, not after it.
    const typed = searchInput.trim() || undefined
    if (typed !== sentSearch.current) {
      window.clearTimeout(searchTimer.current)
      commitSearch(typed)
      // The browser history queues its changes until the next microtask and merges a replace and a
      // push of the same tick into one push: the replace is written first, or the entry under the
      // sheet would keep the old search.
      router.history.flush()
    }
    sheetOpenedHere.current = true
    go((prev) => ({ ...prev, contact: email }))
  }
  const closeContact = () => {
    if (sheetOpenedHere.current) {
      sheetOpenedHere.current = false
      window.history.back()
      return
    }
    go((prev) => ({ ...prev, contact: undefined }), true)
  }
  // The rows J loaded while the sheet was open (its own entry's limit) are kept when Back closes it
  // onto the entry before: the same filters get that limit again. A filter changed on closing
  // starts over, as it does without a sheet.
  const sheetRows = React.useRef<{ filters: string; limit: number } | null>(null)
  React.useEffect(() => {
    const limit = search.limit ?? CONTACTS_PAGE_SIZE
    if (search.contact) {
      sheetRows.current = { filters: filtersKey, limit }
      return
    }
    const kept = sheetRows.current
    sheetRows.current = null
    if (kept && kept.filters === filtersKey && kept.limit > limit) {
      go((prev) => ({ ...prev, limit: kept.limit }), true)
    }
  }, [search.contact, search.limit, filtersKey, go])

  // ---- Row edit and delete
  const [contactToEdit, setContactToEdit] = React.useState<Contact | null>(null)
  // Deleted contacts and when the delete started. During a load more, the table shows the rows the
  // previous page size kept (placeholder data), which the cache update below cannot reach, so the
  // rows are filtered here too, until a fetch that started after the delete answers.
  const [deletedAt, setDeletedAt] = React.useState<ReadonlyMap<string, number>>(new Map())
  const deleteMutation = useMutation({
    mutationFn: (email: string) => contactsApi.delete({ workspace_id: workspaceId, email }),
    onMutate: async (email) => {
      setDeletedAt((prev) => new Map(prev).set(email, Date.now()))
      // Every page size of every view, since the table may show any of them next
      const tables = { queryKey: ['contacts', workspaceId] }
      await queryClient.cancelQueries(tables)
      const previous = queryClient.getQueriesData<ContactsTableData>(tables)
      queryClient.setQueriesData<ContactsTableData>(tables, (old) =>
        old?.contacts ? { ...old, contacts: old.contacts.filter((c) => c.email !== email) } : old
      )
      return { previous }
    },
    onSuccess: (_data, email) => {
      message.success(t`Contact deleted successfully`)
      // The selection must not keep a deleted contact: the count would lie and a bulk add-to-list
      // (an upsert) would bring it back.
      setSelectedEmails((prev) => prev.filter((selected) => selected !== email))
      if (search.contact === email) {
        sheetOpenedHere.current = false
        go((prev) => ({ ...prev, contact: undefined }), true)
      }
    },
    onError: (error: Error, email, context) => {
      setDeletedAt((prev) => {
        const next = new Map(prev)
        next.delete(email)
        return next
      })
      for (const [key, data] of context?.previous ?? []) queryClient.setQueryData(key, data)
      message.error(error?.message || t`Failed to delete contact`)
    },
    onSettled: invalidateAfterContactChange
  })

  // Once the table's data is newer than a delete (the optimistic update stamps it, and so does any
  // later read), that data no longer holds the row: forget the delete, so that a contact created
  // again with that email shows during the next load more.
  React.useEffect(() => {
    if (table.isPlaceholderData) return
    const answered = [...deletedAt].filter(([, at]) => table.dataUpdatedAt > at).map(([email]) => email)
    if (answered.length === 0) return
    setDeletedAt((prev) => {
      const next = new Map(prev)
      for (const email of answered) next.delete(email)
      return next
    })
  }, [deletedAt, table.isPlaceholderData, table.dataUpdatedAt])

  // ---- Lists and segments created or edited from the rail and the list header
  const [listDrawer, setListDrawer] = React.useState<'create' | 'edit' | null>(null)
  // Open with the conditions it starts with: none from the rail, the table's filters from
  // « Advanced conditions: create a segment ».
  const [segmentDrawer, setSegmentDrawer] = React.useState<{ tree?: TreeNode } | null>(null)
  const [exportOpen, setExportOpen] = React.useState(false)

  const handleViewDeleted = () => {
    queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })
    go(
      (prev) => ({
        ...prev,
        list_id: undefined,
        segment_id: undefined,
        contact_list_status: undefined,
        segments: undefined,
        contact: undefined,
        limit: undefined
      }),
      true,
      true
    )
  }

  const handleRefresh = () => {
    for (const key of ['contacts', 'lists', 'lists-with-counts', 'total-contacts', 'segments', 'list-stats']) {
      queryClient.invalidateQueries({ queryKey: [key, workspaceId] })
    }
  }

  const filterFields = useContactFilterFields()

  if (!workspace) return <div>{t`Loading...`}</div>

  if (permissionsLoading) {
    return (
      <div data-contacts-loading className="px-5 pt-4">
        <Skeleton active paragraph={{ rows: 6 }} />
      </div>
    )
  }

  if (!canReadContacts && !canReadLists) {
    return <div className="px-5 pt-6 text-sm text-ink-3">{t`You don't have access to contacts`}</div>
  }

  const rows = (table.data?.contacts ?? []).filter((c) => {
    const at = deletedAt.get(c.email)
    return at === undefined || (!table.isPlaceholderData && table.dataUpdatedAt > at)
  })
  const hasMore = !!table.data?.next_cursor
  const rowFilters = hasRowFilters(tableSearch)
  const filtered = rowFilters || (view.kind === 'list' && !!tableSearch.contact_list_status)
  const atMax = rows.length >= MAX_CONTACT_ROWS
  const exactTotal = exactViewTotal({
    view,
    hasFilters: rowFilters,
    contactsCount: totalQuery.data?.total_contacts,
    listStats: listStats.data?.stats,
    status: tableSearch.contact_list_status,
    segmentCount: segment?.users_count
  })
  // The view's own size next to the title, whatever the search, the filters or the status tab:
  // the workspace's contacts, the list's statuses summed, the segment's members.
  const viewTotal =
    view.kind === 'list'
      ? listStats.data
        ? LIST_STATUSES.reduce((sum, key) => sum + listStats.data.stats[LIST_STAT_KEYS[key]], 0)
        : undefined
      : view.kind === 'segment'
        ? segment?.users_count
        : totalQuery.data?.total_contacts

  // With every row loaded, the rows are the count. While more remain: « 10 of 1,288 contacts » when
  // the total is known and above the rows (a segment's count can lag), else « 50+ contacts ».
  const shown = rows.length
  const shownCount = int(shown)
  const selectedCount = selectedEmails.length
  const footerCount = !hasMore
    ? plural(shown, { one: '# contact', other: '# contacts' })
    : exactTotal !== undefined && exactTotal > shown
      ? plural(exactTotal, { one: `${shownCount} of # contact`, other: `${shownCount} of # contacts` })
      : plural(shown, { one: '#+ contact', other: '#+ contacts' })
  const viewName =
    view.kind === 'list'
      ? (list?.name ?? view.listId)
      : view.kind === 'segment'
        ? (segment?.name ?? view.segmentId)
        : t`All contacts`

  const clearFilters = () => go((prev) => ({ ...prev, ...ROW_FILTER_RESET, limit: undefined }), true)

  const emptyText = (() => {
    if (filtered) {
      return (
        <EmptyState text={t`No contacts match your filters`}>
          <Button onClick={clearFilters}>{t`Clear filters`}</Button>
        </EmptyState>
      )
    }
    if (view.kind === 'list') {
      return (
        <EmptyState text={t`No contacts in this list yet`}>
          {canWriteContacts && canWriteLists && list ? (
            <Button
              onClick={() => openDrawerWithSelectedList(workspaceId, lists, list.id, true)}
            >{t`Import to list`}</Button>
          ) : null}
        </EmptyState>
      )
    }
    if (view.kind === 'segment') {
      return (
        <EmptyState
          text={segment?.status === 'building' ? t`Computing this segment…` : t`No contacts match this segment`}
        />
      )
    }
    return (
      <EmptyState text={t`No contacts yet`}>
        {canWriteContacts ? (
          <>
            <Button onClick={() => openImportDrawer(workspaceId, lists, true)}>{t`Import`}</Button>
            <ContactUpsertDrawer
              workspace={workspace}
              onSuccess={invalidateAfterContactChange}
              buttonProps={{ buttonContent: t`New contact` }}
            />
          </>
        ) : null}
      </EmptyState>
    )
  })()

  const tableArea = (() => {
    if (!canReadContacts) {
      return <div className="px-4 py-10 text-center text-sm text-ink-3">{t`You don't have access to contacts`}</div>
    }
    if (table.isError && !table.data) return <BlockError onRetry={() => table.refetch()} />
    if (table.isLoading) {
      return (
        <div data-contacts-skeleton className="px-4 py-3">
          <Skeleton active title={false} paragraph={{ rows: 10 }} />
        </div>
      )
    }
    return (
      <ContactsTable
        workspace={workspace}
        view={view}
        contacts={rows}
        loading={table.isFetching && table.isPlaceholderData}
        lists={lists}
        segments={segments}
        permissions={permissions}
        visibleColumns={visibleColumns}
        selectedEmails={selectedEmails}
        onSelectionChange={setSelectedEmails}
        onOpen={openContact}
        onEdit={setContactToEdit}
        // Confirmed in the row's popconfirm.
        onDelete={(email) => deleteMutation.mutate(email)}
        emptyText={emptyText}
      />
    )
  })()

  const gone = listGone || segmentGone

  // The lists and segments as a rounded card beside the table.
  const rail = (
    <RailCard ariaLabel={t`Contacts`}>
      <ContactsRail
        workspaceId={workspaceId}
        view={view}
        permissions={permissions}
        contactsCount={totalQuery.data?.total_contacts}
        segments={segmentsQuery.data?.segments}
        segmentsLoading={segmentsQuery.isLoading}
        onSelect={selectView}
        onCreateList={() => setListDrawer('create')}
        onCreateSegment={() => setSegmentDrawer({})}
      />
    </RailCard>
  )

  return (
    <div data-rail-content className="flex min-w-0 flex-col gap-3 px-5 pt-4 pb-6">
      {gone ? (
        <div className="flex items-start gap-3">
          {rail}
          <div className="flex flex-col items-start gap-2 py-6 text-sm text-ink-2">
            <span>{listGone ? t`This list no longer exists` : t`This segment no longer exists`}</span>
            <Button type="link" className="px-0" onClick={backToAll}>
              {t`All contacts`}
            </Button>
          </div>
        </div>
      ) : (
        <>
          <ContactsViewHeader
            workspace={workspace}
            view={view}
            list={list}
            segment={segment}
            lists={lists}
            permissions={permissions}
            role={role}
            total={viewTotal}
            workspaceTotal={totalQuery.data?.total_contacts}
            onExport={() => setExportOpen(true)}
            onContactCreated={invalidateAfterContactChange}
            onEditList={() => setListDrawer('edit')}
            onViewDeleted={handleViewDeleted}
          />
          {list ? (
            <ListHeader
              workspace={workspace}
              list={list}
              permissions={permissions}
              status={tableSearch.contact_list_status}
              // A second click on the open status removes the filter: there are no « All » tabs.
              onStatusChange={(status) =>
                go((prev) => ({
                  ...prev,
                  contact_list_status: prev.contact_list_status === status ? undefined : status,
                  contact: undefined,
                  limit: undefined
                }))
              }
            />
          ) : null}
          {segment ? (
            <SegmentDefinedBy
              workspaceId={workspaceId}
              segment={segment}
              lists={lists}
              customFieldLabels={workspace.settings?.custom_field_labels ?? {}}
            />
          ) : null}

          {/* Always rendered, so the selection count is announced as it changes. */}
          <span className="sr-only" aria-live="polite">
            {selectedCount > 0
              ? plural(selectedCount, { one: '# contact selected', other: '# contacts selected' })
              : ''}
          </span>
          <div className="flex items-start gap-3">
            {rail}
            <div className="flex min-w-0 flex-1 flex-col gap-3">
              <Panel flush>
                {canReadContacts && selectedEmails.length > 0 ? (
                  <BulkActionsBar
                    selectedCount={selectedCount}
                    lists={lists}
                    canWriteContacts={canWriteContacts}
                    canWriteLists={canWriteLists}
                    disabled={bulk.running}
                    onAddToList={handleBulkAddToList}
                    onRemoveFromList={handleBulkRemoveFromList}
                    onUnsubscribeFromList={handleBulkUnsubscribe}
                    onDelete={handleBulkDelete}
                    onClear={clearSelection}
                  />
                ) : canReadContacts ? (
                  <TableToolbar
                    start={
                      <>
                        <TableToolbar.Search
                          id={CONTACTS_SEARCH_ID}
                          value={searchInput}
                          onChange={setSearchInput}
                          placeholder={t`Email, name or external ID`}
                        />
                        <ContactsFilters
                          search={tableSearch}
                          fields={filterFields}
                          segments={segments}
                          canCreateSegment={canWriteSegments}
                          onApply={(key, value) => go((prev) => ({ ...prev, [key]: value, limit: undefined }))}
                          onCreateSegment={() =>
                            setSegmentDrawer({ tree: filtersToSegmentTree(tableSearch, filterFields) })
                          }
                          onRemove={(key) => go((prev) => ({ ...prev, [key]: undefined, limit: undefined }), true)}
                          onClear={clearFilters}
                        />
                      </>
                    }
                    end={
                      <>
                        <ContactColumnsSelector columns={columnChoices} onColumnVisibilityChange={setColumnVisible} />
                        <Tooltip title={t`Refresh`}>
                          <Button
                            type="text"
                            aria-label={t`Refresh`}
                            icon={<RefreshCw size={14} aria-hidden="true" />}
                            onClick={handleRefresh}
                          />
                        </Tooltip>
                      </>
                    }
                  />
                ) : null}
                {tableArea}
              </Panel>

              {canReadContacts && rows.length > 0 ? (
                <div data-contacts-footer className="flex items-center justify-between gap-3 px-1">
                  <span className="text-xs text-ink-3">{footerCount}</span>
                  {atMax && hasMore ? (
                    <span className="text-xs text-ink-3">
                      {t`Showing the first ${int(MAX_CONTACT_ROWS)} contacts. Refine your filters to see more.`}
                    </span>
                  ) : hasMore ? (
                    <Button
                      size="small"
                      loading={table.isFetching}
                      onClick={() =>
                        go((prev) => ({ ...prev, limit: (prev.limit ?? CONTACTS_PAGE_SIZE) + CONTACTS_PAGE_SIZE }), true)
                      }
                    >
                      {t`Load ${CONTACTS_PAGE_SIZE} more`}
                    </Button>
                  ) : null}
                </div>
              ) : null}
            </div>
          </div>
        </>
      )}

      <ContactDetailsDrawer
        workspace={workspace}
        email={search.contact}
        tableSearch={tableSearch}
        tableEnabled={canReadContacts}
        viewName={viewName}
        exactTotal={exactTotal}
        lists={lists}
        segments={segments}
        permissions={permissions}
        onClose={closeContact}
        onNavigate={(email) => go((prev) => ({ ...prev, contact: email }), true)}
        onLoadMore={() =>
          go((prev) => ({ ...prev, limit: (prev.limit ?? CONTACTS_PAGE_SIZE) + CONTACTS_PAGE_SIZE }), true)
        }
        // Confirmed in the sheet's bar: no second confirmation here.
        onDelete={(email) => deleteMutation.mutate(email)}
      />

      {contactToEdit ? (
        <ContactUpsertDrawer
          workspace={workspace}
          contact={contactToEdit}
          open
          onClose={() => setContactToEdit(null)}
          onContactUpdate={() => {
            queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })
            setContactToEdit(null)
          }}
        />
      ) : null}

      {canWriteLists ? (
        <CreateListDrawer
          workspaceId={workspaceId}
          list={listDrawer === 'edit' ? list : undefined}
          canReadTemplates={!!permissions?.templates?.read}
          open={listDrawer !== null}
          onClose={() => setListDrawer(null)}
          onCreated={(created) => selectView({ kind: 'list', listId: created.id })}
        />
      ) : null}

      {segmentDrawer ? (
        <DrawerSegment
          initialTree={segmentDrawer.tree}
          totalContacts={totalQuery.data?.total_contacts}
          canReadTemplates={!!permissions?.templates?.read}
          setDrawserVisible={(visible) => setSegmentDrawer(visible ? (segmentDrawer ?? {}) : null)}
          onSuccess={() => queryClient.invalidateQueries({ queryKey: ['segments', workspaceId] })}
        />
      ) : null}

      {canReadContacts ? (
        <ExportContactsModal
          visible={exportOpen}
          onCancel={() => setExportOpen(false)}
          workspaceId={workspaceId}
          workspace={workspace}
          filters={tableSearch}
          segmentsData={segments}
          listsData={lists}
        />
      ) : null}

      <BulkActionProgressModal
        open={showProgressModal}
        title={bulkActionLabel}
        progress={bulk.progress}
        onPause={bulk.pause}
        onResume={bulk.resume}
        onCancel={bulk.cancel}
        onClose={handleProgressModalClose}
      />
    </div>
  )
}

function EmptyState({ text, children }: { text: string; children?: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-3 py-8">
      <span className="text-sm text-ink-3">{text}</span>
      {children ? <div className="flex items-center gap-2">{children}</div> : null}
    </div>
  )
}
