import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import { Table, Tag, Button, Space, Tooltip, message, Dropdown } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { MenuProps } from 'antd'
import { useParams, useSearch } from '@tanstack/react-router'
import { contactsApi, type Contact, type ListContactsRequest } from '../services/api/contacts'
import { listsApi } from '../services/api/list'
import React from 'react'
import { workspaceContactsRoute } from '../router'
import { Filter } from '../components/filters/Filter'
import { ContactUpsertDrawer } from '../components/contacts/ContactUpsertDrawer'
import { ImportContactsButton } from '../components/contacts/ImportContactsButton'
import { BulkUpdateDrawer } from '../components/contacts/BulkUpdateDrawer'
import { CountriesFormOptions } from '../components/utils/countries_timezones'
import { Languages } from '../components/utils/languages'
import { FilterField } from '../components/filters/types'
import { ContactColumnsSelector, JsonViewer } from '../components/contacts/ContactColumnsSelector'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faEye, faHourglass } from '@fortawesome/free-regular-svg-icons'
import { faCircleCheck, faFaceFrown, faTrashAlt } from '@fortawesome/free-regular-svg-icons'
import {
  faBan,
  faTriangleExclamation,
  faRefresh,
  faPenToSquare,
  faEllipsisV
} from '@fortawesome/free-solid-svg-icons'
import { ContactDetailsDrawer } from '../components/contacts/ContactDetailsDrawer'
import { DeleteContactModal } from '../components/contacts/DeleteContactModal'
import dayjs from '../lib/dayjs'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'

const STORAGE_KEY = 'contact_columns_visibility'

const DEFAULT_VISIBLE_COLUMNS = {
  name: true,
  language: true,
  timezone: true,
  country: true,
  lists: true,
  phone: false,
  address: false,
  job_title: false,
  lifetime_value: false,
  orders_count: false,
  last_order_at: false,
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
  custom_json_5: false
}

