import React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Skeleton, Tooltip } from 'antd'
import { RefreshCw } from 'lucide-react'
import { useParams, useSearch, useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { workspaceContactsRoute, type ContactsSearch } from '../router'
import { contactsApi, type Contact } from '../services/api/contacts'
import { contactListApi } from '../services/api/contact_list'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { CountTabs, Panel, RailLayout, TableToolbar, useNumberFormat } from '../components/layout'
import { ContactsRail } from '../components/contacts/ContactsRail'
import { ContactsTable, CONTACT_COLUMNS_STORAGE_KEY } from '../components/contacts/ContactsTable'
import {
  DEFAULT_VISIBLE_COLUMNS,
  useSelectableColumns
} from '../components/contacts/contactColumns'
import { ContactsViewHeader } from '../components/contacts/ContactsViewHeader'
import { ContactsFilters } from '../components/contacts/ContactsFilters'
import { useContactFilterFields } from '../components/contacts/contactFilterFields'
import { ContactColumnsSelector } from '../components/contacts/ContactColumnsSelector'
import { ContactDetailsDrawer } from '../components/contacts/ContactDetailsDrawer'
import { ContactUpsertDrawer } from '../components/contacts/ContactUpsertDrawer'
import { DeleteContactModal } from '../components/contacts/DeleteContactModal'
import { ExportContactsModal } from '../components/contacts/ExportContactsModal'
import { BulkActionsBar } from '../components/contacts/BulkActionsBar'
import { BulkActionProgressModal } from '../components/contacts/BulkActionProgressModal'
import { BlockError } from '../components/contacts/BlockError'
import { SegmentDefinedBy } from '../components/contacts/SegmentDefinedBy'
import { useContactsCsvUpload } from '../components/contacts/ContactsCsvUploadProvider'
import {
  contactsTableKey,
  useContactsTableQuery,
  type ContactsTableData
} from '../components/contacts/useContactsTableQuery'
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
import {
  LIST_STATUSES,
  LIST_STAT_KEYS,
  useListStats,
  type ListStatus
} from '../components/lists/listStats'
import { CreateListDrawer } from '../components/lists/ListDrawer'
import { DrawerSegment } from '../components/segment/button_upsert'
import { useBulkContactAction, SkippedAction, type BulkActionResult } from '../hooks/useBulkContactAction'

const SEARCH_DEBOUNCE_MS = 300

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
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const { permissions, loading: permissionsLoading } = useWorkspacePermissions(workspaceId)
  const { openDrawer: openImportDrawer, openDrawerWithSelectedList } = useContactsCsvUpload()
  const workspace = workspaces.find((w) => w.id === workspaceId)

  const canReadContacts = !!permissions?.contacts?.read
  const canWriteContacts = !!permissions?.contacts?.write
  const canReadLists = !!permissions?.lists?.read
  const canWriteLists = !!permissions?.lists?.write
  const canReadSegments = !!permissions?.segments?.read

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
  const tableKey = contactsTableKey(workspaceId, tableSearch)
  const table = useContactsTableQuery(workspaceId, tableSearch, {
    enabled: canReadContacts && !permissionsLoading
  })

  const list = view.kind === 'list' ? lists.find((l) => l.id === view.listId) : undefined
  const segment = view.kind === 'segment' ? segments.find((s) => s.id === view.segmentId) : undefined
  const listGone = view.kind === 'list' && listsQuery.isSuccess && !list
  const segmentGone = view.kind === 'segment' && segmentsQuery.isSuccess && !segment
  const listStats = useListStats(workspaceId, view.kind === 'list' ? view.listId : undefined, !!list)

  const go = React.useCallback(
    (update: (prev: ContactsSearch) => ContactsSearch, replace = false) =>
      navigate({ to: workspaceContactsRoute.to, params: { workspaceId }, search: update, replace }),
    [navigate, workspaceId]
  )

  const selectView = (next: ContactsView) =>
    go((prev) => ({
      ...prev,
      list_id: next.kind === 'list' ? next.listId : undefined,
      segment_id: next.kind === 'segment' ? next.segmentId : undefined,
      contact_list_status: undefined,
      segments: undefined,
      contact: undefined,
      limit: undefined
    }))
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
      return batch.map(
        (email) => results.get(email) || { email, success: false, error: 'Not acknowledged by server' }
      )
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

  const handleProgressModalClose = () => {
    if (bulk.running) bulk.cancel()
    setShowProgressModal(false)
    setSelectedEmails([])
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

  // ---- Row edit and delete
  const [contactToEdit, setContactToEdit] = React.useState<Contact | null>(null)
  const [contactToDelete, setContactToDelete] = React.useState<string | null>(null)
  const deleteMutation = useMutation({
    mutationFn: (email: string) => contactsApi.delete({ workspace_id: workspaceId, email }),
    onMutate: async (email) => {
      await queryClient.cancelQueries({ queryKey: tableKey })
      const previous = queryClient.getQueryData<ContactsTableData>(tableKey)
      queryClient.setQueryData<ContactsTableData>(tableKey, (old) =>
        old ? { ...old, contacts: old.contacts.filter((c) => c.email !== email) } : old
      )
      return { previous }
    },
    onSuccess: (_data, email) => {
      message.success(t`Contact deleted successfully`)
      // The selection must not keep a deleted contact: the count would lie and a bulk add-to-list
      // (an upsert) would bring it back.
      setSelectedEmails((prev) => prev.filter((selected) => selected !== email))
      setContactToDelete(null)
      if (search.contact === email) {
        sheetOpenedHere.current = false
        go((prev) => ({ ...prev, contact: undefined }), true)
      }
    },
    onError: (error: Error, _email, context) => {
      if (context?.previous) queryClient.setQueryData(tableKey, context.previous)
      message.error(error?.message || t`Failed to delete contact`)
    },
    onSettled: invalidateAfterContactChange
  })

  // ---- Lists and segments created or edited from the rail and the list header
  const [listDrawer, setListDrawer] = React.useState<'create' | 'edit' | null>(null)
  const [segmentDrawerOpen, setSegmentDrawerOpen] = React.useState(false)
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
      true
    )
  }

  const handleRefresh = () => {
    for (const key of [
      'contacts',
      'lists',
      'lists-with-counts',
      'total-contacts',
      'segments',
      'list-stats'
    ]) {
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

  const rows = table.data?.contacts ?? []
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
          text={
            segment?.status === 'building' ? t`Computing this segment…` : t`No contacts match this segment`
          }
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

  const statusTabs =
    view.kind === 'list' && list ? (
      <CountTabs<'all' | ListStatus>
        value={(tableSearch.contact_list_status as ListStatus | undefined) ?? 'all'}
        onChange={(status) =>
          go((prev) => ({
            ...prev,
            contact_list_status: status === 'all' ? undefined : status,
            contact: undefined,
            limit: undefined
          }))
        }
        options={[
          {
            value: 'all',
            label: t`All`,
            count: listStats.data
              ? LIST_STATUSES.reduce((sum, key) => sum + listStats.data.stats[LIST_STAT_KEYS[key]], 0)
              : undefined
          },
          ...LIST_STATUSES.map((status) => ({
            value: status,
            label: {
              active: t`Active`,
              pending: t`Pending`,
              unsubscribed: t`Unsubscribed`,
              bounced: t`Bounced`,
              complained: t`Complained`
            }[status],
            count: listStats.data?.stats[LIST_STAT_KEYS[status]]
          }))
        ]}
      />
    ) : null

  const tableArea = (() => {
    if (!canReadContacts) {
      return (
        <div className="px-4 py-10 text-center text-sm text-ink-3">{t`You don't have access to contacts`}</div>
      )
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
        onDelete={setContactToDelete}
        emptyText={emptyText}
      />
    )
  })()

  const gone = listGone || segmentGone

  return (
    <RailLayout
      ariaLabel={t`Contacts`}
      rail={
        <ContactsRail
          workspaceId={workspaceId}
          view={view}
          permissions={permissions}
          contactsCount={totalQuery.data?.total_contacts}
          segments={segmentsQuery.data?.segments}
          segmentsLoading={segmentsQuery.isLoading}
          onSelect={selectView}
          onCreateList={() => setListDrawer('create')}
          onCreateSegment={() => setSegmentDrawerOpen(true)}
        />
      }
    >
      {gone ? (
        <div className="flex flex-col items-start gap-2 py-6 text-sm text-ink-2">
          <span>{listGone ? t`This list no longer exists` : t`This segment no longer exists`}</span>
          <Button type="link" className="px-0" onClick={backToAll}>
            {t`All contacts`}
          </Button>
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
            onExport={() => setExportOpen(true)}
            onContactCreated={invalidateAfterContactChange}
            onViewDeleted={handleViewDeleted}
          />
          {list ? (
            <ListHeader
              workspace={workspace}
              list={list}
              permissions={permissions}
              status={tableSearch.contact_list_status}
              onStatusChange={(status) =>
                go((prev) => ({ ...prev, contact_list_status: status, contact: undefined, limit: undefined }))
              }
              onEditList={() => setListDrawer('edit')}
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

          {canReadContacts ? (
            <BulkActionsBar
              selectedCount={selectedEmails.length}
              lists={lists}
              canWriteContacts={canWriteContacts}
              canWriteLists={canWriteLists}
              disabled={bulk.running}
              onAddToList={handleBulkAddToList}
              onRemoveFromList={handleBulkRemoveFromList}
              onUnsubscribeFromList={handleBulkUnsubscribe}
              onDelete={handleBulkDelete}
              onClear={() => setSelectedEmails([])}
            />
          ) : null}

          <Panel flush>
            {canReadContacts ? (
              <TableToolbar
                start={
                  <>
                    {statusTabs}
                    <TableToolbar.Search
                      value={searchInput}
                      onChange={setSearchInput}
                      placeholder={t`Email, name or external ID`}
                    />
                    <ContactsFilters
                      search={tableSearch}
                      fields={filterFields}
                      segments={segments}
                      onRemove={(key) =>
                        go((prev) => ({ ...prev, [key]: undefined, limit: undefined }), true)
                      }
                      onClear={clearFilters}
                    />
                  </>
                }
                end={
                  <>
                    <ContactColumnsSelector
                      columns={selectableColumns.map((column) => ({
                        ...column,
                        visible: !!visibleColumns[column.key]
                      }))}
                      onColumnVisibilityChange={setColumnVisible}
                    />
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
            atMax && hasMore ? (
              <div className="text-center text-sm text-ink-3">
                {t`Showing the first ${int(MAX_CONTACT_ROWS)} contacts. Refine your filters to see more.`}
              </div>
            ) : hasMore ? (
              <div className="flex justify-center">
                <Button
                  loading={table.isFetching}
                  onClick={() =>
                    go(
                      (prev) => ({ ...prev, limit: (prev.limit ?? CONTACTS_PAGE_SIZE) + CONTACTS_PAGE_SIZE }),
                      true
                    )
                  }
                >
                  {t`Load ${CONTACTS_PAGE_SIZE} more`}
                </Button>
              </div>
            ) : null
          ) : null}
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
        onDelete={setContactToDelete}
      />

      <DeleteContactModal
        visible={!!contactToDelete}
        onCancel={() => setContactToDelete(null)}
        onConfirm={() => contactToDelete && deleteMutation.mutate(contactToDelete)}
        contactEmail={contactToDelete || ''}
        loading={deleteMutation.isPending}
        disabled={!canWriteContacts}
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
        />
      ) : null}

      {segmentDrawerOpen ? (
        <DrawerSegment
          totalContacts={totalQuery.data?.total_contacts}
          canReadTemplates={!!permissions?.templates?.read}
          setDrawserVisible={setSegmentDrawerOpen}
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
    </RailLayout>
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
