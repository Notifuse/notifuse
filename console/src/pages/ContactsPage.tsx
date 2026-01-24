import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import { Table, Tag, Button, Space, Tooltip, message, Dropdown } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { MenuProps } from 'antd'
import { useParams, useSearch, useNavigate } from '@tanstack/react-router'
import { contactsApi, type Contact, type ListContactsRequest } from '../services/api/contacts'
import { listsApi } from '../services/api/list'
import { listSegments } from '../services/api/segment'
import React from 'react'
import { workspaceContactsRoute } from '../router'
import { Filter } from '../components/filters/Filter'
import { ContactUpsertDrawer } from '../components/contacts/ContactUpsertDrawer'
import { BulkUpdateDrawer } from '../components/contacts/BulkUpdateDrawer'
import { CountriesFormOptions } from '../lib/countries_timezones'
import { Languages } from '../lib/languages'
import { FilterField } from '../components/filters/types'
import { ContactColumnsSelector, JsonViewer } from '../components/contacts/ContactColumnsSelector'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faEye, faHourglass } from '@fortawesome/free-regular-svg-icons'
import { faCircleCheck, faFaceFrown } from '@fortawesome/free-regular-svg-icons'
import {
  faBan,
  faTriangleExclamation,
  faRefresh,
  faEllipsisV
} from '@fortawesome/free-solid-svg-icons'
import { ContactDetailsDrawer } from '../components/contacts/ContactDetailsDrawer'
import { DeleteContactModal } from '../components/contacts/DeleteContactModal'
import { SegmentsFilter } from '../components/contacts/SegmentsFilter'
import dayjs from '../lib/dayjs'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import numbro from 'numbro'
import { PlusOutlined, DownloadOutlined, UploadOutlined } from '@ant-design/icons'
import { ExportContactsModal } from '../components/contacts/ExportContactsModal'
import { useContactsCsvUpload } from '../components/contacts/ContactsCsvUploadProvider'
import { getCustomFieldLabel } from '../hooks/useCustomFieldLabel'
import { useLingui } from '@lingui/react/macro'

const STORAGE_KEY = 'contact_columns_visibility'

const DEFAULT_VISIBLE_COLUMNS = {
  first_name: true,
  last_name: true,
  full_name: false,
  language: true,
  timezone: true,
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
  custom_json_5: false
}

