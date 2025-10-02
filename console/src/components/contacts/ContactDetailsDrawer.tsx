import React from 'react'
import {
  Drawer,
  Space,
  Tag,
  Typography,
  Table,
  Spin,
  Empty,
  Tooltip,
  Button,
  Modal,
  Select,
  Form,
  Popover,
  App,
  Statistic,
  Avatar,
  Tabs
} from 'antd'
import { Contact } from '../../services/api/contacts'
import { List, Workspace } from '../../services/api/types'
import dayjs from '../../lib/dayjs'
import numbro from 'numbro'
import { ContactUpsertDrawer } from './ContactUpsertDrawer'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus, faEllipsis, faRotate } from '@fortawesome/free-solid-svg-icons'
import { faPenToSquare } from '@fortawesome/free-regular-svg-icons'
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import { listMessages, MessageHistory } from '../../services/api/messages_history'
import { contactsApi } from '../../services/api/contacts'
import { contactListApi, UpdateContactListStatusRequest } from '../../services/api/contact_list'
import { listsApi } from '../../services/api/list'
import { SubscribeToListsRequest } from '../../services/api/types'
import { MessageHistoryTable } from '../messages/MessageHistoryTable'
import { ContactTimeline } from '../timeline'
import { contactTimelineApi, ContactTimelineEntry } from '../../services/api/contact_timeline'

const { Title, Text } = Typography

interface ContactDetailsDrawerProps {
  workspace: Workspace
  contactEmail: string
  visible?: boolean
  onClose?: () => void
  lists?: List[]
  onContactUpdate?: (contact: Contact) => void
  buttonProps: {
    type?: 'primary' | 'default' | 'dashed' | 'link' | 'text'
    icon?: React.ReactNode
    buttonContent?: React.ReactNode
    className?: string
    style?: React.CSSProperties
    size?: 'large' | 'middle' | 'small'
    disabled?: boolean
    loading?: boolean
    danger?: boolean
    ghost?: boolean
    block?: boolean
  }
}

// Add this type definition for the lists with name
interface ContactListWithName {
  list_id: string
  status: string
  name: string
  created_at?: string
}

// Helper function to generate Gravatar URL using SHA-256
const getGravatarUrl = async (email: string, size: number = 80) => {
  const trimmedEmail = email.toLowerCase().trim()
  const encoder = new TextEncoder()
  const data = encoder.encode(trimmedEmail)
  const hashBuffer = await crypto.subtle.digest('SHA-256', data)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  const hashHex = hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
  return `https://www.gravatar.com/avatar/${hashHex}?s=${size}&d=mp`
}

