import { useState } from 'react'
import { Button, Dropdown, Popconfirm, Popover, Table, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { MenuProps } from 'antd'
import { MoreHorizontal } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import dayjs from '../../lib/dayjs'
import type { Contact } from '../../services/api/contacts'
import type { Segment } from '../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../services/api/types'
import { getCustomFieldLabel } from '../../hooks/useCustomFieldLabel'
import { Pill } from '../layout'
import { segmentDotColor } from '../segment/segmentColor'
import { JsonViewer } from './ContactColumnsSelector'
import { TrackingConsentSummary } from './TrackingConsent'
import { contactDisplayName, contactInitials, LIST_STATUS_LABEL, LIST_STATUS_TONE } from './contactDisplay'
import type { ContactsView } from './contactsView'
import { CUSTOM_DATETIME, CUSTOM_JSON, CUSTOM_NUMBER, CUSTOM_STRING } from './contactColumns'

export const CONTACT_COLUMNS_STORAGE_KEY = 'contact_columns_visibility'

// A row stays on one line: the first lists and segments, then « +N » naming the rest.
const SHOWN_LISTS = 2
const SHOWN_SEGMENTS = 1

/**
 * « +2 », with the names it stands for in a tooltip (and to screen readers). `tooltip={false}` when
 * the whole cell already opens a popover naming them.
 */
function MoreCount({ names, tooltip = true }: { names: string[]; tooltip?: boolean }) {
  const { t } = useLingui()
  if (names.length === 0) return null
  const count = names.length
  const joined = names.join(', ')
  const badge = (
    <span data-more-count className="shrink-0 text-xs text-ink-3 tabular-nums">
      <span aria-hidden="true">{t`+${count}`}</span>
      <span className="sr-only">
        {plural(count, { one: `and # more: ${joined}`, other: `and # more: ${joined}` })}
      </span>
    </span>
  )
  return tooltip ? <Tooltip title={joined}>{badge}</Tooltip> : badge
}

/** A segment's own colour, chosen by the user: not a theme token. */
function SegmentDot({ color, className }: { color?: string; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`size-1.5 shrink-0 rounded-full ${className ?? ''}`}
      style={{ backgroundColor: segmentDotColor(color) }}
    />
  )
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

/**
 * The row's « … » menu. Delete asks first, over the « … » button, as the sheet's delete does: one
 * contact, so a popconfirm rather than a dialog.
 */
function RowActions({
  contact,
  onEdit,
  onDelete
}: {
  contact: Contact
  onEdit: (contact: Contact) => void
  onDelete: (email: string) => void
}) {
  const { t } = useLingui()
  const [confirming, setConfirming] = useState(false)
  const items: MenuProps['items'] = [
    { key: 'edit', label: t`Edit`, onClick: () => onEdit(contact) },
    { key: 'delete', label: t`Delete`, danger: true, onClick: () => setConfirming(true) }
  ]
  return (
    <Popconfirm
      open={confirming}
      // Only the menu opens it: a click on « … » itself opens the menu, not the confirmation.
      onOpenChange={(open) => {
        if (!open) setConfirming(false)
      }}
      title={t`Delete contact`}
      description={t`This will permanently remove the contact and their subscriptions.`}
      okText={t`Delete`}
      cancelText={t`Cancel`}
      okButtonProps={{ danger: true }}
      placement="bottomRight"
      onConfirm={() => {
        setConfirming(false)
        onDelete(contact.email)
      }}
    >
      <Dropdown menu={{ items }} trigger={['click']} placement="bottomRight">
        <Button
          type="text"
          aria-label={t`Actions for ${contact.email}`}
          icon={<MoreHorizontal size={15} aria-hidden="true" />}
        />
      </Dropdown>
    </Popconfirm>
  )
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
  const statusLabel = (status: string) => (LIST_STATUS_LABEL[status] ? i18n._(LIST_STATUS_LABEL[status]) : status)

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
      render: (_: unknown, record: Contact) => (record[key] ? dayjs(record[key]).tz(timezone).format('LL') : '—'),
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
              className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary-50 text-2xs font-semibold text-primary-700"
            >
              {contactInitials(record)}
            </span>
            <span className="flex min-w-0 flex-col leading-tight">
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
                    <span className="text-xs text-ink-3">{t`for ${dayjs(subscription.created_at).fromNow(true)}`}</span>
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
          render: (_: unknown, record: Contact) => {
            // The server gives them in no set order: by name, so « +N » always hides the same ones.
            const subscriptions = [...(record.contact_lists ?? [])].sort((a, b) =>
              listName(a.list_id).localeCompare(listName(b.list_id))
            )
            return (
              <span className="flex items-center gap-1 whitespace-nowrap">
                {subscriptions.slice(0, SHOWN_LISTS).map((subscription) => (
                  <Tooltip
                    key={subscription.list_id}
                    title={
                      <>
                        <div className="font-medium">{listName(subscription.list_id)}</div>
                        <div>{statusLabel(subscription.status)}</div>
                        {subscription.created_at ? (
                          <div>{t`Subscribed on: ${fullDate(subscription.created_at)}`}</div>
                        ) : null}
                      </>
                    }
                  >
                    <span className="flex">
                      <Pill tone={LIST_STATUS_TONE[subscription.status] ?? 'neutral'}>
                        {/* A long name is cut: the tooltip gives it whole. */}
                        <span className="max-w-32 truncate">{listName(subscription.list_id)}</span>
                      </Pill>
                    </span>
                  </Tooltip>
                ))}
                <MoreCount
                  names={subscriptions.slice(SHOWN_LISTS).map((subscription) => listName(subscription.list_id))}
                />
              </span>
            )
          }
        },
    {
      title: t`Segments`,
      key: 'segments',
      hidden: !canReadSegments || hidden('segments'),
      render: (_: unknown, record: Contact) => {
        const segmentName = (id: string) => segments.find((s) => s.id === id)?.name ?? id
        // In a segment view that segment comes first, as every row belongs to it; then by name.
        const openSegment = view.kind === 'segment' ? view.segmentId : undefined
        const memberships = [...(record.contact_segments ?? [])].sort(
          (a, b) =>
            Number(b.segment_id === openSegment) - Number(a.segment_id === openSegment) ||
            segmentName(a.segment_id).localeCompare(segmentName(b.segment_id))
        )
        if (memberships.length === 0) return null
        const segmentOf = (id: string) => segments.find((s) => s.id === id)
        // One popover for the whole cell: every segment of the contact, with when it matched.
        const details = (
          <div data-segments-popover className="flex max-w-80 flex-col gap-2">
            {memberships.map((membership) => (
              <div key={membership.segment_id} className="flex items-start gap-2">
                <SegmentDot color={segmentOf(membership.segment_id)?.color} className="mt-1.5" />
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium text-ink">{segmentName(membership.segment_id)}</div>
                  <div className="text-xs text-ink-3">
                    {t`Matched on:`} {membership.matched_at ? fullDate(membership.matched_at) : t`Unknown date`}
                  </div>
                </div>
              </div>
            ))}
            <div className="border-t border-line-soft pt-2 text-xs text-ink-3">
              {t`Timezone:`} {timezone}
            </div>
          </div>
        )
        return (
          <Popover content={details}>
            <span data-segments-cell className="inline-flex items-center gap-1.5 whitespace-nowrap">
              {memberships.slice(0, SHOWN_SEGMENTS).map((membership) => (
                <span key={membership.segment_id} className="flex items-center gap-1 text-sm text-ink-2">
                  <SegmentDot color={segmentOf(membership.segment_id)?.color} />
                  <span className="max-w-40 truncate">{segmentName(membership.segment_id)}</span>
                </span>
              ))}
              <MoreCount
                tooltip={false}
                names={memberships.slice(SHOWN_SEGMENTS).map((membership) => segmentName(membership.segment_id))}
              />
            </span>
          </Popover>
        )
      }
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
        <TrackingConsentSummary opens={record.tracking_opens_consent} clicks={record.tracking_clicks_consent} />
      ),
      hidden: hidden('tracking_consent')
    },
    {
      title: t`Address`,
      key: 'address',
      render: (_: unknown, record: Contact) =>
        [record.address_line_1, record.address_line_2, record.state, record.postcode].filter(Boolean).join(', '),
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
      render: (_: unknown, record: Contact) => <RowActions contact={record} onEdit={onEdit} onDelete={onDelete} />
    }
  ]

  return (
    <Table
      columns={columns.filter((column) => !column.hidden)}
      dataSource={contacts}
      rowKey={(record) => record.email}
      loading={loading}
      size="small"
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