export function ContactsPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/contacts' })
  const search = useSearch({ from: workspaceContactsRoute.id })
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const { openDrawer: openImportDrawer } = useContactsCsvUpload()

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
  // Export modal state
  const [exportModalVisible, setExportModalVisible] = React.useState(false)

  // Fetch lists for the current workspace
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId })
  })

  // Fetch segments for the current workspace with contact counts
  const { data: segmentsData } = useQuery({
    queryKey: ['segments', workspaceId],
    queryFn: () => listSegments({ workspace_id: workspaceId, with_count: true }),
    refetchInterval: (query) => {
      // Check if any segment is building
      const hasBuilding = query.state.data?.segments?.some(
        (segment: { status?: string }) => segment.status === 'building'
      )
      return hasBuilding ? 15000 : false // 15 seconds if building, otherwise no interval
    }
  })

  // Fetch total contacts count
  const { data: totalContactsData } = useQuery({
    queryKey: ['total-contacts', workspaceId],
    queryFn: () => contactsApi.getTotalContacts({ workspace_id: workspaceId })
  })

  // Delete contact mutation
  const deleteContactMutation = useMutation({
    mutationFn: (email: string) =>
      contactsApi.delete({
        workspace_id: workspaceId,
        email: email
      }),
    onSuccess: (_, deletedEmail) => {
      message.success(t`Contact deleted successfully`)
      // Remove the deleted contact from the local state
      setAllContacts((prev) => prev.filter((contact) => contact.email !== deletedEmail))
      // Invalidate and refetch the contacts query to ensure data consistency
      queryClient.invalidateQueries({ queryKey: ['contacts', workspaceId] })
      // Invalidate total contacts count
      queryClient.invalidateQueries({ queryKey: ['total-contacts', workspaceId] })
      // Close modal and reset state
      setDeleteModalVisible(false)
      setContactToDelete(null)
    },
    onError: (error: Error) => {
      message.error(error?.message || t`Failed to delete contact`)
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

  const filterFields: FilterField[] = React.useMemo(() => [
    { key: 'email', label: t`Email`, type: 'string' as const },
    { key: 'external_id', label: t`External ID`, type: 'string' as const },
    { key: 'first_name', label: t`First Name`, type: 'string' as const },
    { key: 'last_name', label: t`Last Name`, type: 'string' as const },
    { key: 'full_name', label: t`Full Name`, type: 'string' as const },
    { key: 'phone', label: t`Phone`, type: 'string' as const },
    { key: 'language', label: t`Language`, type: 'string' as const, options: Languages },
    { key: 'country', label: t`Country`, type: 'string' as const, options: CountriesFormOptions },
    {
      key: 'list_id',
      label: t`List`,
      type: 'string' as const,
      options:
        listsData?.lists?.map((list: { id: string; name: string }) => ({
          value: list.id,
          label: list.name
        })) || []
    },
    {
      key: 'contact_list_status',
      label: t`List Status`,
      type: 'string' as const,
      options: [
        { value: 'active', label: t`Active` },
        { value: 'pending', label: t`Pending` },
        { value: 'unsubscribed', label: t`Unsubscribed` },
        { value: 'bounced', label: t`Bounced` },
        { value: 'complained', label: t`Complained` }
      ]
    }
  ], [listsData?.lists, t])

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
    { key: 'custom_string_1', title: getCustomFieldLabel('custom_string_1', currentWorkspace) },
    { key: 'custom_string_2', title: getCustomFieldLabel('custom_string_2', currentWorkspace) },
    { key: 'custom_string_3', title: getCustomFieldLabel('custom_string_3', currentWorkspace) },
    { key: 'custom_string_4', title: getCustomFieldLabel('custom_string_4', currentWorkspace) },
    { key: 'custom_string_5', title: getCustomFieldLabel('custom_string_5', currentWorkspace) },
    { key: 'custom_number_1', title: getCustomFieldLabel('custom_number_1', currentWorkspace) },
    { key: 'custom_number_2', title: getCustomFieldLabel('custom_number_2', currentWorkspace) },
    { key: 'custom_number_3', title: getCustomFieldLabel('custom_number_3', currentWorkspace) },
    { key: 'custom_number_4', title: getCustomFieldLabel('custom_number_4', currentWorkspace) },
    { key: 'custom_number_5', title: getCustomFieldLabel('custom_number_5', currentWorkspace) },
    { key: 'custom_datetime_1', title: getCustomFieldLabel('custom_datetime_1', currentWorkspace) },
    { key: 'custom_datetime_2', title: getCustomFieldLabel('custom_datetime_2', currentWorkspace) },
    { key: 'custom_datetime_3', title: getCustomFieldLabel('custom_datetime_3', currentWorkspace) },
    { key: 'custom_datetime_4', title: getCustomFieldLabel('custom_datetime_4', currentWorkspace) },
    { key: 'custom_datetime_5', title: getCustomFieldLabel('custom_datetime_5', currentWorkspace) },
    { key: 'custom_json_1', title: getCustomFieldLabel('custom_json_1', currentWorkspace) },
    { key: 'custom_json_2', title: getCustomFieldLabel('custom_json_2', currentWorkspace) },
    { key: 'custom_json_3', title: getCustomFieldLabel('custom_json_3', currentWorkspace) },
    { key: 'custom_json_4', title: getCustomFieldLabel('custom_json_4', currentWorkspace) },
    { key: 'custom_json_5', title: getCustomFieldLabel('custom_json_5', currentWorkspace) }
  ]

  const activeFilters = React.useMemo(() => {
    return Object.entries(search)
      .filter(
        ([key, value]) =>
          key !== 'segments' && // Exclude segments as they are shown separately
          key !== 'limit' && // Exclude limit as it's a pagination param
          key !== 'cursor' && // Exclude cursor as it's a pagination param
          filterFields.some((field) => field.key === key) &&
          value !== undefined &&
          value !== ''
      )
      .map(([key, value]) => {
        const field = filterFields.find((f) => f.key === key)
        return {
          field: key,
          value: value as string | number | boolean | Date,
          label: field?.label || key
        }
      })
  }, [search, filterFields])

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
        full_name: search.full_name,
        phone: search.phone,
        country: search.country,
        language: search.language,
        list_id: search.list_id,
        contact_list_status: search.contact_list_status,
        segments: search.segments,
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
    search.full_name,
    search.phone,
    search.country,
    search.language,
    search.list_id,
    search.contact_list_status,
    search.segments,
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
    queryClient.invalidateQueries({ queryKey: ['total-contacts', workspaceId] })
    refetch()
  }

  if (!currentWorkspace) {
    return <div>{t`Loading...`}</div>
  }

  const columns: ColumnsType<Contact> = [
    {
      title: t`Email`,
      dataIndex: 'email',
      key: 'email',
      fixed: 'left' as const,
      onHeaderCell: () => ({
        style: { backgroundColor: '#F9F9F9' }
      }),
      onCell: () => ({
        style: { backgroundColor: '#F9F9F9' }
      })
    },
    {
      title: t`Lists`,
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
                  statusText = t`Active subscriber`
                  break
                case 'pending':
                  color = 'blue'
                  icon = <FontAwesomeIcon icon={faHourglass} style={{ marginRight: '4px' }} />
                  statusText = t`Pending confirmation`
                  break
                case 'unsubscribed':
                  color = 'gray'
                  icon = <FontAwesomeIcon icon={faBan} style={{ marginRight: '4px' }} />
                  statusText = t`Unsubscribed from list`
                  break
                case 'bounced':
                  color = 'orange'
                  icon = (
                    <FontAwesomeIcon icon={faTriangleExclamation} style={{ marginRight: '4px' }} />
                  )
                  statusText = t`Email bounced`
                  break
                case 'complained':
                  color = 'red'
                  icon = <FontAwesomeIcon icon={faFaceFrown} style={{ marginRight: '4px' }} />
                  statusText = t`Marked as spam`
                  break
                default:
                  color = 'blue'
                  statusText = t`Status unknown`
                  break
              }

              // Find list name from listsData
              const listData = listsData?.lists?.find((l) => l.id === list.list_id)
              const listName = listData?.name || list.list_id

              // Format creation date if available using workspace timezone
              const creationDate = list.created_at
                ? dayjs(list.created_at).tz(workspaceTimezone).format('LL - HH:mm')
                : t`Unknown date`

              const tooltipTitle = (
                <>
                  <div>
                    <strong>{statusText}</strong>
                  </div>
                  <div>{t`Subscribed on:`} {creationDate}</div>
                  <div>
                    <small>{t`Timezone:`} {workspaceTimezone}</small>
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
      title: t`Segments`,
      key: 'segments',
      render: (_: unknown, record: Contact) => (
        <Space direction="vertical" size={2}>
          {record.contact_segments?.map(
            (segment: {
              segment_id: string
              version?: number
              matched_at?: string
              computed_at?: string
            }) => {
              // Find segment data from segmentsData to get the name and color
              const segmentData = segmentsData?.segments?.find((s) => s.id === segment.segment_id)
              const segmentName = segmentData?.name || segment.segment_id
              const segmentColor = segmentData?.color || '#1890ff'

              // Format matched date if available using workspace timezone
              const matchedDate = segment.matched_at
                ? dayjs(segment.matched_at).tz(workspaceTimezone).format('LL - HH:mm')
                : t`Unknown date`

              const tooltipTitle = (
                <>
                  <div>
                    <strong>{segmentName}</strong>
                  </div>
                  <div>{t`Matched on:`} {matchedDate}</div>
                  {segment.version && <div>{t`Version:`} {segment.version}</div>}
                  <div>
                    <small>{t`Timezone:`} {workspaceTimezone}</small>
                  </div>
                </>
              )

              return (
                <Tooltip key={segment.segment_id} title={tooltipTitle}>
                  <Tag bordered={false} color={segmentColor} style={{ marginBottom: '2px' }}>
                    {segmentName}
                  </Tag>
                </Tooltip>
              )
            }
          ) || []}
        </Space>
      ),
      hidden: !visibleColumns.segments
    },
    {
      title: t`First Name`,
      dataIndex: 'first_name',
      key: 'first_name',
      render: (_: unknown, record: Contact) => record.first_name || '-',
      hidden: !visibleColumns.first_name
    },
    {
      title: t`Last Name`,
      dataIndex: 'last_name',
      key: 'last_name',
      render: (_: unknown, record: Contact) => record.last_name || '-',
      hidden: !visibleColumns.last_name
    },
    {
      title: t`Full Name`,
      dataIndex: 'full_name',
      key: 'full_name',
      render: (_: unknown, record: Contact) => record.full_name || '-',
      hidden: !visibleColumns.full_name
    },
    {
      title: t`Phone`,
      dataIndex: 'phone',
      key: 'phone',
      hidden: !visibleColumns.phone
    },
    {
      title: t`Country`,
      dataIndex: 'country',
      key: 'country',
      hidden: !visibleColumns.country
    },
    {
      title: t`Language`,
      dataIndex: 'language',
      key: 'language',
      hidden: !visibleColumns.language
    },
    {
      title: t`Timezone`,
      dataIndex: 'timezone',
      key: 'timezone',
      hidden: !visibleColumns.timezone
    },
    {
      title: t`Address`,
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
      title: t`Job Title`,
      dataIndex: 'job_title',
      key: 'job_title',
      hidden: !visibleColumns.job_title
    },
    {
      title: t`Created At`,
      dataIndex: 'created_at',
      key: 'created_at',
      render: (_: unknown, record: Contact) =>
        record.created_at
          ? dayjs(record.created_at).tz(workspaceTimezone).format('LL - HH:mm')
          : '-',
      hidden: !visibleColumns.created_at
    },
    {
      title: getCustomFieldLabel('custom_string_1', currentWorkspace),
      dataIndex: 'custom_string_1',
      key: 'custom_string_1',
      hidden: !visibleColumns.custom_string_1
    },
    {
      title: getCustomFieldLabel('custom_string_2', currentWorkspace),
      dataIndex: 'custom_string_2',
      key: 'custom_string_2',
      hidden: !visibleColumns.custom_string_2
    },
    {
      title: getCustomFieldLabel('custom_string_3', currentWorkspace),
      dataIndex: 'custom_string_3',
      key: 'custom_string_3',
      hidden: !visibleColumns.custom_string_3
    },
    {
      title: getCustomFieldLabel('custom_string_4', currentWorkspace),
      dataIndex: 'custom_string_4',
      key: 'custom_string_4',
      hidden: !visibleColumns.custom_string_4
    },
    {
      title: getCustomFieldLabel('custom_string_5', currentWorkspace),
      dataIndex: 'custom_string_5',
      key: 'custom_string_5',
      hidden: !visibleColumns.custom_string_5
    },
    {
      title: getCustomFieldLabel('custom_number_1', currentWorkspace),
      dataIndex: 'custom_number_1',
      key: 'custom_number_1',
      hidden: !visibleColumns.custom_number_1
    },
    {
      title: getCustomFieldLabel('custom_number_2', currentWorkspace),
      dataIndex: 'custom_number_2',
      key: 'custom_number_2',
      hidden: !visibleColumns.custom_number_2
    },
    {
      title: getCustomFieldLabel('custom_number_3', currentWorkspace),
      dataIndex: 'custom_number_3',
      key: 'custom_number_3',
      hidden: !visibleColumns.custom_number_3
    },
    {
      title: getCustomFieldLabel('custom_number_4', currentWorkspace),
      dataIndex: 'custom_number_4',
      key: 'custom_number_4',
      hidden: !visibleColumns.custom_number_4
    },
    {
      title: getCustomFieldLabel('custom_number_5', currentWorkspace),
      dataIndex: 'custom_number_5',
      key: 'custom_number_5',
      hidden: !visibleColumns.custom_number_5
    },
    {
      title: getCustomFieldLabel('custom_datetime_1', currentWorkspace),
      dataIndex: 'custom_datetime_1',
      key: 'custom_datetime_1',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_1 ? new Date(record.custom_datetime_1).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_1
    },
    {
      title: getCustomFieldLabel('custom_datetime_2', currentWorkspace),
      dataIndex: 'custom_datetime_2',
      key: 'custom_datetime_2',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_2 ? new Date(record.custom_datetime_2).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_2
    },
    {
      title: getCustomFieldLabel('custom_datetime_3', currentWorkspace),
      dataIndex: 'custom_datetime_3',
      key: 'custom_datetime_3',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_3 ? new Date(record.custom_datetime_3).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_3
    },
    {
      title: getCustomFieldLabel('custom_datetime_4', currentWorkspace),
      dataIndex: 'custom_datetime_4',
      key: 'custom_datetime_4',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_4 ? new Date(record.custom_datetime_4).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_4
    },
    {
      title: getCustomFieldLabel('custom_datetime_5', currentWorkspace),
      dataIndex: 'custom_datetime_5',
      key: 'custom_datetime_5',
      render: (_: unknown, record: Contact) =>
        record.custom_datetime_5 ? new Date(record.custom_datetime_5).toLocaleDateString() : '-',
      hidden: !visibleColumns.custom_datetime_5
    },
    {
      title: getCustomFieldLabel('custom_json_1', currentWorkspace),
      dataIndex: 'custom_json_1',
      key: 'custom_json_1',
      render: (_: unknown, record: Contact) => (
        <JsonViewer
          json={record.custom_json_1}
          title={getCustomFieldLabel('custom_json_1', currentWorkspace)}
        />
      ),
      hidden: !visibleColumns.custom_json_1
    },
    {
      title: getCustomFieldLabel('custom_json_2', currentWorkspace),
      dataIndex: 'custom_json_2',
      key: 'custom_json_2',
      render: (_: unknown, record: Contact) => (
        <JsonViewer
          json={record.custom_json_2}
          title={getCustomFieldLabel('custom_json_2', currentWorkspace)}
        />
      ),
      hidden: !visibleColumns.custom_json_2
    },
    {
      title: getCustomFieldLabel('custom_json_3', currentWorkspace),
      dataIndex: 'custom_json_3',
      key: 'custom_json_3',
      render: (_: unknown, record: Contact) => (
        <JsonViewer
          json={record.custom_json_3}
          title={getCustomFieldLabel('custom_json_3', currentWorkspace)}
        />
      ),
      hidden: !visibleColumns.custom_json_3
    },
    {
      title: getCustomFieldLabel('custom_json_4', currentWorkspace),
      dataIndex: 'custom_json_4',
      key: 'custom_json_4',
      render: (_: unknown, record: Contact) => (
        <JsonViewer
          json={record.custom_json_4}
          title={getCustomFieldLabel('custom_json_4', currentWorkspace)}
        />
      ),
      hidden: !visibleColumns.custom_json_4
    },
    {
      title: getCustomFieldLabel('custom_json_5', currentWorkspace),
      dataIndex: 'custom_json_5',
      key: 'custom_json_5',
      render: (_: unknown, record: Contact) => (
        <JsonViewer
          json={record.custom_json_5}
          title={getCustomFieldLabel('custom_json_5', currentWorkspace)}
        />
      ),
      hidden: !visibleColumns.custom_json_5
    },
    {
      title: (
        <Space size="small">
          <Tooltip title={t`Refresh`}>
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
      align: 'right' as const,
      onHeaderCell: () => ({
        style: { backgroundColor: '#F9F9F9' }
      }),
      onCell: () => ({
        style: { backgroundColor: '#F9F9F9' }
      }),
      render: (_: unknown, record: Contact) => {
        const menuItems: MenuProps['items'] = [
          {
            key: 'edit',
            label: t`Edit`,
            disabled: !permissions?.contacts?.write,
            onClick: () => handleEditClick(record)
          },
          {
            key: 'delete',
            label: t`Delete`,
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
              segments={segmentsData?.segments || []}
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
      {/* Header with title and actions */}
      <div className="flex justify-between items-center mb-6">
        <div className="flex items-center gap-3">
          <div className="text-2xl font-medium">{t`Contacts`}</div>
          {totalContactsData?.total_contacts !== undefined && (
            <Tag bordered={false} color="blue">
              {numbro(totalContactsData.total_contacts).format({
                thousandSeparated: true,
                mantissa: 0
              })}
            </Tag>
          )}
        </div>
        <Space>
          <Dropdown
            menu={{
              items: [
                {
                  key: 'import',
                  label: t`Import`,
                  icon: <UploadOutlined />,
                  disabled: !permissions?.contacts?.write,
                  onClick: () => openImportDrawer(workspaceId, listsData?.lists || [], true)
                },
                {
                  key: 'export',
                  label: t`Export`,
                  icon: <DownloadOutlined />,
                  onClick: () => setExportModalVisible(true)
                }
              ]
            }}
          >
            <Button type="text">CSV</Button>
          </Dropdown>
          <Tooltip
            title={
              !permissions?.contacts?.write
                ? t`You don't have write permission for contacts`
                : undefined
            }
          >
            <span>
              <BulkUpdateDrawer
                workspaceId={workspaceId}
                lists={listsData?.lists || []}
                buttonProps={{
                  type: 'text',
                  children: t`Bulk Update`,
                  disabled: !permissions?.contacts?.write
                }}
              />
            </span>
          </Tooltip>
          <Tooltip
            title={
              !permissions?.contacts?.write
                ? t`You don't have write permission for contacts`
                : undefined
            }
          >
            <div>
              <ContactUpsertDrawer
                workspace={currentWorkspace}
                buttonProps={{
                  buttonContent: (
                    <>
                      <PlusOutlined /> {t`Add`}
                    </>
                  ),
                  disabled: !permissions?.contacts?.write
                }}
              />
            </div>
          </Tooltip>
        </Space>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-2 mb-4">
        <div className="text-sm font-medium">{t`Filters:`}</div>
        <Filter fields={filterFields} activeFilters={activeFilters} />
      </div>

      {/* Segments */}
      <SegmentsFilter
        workspaceId={workspaceId}
        segments={segmentsData?.segments || []}
        selectedSegmentIds={search.segments}
        totalContacts={totalContactsData?.total_contacts}
        onSegmentToggle={(segmentId: string) => {
          const currentSegments = search.segments || []
          const newSegments = currentSegments.includes(segmentId)
            ? currentSegments.filter((id) => id !== segmentId)
            : [...currentSegments, segmentId]

          navigate({
            to: workspaceContactsRoute.to,
            params: { workspaceId },
            search: {
              ...search,
              segments: newSegments.length > 0 ? newSegments : undefined
            }
          })
        }}
      />

      {/* Contacts Table */}
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
            ? t`No contacts found. Add some contacts to get started.`
            : t`Loading...`
        }}
        className="border border-gray-200 rounded-md"
      />

      {data?.next_cursor && (
        <div className="flex justify-center mt-4">
          <Button onClick={handleLoadMore} loading={isLoading || isFetching}>
            {t`Load More`}
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

      <ExportContactsModal
        visible={exportModalVisible}
        onCancel={() => setExportModalVisible(false)}
        workspaceId={workspaceId}
        workspace={currentWorkspace}
        filters={search}
        segmentsData={segmentsData?.segments || []}
        listsData={listsData?.lists || []}
      />
    </div>
  )
}
