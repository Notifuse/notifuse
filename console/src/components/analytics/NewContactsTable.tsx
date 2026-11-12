import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Button, Empty, Skeleton, Table } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Panel } from '../layout'
import { contactsApi, type Contact } from '../../services/api/contacts'
import { useContactsCsvUpload } from '../contacts/ContactsCsvUploadProvider'
import { BlockError } from './BlockError'
import { DASHBOARD_REFRESH_MS } from './dashboardClient'
import { formatAge } from './dashboardFormat'
import { openableRow } from './dashboardTable'

interface NewContactsTableProps {
  workspaceId: string
  /** Shows the import button of the empty card. */
  canImport: boolean
}

const NEW_CONTACTS_LIMIT = 5

function contactName(contact: Contact): string {
  const name = [contact.first_name, contact.last_name].filter(Boolean).join(' ').trim()
  return name || contact.full_name?.trim() || ''
}

/** The five latest contacts, newest first as contacts.list returns them. */
export function NewContactsTable({ workspaceId, canImport }: NewContactsTableProps) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { openDrawer } = useContactsCsvUpload()

  const contacts = useQuery({
    // Under ['contacts', ws], which the CSV import invalidates when it finishes.
    queryKey: ['contacts', workspaceId, 'dashboard-latest'],
    queryFn: () =>
      contactsApi.list({
        workspace_id: workspaceId,
        limit: NEW_CONTACTS_LIMIT
      }),
    refetchInterval: DASHBOARD_REFRESH_MS
  })

  const openContact = (contact: Contact) =>
    void navigate({
      to: '/console/workspace/$workspaceId/contacts',
      params: { workspaceId },
      search: { email: contact.email }
    })

  const extra = (
    <Link to="/console/workspace/$workspaceId/contacts" params={{ workspaceId }}>
      {t`All contacts`}
    </Link>
  )

  const rows = contacts.data?.contacts ?? []
  const now = new Date()
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
          {canImport ? (
            <Button size="small" onClick={() => openDrawer(workspaceId)}>
              {t`Import contacts`}
            </Button>
          ) : null}
        </Empty>
      </div>
    )
  } else {
    body = (
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
    )
  }

  return (
    <Panel title={t`New contacts`} extra={extra} flush>
      {body}
    </Panel>
  )
}
