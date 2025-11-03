import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Typography,
  Space,
  Button,
  Select,
  Input,
  Popover,
  Table,
  Tag,
  Tooltip,
  Spin,
  Empty
} from 'antd'
import { listWebhookEvents, WebhookEvent, EmailEventType } from '../../services/api/webhook_event'
import { useAuth } from '../../contexts/AuthContext'
import React, { useState, useMemo, useEffect } from 'react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCircleCheck, faCircleXmark } from '@fortawesome/free-regular-svg-icons'
import { faTriangleExclamation, faRefresh } from '@fortawesome/free-solid-svg-icons'
import dayjs from '../../lib/dayjs'
import { getProviderIcon, getProviderName } from '../integrations/EmailProviders'

const { Title, Text } = Typography

// Define event type icon and color mappings
const eventTypeConfig: Record<
  EmailEventType,
  { icon: React.ReactNode; color: string; label: string }
> = {
  delivered: {
    icon: <FontAwesomeIcon className="!mr-1 opacity-70" icon={faCircleCheck} />,
    color: 'green',
    label: 'Delivered'
  },
  bounce: {
    icon: <FontAwesomeIcon className="!mr-1 opacity-70" icon={faTriangleExclamation} />,
    color: 'orange',
    label: 'Bounce'
  },
  complaint: {
    icon: <FontAwesomeIcon className="!mr-1 opacity-70" icon={faCircleXmark} />,
    color: 'red',
    label: 'Complaint'
  },
  auth_email: {
    icon: null,
    color: 'blue',
    label: 'Auth Email'
  },
  before_user_created: {
    icon: null,
    color: 'cyan',
    label: 'User Created'
  }
}

// Simple filter field type
interface FilterOption {
  key: string
  label: string
  options?: { value: string; label: string }[]
}

// Define filter fields for webhook events
const filterOptions: FilterOption[] = [
  {
    key: 'event_type',
    label: 'Event Type',
    options: Object.entries(eventTypeConfig).map(([value, { label }]) => ({
      value,
      label
    }))
  },
  { key: 'recipient_email', label: 'Recipient Email' },
  { key: 'message_id', label: 'Message ID' },
  { key: 'transactional_id', label: 'Transactional ID' },
  { key: 'broadcast_id', label: 'Broadcast ID' }
]

// Simple filter interface
interface Filter {
  field: string
  value: string
  label: string
}

interface WebhookEventsTabProps {
  workspaceId: string
  onRefresh?: () => void
}