export function ContactDetailsDrawer({
  workspace,
  contactEmail,
  visible: externalVisible,
  onClose: externalOnClose,
  lists = [],
  onContactUpdate,
  buttonProps
}: ContactDetailsDrawerProps) {
  if (!contactEmail) return null

  // Internal drawer visibility state
  const [internalVisible, setInternalVisible] = React.useState(false)
  const [gravatarUrl, setGravatarUrl] = React.useState<string>('')
  const { message: messageApi } = App.useApp()

  // Determine if drawer is visible (either controlled externally or internally)
  const isVisible = externalVisible !== undefined ? externalVisible : internalVisible

  // Generate Gravatar URL
  React.useEffect(() => {
    if (contactEmail) {
      getGravatarUrl(contactEmail, 80).then(setGravatarUrl)
    }
  }, [contactEmail])

  // Handle drawer close
  const handleClose = () => {
    if (externalOnClose) {
      externalOnClose()
    } else {
      setInternalVisible(false)
    }
  }

  // Handle drawer open
  const handleOpen = () => {
    setInternalVisible(true)
  }

  const queryClient = useQueryClient()
  const [statusModalVisible, setStatusModalVisible] = React.useState(false)
  const [subscribeModalVisible, setSubscribeModalVisible] = React.useState(false)
  const [selectedList, setSelectedList] = React.useState<ContactListWithName | null>(null)
  const [statusForm] = Form.useForm()
  const [subscribeForm] = Form.useForm()

  // State for message history pagination
  const [currentCursor, setCurrentCursor] = React.useState<string | undefined>(undefined)
  const [allMessages, setAllMessages] = React.useState<MessageHistory[]>([])
  const [isLoadingMore, setIsLoadingMore] = React.useState(false)

  // State for timeline pagination
  const [timelineCursor, setTimelineCursor] = React.useState<string | undefined>(undefined)
  const [allTimelineEntries, setAllTimelineEntries] = React.useState<ContactTimelineEntry[]>([])
  const [isLoadingMoreTimeline, setIsLoadingMoreTimeline] = React.useState(false)

  // Load message history for this contact
  const {
    data: messageHistory,
    isLoading: loadingMessages,
    refetch: refetchMessages
  } = useQuery({
    queryKey: ['message_history', workspace.id, contactEmail, currentCursor],
    queryFn: () =>
      listMessages(workspace.id, {
        contact_email: contactEmail,
        limit: 5,
        cursor: currentCursor
      }),
    enabled: isVisible && !!contactEmail
  })

  // Load timeline for this contact
  const {
    data: timelineData,
    isLoading: loadingTimeline,
    refetch: refetchTimeline
  } = useQuery({
    queryKey: ['contact_timeline', workspace.id, contactEmail, timelineCursor],
    queryFn: () =>
      contactTimelineApi.list({
        workspace_id: workspace.id,
        email: contactEmail,
        limit: 10,
        cursor: timelineCursor
      }),
    enabled: isVisible && !!contactEmail
  })

  // Update allMessages when data changes
  React.useEffect(() => {
    // If data is still loading or not available, don't update
    if (loadingMessages || !messageHistory) return

    if (messageHistory.messages) {
      if (!currentCursor) {
        // Initial load - replace all messages
        setAllMessages(messageHistory.messages)
      } else if (messageHistory.messages.length > 0) {
        // If we have a cursor and new messages, append them
        setAllMessages((prev) => [...prev, ...messageHistory.messages])
      }
    }

    // Reset loading more flag
    setIsLoadingMore(false)
  }, [messageHistory, currentCursor, loadingMessages])

  // Update timeline entries when data changes
  React.useEffect(() => {
    if (loadingTimeline || !timelineData) return

    if (timelineData.timeline) {
      if (!timelineCursor) {
        // Initial load - replace all entries
        setAllTimelineEntries(timelineData.timeline)
      } else if (timelineData.timeline.length > 0) {
        // If we have a cursor and new entries, append them
        setAllTimelineEntries((prev) => [...prev, ...timelineData.timeline])
      }
    }

    // Reset loading more flag
    setIsLoadingMoreTimeline(false)
  }, [timelineData, timelineCursor, loadingTimeline])

  // Load more messages
  const handleLoadMore = () => {
    if (messageHistory?.next_cursor) {
      setIsLoadingMore(true)
      setCurrentCursor(messageHistory.next_cursor)
    }
  }

  // Load more timeline entries
  const handleLoadMoreTimeline = () => {
    if (timelineData?.next_cursor) {
      setIsLoadingMoreTimeline(true)
      setTimelineCursor(timelineData.next_cursor)
    }
  }

  // Refresh all drawer data from the beginning
  const handleRefreshAll = () => {
    // Reset pagination cursors
    setTimelineCursor(undefined)
    setCurrentCursor(undefined)

    // Refetch all queries
    refetchContact()
    refetchTimeline()
    refetchMessages()
  }

  // Fetch the single contact to ensure we have the latest data
  const {
    data: contact,
    isLoading: isLoadingContact,
    refetch: refetchContact
  } = useQuery({
    queryKey: ['contact_details', workspace.id, contactEmail],
    queryFn: async () => {
      const response = await contactsApi.list({
        workspace_id: workspace.id,
        email: contactEmail,
        with_contact_lists: true,
        limit: 1
      })
      return response.contacts[0]
    },
    enabled: isVisible && !!contactEmail,
    refetchOnWindowFocus: true
  })

  // Mutation for updating subscription status
  const updateStatusMutation = useMutation({
    mutationFn: (params: UpdateContactListStatusRequest) => contactListApi.updateStatus(params),
    onSuccess: () => {
      messageApi.success('Subscription status updated successfully')
      queryClient.invalidateQueries({ queryKey: ['contact_details', workspace.id, contactEmail] })
      queryClient.invalidateQueries({ queryKey: ['contacts', workspace.id] })
      setStatusModalVisible(false)
      statusForm.resetFields()
      // Refresh timeline to show the subscription status update event
      refetchTimeline()

      // After successful update, fetch the latest contact data to pass to the parent
      contactsApi
        .list({
          workspace_id: workspace.id,
          email: contactEmail,
          with_contact_lists: true,
          limit: 1
        })
        .then((response) => {
          if (response.contacts && response.contacts.length > 0 && onContactUpdate) {
            onContactUpdate(response.contacts[0])
          }
        })
    },
    onError: (error) => {
      messageApi.error(`Failed to update status: ${error}`)
    }
  })

  // Mutation for adding contact to a list
  const addToListMutation = useMutation({
    mutationFn: (params: SubscribeToListsRequest) => listsApi.subscribe(params),
    onSuccess: () => {
      messageApi.success('Contact added to list successfully')
      queryClient.invalidateQueries({ queryKey: ['contact_details', workspace.id, contactEmail] })
      setSubscribeModalVisible(false)
      subscribeForm.resetFields()
      // Refresh timeline to show the subscription event
      refetchTimeline()

      // After successful addition, fetch the latest contact data to pass to the parent
      contactsApi
        .list({
          workspace_id: workspace.id,
          email: contactEmail,
          with_contact_lists: true,
          limit: 1
        })
        .then((response) => {
          if (response.contacts && response.contacts.length > 0 && onContactUpdate) {
            onContactUpdate(response.contacts[0])
          }
        })
    },
    onError: (error) => {
      messageApi.error(`Failed to add to list: ${error}`)
    }
  })

  const handleContactUpdated = async (updatedContact: Contact) => {
    // Invalidate both the contact details
    await queryClient.invalidateQueries({
      queryKey: ['contact_details', workspace.id, contactEmail]
    })
    // Refresh timeline to show the contact update event
    refetchTimeline()
    // Call the onContactUpdate prop if it exists and we have the contact data
    if (onContactUpdate && updatedContact) {
      onContactUpdate(updatedContact)
    }
  }

  // Find list names based on list IDs
  const getListName = (listId: string): string => {
    const list = lists.find((list) => list.id === listId)
    return list ? list.name : listId
  }

  // Handle opening the status change modal
  const openStatusModal = (list: ContactListWithName) => {
    setSelectedList(list)
    statusForm.setFieldsValue({
      status: list.status
    })
    setStatusModalVisible(true)
  }

  // Handle status change submission
  const handleStatusChange = (values: { status: string }) => {
    if (!selectedList) return

    updateStatusMutation.mutate({
      workspace_id: workspace.id,
      email: contactEmail,
      list_id: selectedList.list_id,
      status: values.status
    })
  }

  // Handle opening the subscribe to list modal
  const openSubscribeModal = () => {
    subscribeForm.resetFields()
    setSubscribeModalVisible(true)
  }

  // Handle subscribe to list submission
  const handleSubscribe = (values: { list_id: string; status: string }) => {
    addToListMutation.mutate({
      workspace_id: workspace.id,
      contact: {
        email: contactEmail
      } as Contact,
      list_ids: [values.list_id]
    })
  }

  // Create name from first and last name
  const fullName = [contact?.first_name, contact?.last_name].filter(Boolean).join(' ') || ''

  const formatValue = (value: any) => {
    if (value === null || value === undefined) return '-'

    // Format number values with numbro
    if (typeof value === 'number') {
      // For currency-like fields
      if (String(value).includes('.') && value > 0) {
        return numbro(value).format({
          thousandSeparated: true,
          mantissa: 2,
          trimMantissa: true
        })
      }
      // For integer values
      return numbro(value).format({
        thousandSeparated: true,
        mantissa: 0
      })
    }

    if (typeof value === 'object') return JSON.stringify(value, null, 2)
    return value
  }

  // Format JSON with truncation and popover for full view
  const formatJson = (jsonData: any): React.ReactNode => {
    if (!jsonData) return '-'

    try {
      // If it's already an object, stringify it
      const jsonStr = typeof jsonData === 'string' ? jsonData : JSON.stringify(jsonData)
      const obj = typeof jsonData === 'string' ? JSON.parse(jsonData) : jsonData

      // Pretty format for popover
      const prettyJson = JSON.stringify(obj, null, 2)

      // Truncate for display
      const displayText = jsonStr.length > 100 ? jsonStr.substring(0, 100) + '...' : jsonStr

      const popoverContent = (
        <div
          className="p-2 bg-gray-50 rounded border border-gray-200 max-h-96 overflow-auto"
          style={{ maxWidth: '500px' }}
        >
          <pre className="text-xs m-0 whitespace-pre-wrap break-all">{prettyJson}</pre>
        </div>
      )

      return (
        <Popover
          content={popoverContent}
          title="JSON Data"
          placement="rightTop"
          trigger="click"
          styles={{
            root: {
              maxWidth: '400px'
            }
          }}
        >
          <div className="text-xs bg-gray-50 p-2 rounded border border-gray-200 cursor-pointer hover:bg-gray-100">
            <code className="truncate block">{displayText}</code>
            <div className="text-right mt-1 text-blue-500">
              <small>
                <FontAwesomeIcon icon={faEllipsis} className="mr-1" />
                Click to view full JSON
              </small>
            </div>
          </div>
        </Popover>
      )
    } catch (e) {
      return <Text type="danger">Invalid JSON</Text>
    }
  }

  // Format date using dayjs
  const formatDate = (dateString: string | undefined): string => {
    if (!dateString) return '-'
    return `${dayjs(dateString).format('lll')} in ${workspace.settings.timezone}`
  }

  // Format currency value using numbro
  const formatCurrency = (value: number | undefined): string => {
    if (value === undefined || value === null) return '$0.00'
    return numbro(value).formatCurrency({
      mantissa: 2,
      currencySymbol: '$',
      thousandSeparated: true,
      trimMantissa: true,
      spaceSeparatedCurrency: false
    })
  }

  // Format number with thousand separators
  const formatNumber = (value: number | undefined): string => {
    if (value === undefined || value === null) return '0'
    return numbro(value).format({
      thousandSeparated: true,
      mantissa: 0,
      trimMantissa: true,
      average: false
    })
  }

  // Format average number (with K, M, B, etc. for large numbers)
  const formatAverage = (value: number | undefined): string => {
    if (value === undefined || value === null) return '0'
    return numbro(value).format({
      average: true,
      mantissa: 1,
      spaceSeparated: true,
      trimMantissa: true
    })
  }

  // Get color for list status
  const getStatusColor = (status: string): string => {
    const statusColors: Record<string, string> = {
      active: 'green',
      subscribed: 'green',
      pending: 'orange',
      unsubscribed: 'red',
      bounced: 'volcano',
      complained: 'magenta',
      blacklisted: 'black'
    }
    return statusColors[status.toLowerCase()] || 'blue'
  }

  // Field display definitions without icons
  const contactFields = [
    { key: 'first_name', label: 'First Name', value: contact?.first_name },
    { key: 'last_name', label: 'Last Name', value: contact?.last_name },
    { key: 'email', label: 'Email', value: contact?.email },
    { key: 'phone', label: 'Phone', value: contact?.phone },
    {
      key: 'address',
      label: 'Address',
      value: [
        contact?.address_line_1,
        contact?.address_line_2,
        [contact?.state, contact?.postcode, contact?.country].filter(Boolean).join(', ')
      ]
        .filter(Boolean)
        .join(', '),
      show: !!(
        contact?.address_line_1 ||
        contact?.address_line_2 ||
        contact?.country ||
        contact?.state ||
        contact?.postcode
      )
    },
    { key: 'job_title', label: 'Job Title', value: contact?.job_title },
    { key: 'timezone', label: 'Timezone', value: contact?.timezone },
    { key: 'language', label: 'Language', value: contact?.language },
    { key: 'external_id', label: 'External ID', value: contact?.external_id },
    {
      key: 'lifetime_value',
      label: 'Lifetime Value',
      value: contact?.lifetime_value
    },
    {
      key: 'orders_count',
      label: 'Orders Count',
      value: contact?.orders_count
    },
    {
      key: 'last_order_at',
      label: 'Last Order At',
      value: formatDate(contact?.last_order_at)
    },
    {
      key: 'created_at',
      label: 'Created At',
      value: formatDate(contact?.created_at)
    },
    {
      key: 'updated_at',
      label: 'Updated At',
      value: formatDate(contact?.updated_at)
    },
    // Custom string fields
    {
      key: 'custom_string_1',
      label: 'Custom String 1',
      value: contact?.custom_string_1
    },
    {
      key: 'custom_string_2',
      label: 'Custom String 2',
      value: contact?.custom_string_2
    },
    {
      key: 'custom_string_3',
      label: 'Custom String 3',
      value: contact?.custom_string_3
    },
    {
      key: 'custom_string_4',
      label: 'Custom String 4',
      value: contact?.custom_string_4
    },
    {
      key: 'custom_string_5',
      label: 'Custom String 5',
      value: contact?.custom_string_5
    },
    // Custom number fields
    {
      key: 'custom_number_1',
      label: 'Custom Number 1',
      value: contact?.custom_number_1
    },
    {
      key: 'custom_number_2',
      label: 'Custom Number 2',
      value: contact?.custom_number_2
    },
    {
      key: 'custom_number_3',
      label: 'Custom Number 3',
      value: contact?.custom_number_3
    },
    {
      key: 'custom_number_4',
      label: 'Custom Number 4',
      value: contact?.custom_number_4
    },
    {
      key: 'custom_number_5',
      label: 'Custom Number 5',
      value: contact?.custom_number_5
    },
    // Custom date fields
    {
      key: 'custom_datetime_1',
      label: 'Custom Date 1',
      value: formatDate(contact?.custom_datetime_1)
    },
    {
      key: 'custom_datetime_2',
      label: 'Custom Date 2',
      value: formatDate(contact?.custom_datetime_2)
    },
    {
      key: 'custom_datetime_3',
      label: 'Custom Date 3',
      value: formatDate(contact?.custom_datetime_3)
    },
    {
      key: 'custom_datetime_4',
      label: 'Custom Date 4',
      value: formatDate(contact?.custom_datetime_4)
    },
    {
      key: 'custom_datetime_5',
      label: 'Custom Date 5',
      value: formatDate(contact?.custom_datetime_5)
    }
  ]

  // Add a separate section for JSON fields
  const jsonFields = [
    {
      key: 'custom_json_1',
      label: 'Custom JSON 1',
      value: contact?.custom_json_1,
      show: !!contact?.custom_json_1
    },
    {
      key: 'custom_json_2',
      label: 'Custom JSON 2',
      value: contact?.custom_json_2,
      show: !!contact?.custom_json_2
    },
    {
      key: 'custom_json_3',
      label: 'Custom JSON 3',
      value: contact?.custom_json_3,
      show: !!contact?.custom_json_3
    },
    {
      key: 'custom_json_4',
      label: 'Custom JSON 4',
      value: contact?.custom_json_4,
      show: !!contact?.custom_json_4
    },
    {
      key: 'custom_json_5',
      label: 'Custom JSON 5',
      value: contact?.custom_json_5,
      show: !!contact?.custom_json_5
    }
  ]

  // Check if there are any JSON fields to display
  const hasJsonFields = jsonFields.some((field) => field.show)

  // Prepare contact lists with enhanced information
  const contactListsWithNames = contact?.contact_lists.map((list) => ({
    ...list,
    name: getListName(list.list_id)
  }))

  // Get lists that the contact is not subscribed to
  const availableLists = lists.filter(
    (list) => !contact?.contact_lists.some((cl) => cl.list_id === list.id)
  )

  // Status options for dropdown
  const statusOptions = [
    { label: 'Active', value: 'active' },
    { label: 'Pending', value: 'pending' },
    { label: 'Unsubscribed', value: 'unsubscribed' },
    { label: 'Blacklisted', value: 'blacklisted' }
  ]

  // If buttonProps is provided, render a button that opens the drawer
  const {
    type = 'default',
    icon,
    buttonContent,
    className,
    style,
    size,
    disabled,
    loading,
    danger,
    ghost,
    block
  } = buttonProps

  return (
    <>
      <Button
        type={type}
        icon={icon}
        className={className}
        style={style}
        size={size}
        disabled={disabled}
        loading={loading}
        danger={danger}
        ghost={ghost}
        block={block}
        onClick={handleOpen}
      >
        {buttonContent}
      </Button>

      <Drawer
        title="Contact Details"
        width={1200}
        placement="right"
        className="drawer-body-no-padding"
        onClose={handleClose}
        open={internalVisible}
        extra={
          <Space>
            <Tooltip title="Refresh">
              <Button
                type="text"
                icon={<FontAwesomeIcon icon={faRotate} />}
                onClick={handleRefreshAll}
                loading={isLoadingContact || loadingTimeline || loadingMessages}
              />
            </Tooltip>
            <ContactUpsertDrawer
              workspace={workspace}
              contact={contact}
              onSuccess={handleContactUpdated}
              buttonProps={{
                icon: <FontAwesomeIcon icon={faPenToSquare} />,
                type: 'primary',
                ghost: true,
                buttonContent: 'Update'
              }}
            />
          </Space>
        }
      >
        <div className="flex h-full">
          {/* Left column - Contact Details (400px fixed width) */}
          <div
            className="bg-gray-50 overflow-y-auto h-full"
            style={{ width: '400px', minWidth: '400px', maxWidth: '400px' }}
          >
            {/* Contact info at the top */}
            <div className="p-6 pb-4 border-b border-gray-200 flex items-center gap-3">
              <Avatar src={gravatarUrl} size={64} />
              <div className="flex flex-col">
                <Title level={4} style={{ margin: 0, marginBottom: '4px' }}>
                  {fullName}
                </Title>
                <Text type="secondary">{contact?.email}</Text>
              </div>
            </div>

            {/* E-commerce Stats */}
            <div className="px-6 py-3 border-b border-gray-200">
              <div className="grid grid-cols-3 gap-4">
                {/* Lifetime Value */}
                <Tooltip
                  title={
                    contact?.lifetime_value ? formatCurrency(contact?.lifetime_value) : '$0.00'
                  }
                >
                  <div className="cursor-help">
                    <Statistic
                      title={<span className="text-[10px]">LTV</span>}
                      value={formatAverage(contact?.lifetime_value || 0)}
                      valueStyle={{ fontSize: '14px', fontWeight: 600 }}
                    />
                  </div>
                </Tooltip>

                {/* Orders Count */}
                <Tooltip title={`${formatNumber(contact?.orders_count || 0)} orders`}>
                  <div className="cursor-help">
                    <Statistic
                      title={<span className="text-[10px]">Orders</span>}
                      value={formatAverage(contact?.orders_count || 0)}
                      valueStyle={{ fontSize: '14px', fontWeight: 600 }}
                    />
                  </div>
                </Tooltip>

                {/* Last Order */}
                <Tooltip
                  title={
                    contact?.last_order_at
                      ? `${dayjs(contact?.last_order_at).format('LLLL')} in ${workspace.settings.timezone}`
                      : 'No orders yet'
                  }
                >
                  <div className="cursor-help">
                    <Statistic
                      title={<span className="text-[10px]">Last Order</span>}
                      value={
                        contact?.last_order_at ? dayjs(contact?.last_order_at).fromNow() : 'Never'
                      }
                      valueStyle={{ fontSize: '12px', fontWeight: 600 }}
                    />
                  </div>
                </Tooltip>
              </div>
            </div>

            <div className="contact-details">
              {isLoadingContact && (
                <div className="mb-4 p-2 bg-blue-50 text-blue-600 rounded text-center">
                  <Spin size="small" className="mr-2" />
                  <span>Refreshing contact data...</span>
                </div>
              )}

              {/* Display fields in a side-by-side layout */}
              {contactFields
                .filter(
                  (field) =>
                    field.value !== undefined &&
                    field.value !== null &&
                    field.value !== '-' &&
                    (field.show === undefined || field.show) &&
                    // Skip email as it's already shown at the top
                    field.key !== 'email' &&
                    // Skip name fields as they're already shown at the top
                    field.key !== 'first_name' &&
                    field.key !== 'last_name' &&
                    // Skip JSON fields as they'll be shown separately
                    !field.key.startsWith('custom_json_')
                )
                .map((field) => (
                  <div
                    key={field.key}
                    className="py-2 px-4 grid grid-cols-2 text-xs gap-1 border-b border-dashed border-gray-300"
                  >
                    <Tooltip title={`Field ID: ${field.key}`}>
                      <span className="font-semibold text-slate-600">{field.label}</span>
                    </Tooltip>
                    <span>{formatValue(field.value)}</span>
                  </div>
                ))}

              {/* Custom JSON fields */}
              {hasJsonFields && (
                <div>
                  {jsonFields
                    .filter((field) => field.show)
                    .map((field) => (
                      <div
                        key={field.key}
                        className="py-2 px-4 grid grid-cols-2 text-xs gap-1 border-b border-dashed border-gray-300"
                      >
                        <Tooltip title={`Field ID: ${field.key}`}>
                          <span className="font-semibold text-slate-600">{field.label}</span>
                        </Tooltip>
                        {formatJson(field.value)}
                      </div>
                    ))}
                </div>
              )}
            </div>
          </div>

          {/* Right column - Timeline and Message History (remaining space) */}
          <div className="flex-1 p-8 overflow-y-auto h-full">
            {/* List subscriptions with action buttons */}
            <div className="flex justify-between items-center mb-3">
              <Title level={5} style={{ margin: 0 }}>
                List Subscriptions
              </Title>
              <Button
                type="primary"
                ghost
                size="small"
                icon={<FontAwesomeIcon icon={faPlus} />}
                onClick={openSubscribeModal}
                disabled={availableLists.length === 0}
              >
                Subscribe to List
              </Button>
            </div>

            {contactListsWithNames && contactListsWithNames.length > 0 ? (
              <Table
                dataSource={contactListsWithNames}
                rowKey={(record) => `${record.list_id}_${record.status}`}
                pagination={false}
                size="small"
                columns={[
                  {
                    title: 'Subscription list',
                    dataIndex: 'name',
                    key: 'name',
                    width: '30%',
                    render: (name: string, record: any) => (
                      <Tooltip title={`List ID: ${record.list_id}`}>
                        <span style={{ cursor: 'help' }}>{name}</span>
                      </Tooltip>
                    )
                  },
                  {
                    title: 'Status',
                    dataIndex: 'status',
                    key: 'status',
                    width: '20%',
                    render: (status: string) => (
                      <Tag bordered={false} color={getStatusColor(status)}>
                        {status}
                      </Tag>
                    )
                  },
                  {
                    title: 'Subscribed on',
                    dataIndex: 'created_at',
                    key: 'created_at',
                    width: '30%',
                    render: (date: string) => {
                      if (!date) return '-'

                      return (
                        <Tooltip
                          title={`${dayjs(date).format('LLLL')} in ${workspace.settings.timezone}`}
                        >
                          <span>{dayjs(date).fromNow()}</span>
                        </Tooltip>
                      )
                    }
                  },
                  {
                    title: '',
                    key: 'actions',
                    width: '20%',
                    render: (_: any, record: ContactListWithName) => (
                      <Button
                        size="small"
                        onClick={() => openStatusModal(record)}
                        loading={
                          updateStatusMutation.isPending && selectedList?.list_id === record.list_id
                        }
                      >
                        Change Status
                      </Button>
                    )
                  }
                ]}
              />
            ) : (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description="This contact is not subscribed to any lists"
                style={{ margin: '20px 0' }}
              >
                <Button
                  type="primary"
                  onClick={openSubscribeModal}
                  disabled={availableLists.length === 0}
                  icon={<FontAwesomeIcon icon={faPlus} />}
                >
                  Subscribe to List
                </Button>
              </Empty>
            )}

            <div className="mt-6">
              <Title level={5} style={{ margin: 0, marginBottom: '16px' }}>
                Activity
              </Title>

              <Tabs
                defaultActiveKey="timeline"
                items={[
                  {
                    key: 'timeline',
                    label: 'Timeline',
                    children: (
                      <div className="pt-4">
                        <ContactTimeline
                          entries={allTimelineEntries}
                          loading={loadingTimeline}
                          timezone={contact?.timezone || workspace.settings.timezone}
                          workspace={workspace}
                          onLoadMore={handleLoadMoreTimeline}
                          hasMore={!!timelineData?.next_cursor}
                          isLoadingMore={isLoadingMoreTimeline}
                        />
                      </div>
                    )
                  },
                  {
                    key: 'messages',
                    label: 'Messages',
                    children: (
                      <div className="pt-4">
                        <MessageHistoryTable
                          messages={allMessages}
                          loading={loadingMessages}
                          isLoadingMore={isLoadingMore}
                          workspace={workspace}
                          nextCursor={messageHistory?.next_cursor}
                          onLoadMore={handleLoadMore}
                          show_email={false} // Hide email since we're in contact details
                          size="small"
                        />
                      </div>
                    )
                  }
                ]}
              />
            </div>
          </div>
        </div>

        {/* Change Status Modal */}
        <Modal
          title={`Change Status for ${selectedList?.name || 'List'}`}
          open={statusModalVisible}
          onCancel={() => setStatusModalVisible(false)}
          footer={null}
        >
          <Form form={statusForm} layout="vertical" onFinish={handleStatusChange}>
            <Form.Item
              name="status"
              label="Subscription Status"
              rules={[{ required: true, message: 'Please select a status' }]}
            >
              <Select options={statusOptions} />
            </Form.Item>
            <Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" loading={updateStatusMutation.isPending}>
                  Update Status
                </Button>
                <Button onClick={() => setStatusModalVisible(false)}>Cancel</Button>
              </Space>
            </Form.Item>
          </Form>
        </Modal>

        {/* Subscribe to List Modal */}
        <Modal
          title="Subscribe to List"
          open={subscribeModalVisible}
          onCancel={() => setSubscribeModalVisible(false)}
          footer={null}
        >
          <Form form={subscribeForm} layout="vertical" onFinish={handleSubscribe}>
            <Form.Item
              name="list_id"
              label="Select List"
              rules={[{ required: true, message: 'Please select a list' }]}
            >
              <Select
                options={availableLists.map((list) => ({
                  label: list.name,
                  value: list.id
                }))}
                placeholder="Select a list"
              />
            </Form.Item>
            <Form.Item
              name="status"
              label="Subscription Status"
              initialValue="active"
              rules={[{ required: true, message: 'Please select a status' }]}
            >
              <Select
                options={[
                  { label: 'Active', value: 'active' },
                  { label: 'Pending', value: 'pending' }
                ]}
              />
            </Form.Item>
            <Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" loading={addToListMutation.isPending}>
                  Subscribe
                </Button>
                <Button onClick={() => setSubscribeModalVisible(false)}>Cancel</Button>
              </Space>
            </Form.Item>
          </Form>
        </Modal>
      </Drawer>
    </>
  )
}
