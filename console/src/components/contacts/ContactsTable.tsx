import { Button, Dropdown, Table, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { MenuProps } from 'antd'
import { MoreHorizontal } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../lib/dayjs'
import type { Contact } from '../../services/api/contacts'
import type { Segment } from '../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../services/api/types'
import { getCustomFieldLabel } from '../../hooks/useCustomFieldLabel'
import { Pill } from '../layout'
import { JsonViewer } from './ContactColumnsSelector'
import { TrackingConsentSummary } from './TrackingConsent'
import { contactDisplayName, contactInitials, LIST_STATUS_LABEL, LIST_STATUS_TONE } from './contactDisplay'
import type { ContactsView } from './contactsView'

export const CONTACT_COLUMNS_STORAGE_KEY = 'contact_columns_visibility'

export const DEFAULT_VISIBLE_COLUMNS: Record<string, boolean> = {
  first_name: false,
  last_name: false,
  full_name: false,
  language: true,
  timezone: false,
  country: true,
  lists: true,
  segments: true,
  phone: false,
  address: false,
  job_title: false,
  created_at: false,
  custom_string_1: false,
  custom_string_2: false,
  custom_string_3: false,
  custom_string_4: false,
  custom_string_5: false,
  custom_number_1: false,
  custom_number_2: false,
  custom_number_3: false,
  custom_number_4: false,
  custom_number_5: false,
  custom_datetime_1: false,
  custom_datetime_2: false,
  custom_datetime_3: false,
  custom_datetime_4: false,
  custom_datetime_5: false,
  custom_json_1: false,
  custom_json_2: false,
  custom_json_3: false,
  custom_json_4: false,
  custom_json_5: false,
  tracking_consent: false
}

const CUSTOM_STRING = [1, 2, 3, 4, 5].map((n) => `custom_string_${n}` as `custom_string_${1 | 2 | 3 | 4 | 5}`)
const CUSTOM_NUMBER = [1, 2, 3, 4, 5].map((n) => `custom_number_${n}` as `custom_number_${1 | 2 | 3 | 4 | 5}`)
const CUSTOM_DATETIME = [1, 2, 3, 4, 5].map(
  (n) => `custom_datetime_${n}` as `custom_datetime_${1 | 2 | 3 | 4 | 5}`
)
const CUSTOM_JSON = [1, 2, 3, 4, 5].map((n) => `custom_json_${n}` as `custom_json_${1 | 2 | 3 | 4 | 5}`)

/** The columns a member can show or hide, in the selector's order. */
export function useSelectableColumns(workspace: Workspace | undefined): { key: string; title: string }[] {
  const { t } = useLingui()
  return [
    { key: 'lists', title: t`Lists` },
    { key: 'segments', title: t`Segments` },
    { key: 'first_name', title: t`First Name` },
    { key: 'last_name', title: t`Last Name` },
    { key: 'full_name', title: t`Full Name` },
    { key: 'phone', title: t`Phone` },
    { key: 'country', title: t`Country` },
    { key: 'language', title: t`Language` },
    { key: 'timezone', title: t`Timezone` },
    { key: 'address', title: t`Address` },
    { key: 'job_title', title: t`Job Title` },
    { key: 'created_at', title: t`Created At` },
    { key: 'tracking_consent', title: t`Tracking consent` },
    ...[...CUSTOM_STRING, ...CUSTOM_NUMBER, ...CUSTOM_DATETIME, ...CUSTOM_JSON].map((key) => ({
      key,
      title: getCustomFieldLabel(key, workspace)
    }))
  ]
}

export interface ContactsTableProps {
  workspace: Workspace
  view: ContactsView
  contacts: Contact[]
  loading: boolean
  lists: List[]
  segments: Segment[]
  permissions: UserPermissions | null
  visibleColumns: Record<string, boolean>
  selectedEmails: string[]
  onSelectionChange: (emails: string[]) => void
  onOpen: (email: string) => void
  onEdit: (contact: Contact) => void
  onDelete: (email: string) => void
  emptyText: React.ReactNode
}

/** The contacts of the view; a row opens the contact's sheet and its checkbox selects it. */
export function ContactsTable({
  workspace,
  view,
  contacts,
  loading,
  lists,
  segments,
  permissions,
  visibleColumns,
  selectedEmails,
  onSelectionChange,
  onOpen,
  onEdit,
  onDelete,
  emptyText
}: ContactsTableProps) {
  const { t, i18n } = useLingui()
  const timezone = workspace.settings?.timezone || 'UTC'
  const canWrite = !!permissions?.contacts?.write
  const canReadLists = !!permissions?.lists?.read
  const canReadSegments = !!permissions?.segments?.read
  const listName = (id: string) => lists.find((list) => list.id === id)?.name ?? id
  const fullDate = (value: string) => dayjs(value).tz(timezone).format('LLL')
  const statusLabel = (status: string) =>
    LIST_STATUS_LABEL[status] ? i18n._(LIST_STATUS_LABEL[status]) : status

  const hidden = (key: string) => !visibleColumns[key]

  const customColumns: ColumnsType<Contact> = [
    ...[...CUSTOM_STRING, ...CUSTOM_NUMBER].map((key) => ({
      title: getCustomFieldLabel(key, workspace),
      dataIndex: key,
      key,
      hidden: hidden(key)
    })),
    ...CUSTOM_DATETIME.map((key) => ({
      title: getCustomFieldLabel(key, workspace),
      key,
      render: (_: unknown, record: Contact) =>
        record[key] ? dayjs(record[key]).tz(timezone).format('LL') : '—',
      hidden: hidden(key)
    })),
    ...CUSTOM_JSON.map((key) => ({
      title: getCustomFieldLabel(key, workspace),
      key,
      render: (_: unknown, record: Contact) => (
        <JsonViewer json={record[key]} title={getCustomFieldLabel(key, workspace)} />
      ),
      hidden: hidden(key)
    }))
  ]

  const columns: ColumnsType<Contact> = [
    {
      title: t`Contact`,
      key: 'contact',
      fixed: 'left' as const,
      render: (_: unknown, record: Contact) => {
        const name = contactDisplayName(record)
        return (
          <div className="flex min-w-[220px] items-center gap-2.5">
            <span
              aria-hidden="true"
              className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary-50 text-2xs font-semibold text-primary-700"
            >
              {contactInitials(record)}
            </span>
            <span className="flex min-w-0 flex-col">
              {name ? <span className="truncate font-medium text-ink">{name}</span> : null}
              <span className={`truncate ${name ? 'text-xs text-ink-3' : 'text-ink'}`}>{record.email}</span>
            </span>
          </div>
        )
      }
    },
    view.kind === 'list'
      ? {
          title: t`Status in list`,
          key: 'list_status',
          hidden: !canReadLists,
          render: (_: unknown, record: Contact) => {
            const subscription = record.contact_lists?.find((cl) => cl.list_id === view.listId)
            if (!subscription) return null
            return (
              <span className="flex items-center gap-1.5 whitespace-nowrap">
                <Pill tone={LIST_STATUS_TONE[subscription.status] ?? 'neutral'}>
                  {statusLabel(subscription.status)}
                </Pill>
                {subscription.created_at ? (
                  <Tooltip title={fullDate(subscription.created_at)}>
                    <span className="text-xs text-ink-3">
                      {t`for ${dayjs(subscription.created_at).fromNow(true)}`}
                    </span>
                  </Tooltip>
                ) : null}
              </span>
            )
          }
        }
      : {
          title: t`Lists`,
          key: 'lists',
          hidden: !canReadLists || hidden('lists'),
          render: (_: unknown, record: Contact) => (
            <span className="flex max-w-[320px] flex-wrap gap-1">
              {(record.contact_lists ?? []).map((subscription) => (
                <Tooltip
                  key={subscription.list_id}
                  title={
                    <>
                      <div>{statusLabel(subscription.status)}</div>
                      {subscription.created_at ? (
                        <div>{t`Subscribed on: ${fullDate(subscription.created_at)}`}</div>
                      ) : null}
                    </>
                  }
                >
                  <span className="flex">
                    <Pill tone={LIST_STATUS_TONE[subscription.status] ?? 'neutral'}>
                      {listName(subscription.list_id)}
                    </Pill>
                  </span>
                </Tooltip>
              ))}
            </span>
          )
        },
    {
      title: t`Segments`,
      key: 'segments',
      hidden: !canReadSegments || hidden('segments'),
      render: (_: unknown, record: Contact) => (
        <span className="flex max-w-[280px] flex-wrap gap-x-2.5 gap-y-1">
          {(record.contact_segments ?? []).map((membership) => {
            const segment = segments.find((s) => s.id === membership.segment_id)
            return (
              <Tooltip
                key={membership.segment_id}
                title={
                  <>
                    <div className="font-medium">{segment?.name ?? membership.segment_id}</div>
                    <div>
                      {t`Matched on:`}{' '}
                      {membership.matched_at ? fullDate(membership.matched_at) : t`Unknown date`}
                    </div>
                    <div className="text-xs">
                      {t`Timezone:`} {timezone}
                    </div>
                  </>
                }
              >
                <span className="flex items-center gap-1 text-sm whitespace-nowrap text-ink-2">
                  <span
                    aria-hidden="true"
                    className="size-1.5 rounded-full"
                    // The segment's own colour, chosen by the user: not a theme token.
                    style={{ backgroundColor: segment?.color }}
                  />
                  {segment?.name ?? membership.segment_id}
                </span>
              </Tooltip>
            )
          })}
        </span>
      )
    },
    { title: t`First Name`, dataIndex: 'first_name', key: 'first_name', hidden: hidden('first_name') },
    { title: t`Last Name`, dataIndex: 'last_name', key: 'last_name', hidden: hidden('last_name') },
    { title: t`Full Name`, dataIndex: 'full_name', key: 'full_name', hidden: hidden('full_name') },
    { title: t`Phone`, dataIndex: 'phone', key: 'phone', hidden: hidden('phone') },
    { title: t`Country`, dataIndex: 'country', key: 'country', hidden: hidden('country') },
    { title: t`Language`, dataIndex: 'language', key: 'language', hidden: hidden('language') },
    { title: t`Timezone`, dataIndex: 'timezone', key: 'timezone', hidden: hidden('timezone') },
    {
      title: t`Tracking consent`,
      key: 'tracking_consent',
      render: (_: unknown, record: Contact) => (
        <TrackingConsentSummary
          opens={record.tracking_opens_consent}
          clicks={record.tracking_clicks_consent}
        />
      ),
      hidden: hidden('tracking_consent')
    },
    {
      title: t`Address`,
      key: 'address',
      render: (_: unknown, record: Contact) =>
        [record.address_line_1, record.address_line_2, record.state, record.postcode]
          .filter(Boolean)
          .join(', '),
      hidden: hidden('address')
    },
    { title: t`Job Title`, dataIndex: 'job_title', key: 'job_title', hidden: hidden('job_title') },
    {
      title: t`Created At`,
      key: 'created_at',
      render: (_: unknown, record: Contact) => (record.created_at ? fullDate(record.created_at) : '—'),
      hidden: hidden('created_at')
    },
    ...customColumns,
    {
      title: t`Added`,
      key: 'added',
      align: 'right' as const,
      render: (_: unknown, record: Contact) =>
        record.created_at ? (
          <Tooltip title={fullDate(record.created_at)}>
            <span className="text-sm whitespace-nowrap text-ink-3">{dayjs(record.created_at).fromNow()}</span>
          </Tooltip>
        ) : null
    },
    {
      title: '',
      key: 'actions',
      // Without contacts.write there is nothing to do from a row: the column goes.
      hidden: !canWrite,
      width: 48,
      fixed: 'right' as const,
      align: 'right' as const,
      // `contacts-actions-col` is only on the body cells: it is what the row click handler
      // looks for to leave the controls in this column alone.
      onCell: () => ({ className: 'contacts-actions-col' }),
      render: (_: unknown, record: Contact) => {
        const items: MenuProps['items'] = [
          { key: 'edit', label: t`Edit`, onClick: () => onEdit(record) },
          { key: 'delete', label: t`Delete`, danger: true, onClick: () => onDelete(record.email) }
        ]
        return (
          <Dropdown menu={{ items }} trigger={['click']} placement="bottomRight">
            <Button
              type="text"
              size="small"
              aria-label={t`Actions for ${record.email}`}
              icon={<MoreHorizontal size={15} aria-hidden="true" />}
            />
          </Dropdown>
        )
      }
    }
  ]

  return (
    <Table
      columns={columns.filter((column) => !column.hidden)}
      dataSource={contacts}
      rowKey={(record) => record.email}
      loading={loading}
      pagination={false}
      scroll={{ x: 'max-content' }}
      rowSelection={
        canWrite
          ? {
              selectedRowKeys: selectedEmails,
              onChange: (keys) => onSelectionChange(keys as string[]),
              columnWidth: 40
            }
          : undefined
      }
      onRow={(record) => ({
        onClick: (event) => {
          const target = event.target as HTMLElement
          // React events bubble through portals along the component tree, so a click in the
          // row's dropdown menu (rendered under document.body) lands here too: its DOM target is
          // outside the row.
          if (!event.currentTarget.contains(target)) return
          // The checkbox selects and the actions cell has its own menu. The actions cell is
          // matched through the class set on it above, which this file owns.
          if (target.closest('.ant-table-selection-column, .contacts-actions-col')) return
          onOpen(record.email)
        },
        className: 'cursor-pointer'
      })}
      locale={{ emptyText }}
    />
  )
}
