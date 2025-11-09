import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Typography, Space, Button, Select, Input, Popover, Tooltip, Radio } from 'antd'
import { listMessages, MessageHistory } from '../../services/api/messages_history'
import { useAuth } from '../../contexts/AuthContext'
import React, { useState, useMemo, useEffect } from 'react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faCircleCheck,
  faCircleXmark,
  faEye,
  faHandPointer,
  faPaperPlane
} from '@fortawesome/free-regular-svg-icons'
import {
  faTriangleExclamation,
  faBan,
  faArrowRightFromBracket
} from '@fortawesome/free-solid-svg-icons'
import { MessageHistoryTable } from './MessageHistoryTable'
import { broadcastApi } from '../../services/api/broadcast'
import { listsApi } from '../../services/api/list'

const { Title, Text } = Typography

const STORAGE_KEY = 'message_columns_visibility'

const DEFAULT_VISIBLE_COLUMNS = {
  id: true,
  external_id: false,
  contact_email: true,
  template_id: true,
  broadcast_id: true,
  list_id: true,
  events: true,
  error: false,
  created_at: true
}

// Simple filter field type
interface FilterOption {
  key: string
  label: React.ReactNode
  options?: { value: string; label: string }[]
}

// Define status filter fields (first line)
const statusFilterOptions: FilterOption[] = [
  {
    key: 'is_sent',
    label: (
      <Tooltip title="Sent">
        <FontAwesomeIcon className="!mr-1 opacity-70 text-blue-500" icon={faPaperPlane} /> Sent
      </Tooltip>
    ),
    options: [
      { value: 'true', label: 'Yes' },
      { value: 'false', label: 'No' }
    ]
  },
  {
    key: 'is_delivered',
    label: (
      <Tooltip title="Delivered">
        <FontAwesomeIcon className="!mr-1 opacity-70 text-green-500" icon={faCircleCheck} />{' '}
        Delivered
      </Tooltip>
    ),
    options: [
      { value: 'true', label: 'Yes' },
      { value: 'false', label: 'No' }
    ]
  },
  {
    key: 'is_failed',
    label: (
      <Tooltip title="Failed">
        <FontAwesomeIcon className="!mr-1 opacity-70 text-red-500" icon={faCircleXmark} /> Failed
      </Tooltip>
    ),
    options: [
      { value: 'true', label: 'Yes' },
      { value: 'false', label: 'No' }
    ]
  },
  {
    key: 'is_opened',
    label: (
      <Tooltip title="Opened">
        <FontAwesomeIcon className="!mr-1 opacity-70 text-purple-500" icon={faEye} /> Opened
      </Tooltip>
    ),
    options: [
      { value: 'true', label: 'Yes' },
      { value: 'false', label: 'No' }
    ]
  },
  {
    key: 'is_clicked',
    label: (
      <Tooltip title="Clicked">
        <FontAwesomeIcon className="!mr-1 opacity-70 text-blue-500" icon={faHandPointer} /> Clicked
      </Tooltip>
    ),
    options: [
      { value: 'true', label: 'Yes' },
      { value: 'false', label: 'No' }
    ]
  },
  {
    key: 'is_bounced',
    label: (
      <Tooltip title="Bounced">
        <FontAwesomeIcon
          className="!mr-1 opacity-70 text-orange-500"
          icon={faTriangleExclamation}
        />{' '}
        Bounced
      </Tooltip>
    ),
    options: [
      { value: 'true', label: 'Yes' },
      { value: 'false', label: 'No' }
    ]
  },
  {
    key: 'is_complained',
    label: (
      <Tooltip title="Complained">
        <FontAwesomeIcon className="!mr-1 opacity-70 text-red-500" icon={faBan} /> Complained
      </Tooltip>
    ),
    options: [
      { value: 'true', label: 'Yes' },
      { value: 'false', label: 'No' }
    ]
  },
  {
    key: 'is_unsubscribed',
    label: (
      <Tooltip title="Unsubscribed">
        <FontAwesomeIcon className="!mr-1 opacity-70 text-red-500" icon={faArrowRightFromBracket} />{' '}
        Unsubscribed
      </Tooltip>
    ),
    options: [
      { value: 'true', label: 'Yes' },
      { value: 'false', label: 'No' }
    ]
  }
]