export const WebhookEventsTab: React.FC<WebhookEventsTabProps> = ({ workspaceId, onRefresh }) => {
  const { workspaces } = useAuth()
  const [currentCursor, setCurrentCursor] = useState<string | undefined>(undefined)
  const [allEvents, setAllEvents] = useState<WebhookEvent[]>([])
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const queryClient = useQueryClient()

  // State for filters
  const [activeFilters, setActiveFilters] = useState<Filter[]>([])
  const [openPopovers, setOpenPopovers] = useState<Record<string, boolean>>({})
  const [tempFilterValues, setTempFilterValues] = useState<Record<string, string>>({})

  // Create API filters from active filters
  const apiFilters = useMemo(() => {
    return activeFilters.reduce(
      (filters, filter) => {
        const { field, value } = filter
        filters[field] = value
        return filters
      },
      {} as Record<string, string>
    )
  }, [activeFilters])

  // Find the current workspace from the workspaces array
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)

  // Load initial filters from URL on mount
  useEffect(() => {
    const searchParams = new URLSearchParams(window.location.search)
    const initialFilters: Filter[] = []

    filterOptions.forEach((option) => {
      const value = searchParams.get(option.key)
      if (value) {
        initialFilters.push({
          field: option.key,
          value,
          label: option.label
        })
      }
    })

    if (initialFilters.length > 0) {
      setActiveFilters(initialFilters)
    }
  }, [])

  // Update URL when filters change
  React.useEffect(() => {
    const searchParams = new URLSearchParams()

    activeFilters.forEach((filter) => {
      searchParams.set(filter.field, filter.value)
    })

    const newUrl =
      window.location.pathname + (searchParams.toString() ? `?${searchParams.toString()}` : '')

    window.history.pushState({ path: newUrl }, '', newUrl)
  }, [activeFilters])

  // Fetch webhook events
  const {
    data: eventsData,
    isLoading,
    error
  } = useQuery({
    queryKey: ['webhook-events', workspaceId, apiFilters, currentCursor],
    queryFn: async () => {
      return listWebhookEvents(workspaceId, {
        ...apiFilters,
        limit: 20,
        cursor: currentCursor
      })
    },
    staleTime: 5000,
    refetchOnWindowFocus: false
  })

  // Reset the cursor and accumulated events when filters change
  React.useEffect(() => {
    setAllEvents([])
    setCurrentCursor(undefined)
    queryClient.resetQueries({ queryKey: ['webhook-events', workspaceId] })
  }, [apiFilters, workspaceId, queryClient])

  // Update allEvents when data changes
  React.useEffect(() => {
    // If data is still loading or not available, don't update
    if (isLoading || !eventsData) return

    if (eventsData.events) {
      if (!currentCursor) {
        // Initial load or filter change - replace all events
        setAllEvents(eventsData.events)
      } else if (eventsData.events.length > 0) {
        // If we have a cursor and new events, append them
        setAllEvents((prev) => [...prev, ...eventsData.events])
      }
    }

    // Reset loading more flag
    setIsLoadingMore(false)
  }, [eventsData, currentCursor, isLoading])

  // Load more events
  const handleLoadMore = () => {
    if (eventsData?.next_cursor) {
      setIsLoadingMore(true)
      setCurrentCursor(eventsData.next_cursor)
    }
  }

  // Handle applying a filter
  const applyFilter = (field: string, value: string) => {
    // Remove any existing filter for this field
    const updatedFilters = activeFilters.filter((f) => f.field !== field)

    // Add the new filter if it has a value
    if (value) {
      const filterOption = filterOptions.find((option) => option.key === field)
      if (filterOption) {
        updatedFilters.push({
          field,
          value,
          label: filterOption.label
        })
      }
    }

    setActiveFilters(updatedFilters)
    setOpenPopovers({ ...openPopovers, [field]: false })
  }

  // Handle clearing a filter
  const clearFilter = (field: string) => {
    setActiveFilters(activeFilters.filter((f) => f.field !== field))
    setTempFilterValues({ ...tempFilterValues, [field]: '' })
    setOpenPopovers({ ...openPopovers, [field]: false })
  }

  // Clear all filters
  const clearAllFilters = () => {
    setActiveFilters([])
    setTempFilterValues({})
    // Clear URL params
    window.history.pushState({ path: window.location.pathname }, '', window.location.pathname)
  }

  // Render filter buttons
  const renderFilterButtons = () => {
    return (
      <Space wrap>
        {filterOptions.map((option) => {
          const isActive = activeFilters.some((f) => f.field === option.key)
          const activeFilter = activeFilters.find((f) => f.field === option.key)

          return (
            <Popover
              key={option.key}
              trigger="click"
              open={openPopovers[option.key]}
              onOpenChange={(visible) => {
                // Initialize temp value when opening
                if (visible && activeFilter) {
                  setTempFilterValues({
                    ...tempFilterValues,
                    [option.key]: activeFilter.value
                  })
                }
                setOpenPopovers({ ...openPopovers, [option.key]: visible })
              }}
              content={
                <div style={{ width: 200 }}>
                  {option.options ? (
                    <Select
                      style={{ width: '100%', marginBottom: 8 }}
                      placeholder={`Select ${option.label}`}
                      value={tempFilterValues[option.key] || undefined}
                      onChange={(value) =>
                        setTempFilterValues({
                          ...tempFilterValues,
                          [option.key]: value
                        })
                      }
                      options={option.options}
                      allowClear
                    />
                  ) : (
                    <Input
                      placeholder={`Enter ${option.label}`}
                      value={tempFilterValues[option.key] || ''}
                      onChange={(e) =>
                        setTempFilterValues({
                          ...tempFilterValues,
                          [option.key]: e.target.value
                        })
                      }
                      style={{ marginBottom: 8 }}
                    />
                  )}

                  <div className="flex gap-2">
                    <Button
                      type="primary"
                      size="small"
                      style={{ flex: 1 }}
                      onClick={() => applyFilter(option.key, tempFilterValues[option.key] || '')}
                    >
                      Apply
                    </Button>

                    {isActive && (
                      <Button danger size="small" onClick={() => clearFilter(option.key)}>
                        Clear
                      </Button>
                    )}
                  </div>
                </div>
              }
            >
              <Button type={isActive ? 'primary' : 'default'} size="small">
                {isActive ? `${option.label}: ${activeFilter!.value}` : option.label}
              </Button>
            </Popover>
          )
        })}

        {activeFilters.length > 0 && (
          <Button size="small" onClick={clearAllFilters}>
            Clear All
          </Button>
        )}
      </Space>
    )
  }

  // Format date using dayjs
  const formatDate = (dateString: string | undefined): string => {
    if (!dateString) return '-'
    return `${dayjs(dateString).format('lll')} in ${currentWorkspace?.settings.timezone || 'UTC'}`
  }

  // Define table columns
  const columns = [
    {
      title: 'Provider',
      dataIndex: 'source',
      key: 'source',
      width: 80,
      render: (source: string) => (
        <Tooltip title={getProviderName(source)}>
          {getProviderIcon(source, 'small')}
        </Tooltip>
      )
    },
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      render: (id: string) => (
        <Tooltip title={id}>
          <span className="text-xs text-gray-500">{id.substring(0, 8) + '...'}</span>
        </Tooltip>
      )
    },
    {
      title: 'Type',
      dataIndex: 'type',
      key: 'type',
      render: (type: EmailEventType) => {
        const config = eventTypeConfig[type]
        return (
          <Tag bordered={false} color={config?.color || 'default'}>
            {config?.icon} {config?.label || type}
          </Tag>
        )
      }
    },
    {
      title: 'Recipient',
      dataIndex: 'recipient_email',
      key: 'recipient_email',
      render: (email: string) => <span className="text-xs">{email}</span>
    },
    {
      title: 'Message ID',
      dataIndex: 'message_id',
      key: 'message_id',
      render: (id: string | undefined) => 
        id ? (
          <Tooltip title={id}>
            <span className="text-xs text-gray-500">{id.substring(0, 8) + '...'}</span>
          </Tooltip>
        ) : (
          <span className="text-xs text-gray-400">-</span>
        )
    },
    {
      title: 'Broadcast',
      dataIndex: 'broadcast_id',
      key: 'broadcast_id',
      render: (id: string) =>
        id && (
          <Tooltip title={id}>
            <span className="text-xs text-gray-500">{id.substring(0, 8) + '...'}</span>
          </Tooltip>
        )
    },
    {
      title: 'Transactional',
      dataIndex: 'transactional_id',
      key: 'transactional_id',
      render: (id: string) =>
        id && (
          <Tooltip title={id}>
            <span className="text-xs text-gray-500">{id.substring(0, 8) + '...'}</span>
          </Tooltip>
        )
    },
    {
      title: 'Timestamp',
      dataIndex: 'timestamp',
      key: 'timestamp',
      render: (date: string) => <Tooltip title={formatDate(date)}>{dayjs(date).fromNow()}</Tooltip>
    }
  ]

  const actionColumn = {
    title: (
      <>
        <Tooltip title="Refresh">
          <Button
            type="text"
            size="small"
            icon={<FontAwesomeIcon icon={faRefresh} />}
            onClick={onRefresh}
            className="opacity-70 hover:opacity-100"
          />
        </Tooltip>
      </>
    ),
    key: 'actions',
    width: 100,
    render: null
  }

  // Additional bounce-specific columns
  const bounceColumns = [
    {
      title: 'Bounce Type',
      dataIndex: 'bounce_type',
      key: 'bounce_type',
      render: (type: string) => type && <span className="text-xs">{type}</span>
    },
    {
      title: 'Bounce Category',
      dataIndex: 'bounce_category',
      key: 'bounce_category',
      render: (category: string) => category && <span className="text-xs">{category}</span>
    }
  ]

  // Additional complaint-specific columns
  const complaintColumns = [
    {
      title: 'Feedback Type',
      dataIndex: 'complaint_feedback_type',
      key: 'complaint_feedback_type',
      render: (type: string) => type && <span className="text-xs">{type}</span>
    }
  ]

  // Function to get additional columns based on event type
  const getAdditionalColumns = (events: WebhookEvent[]) => {
    const hasBouncedEvents = events.some((event) => event.type === 'bounce')
    const hasComplaintEvents = events.some((event) => event.type === 'complaint')

    let additionalColumns: any[] = []

    if (hasBouncedEvents) {
      additionalColumns = [...additionalColumns, ...bounceColumns]
    }

    if (hasComplaintEvents) {
      additionalColumns = [...additionalColumns, ...complaintColumns]
    }

    return additionalColumns
  }

  if (error) {
    return (
      <div>
        <Title level={4}>Error loading data</Title>
        <Text type="danger">{(error as Error)?.message}</Text>
      </div>
    )
  }

  if (!currentWorkspace) {
    return <div>Loading...</div>
  }

  // Determine if we should show additional columns
  const allColumns = [...columns, ...getAdditionalColumns(allEvents), actionColumn]

  return (
    <div>
      <div className="flex justify-between items-center my-6">{renderFilterButtons()}</div>

      {isLoading && !isLoadingMore ? (
        <div className="loading-container" style={{ padding: '40px 0', textAlign: 'center' }}>
          <Spin size="large" />
          <div style={{ marginTop: 16 }}>Loading webhook events...</div>
        </div>
      ) : allEvents.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description="No webhook events found"
          style={{ margin: '40px 0' }}
        />
      ) : (
        <>
          <Table
            dataSource={allEvents}
            columns={allColumns}
            rowKey="id"
            pagination={false}
            size="middle"
            className="border border-gray-300 rounded"
            expandable={{
              expandedRowRender: (record) => (
                <div className="px-4 py-2">
                  <div className="text-xs mb-2">
                    <strong>Integration ID:</strong> {record.integration_id}
                  </div>
                  {record.bounce_diagnostic && (
                    <div className="text-xs mb-2">
                      <strong>Bounce Diagnostic:</strong> {record.bounce_diagnostic}
                    </div>
                  )}
                  <div className="text-xs mb-2">
                    <strong>Raw Payload:</strong>
                    <pre className="mt-1 p-2 bg-gray-100 rounded text-xs overflow-auto">
                      {JSON.stringify(JSON.parse(record.raw_payload), null, 2)}
                    </pre>
                  </div>
                </div>
              )
            }}
          />

          {eventsData?.next_cursor && (
            <div className="flex justify-center mt-4 mb-8">
              <Button size="small" onClick={handleLoadMore} loading={isLoadingMore}>
                Load More
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
