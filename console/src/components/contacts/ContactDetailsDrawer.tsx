import { useEffect, useRef, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import type { Segment } from '../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../services/api/types'
import { FullDrawer, useNumberFormat, type DrawerNav } from '../layout'
import { contactNavigation } from './contactNavigation'
import { MAX_CONTACT_ROWS, type ContactsTableSearch } from './contactsView'
import { ContactDetailsBody } from './sheet/ContactDetailsBody'
import { ContactSheetTopActions } from './sheet/ContactSheetTopActions'
import { useContactsTableQuery } from './useContactsTableQuery'

export interface ContactDetailsDrawerProps {
  workspace: Workspace
  /** The open contact (`?contact=`); absent, the sheet is closed. */
  email?: string
  /** The table's search: the sheet walks the table's loaded rows. */
  tableSearch: ContactsTableSearch
  tableEnabled: boolean
  /** « All contacts », the list's or the segment's name. */
  viewName: string
  /** The view's real total, when no search or filter applies. */
  exactTotal?: number
  lists: List[]
  segments: Segment[]
  permissions: UserPermissions | null
  onClose: () => void
  /** Shows another contact, replacing the current history entry. */
  onNavigate: (email: string) => void
  /** Loads 50 more rows, as « Load 50 more » does. */
  onLoadMore: () => void
  /** Deletes the contact, once confirmed in the bar's popconfirm. */
  onDelete: (email: string) => void
}

interface PendingNext {
  email: string
  /** The table's limit when J asked for more rows: the request is answered once it changes. */
  fromLimit: number | undefined
}

/** The contact sheet, a full-width drawer opened by the URL, with J and K through the table's rows. */
export function ContactDetailsDrawer({
  workspace,
  email,
  tableSearch,
  tableEnabled,
  viewName,
  exactTotal,
  lists,
  segments,
  permissions,
  onClose,
  onNavigate,
  onLoadMore,
  onDelete
}: ContactDetailsDrawerProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  // Always called, with the page's key and options: the page already fetched it, so opening or
  // walking the sheet reads the cache and never runs contacts.list again.
  const table = useContactsTableQuery(workspace.id, tableSearch, { enabled: tableEnabled })

  // While the drawer closes, keep showing the contact it showed, not an empty body.
  // State adjusted during render (React's pattern for deriving from a changed prop).
  const [lastEmail, setLastEmail] = useState(email)
  if (email && email !== lastEmail) setLastEmail(email)
  const shown = email ?? lastEmail

  const rows = table.data?.contacts ?? []
  const emails = rows.map((row) => row.email)
  const hasMore = !!table.data?.next_cursor && rows.length < MAX_CONTACT_ROWS
  const position = shown
    ? contactNavigation({ emails, email: shown, hasMore, exactTotal })
    : { needsMore: false }

  const [pendingNext, setPendingNext] = useState<PendingNext | null>(null)
  const emailsKey = emails.join('\n')
  // Given up when the sheet closes or the rows fail: adjusted during render, as for lastEmail.
  if (pendingNext && (!email || table.isError)) setPendingNext(null)
  // The request already answered. The router's store re-renders synchronously on every navigate,
  // ahead of the setPendingNext(null) below: without this the effect would see the request again
  // from that render and navigate again, forever (the tab froze).
  const answered = useRef<PendingNext | null>(null)
  useEffect(() => {
    if (!pendingNext || !email || table.isError) return
    if (answered.current === pendingNext) return
    if (tableSearch.limit === pendingNext.fromLimit || table.isPlaceholderData || table.isFetching) return
    answered.current = pendingNext
    // The rows can shift while they load (newest first): look the contact up again.
    const current = emailsKey.split('\n')
    const index = current.indexOf(pendingNext.email)
    // Cleared with the navigation it triggers, once the larger page has settled.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPendingNext(null)
    if (index !== -1 && index < current.length - 1) onNavigate(current[index + 1])
  }, [
    pendingNext,
    email,
    table.isError,
    table.isPlaceholderData,
    table.isFetching,
    tableSearch.limit,
    emailsKey,
    onNavigate
  ])

  const loading = pendingNext !== null
  const nav: DrawerNav = {
    buttons: position.position !== undefined,
    position: position.position,
    total: position.total,
    totalLabel: position.more && position.total !== undefined ? `${int(position.total)}+` : undefined,
    context: loading ? t`Loading…` : viewName,
    onPrev: position.prevEmail
      ? () => {
          setPendingNext(null)
          onNavigate(position.prevEmail!)
        }
      : undefined,
    onNext: loading
      ? undefined
      : position.nextEmail
        ? () => onNavigate(position.nextEmail!)
        : position.needsMore && shown
          ? () => {
              setPendingNext({ email: shown, fromLimit: tableSearch.limit })
              onLoadMore()
            }
          : undefined
  }

  return (
    <FullDrawer
      open={!!email}
      onClose={onClose}
      backLabel={t`Contacts`}
      nav={nav}
      topActions={
        shown ? (
          <ContactSheetTopActions
            key={shown}
            workspaceId={workspace.id}
            email={shown}
            canWriteContacts={!!permissions?.contacts?.write}
            onDelete={onDelete}
          />
        ) : undefined
      }
    >
      {shown ? (
        <ContactDetailsBody
          key={shown}
          workspace={workspace}
          email={shown}
          lists={lists}
          segments={segments}
          permissions={permissions}
          onClose={onClose}
        />
      ) : null}
    </FullDrawer>
  )
}