export function ContactsPage() {
  const { workspaceId } = useParams({ from: '/workspace/$workspaceId/contacts' })
  const search = useSearch({ from: workspaceContactsRoute.id })
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)

  // Get the current workspace timezone
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)
  const workspaceTimezone = currentWorkspace?.settings.timezone || 'UTC'

  const [visibleColumns, setVisibleColumns] =
    React.useState<Record<string, boolean>>(DEFAULT_VISIBLE_COLUMNS)

  // Track accumulated contacts
  const [allContacts, setAllContacts] = React.useState<Contact[]>([])
  // Track cursor state internally instead of in URL
  const [currentCursor, setCurrentCursor] = React.useState<string | undefined>(undefined)
  // Delete modal state
  const [deleteModalVisible, setDeleteModalVisible] = React.useState(false)
  const [contactToDelete, setContactToDelete] = React.useState<string | null>(null)
  // Edit drawer state
  const [editDrawerVisible, setEditDrawerVisible] = React.useState(false)
  const [contactToEdit, setContactToEdit] = React.useState<Contact | null>(null)

  // Fetch lists for the current workspace
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId })
  })

  // Delete contact mutation
  const deleteContactMutation = useMutation({
    mutationFn: (email: string) =>
      contactsApi.delete({
        workspace_id: workspaceId,
        email: email
      }),
    onSuccess: (_, deletedEmail) => {
      message.success('Contact deleted successfully')
      // Remove the deleted contact from the local state
      setAllContacts((prev) => prev.filter((contact) => contact.email !== deletedEmail))
      // Invalidate and refetch the contacts query to ensure data consistency
      queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })
      // Close modal and reset state
      setDeleteModalVisible(false)
      setContactToDelete(null)
    },
    onError: (error: any) => {
      message.error(error?.message || 'Failed to delete contact')
    }
  })

  // Delete modal handlers
  const handleDeleteClick = (email: string) => {
    setContactToDelete(email)
    setDeleteModalVisible(true)
  }

  const handleDeleteCancel = () => {
    setDeleteModalVisible(false)
    setContactToDelete(null)
  }

  const handleDeleteConfirm = () => {
    if (contactToDelete) {
      deleteContactMutation.mutate(contactToDelete)
    }
  }

  // Edit drawer handlers
  const handleEditClick = (contact: Contact) => {
    setContactToEdit(contact)
    setEditDrawerVisible(true)
  }

  const handleEditClose = () => {
    setEditDrawerVisible(false)
    setContactToEdit(null)
  }

  const handleContactUpdate = (updatedContact: Contact) => {
    // Update the contact in the allContacts array
    setAllContacts((prev) =>
      prev.map((contact) => (contact.email === updatedContact.email ? updatedContact : contact))
    )
    handleEditClose()
  }

  const filterFields: FilterField[] = [
    { key: 'email', label: 'Email', type: 'string' as const },
    { key: 'external_id', label: 'External ID', type: 'string' as const },
    { key: 'first_name', label: 'First Name', type: 'string' as const },
    { key: 'last_name', label: 'Last Name', type: 'string' as const },
    { key: 'phone', label: 'Phone', type: 'string' as const },
    { key: 'language', label: 'Language', type: 'string' as const, options: Languages },
    { key: 'country', label: 'Country', type: 'string' as const, options: CountriesFormOptions },
    {
      key: 'list_id',
      label: 'List',
      type: 'string' as const,
      options:
        listsData?.lists?.map((list: { id: string; name: string }) => ({
          value: list.id,
          label: list.name
        })) || []
    },
    {
      key: 'contact_list_status',
      label: 'List Status',
      type: 'string' as const,
      options: [
        { value: 'active', label: 'Active' },
        { value: 'pending', label: 'Pending' },
        { value: 'unsubscribed', label: 'Unsubscribed' },
        { value: 'bounced', label: 'Bounced' },
        { value: 'complained', label: 'Complained' }
      ]
    }
  ]

  // Load saved state from localStorage on mount
  React.useEffect(() => {
    const savedState = localStorage.getItem(STORAGE_KEY)
    if (savedState) {
      const parsedState = JSON.parse(savedState)
      // Merge with defaults to ensure all fields exist
      setVisibleColumns({
        ...DEFAULT_VISIBLE_COLUMNS,
        ...parsedState
      })
    }
  }, [])

  const handleColumnVisibilityChange = (key: string, visible: boolean) => {
    setVisibleColumns((prev) => {
      const newState = { ...prev, [key]: visible }
      // Save to localStorage
      localStorage.setItem(STORAGE_KEY, JSON.stringify(newState))
      return newState
    })
  }

  const allColumns: { key: string; title: string }[] = [
    { key: 'lists', title: 'Lists' },
    { key: 'name', title: 'Name' },
    { key: 'phone', title: 'Phone' },
    { key: 'country', title: 'Country' },
    { key: 'language', title: 'Language' },
    { key: 'timezone', title: 'Timezone' },
    { key: 'address', title: 'Address' },
    { key: 'job_title', title: 'Job Title' },
    { key: 'lifetime_value', title: 'Lifetime Value' },
    { key: 'orders_count', title: 'Orders Count' },
    { key: 'last_order_at', title: 'Last Order' },
    { key: 'created_at', title: 'Created At' },
    { key: 'custom_string_1', title: 'Custom String 1' },
    { key: 'custom_string_2', title: 'Custom String 2' },
    { key: 'custom_string_3', title: 'Custom String 3' },
    { key: 'custom_string_4', title: 'Custom String 4' },
    { key: 'custom_string_5', title: 'Custom String 5' },
    { key: 'custom_number_1', title: 'Custom Number 1' },
    { key: 'custom_number_2', title: 'Custom Number 2' },
    { key: 'custom_number_3', title: 'Custom Number 3' },
    { key: 'custom_number_4', title: 'Custom Number 4' },
    { key: 'custom_number_5', title: 'Custom Number 5' },
    { key: 'custom_datetime_1', title: 'Custom Date 1' },
    { key: 'custom_datetime_2', title: 'Custom Date 2' },
    { key: 'custom_datetime_3', title: 'Custom Date 3' },
    { key: 'custom_datetime_4', title: 'Custom Date 4' },
    { key: 'custom_datetime_5', title: 'Custom Date 5' },
    { key: 'custom_json_1', title: 'Custom JSON 1' },
    { key: 'custom_json_2', title: 'Custom JSON 2' },
    { key: 'custom_json_3', title: 'Custom JSON 3' },
    { key: 'custom_json_4', title: 'Custom JSON 4' },
    { key: 'custom_json_5', title: 'Custom JSON 5' }
  ]

  const activeFilters = React.useMemo(() => {
    return Object.entries(search)
      .filter(
        ([key, value]) =>
          filterFields.some((field) => field.key === key) && value !== undefined && value !== ''
      )
      .map(([key, value]) => {
        const field = filterFields.find((f) => f.key === key)
        return {
          field: key,
          value,
          label: field?.label || key
        }
      })
  }, [search])

  // Force data refresh on mount
  React.useEffect(() => {
    // Reset the query on mount to force a refetch
    queryClient.resetQueries({ queryKey: ['contacts', workspaceId] })

    // Cleanup function to reset state when component unmounts
    return () => {
      setAllContacts([])
      setCurrentCursor(undefined)
    }
  }, [workspaceId, queryClient])

  const { data, isLoading, isFetching, refetch } = useQuery({
    queryKey: ['contacts', workspaceId, { ...search, cursor: currentCursor }],
    queryFn: async () => {
      const request: ListContactsRequest = {
        workspace_id: workspaceId,
        cursor: currentCursor,
        limit: search.limit || 10,
        email: search.email,
        external_id: search.external_id,
        first_name: search.first_name,
        last_name: search.last_name,
        phone: search.phone,
        country: search.country,
        language: search.language,
        list_id: search.list_id,
        contact_list_status: search.contact_list_status,
        with_contact_lists: true
      }
      return contactsApi.list(request)
    },
    // Reduce staleTime to make filter changes more responsive
    staleTime: 5000,
    refetchOnMount: true,
    refetchOnWindowFocus: false
  })

  // Update allContacts when data changes - modified to handle first load correctly
  React.useEffect(() => {
    // If data is still loading or not available, don't update
    if (isLoading || !data) return

    // If we have data
    if (data.contacts) {
      if (!currentCursor) {
        // Initial load or filter change - replace all contacts
        setAllContacts(data.contacts)
      } else if (data.contacts.length > 0) {
        // If we have a cursor and new contacts, append them
        setAllContacts((prev) => [...prev, ...data.contacts])
      }
    }
  }, [data, currentCursor, isLoading])

  // Reset contacts and cursor when filters change, and trigger a refetch
  React.useEffect(() => {
    // Reset accumulated contacts and cursor when search params change
    setAllContacts([])
    setCurrentCursor(undefined)

    // Reset the entire query to force a fresh fetch
    queryClient.resetQueries({ queryKey: ['contacts', workspaceId] })

    // Schedule a refetch (give time for the UI to update first)
    setTimeout(() => {
      refetch()
    }, 0)
  }, [
    search.email,
    search.external_id,
    search.first_name,
    search.last_name,
    search.phone,
    search.country,
    search.language,
    search.list_id,
    search.contact_list_status,
    search.limit,
    refetch,
    queryClient,
    workspaceId
  ])

  const handleRefresh = () => {
    // Reset accumulated contacts and cursor
    setAllContacts([])
    setCurrentCursor(undefined)
    // Reset and refetch the query
    queryClient.resetQueries({ queryKey: ['contacts', workspaceId] })
    queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
    refetch()
  }

  if (!currentWorkspace) {
    return <div>Loading...</div>
  }

  const columns: ColumnsType<Contact> = [
    {
      title: 'Email',
      dataIndex: 'email',
      key: 'email',
      fixed: 'left' as const,
      onHeaderCell: () => ({
        className: '!bg-white'
      })
    },
    {
      title: 'Lists',
      key: 'lists',
      render: (_: unknown, record: Contact) => (
        <Space direction="vertical" size={2}>
          {record.contact_lists.map(
            (list: { list_id: string; status?: string; created_at?: string }) => {
              let color = 'blue'
              let icon = null
              let statusText = ''

              // Match status to color and icon
              switch (list.status) {
                case 'active':
                  color = 'green'
                  icon = <FontAwesomeIcon icon={faCircleCheck} style={{ marginRight: '4px' }} />
                  statusText = 'Active subscriber'
                  break
                case 'pending':
                  color = 'blue'
                  icon = <FontAwesomeIcon icon={faHourglass} style={{ marginRight: '4px' }} />
                  statusText = 'Pending confirmation'
                  break
                case 'unsubscribed':
                  color = 'gray'
                  icon = <FontAwesomeIcon icon={faBan} style={{ marginRight: '4px' }} />
                  statusText = 'Unsubscribed from list'
                  break
                case 'bounced':
                  color = 'orange'
                  icon = (
                    <FontAwesomeIcon icon={faTriangleExclamation} style={{ marginRight: '4px' }} />
                  )
                  statusText = 'Email bounced'
                  break
                case 'complained':
                  color = 'red'
                  icon = <FontAwesomeIcon icon={faFaceFrown} style={{ marginRight: '4px' }} />
                  statusText = 'Marked as spam'
                  break
                default:
                  color = 'blue'
                  statusText = 'Status unknown'
                  break
              }

              // Find list name from listsData
              const listData = listsData?.lists?.find((l) => l.id === list.list_id)
              const listName = listData?.name || list.list_id

              // Format creation date if available using workspace timezone
              const creationDate = list.created_at
                ? dayjs(list.created_at).tz(workspaceTimezone).format('LL - HH:mm')
                : 'Unknown date'

              const tooltipTitle = (
                <>
                  <div>
                    <strong>{statusText}</strong>
                  </div>
                  <div>Subscribed on: {creationDate}</div>
                  <div>
                    <small>Timezone: {workspaceTimezone}</small>
                  </div>
                </>
              )

              return (
                <Tooltip key={list.list_id} title={tooltipTitle}>
                  <Tag bordered={false} color={color} style={{ marginBottom: '2px' }}>
                    {icon}
                    {listName}
                  </Tag>
                </Tooltip>
              )
            }
          )}
        </Space>
      ),
      hidden: !visibleColumns.lists
    },
    {
      title: 'Name',
      key: 'name',
      render: (_: unknown, record: Contact) =>
        `${record.first_name || ''} ${record.last_name || ''}`,
      hidden: !visibleColumns.name
    },
    {
      title: 'Phone',
      dataIndex: 'phone',
      key: 'phone',
      hidden: !visibleColumns.phone
    },
    {
      title: 'Country',
      dataIndex: 'country',
      key: 'country',
      hidden: !visibleColumns.country
    },
    {
      title: 'Language',
      dataIndex: 'language',
      key: 'language',
      hidden: !visibleColumns.language
    },
    {
      title: 'Timezone',
      dataIndex: 'timezone',
      key: 'timezone',
      hidden: !visibleColumns.timezone
    },
    {
      title: 'Address',
      key: 'address',
      render: (_: unknown, record: Contact) => {
        const parts = [
          record.address_line_1,
          record.address_line_2,
          record.state,
          record.postcode
        ].filter(Boolean)
        return parts.join(', ')
      },
      hidden: !visibleColumns.address
    },
    {
      title: 'Job Title',
      dataIndex: 'job_title',
      key: 'job_title',
      hidden: !visibleColumns.job_title
    },
    {
      title: 'Lifetime Value',
      dataIndex: 'lifetime_value',
      key: 'lifetime_value',
      render: (_: unknown, record: Contact) =>
        record.lifetime_value ? `$${record.lifetime_value.toFixed(2)}` : '-',
      hidden: !visibleColumns.lifetime_value
    },
    {
      title: 'Orders Count',
      dataIndex: 'orders_count',
      key: 'orders_count',
      hidden: !visibleColumns.orders_count
    },
    {
      title: 'Last Order',
      dataIndex: 'last_order_at',
      key: 'last_order_at',
      render: (_: unknown, record: Contact) =>
        record.last_order_at ? new Date(record.last_order_at).toLocaleDateString() : '-',
      hidden: !visibleColumns.last_order_at
    },
    {
      title: 'Created At',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (_: unknown, record: Contact) =>
        record.created_at
          ? dayjs(record.created_at).tz(workspaceTimezone).format('LL - HH:mm')
          : '-',
      hidden: !visibleColumns.created_at
    },
    {
      title: 'Custom String 1',
      dataIndex: 'custom_string_1',
      key: 'custom_string_1',
      hidden: !visibleColumns.custom_string_1
    },
    {
      title: 'Custom String 2',
      dataIndex: 'custom_string_2',
      key: 'custom_string_2',
      hidden: !visibleColumns.custom_string_2
    },
    {
      title: 'Custom String 3',
      dataIndex: 'custom_string_3',
      key: 'custom_string_3',
      hidden: !visibleColumns.custom_string_3
    },
    {
      title: 'Custom String 4',
      dataIndex: 'custom_string_4',
      key: 'custom_string_4',
      hidden: !visibleColumns.custom_string_4
    },
    {
      title: 'Custom String 5',
      dataIndex: 'custom_string_5',
      key: 'custom_string_5',
      hidden: !visibleColumns.custom_string_5
    },
    {
      title: 'Custom Number 1',
      dataIndex: 'custom_number_1',
      key: 'custom_number_1',
      hidden: !visibleColumns.custom_number_1
    },
    {
      title: 'Custom Number 2',
      dataIndex: 'custom_number_2',
      key: 'custom_number_2',
      hidden: !visibleColumns.custom_number_2
    },
    {
      title: 'Custom Number 3',
      dataIndex: 'custom_number_3',
      key: 'custom_number_3',
      hidden: !visibleColumns.custom_number_3
    },
    {
      title: 'Custom Number 4',
      dataIndex: 'custom_number_4',
      key: 'custom_number_4',
      hidden: !visibleColumns.custom_number_4
    },
    {
      title: 'Custom Number 5',
      dataIndex: 'custom_number_5',
      key: 'custom_number_5',
      hidden: !visibleColumns.custom_number_5
    },
    {
      title: 'Custom Date 1',
      dataIndex: 'custom_datetime_1',
      key: 'custom_datetime_1',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_1 ? new Date(record.custom_datetime_1).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_1
    },
    {
      title: 'Custom Date 2',
      dataIndex: 'custom_datetime_2',
      key: 'custom_datetime_2',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_2 ? new Date(record.custom_datetime_2).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_2
    },
    {
      title: 'Custom Date 3',
      dataIndex: 'custom_datetime_3',
      key: 'custom_datetime_3',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_3 ? new Date(record.custom_datetime_3).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_3
    },
    {
      title: 'Custom Date 4',
      dataIndex: 'custom_datetime_4',
      key: 'custom_datetime_4',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_4 ? new Date(record.custom_datetime_4).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_4
    },
    {
      title: 'Custom Date 5',
      dataIndex: 'custom_datetime_5',
      key: 'custom_datetime_5',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_5 ? new Date(record.custom_datetime_5).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_5
    },
    {
      title: 'Custom JSON 1',
      dataIndex: 'custom_json_1',
      key: 'custom_json_1',
      render: (_: unknown, record: Contact) => (
        <JsonViewer json={record.custom_json_1} title="Custom JSON 1" />
      ),
      hidden: !visibleColumns.custom_json_1
    },
    {
      title: 'Custom JSON 2',
      dataIndex: 'custom_json_2',
      key: 'custom_json_2',
      render: (_: unknown, record: Contact) => (
        <JsonViewer json={record.custom_json_2} title="Custom JSON 2" />
      ),
      hidden: !visibleColumns.custom_json_2
    },
    {
      title: 'Custom JSON 3',
      dataIndex: 'custom_json_3',
      key: 'custom_json_3',
      render: (_: unknown, record: Contact) => (
        <JsonViewer json={record.custom_json_3} title="Custom JSON 3" />
      ),
      hidden: !visibleColumns.custom_json_3
    },
    {
      title: 'Custom JSON 4',
      dataIndex: 'custom_json_4',
      key: 'custom_json_4',
      render: (_: unknown, record: Contact) => (
        <JsonViewer json={record.custom_json_4} title="Custom JSON 4" />
      ),
      hidden: !visibleColumns.custom_json_4
    },
    {
      title: 'Custom JSON 5',
      dataIndex: 'custom_json_5',
      key: 'custom_json_5',
      render: (_: unknown, record: Contact) => (
        <JsonViewer json={record.custom_json_5} title="Custom JSON 5" />
      ),
      hidden: !visibleColumns.custom_json_5
    },
    {
      title: (
        <Space size="small">
          <Tooltip title="Refresh">
            <Button
              type="text"
              size="small"
              icon={<FontAwesomeIcon icon={faRefresh} />}
              onClick={handleRefresh}
              className="opacity-70 hover:opacity-100"
            />
          </Tooltip>
          <ContactColumnsSelector
            columns={allColumns.map((col) => ({
              ...col,
              visible: visibleColumns[col.key]
            }))}
            onColumnVisibilityChange={handleColumnVisibilityChange}
          />
        </Space>
      ),
      key: 'actions',
      width: 120,
      fixed: 'right' as const,
      onHeaderCell: () => ({
        className: '!bg-white'
      }),
      render: (_: unknown, record: Contact) => {
        const menuItems: MenuProps['items'] = [
          {
            key: 'edit',
            label: 'Edit',
            disabled: !permissions?.contacts?.write,
            onClick: () => handleEditClick(record)
          },
          {
            key: 'delete',
            label: 'Delete',
            disabled: !permissions?.contacts?.write,
            onClick: () => handleDeleteClick(record.email)
          }
        ]

        return (
          <Space size="small">
            <Dropdown menu={{ items: menuItems }} trigger={['click']}>
              <Button type="text" icon={<FontAwesomeIcon icon={faEllipsisV} />} />
            </Dropdown>
            <ContactDetailsDrawer
              workspace={currentWorkspace}
              contactEmail={record.email}
              lists={listsData?.lists || []}
              key={record.email}
              onContactUpdate={(updatedContact) => {
                // Update the contact in the allContacts array
                setAllContacts((prev) =>
                  prev.map((contact) =>
                    contact.email === updatedContact.email ? updatedContact : contact
                  )
                )
              }}
              buttonProps={{
                icon: <FontAwesomeIcon icon={faEye} />,
                type: 'text'
              }}
            />
          </Space>
        )
      }
    }
  ].filter((col) => !col.hidden)

  const handleLoadMore = () => {
    if (data?.next_cursor) {
      setCurrentCursor(data.next_cursor)
    }
  }

  // Show empty state when there's no data and no loading
  const showEmptyState =
    !isLoading &&
    !isFetching &&
    (!data?.contacts || data.contacts.length === 0) &&
    allContacts.length === 0

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div className="text-2xl font-medium">Contacts</div>
        <Space>
          <Tooltip
            title={
              !permissions?.contacts?.write
                ? "You don't have write permission for contacts"
                : undefined
            }
          >
            <span>
              <BulkUpdateDrawer
                workspaceId={workspaceId}
                lists={listsData?.lists || []}
                buttonProps={{
                  type: 'default',
                  buttonContent: 'Bulk Update',
                  disabled: !permissions?.contacts?.write
                }}
              />
            </span>
          </Tooltip>
          <Tooltip
            title={
              !permissions?.contacts?.write
                ? "You don't have write permission for contacts"
                : undefined
            }
          >
            <span>
              <ImportContactsButton
                lists={listsData?.lists || []}
                workspaceId={workspaceId}
                disabled={!permissions?.contacts?.write}
              />
            </span>
          </Tooltip>
          <Tooltip
            title={
              !permissions?.contacts?.write
                ? "You don't have write permission for contacts"
                : undefined
            }
          >
            <div>
              <ContactUpsertDrawer
                workspace={currentWorkspace}
                buttonProps={{
                  buttonContent: 'Create',
                  disabled: !permissions?.contacts?.write
                }}
              />
            </div>
          </Tooltip>
        </Space>
      </div>

      <div className="flex justify-between items-center mb-6">
        <Filter fields={filterFields} activeFilters={activeFilters} />
      </div>

      <Table
        columns={columns}
        dataSource={allContacts}
        rowKey={(record) => record.email}
        loading={isLoading || isFetching}
        pagination={false}
        scroll={{ x: 'max-content' }}
        style={{ minWidth: 800 }}
        locale={{
          emptyText: showEmptyState
            ? 'No contacts found. Add some contacts to get started.'
            : 'Loading...'
        }}
        className="border border-gray-200 rounded-md"
      />

      {data?.next_cursor && (
        <div className="flex justify-center mt-4">
          <Button onClick={handleLoadMore} loading={isLoading || isFetching}>
            Load More
          </Button>
        </div>
      )}

      <DeleteContactModal
        visible={deleteModalVisible}
        onCancel={handleDeleteCancel}
        onConfirm={handleDeleteConfirm}
        contactEmail={contactToDelete || ''}
        loading={deleteContactMutation.isPending}
        disabled={!permissions?.contacts?.write}
      />

      {contactToEdit && (
        <ContactUpsertDrawer
          workspace={currentWorkspace}
          contact={contactToEdit}
          open={editDrawerVisible}
          onClose={handleEditClose}
          onContactUpdate={handleContactUpdate}
        />
      )}
    </div>
  )
}
