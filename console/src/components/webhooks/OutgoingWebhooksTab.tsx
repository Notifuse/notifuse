import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, useMemo, useEffect } from 'react'
import { Table, Tag, Space, Button, Tooltip, Empty, Spin, Select, Popover } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCheck, faTimes, faRefresh, faClock } from '@fortawesome/free-solid-svg-icons'
import dayjs from '../../lib/dayjs'
import { useAuth } from '../../contexts/AuthContext'
import {
  webhookSubscriptionApi,
  WebhookSubscription,
  WebhookDelivery
} from '../../services/api/webhook_subscription'
import { useLingui } from '@lingui/react/macro'

interface FilterOption {
  key: string
  label: string
  options?: { value: string; label: string }[]
}

interface Filter {
  field: string
  value: string
  label: string
}

interface OutgoingWebhooksTabProps {
  workspaceId: string
}

export function OutgoingWebhooksTab({ workspaceId }: OutgoingWebhooksTabProps) {
  const { t } = useLingui()
  const { workspaces } = useAuth()
  const queryClient = useQueryClient()
  const [currentPage, setCurrentPage] = useState(1)
  const [allDeliveries, setAllDeliveries] = useState<WebhookDelivery[]>([])
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const pageSize = 20

  // State for filters
  const [activeFilters, setActiveFilters] = useState<Filter[]>([])
  const [openPopovers, setOpenPopovers] = useState<Record<string, boolean>>({})
  const [tempFilterValues, setTempFilterValues] = useState<Record<string, string>>({})

  // Find the current workspace
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)

  // Fetch subscriptions for filter dropdown
  const { data: subscriptionsData } = useQuery({
    queryKey: ['webhook-subscriptions', workspaceId],
    queryFn: async () => {
      return webhookSubscriptionApi.list(workspaceId)
    },
    staleTime: 5 * 60 * 1000
  })

  // Create subscription map for display
  const subscriptionMap = useMemo(() => {
    if (!subscriptionsData?.subscriptions) return new Map<string, WebhookSubscription>()
    return new Map(subscriptionsData.subscriptions.map((s) => [s.id, s]))
  }, [subscriptionsData])

  // Build filter options dynamically
  const filterOptions: FilterOption[] = useMemo(() => {
    const options: FilterOption[] = [
      {
        key: 'subscription_id',
        label: t`Subscription`,
        options: subscriptionsData?.subscriptions?.map((s) => ({
          value: s.id,
          label: s.name
        })) || []
      },
      {
        key: 'status',
        label: t`Status`,
        options: [
          { value: 'delivered', label: t`Delivered` },
          { value: 'pending', label: t`Pending` },
          { value: 'failed', label: t`Failed` }
        ]
      }
    ]
    return options
  }, [subscriptionsData, t])

  // Create API filters from active filters
  const apiFilters = useMemo(() => {
    return activeFilters.reduce(
      (filters, filter) => {
        filters[filter.field] = filter.value
        return filters
      },
      {} as Record<string, string>
    )
  }, [activeFilters])

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
      // eslint-disable-next-line react-hooks/set-state-in-effect -- Syncing URL params to state on mount
      setActiveFilters(initialFilters)
    }
  }, [filterOptions])

  // Update URL when filters change
  useEffect(() => {
    const searchParams = new URLSearchParams()

    activeFilters.forEach((filter) => {
      searchParams.set(filter.field, filter.value)
    })

    const newUrl =
      window.location.pathname + (searchParams.toString() ? `?${searchParams.toString()}` : '')

    window.history.pushState({ path: newUrl }, '', newUrl)
  }, [activeFilters])

  // Fetch deliveries
  const {
    data: deliveriesData,
    isLoading,
    isFetching,
    error
  } = useQuery({
    queryKey: ['webhook-deliveries', workspaceId, apiFilters, currentPage],
    queryFn: async () => {
      const offset = (currentPage - 1) * pageSize
      return webhookSubscriptionApi.getDeliveries(
        workspaceId,
        apiFilters.subscription_id,
        pageSize,
        offset
      )
    },
    staleTime: 5000,
    refetchOnWindowFocus: false
  })

  // Reset page and accumulated deliveries when filters change
  useEffect(() => {
    setAllDeliveries([]) // eslint-disable-line react-hooks/set-state-in-effect -- Required to reset pagination
    setCurrentPage(1)
    queryClient.resetQueries({ queryKey: ['webhook-deliveries', workspaceId] })
  }, [apiFilters, workspaceId, queryClient])

  // Update allDeliveries when data changes
  useEffect(() => {
    if (isLoading || isFetching || !deliveriesData) return

    if (deliveriesData.deliveries) {
      if (currentPage === 1) {
        setAllDeliveries(deliveriesData.deliveries) // eslint-disable-line react-hooks/set-state-in-effect -- Syncing query data
      } else if (deliveriesData.deliveries.length > 0) {
        setAllDeliveries((prev) => [...prev, ...deliveriesData.deliveries])
      }
    }

    setIsLoadingMore(false)
  }, [deliveriesData, currentPage, isLoading, isFetching])

  // Load more deliveries
  const handleLoadMore = () => {
    if (deliveriesData && allDeliveries.length < deliveriesData.total) {
      setIsLoadingMore(true)
      setCurrentPage((prev) => prev + 1)
    }
  }

  // Handle refresh
  const handleRefresh = () => {
    setAllDeliveries([])
    setCurrentPage(1)
    queryClient.invalidateQueries({ queryKey: ['webhook-deliveries', workspaceId] })
  }

  // Handle applying a filter
  const applyFilter = (field: string, value: string) => {
    const updatedFilters = activeFilters.filter((f) => f.field !== field)

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
    window.history.pushState({ path: window.location.pathname }, '', window.location.pathname)
  }

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'delivered':
        return (
          <Tag color="green" bordered={false}>
            <FontAwesomeIcon icon={faCheck} className="mr-1 opacity-70" /> {t`Delivered`}
          </Tag>
        )
      case 'pending':
        return (
          <Tag color="blue" bordered={false}>
            <FontAwesomeIcon icon={faClock} className="mr-1 opacity-70" /> {t`Pending`}
          </Tag>
        )
      case 'failed':
        return (
          <Tag color="red" bordered={false}>
            <FontAwesomeIcon icon={faTimes} className="mr-1 opacity-70" /> {t`Failed`}
          </Tag>
        )
      default:
        return <Tag bordered={false}>{status}</Tag>
    }
  }

  // Get subscription name by ID
  const getSubscriptionName = (subscriptionId: string) => {
    const subscription = subscriptionMap.get(subscriptionId)
    return subscription?.name || subscriptionId.substring(0, 8) + '...'
  }

  // Format date
  const formatDate = (dateString: string | undefined): string => {
    if (!dateString) return '-'
    return t`${dayjs(dateString).format('lll')} in ${currentWorkspace?.settings.timezone || 'UTC'}`
  }

  // Render filter buttons
  const renderFilterButtons = () => {
    return (
      <Space wrap>
        {filterOptions.map((option) => {
          const isActive = activeFilters.some((f) => f.field === option.key)
          const activeFilter = activeFilters.find((f) => f.field === option.key)

          // Get display value for active filter
          const getDisplayValue = () => {
            if (!activeFilter) return ''
            if (option.key === 'subscription_id') {
              return getSubscriptionName(activeFilter.value)
            }
            return activeFilter.value
          }

          return (
            <Popover
              key={option.key}
              trigger="click"
              placement="bottom"
              open={openPopovers[option.key]}
              onOpenChange={(visible) => {
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
                  <Select
                    style={{ width: '100%', marginBottom: 8 }}
                    placeholder={t`Select ${option.label}`}
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

                  <div className="flex gap-2">
                    <Button
                      type="primary"
                      size="small"
                      style={{ flex: 1 }}
                      onClick={() => applyFilter(option.key, tempFilterValues[option.key] || '')}
                    >
                      {t`Apply`}
                    </Button>

                    {isActive && (
                      <Button danger size="small" onClick={() => clearFilter(option.key)}>
                        {t`Clear`}
                      </Button>
                    )}
                  </div>
                </div>
              }
            >
              <Button type={isActive ? 'primary' : 'default'} size="small">
                {isActive ? `${option.label}: ${getDisplayValue()}` : option.label}
              </Button>
            </Popover>
          )
        })}

        {activeFilters.length > 0 && (
          <Button size="small" onClick={clearAllFilters}>
            {t`Clear All`}
          </Button>
        )}
      </Space>
    )
  }

  const columns = [
    {
      title: t`Event`,
      dataIndex: 'event_type',
      key: 'event_type',
      render: (type: string) => <Tag color="green" bordered={false}>{type}</Tag>
    },
    {
      title: t`Subscription`,
      dataIndex: 'subscription_id',
      key: 'subscription_id',
      render: (subscriptionId: string) => (
        <Tooltip title={subscriptionId}>
          <span className="text-sm">{getSubscriptionName(subscriptionId)}</span>
        </Tooltip>
      )
    },
    {
      title: t`Status`,
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status)
    },
    {
      title: t`Attempts`,
      key: 'attempts',
      render: (_: unknown, record: WebhookDelivery) => (
        <span>
          {record.attempts}/{record.max_attempts}
        </span>
      )
    },
    {
      title: t`Response`,
      key: 'response',
      render: (_: unknown, record: WebhookDelivery) => (
        <div className="text-xs">
          {record.last_response_status && (
            <Tag
              color={
                record.last_response_status >= 200 && record.last_response_status < 300
                  ? 'green'
                  : 'red'
              }
              bordered={false}
            >
              HTTP {record.last_response_status}
            </Tag>
          )}
          {record.last_error && (
            <Tooltip title={record.last_error}>
              <span className="text-red-500 truncate block max-w-[150px]">{record.last_error}</span>
            </Tooltip>
          )}
        </div>
      )
    },
    {
      title: t`Created`,
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => (
        <Tooltip title={formatDate(date)}>
          <span className="text-xs">{dayjs(date).fromNow()}</span>
        </Tooltip>
      )
    },
    {
      title: (
        <Tooltip title={t`Refresh`}>
          <Button
            type="text"
            size="small"
            icon={<FontAwesomeIcon icon={faRefresh} />}
            onClick={handleRefresh}
            className="opacity-70 hover:opacity-100"
          />
        </Tooltip>
      ),
      key: 'actions',
      width: 60,
      render: () => null
    }
  ]

  if (error) {
    return (
      <div>
        <div className="text-lg font-medium">{t`Error loading data`}</div>
        <div className="text-red-500">{(error as Error)?.message}</div>
      </div>
    )
  }

  if (!currentWorkspace) {
    return <div>{t`Loading...`}</div>
  }

  const hasMore = deliveriesData ? allDeliveries.length < deliveriesData.total : false

  return (
    <div>
      <div className="flex justify-between items-center my-6">{renderFilterButtons()}</div>

      {(isLoading || (isFetching && allDeliveries.length === 0)) && !isLoadingMore ? (
        <div className="loading-container" style={{ padding: '40px 0', textAlign: 'center' }}>
          <Spin size="large" />
          <div style={{ marginTop: 16 }}>{t`Loading webhook deliveries...`}</div>
        </div>
      ) : allDeliveries.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t`No webhook deliveries found`}
          style={{ margin: '40px 0' }}
        />
      ) : (
        <>
          <Table
            dataSource={allDeliveries}
            columns={columns}
            rowKey="id"
            pagination={false}
            size="middle"
            className="border border-gray-300 rounded"
            expandable={{
              expandedRowRender: (record) => (
                <div className="px-4 py-2">
                  <div className="text-xs mb-2">
                    <strong>{t`Delivery ID:`}</strong> {record.id}
                  </div>
                  <div className="text-xs mb-2">
                    <strong>{t`Subscription ID:`}</strong> {record.subscription_id}
                  </div>
                  {record.last_error && (
                    <div className="text-xs mb-2">
                      <strong>{t`Error:`}</strong>{' '}
                      <span className="text-red-500">{record.last_error}</span>
                    </div>
                  )}
                  {record.last_response_body && (
                    <div className="text-xs mb-2">
                      <strong>{t`Response Body:`}</strong>
                      <pre className="mt-1 p-2 bg-gray-100 rounded text-xs overflow-auto max-h-40">
                        {record.last_response_body}
                      </pre>
                    </div>
                  )}
                  <div className="text-xs mb-2">
                    <strong>{t`Payload:`}</strong>
                    <pre className="mt-1 p-2 bg-gray-100 rounded text-xs overflow-auto max-h-40">
                      {JSON.stringify(record.payload, null, 2)}
                    </pre>
                  </div>
                </div>
              )
            }}
          />

          {hasMore && (
            <div className="flex justify-center mt-4 mb-8">
              <Button size="small" onClick={handleLoadMore} loading={isLoadingMore}>
                {t`Load More`}
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