// Define other filter fields (second line)
const otherFilterOptions: FilterOption[] = [
  {
    key: 'channel',
    label: 'Channel',
    options: [
      { value: 'email', label: 'Email' },
      { value: 'sms', label: 'SMS' },
      { value: 'push', label: 'Push' }
    ]
  },
  { key: 'contact_email', label: 'Contact Email' },
  { key: 'id', label: 'Message ID' },
  { key: 'external_id', label: 'External ID' },
  { key: 'list_id', label: 'List ID' },
  { key: 'template_id', label: 'Template ID' },
  { key: 'broadcast_id', label: 'Broadcast ID' },
  {
    key: 'has_error',
    label: 'Has Error',
    options: [
      { value: 'true', label: 'With Errors' },
      { value: 'false', label: 'No Errors' }
    ]
  }
]

// Combined filter options for lookups
const filterOptions: FilterOption[] = [...statusFilterOptions, ...otherFilterOptions]

// Simple filter interface
interface Filter {
  field: string
  value: string
  label: string
}

interface MessageHistoryTabProps {
  workspaceId: string
}

export const MessageHistoryTab: React.FC<MessageHistoryTabProps> = ({ workspaceId }) => {
  const { workspaces } = useAuth()
  const [currentCursor, setCurrentCursor] = useState<string | undefined>(undefined)
  const [allMessages, setAllMessages] = useState<MessageHistory[]>([])
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const queryClient = useQueryClient()

  // State for filters
  const [activeFilters, setActiveFilters] = useState<Filter[]>([])
  const [openPopovers, setOpenPopovers] = useState<Record<string, boolean>>({})
  const [tempFilterValues, setTempFilterValues] = useState<Record<string, string>>({})

  // State for column visibility
  const [visibleColumns, setVisibleColumns] =
    useState<Record<string, boolean>>(DEFAULT_VISIBLE_COLUMNS)

  // Fetch broadcasts for the workspace
  const { data: broadcastsData } = useQuery({
    queryKey: ['broadcasts', workspaceId],
    queryFn: async () => {
      return broadcastApi.list({
        workspace_id: workspaceId,
        limit: 1000
      })
    },
    staleTime: 5 * 60 * 1000 // 5 minutes
  })

  // Fetch lists for the workspace
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: async () => {
      return listsApi.list({ workspace_id: workspaceId })
    },
    staleTime: 5 * 60 * 1000 // 5 minutes
  })

  // Create lookup maps
  const broadcastMap = useMemo(() => {
    if (!broadcastsData?.broadcasts) return new Map()
    return new Map(broadcastsData.broadcasts.map((b) => [b.id, b]))
  }, [broadcastsData])

  const listMap = useMemo(() => {
    if (!listsData?.lists) return new Map()
    return new Map(listsData.lists.map((l) => [l.id, l]))
  }, [listsData])

  // Create API filters from active filters
  const apiFilters = useMemo(() => {
    return activeFilters.reduce(
      (filters, filter) => {
        const { field, value } = filter

        // Special case for has_error which needs to be converted to boolean
        if (field === 'has_error') {
          filters[field] = value === 'true'
        } else if (field === 'is_sent') {
          filters[field] = value === 'true'
        } else if (field === 'is_delivered') {
          filters[field] = value === 'true'
        } else if (field === 'is_failed') {
          filters[field] = value === 'true'
        } else if (field === 'is_opened') {
          filters[field] = value === 'true'
        } else if (field === 'is_clicked') {
          filters[field] = value === 'true'
        } else if (field === 'is_bounced') {
          filters[field] = value === 'true'
        } else if (field === 'is_complained') {
          filters[field] = value === 'true'
        } else if (field === 'is_unsubscribed') {
          filters[field] = value === 'true'
        } else {
          filters[field] = value
        }

        return filters
      },
      {} as Record<string, any>
    )
  }, [activeFilters])

  // Find the current workspace from the workspaces array
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)

  // Load saved column visibility from localStorage on mount
  useEffect(() => {
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

  // Handle column visibility change
  const handleColumnVisibilityChange = (key: string, visible: boolean) => {
    setVisibleColumns((prev) => {
      const newState = { ...prev, [key]: visible }
      // Save to localStorage
      localStorage.setItem(STORAGE_KEY, JSON.stringify(newState))
      return newState
    })
  }

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
          label: '' // Convert ReactNode to string
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

  // Fetch message history
  const {
    data: messagesData,
    isLoading,
    error
  } = useQuery({
    queryKey: ['messages-history', workspaceId, apiFilters, currentCursor],
    queryFn: async () => {
      return listMessages(workspaceId, {
        ...apiFilters,
        limit: 20,
        cursor: currentCursor
      })
    },
    staleTime: 5000,
    refetchOnWindowFocus: false
  })

  // Reset the cursor and accumulated messages when filters change
  React.useEffect(() => {
    setAllMessages([])
    setCurrentCursor(undefined)
    queryClient.resetQueries({ queryKey: ['messages-history', workspaceId] })
  }, [apiFilters, workspaceId, queryClient])

  // Update allMessages when data changes
  React.useEffect(() => {
    // If data is still loading or not available, don't update
    if (isLoading || !messagesData) return

    if (messagesData.messages) {
      if (!currentCursor) {
        // Initial load or filter change - replace all messages
        setAllMessages(messagesData.messages)
      } else if (messagesData.messages.length > 0) {
        // If we have a cursor and new messages, append them
        setAllMessages((prev) => [...prev, ...messagesData.messages])
      }
    }

    // Reset loading more flag
    setIsLoadingMore(false)
  }, [messagesData, currentCursor, isLoading])

  // Load more messages
  const handleLoadMore = () => {
    if (messagesData?.next_cursor) {
      setIsLoadingMore(true)
      setCurrentCursor(messagesData.next_cursor)
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
          label: ''
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

  // Render filter buttons for a specific filter group
  const renderFilterGroup = (options: FilterOption[]) => {
    return options.map((option) => {
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
                // Check if this is a boolean field (has only Yes/No options)
                option.options.length === 2 &&
                option.options.every((opt) => opt.value === 'true' || opt.value === 'false') ? (
                  <Radio.Group
                    style={{ width: '100%', marginBottom: 8 }}
                    value={tempFilterValues[option.key] || undefined}
                    onChange={(e) =>
                      setTempFilterValues({
                        ...tempFilterValues,
                        [option.key]: e.target.value
                      })
                    }
                  >
                    <div className="flex flex-col gap-1">
                      {option.options.map((opt) => (
                        <Radio key={opt.value} value={opt.value}>
                          {opt.label}
                        </Radio>
                      ))}
                    </div>
                  </Radio.Group>
                ) : (
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
                )
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
            {isActive ? (
              <span>
                {option.label}: {activeFilter!.value}
              </span>
            ) : (
              option.label
            )}
          </Button>
        </Popover>
      )
    })
  }

  // Render filter buttons
  const renderFilterButtons = () => {
    return (
      <div className="flex flex-col gap-2">
        {/* First line: Status filters */}
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-gray-600">Status:</span>
          <Space wrap>{renderFilterGroup(statusFilterOptions)}</Space>
        </div>

        {/* Second line: Other filters */}
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-gray-600">Filters:</span>
          <Space wrap>
            {renderFilterGroup(otherFilterOptions)}
            {activeFilters.length > 0 && (
              <Button size="small" onClick={clearAllFilters}>
                Clear All
              </Button>
            )}
          </Space>
        </div>
      </div>
    )
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

  return (
    <div>
      <div className="my-6">{renderFilterButtons()}</div>

      <MessageHistoryTable
        messages={allMessages}
        loading={isLoading}
        isLoadingMore={isLoadingMore}
        workspace={currentWorkspace}
        nextCursor={messagesData?.next_cursor}
        onLoadMore={handleLoadMore}
        show_email={true}
        onRefresh={() => {
          queryClient.resetQueries({ queryKey: ['messages-history', workspaceId, apiFilters] })
        }}
        bordered={true}
        size="middle"
        broadcastMap={broadcastMap}
        listMap={listMap}
        visibleColumns={visibleColumns}
        onColumnVisibilityChange={handleColumnVisibilityChange}
      />
    </div>
  )
}
