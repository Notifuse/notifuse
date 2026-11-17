import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Empty, Skeleton, Table } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Panel } from '../layout'
import type { Contact } from '../../services/api/contacts'
import { BlockError } from './BlockError'
import { DASHBOARD_REFRESH_MS } from './dashboardClient'
import { latestContactsQuery } from './dashboardQueries'
import { formatAge } from './dashboardFormat'
import { openableRow } from './dashboardTable'
import { hasOnlyOwnerContact } from './getStartedModel'
import { ImportContactsAction } from './ImportContactsAction'

interface NewContactsTableProps {
  workspaceId: string
  /** Shows the import button of the empty card. */
  canImport: boolean
  /** Offers the workspace's lists in the import. */
  canReadLists?: boolean
  /**
   * The owner's emails, lower-cased: a workspace starts with its creator as its only contact, and a
   * card listing only them is as empty as one with none, so it offers the import too.
   */
  ownerEmails?: Set<string>
}

const NO_OWNERS = new Set<string>()

function contactName(contact: Contact): string {
  const name = [contact.first_name, contact.last_name].filter(Boolean).join(' ').trim()
  return name || contact.full_name?.trim() || ''
}

/** The five latest contacts, newest first as contacts.list returns them. */
export function NewContactsTable({
  workspaceId,
  canImport,
  canReadLists = false,
  ownerEmails = NO_OWNERS
}: NewContactsTableProps) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()

  const contacts = useQuery({
    ...latestContactsQuery(workspaceId),
    refetchInterval: DASHBOARD_REFRESH_MS
  })

  // The contact's sheet, over the whole list: an email filter left a list of one behind it.
  const openContact = (contact: Contact) =>
    void navigate({
      to: '/console/workspace/$workspaceId/contacts',
      params: { workspaceId },
      search: { contact: contact.email }
    })

  const extra = (
    <Link to="/console/workspace/$workspaceId/contacts" params={{ workspaceId }}>
      {t`All contacts`}
    </Link>
  )

  const rows = contacts.data?.contacts ?? []
  const now = new Date()
  const importButton = canImport ? (
    <ImportContactsAction workspaceId={workspaceId} canReadLists={canReadLists}>
      {t`Import contacts`}
    </ImportContactsAction>
  ) : null
  let body
  if (contacts.isError && !contacts.data) {
    body = <BlockError error={contacts.error} onRetry={() => void contacts.refetch()} />
  } else if (contacts.isLoading) {
    body = (
      <div className="px-3.5 py-3">
        <Skeleton active paragraph={{ rows: 3 }} title={false} />
      </div>
    )
  } else if (rows.length === 0) {
    body = (
      <div className="py-4">
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No contact yet`}>
          {importButton}
        </Empty>
      </div>
    )
  } else {
    const ownerOnly = hasOnlyOwnerContact(rows, ownerEmails)
    body = (
      <>
      <Table<Contact>
        size="small"
        showHeader={false}
        pagination={false}
        rowKey="email"
        dataSource={rows}
        onRow={(contact) => openableRow(() => openContact(contact))}
        columns={[
          {
            key: 'age',
            width: 96,
            render: (_, contact) => (
              <span className="whitespace-nowrap text-xs text-ink-3 tabular-nums">
                {formatAge(contact.created_at, now, i18n.locale)}
              </span>
            )
          },
          {
            key: 'email',
            ellipsis: true,
            render: (_, contact) => <span className="text-ink">{contact.email}</span>
          },
          {
            key: 'name',
            ellipsis: true,
            render: (_, contact) => <span className="text-ink-3">{contactName(contact)}</span>
          }
        ]}
      />
      {ownerOnly ? (
        <div className="flex flex-col gap-2 border-t border-line-soft bg-subtle px-3.5 py-2.5">
          <span className="text-sm text-ink-2">
            {t`Only you so far. Import a CSV, or add contacts from your app through the API.`}
          </span>
          {importButton ? <div>{importButton}</div> : null}
        </div>
      ) : null}
      </>
    )
  }

  return (
    <Panel title={t`New contacts`} extra={extra} flush>
      {body}
    </Panel>
  )
}
