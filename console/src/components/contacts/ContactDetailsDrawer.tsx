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
  App,
  Avatar,
  Tabs,
  Collapse
} from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Contact } from '../../services/api/contacts'
import { List, Workspace } from '../../services/api/types'
import { Segment } from '../../services/api/segment'
import dayjs from '../../lib/dayjs'
import { InlineEditableField } from './InlineEditableField'
import { getFieldType } from './fieldTypes'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus, faRotate } from '@fortawesome/free-solid-svg-icons'
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import { listMessages, MessageHistory } from '../../services/api/messages_history'
import { contactsApi } from '../../services/api/contacts'
import { contactListApi, UpdateContactListStatusRequest } from '../../services/api/contact_list'
import { listsApi } from '../../services/api/list'
import { SubscribeToListsRequest } from '../../services/api/types'
import { MessageHistoryTable } from '../messages/MessageHistoryTable'
import { ContactTimeline } from '../timeline'
import { contactTimelineApi, ContactTimelineEntry } from '../../services/api/contact_timeline'
import { computeCustomFieldLabel } from '../../hooks/useCustomFieldLabel'

const { Title, Text } = Typography

interface ContactDetailsDrawerProps {
  workspace: Workspace
  contactEmail: string
  visible?: boolean
  onClose?: () => void
  lists?: List[]
  segments?: Segment[]
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
  segments = [],
  onContactUpdate,
  buttonProps
}: ContactDetailsDrawerProps) {
  const { t } = useLingui()
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
      messageApi.success(t`Subscription status updated successfully`)
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
      messageApi.error(t`Failed to update status: ${error}`)
    }
  })

  // Mutation for adding contact to a list
  const addToListMutation = useMutation({
    mutationFn: (params: SubscribeToListsRequest) => listsApi.subscribe(params),
    onSuccess: () => {
      messageApi.success(t`Contact added to list successfully`)
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
      messageApi.error(t`Failed to add to list: ${error}`)
    }
  })

  // Early return after all hooks
  if (!contactEmail) return null

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

  // Handle inline field update
  const handleFieldUpdate = async (fieldKey: string, value: string | number | object | null) => {
    if (!contact) return

    const contactData: Record<string, string | number | object | null> = {
      email: contact.email,
      [fieldKey]: value
    }

    const response = await contactsApi.upsert({
      workspace_id: workspace.id,
      contact: contactData
    })

    if (response.action === 'error') {
      throw new Error(response.error || t`Failed to update field`)
    }

    // Fetch updated contact data
    const listResponse = await contactsApi.list({
      workspace_id: workspace.id,
      email: contact.email,
      with_contact_lists: true,
      limit: 1
    })

    if (listResponse.contacts && listResponse.contacts.length > 0) {
      handleContactUpdated(listResponse.contacts[0])
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
  const handleSubscribe = (values: { list_id: string }) => {
    addToListMutation.mutate({
      workspace_id: workspace.id,
      contact: {
        email: contactEmail
      } as Contact,
      list_ids: [values.list_id]
    })
  }

  // Create name from full_name, or fallback to first_name + last_name
  const fullName = contact?.full_name || [contact?.first_name, contact?.last_name].filter(Boolean).join(' ') || ''

  // Format date using dayjs
  const formatDate = (dateString: string | undefined): string => {
    if (!dateString) return '-'
    return `${dayjs(dateString).format('lll')} in ${workspace.settings.timezone}`
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

  // Helper function to get custom field label with tooltip info
  const getFieldLabel = (fieldKey: string) => {
    return computeCustomFieldLabel(fieldKey, workspace)
  }

  // Editable fields configuration
  const editableFields = [
    { key: 'first_name', label: t`First Name`, value: contact?.first_name },
    { key: 'last_name', label: t`Last Name`, value: contact?.last_name },
    { key: 'full_name', label: t`Full Name`, value: contact?.full_name },
    { key: 'phone', label: t`Phone`, value: contact?.phone },
    { key: 'address_line_1', label: t`Address Line 1`, value: contact?.address_line_1 },
    { key: 'address_line_2', label: t`Address Line 2`, value: contact?.address_line_2 },
    { key: 'country', label: t`Country`, value: contact?.country },
    { key: 'state', label: t`State`, value: contact?.state },
    { key: 'postcode', label: t`Postcode`, value: contact?.postcode },
    { key: 'job_title', label: t`Job Title`, value: contact?.job_title },
    { key: 'timezone', label: t`Timezone`, value: contact?.timezone },
    { key: 'language', label: t`Language`, value: contact?.language },
    { key: 'external_id', label: t`External ID`, value: contact?.external_id },
    // Custom string fields
    {
      key: 'custom_string_1',
      ...getFieldLabel('custom_string_1'),
      value: contact?.custom_string_1
    },
    {
      key: 'custom_string_2',
      ...getFieldLabel('custom_string_2'),
      value: contact?.custom_string_2
    },
    {
      key: 'custom_string_3',
      ...getFieldLabel('custom_string_3'),
      value: contact?.custom_string_3
    },
    {
      key: 'custom_string_4',
      ...getFieldLabel('custom_string_4'),
      value: contact?.custom_string_4
    },
    {
      key: 'custom_string_5',
      ...getFieldLabel('custom_string_5'),
      value: contact?.custom_string_5
    },
    // Custom number fields
    {
      key: 'custom_number_1',
      ...getFieldLabel('custom_number_1'),
      value: contact?.custom_number_1
    },
    {
      key: 'custom_number_2',
      ...getFieldLabel('custom_number_2'),
      value: contact?.custom_number_2
    },
    {
      key: 'custom_number_3',
      ...getFieldLabel('custom_number_3'),
      value: contact?.custom_number_3
    },
    {
      key: 'custom_number_4',
      ...getFieldLabel('custom_number_4'),
      value: contact?.custom_number_4
    },
    {
      key: 'custom_number_5',
      ...getFieldLabel('custom_number_5'),
      value: contact?.custom_number_5
    },
    // Custom datetime fields (pass raw value, InlineEditableField will format)
    {
      key: 'custom_datetime_1',
      ...getFieldLabel('custom_datetime_1'),
      value: contact?.custom_datetime_1
    },
    {
      key: 'custom_datetime_2',
      ...getFieldLabel('custom_datetime_2'),
      value: contact?.custom_datetime_2
    },
    {
      key: 'custom_datetime_3',
      ...getFieldLabel('custom_datetime_3'),
      value: contact?.custom_datetime_3
    },
    {
      key: 'custom_datetime_4',
      ...getFieldLabel('custom_datetime_4'),
      value: contact?.custom_datetime_4
    },
    {
      key: 'custom_datetime_5',
      ...getFieldLabel('custom_datetime_5'),
      value: contact?.custom_datetime_5
    }
  ]

  // Read-only fields (not editable)
  const readOnlyFields = [
    { key: 'created_at', label: t`Created At`, value: formatDate(contact?.created_at) },
    { key: 'updated_at', label: t`Updated At`, value: formatDate(contact?.updated_at) }
  ]

  // JSON fields (editable)
  const jsonFields = [
    {
      key: 'custom_json_1',
      ...getFieldLabel('custom_json_1'),
      value: contact?.custom_json_1
    },
    {
      key: 'custom_json_2',
      ...getFieldLabel('custom_json_2'),
      value: contact?.custom_json_2
    },
    {
      key: 'custom_json_3',
      ...getFieldLabel('custom_json_3'),
      value: contact?.custom_json_3
    },
    {
      key: 'custom_json_4',
      ...getFieldLabel('custom_json_4'),
      value: contact?.custom_json_4
    },
    {
      key: 'custom_json_5',
      ...getFieldLabel('custom_json_5'),
      value: contact?.custom_json_5
    }
  ]

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
    { label: t`Active`, value: 'active' },
    { label: t`Pending`, value: 'pending' },
    { label: t`Unsubscribed`, value: 'unsubscribed' },
    { label: t`Blacklisted`, value: 'blacklisted' }
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
        title={t`Contact Details`}
        width={1200}
        placement="right"
        className="drawer-body-no-padding"
        onClose={handleClose}
        open={internalVisible}
        extra={
          <Tooltip title={t`Refresh`}>
            <Button
              type="text"
              icon={<FontAwesomeIcon icon={faRotate} />}
              onClick={handleRefreshAll}
              loading={isLoadingContact || loadingTimeline || loadingMessages}
            />
          </Tooltip>
        }
      >
        <div className="flex h-full">
          {/* Left column - Contact Details (400px fixed width) */}
          <div
            className="bg-gray-50 overflow-y-auto h-full"
            style={{
              width: '400px',
              minWidth: '400px',
              maxWidth: '400px',
              borderRight: '1px solid #f0f0f0'
            }}
          >
            {/* Contact info at the top */}
            <div className="p-6 pb-4 border-b border-gray-200 flex items-center gap-3">
              <Avatar src={gravatarUrl} size={64} />
              <div className="flex flex-col">
                <Title level={4} style={{ margin: 0, marginBottom: '4px' }}>
                  {fullName}
                </Title>
                <Text type="secondary">{contact?.email}</Text>
                {/* Contact segments */}
                {contact?.contact_segments && contact.contact_segments.length > 0 && (
                  <Space size={4} wrap style={{ marginTop: '8px' }}>
                    {contact.contact_segments.map((cs) => {
                      const segment = segments.find((s) => s.id === cs.segment_id)
                      if (!segment) return null
                      return (
                        <Tag key={cs.segment_id} bordered={false} color={segment.color}>
                          {segment.name}
                        </Tag>
                      )
                    })}
                  </Space>
                )}
              </div>
            </div>

            <div className="contact-details">
              {isLoadingContact && (
                <div className="mb-4 p-2 bg-blue-50 text-blue-600 rounded text-center">
                  <Spin size="small" className="mr-2" />
                  <span>{t`Refreshing contact data...`}</span>
                </div>
              )}

              {/* Set standard fields (non-custom) */}
              {editableFields
                .filter((field) => !field.key.startsWith('custom_') && field.value !== null && field.value !== undefined && field.value !== '')
                .map((field) => (
                <InlineEditableField
                  key={field.key}
                  fieldKey={field.key}
                  fieldType={getFieldType(field.key)}
                  label={field.label}
                  displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                  showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                  technicalName={'technicalName' in field ? field.technicalName : undefined}
                  value={field.value}
                  workspace={workspace}
                  onSave={handleFieldUpdate}
                  isLoading={isLoadingContact}
                />
              ))}

              {/* Set custom fields */}
              {editableFields
                .filter((field) => field.key.startsWith('custom_') && field.value !== null && field.value !== undefined && field.value !== '')
                .map((field) => (
                <InlineEditableField
                  key={field.key}
                  fieldKey={field.key}
                  fieldType={getFieldType(field.key)}
                  label={field.label}
                  displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                  showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                  technicalName={'technicalName' in field ? field.technicalName : undefined}
                  value={field.value}
                  workspace={workspace}
                  onSave={handleFieldUpdate}
                  isLoading={isLoadingContact}
                />
              ))}

              {/* Set JSON fields */}
              {jsonFields
                .filter((field) => field.value !== null && field.value !== undefined && field.value !== '')
                .map((field) => (
                <InlineEditableField
                  key={field.key}
                  fieldKey={field.key}
                  fieldType="json"
                  label={field.label}
                  displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                  showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                  technicalName={'technicalName' in field ? field.technicalName : undefined}
                  value={field.value as string | number | object | null | undefined}
                  workspace={workspace}
                  onSave={handleFieldUpdate}
                  isLoading={isLoadingContact}
                />
              ))}

              {/* Labeled but unset custom fields (priority display) */}
              {editableFields
                .filter((field) =>
                  field.key.startsWith('custom_') &&
                  !!workspace?.settings?.custom_field_labels?.[field.key] &&
                  (field.value === null || field.value === undefined || field.value === '')
                )
                .map((field) => (
                <InlineEditableField
                  key={field.key}
                  fieldKey={field.key}
                  fieldType={getFieldType(field.key)}
                  label={field.label}
                  displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                  showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                  technicalName={'technicalName' in field ? field.technicalName : undefined}
                  value={field.value}
                  workspace={workspace}
                  onSave={handleFieldUpdate}
                  isLoading={isLoadingContact}
                />
              ))}

              {/* Labeled but unset JSON fields (priority display) */}
              {jsonFields
                .filter((field) =>
                  !!workspace?.settings?.custom_field_labels?.[field.key] &&
                  (field.value === null || field.value === undefined || field.value === '')
                )
                .map((field) => (
                <InlineEditableField
                  key={field.key}
                  fieldKey={field.key}
                  fieldType="json"
                  label={field.label}
                  displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                  showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                  technicalName={'technicalName' in field ? field.technicalName : undefined}
                  value={field.value as string | number | object | null | undefined}
                  workspace={workspace}
                  onSave={handleFieldUpdate}
                  isLoading={isLoadingContact}
                />
              ))}

              {/* Read-only fields */}
              {readOnlyFields.map((field) => (
                <div
                  key={field.key}
                  className="py-2 px-4 grid grid-cols-2 text-xs gap-1 border-b border-dashed border-gray-300"
                >
                  <span className="font-semibold text-slate-600">{field.label}</span>
                  <span className="text-gray-500">{field.value || '—'}</span>
                </div>
              ))}

              {/* Unset standard fields */}
              {editableFields
                .filter((field) => !field.key.startsWith('custom_') && (field.value === null || field.value === undefined || field.value === ''))
                .map((field) => (
                <InlineEditableField
                  key={field.key}
                  fieldKey={field.key}
                  fieldType={getFieldType(field.key)}
                  label={field.label}
                  displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                  showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                  technicalName={'technicalName' in field ? field.technicalName : undefined}
                  value={field.value}
                  workspace={workspace}
                  onSave={handleFieldUpdate}
                  isLoading={isLoadingContact}
                />
              ))}

              {/* Unset custom fields in expandable sections (excluding labeled fields) */}
              {(() => {
                const hasConfiguredLabel = (fieldKey: string) => !!workspace?.settings?.custom_field_labels?.[fieldKey]

                const unsetCustomStrings = editableFields.filter(
                  (field) => field.key.startsWith('custom_string_') &&
                    !hasConfiguredLabel(field.key) &&
                    (field.value === null || field.value === undefined || field.value === '')
                )
                const unsetCustomNumbers = editableFields.filter(
                  (field) => field.key.startsWith('custom_number_') &&
                    !hasConfiguredLabel(field.key) &&
                    (field.value === null || field.value === undefined || field.value === '')
                )
                const unsetCustomDatetimes = editableFields.filter(
                  (field) => field.key.startsWith('custom_datetime_') &&
                    !hasConfiguredLabel(field.key) &&
                    (field.value === null || field.value === undefined || field.value === '')
                )
                const unsetCustomJsons = jsonFields.filter(
                  (field) => !hasConfiguredLabel(field.key) &&
                    (field.value === null || field.value === undefined || field.value === '')
                )

                const collapseItems = []

                if (unsetCustomStrings.length > 0) {
                  collapseItems.push({
                    key: 'custom_strings',
                    label: <span className="text-xs text-gray-500">{t`Custom String Fields`} ({unsetCustomStrings.length})</span>,
                    children: unsetCustomStrings.map((field) => (
                      <InlineEditableField
                        key={field.key}
                        fieldKey={field.key}
                        fieldType="string"
                        label={field.label}
                        displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                        showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                        technicalName={'technicalName' in field ? field.technicalName : undefined}
                        value={field.value}
                        workspace={workspace}
                        onSave={handleFieldUpdate}
                        isLoading={isLoadingContact}
                      />
                    ))
                  })
                }

                if (unsetCustomNumbers.length > 0) {
                  collapseItems.push({
                    key: 'custom_numbers',
                    label: <span className="text-xs text-gray-500">{t`Custom Number Fields`} ({unsetCustomNumbers.length})</span>,
                    children: unsetCustomNumbers.map((field) => (
                      <InlineEditableField
                        key={field.key}
                        fieldKey={field.key}
                        fieldType="number"
                        label={field.label}
                        displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                        showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                        technicalName={'technicalName' in field ? field.technicalName : undefined}
                        value={field.value}
                        workspace={workspace}
                        onSave={handleFieldUpdate}
                        isLoading={isLoadingContact}
                      />
                    ))
                  })
                }

                if (unsetCustomDatetimes.length > 0) {
                  collapseItems.push({
                    key: 'custom_datetimes',
                    label: <span className="text-xs text-gray-500">{t`Custom Datetime Fields`} ({unsetCustomDatetimes.length})</span>,
                    children: unsetCustomDatetimes.map((field) => (
                      <InlineEditableField
                        key={field.key}
                        fieldKey={field.key}
                        fieldType="datetime"
                        label={field.label}
                        displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                        showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                        technicalName={'technicalName' in field ? field.technicalName : undefined}
                        value={field.value}
                        workspace={workspace}
                        onSave={handleFieldUpdate}
                        isLoading={isLoadingContact}
                      />
                    ))
                  })
                }

                if (unsetCustomJsons.length > 0) {
                  collapseItems.push({
                    key: 'custom_jsons',
                    label: <span className="text-xs text-gray-500">{t`Custom JSON Fields`} ({unsetCustomJsons.length})</span>,
                    children: unsetCustomJsons.map((field) => (
                      <InlineEditableField
                        key={field.key}
                        fieldKey={field.key}
                        fieldType="json"
                        label={field.label}
                        displayLabel={'displayLabel' in field ? field.displayLabel : undefined}
                        showTooltip={'showTooltip' in field ? field.showTooltip : undefined}
                        technicalName={'technicalName' in field ? field.technicalName : undefined}
                        value={field.value as string | number | object | null | undefined}
                        workspace={workspace}
                        onSave={handleFieldUpdate}
                        isLoading={isLoadingContact}
                      />
                    ))
                  })
                }

                if (collapseItems.length === 0) return null

                return (
                  <Collapse
                    ghost
                    size="small"
                    items={collapseItems}
                    className="mt-2"
                  />
                )
              })()}
            </div>
          </div>

          {/* Right column - Timeline and Message History (remaining space) */}
          <div className="flex-1 p-8 overflow-y-auto h-full">
            {/* List subscriptions with action buttons */}
            <div className="flex justify-between items-center mb-3">
              <Title level={5} style={{ margin: 0 }}>
                {t`List Subscriptions`}
              </Title>
              <Button
                type="primary"
                ghost
                size="small"
                icon={<FontAwesomeIcon icon={faPlus} />}
                onClick={openSubscribeModal}
                disabled={availableLists.length === 0}
              >
                {t`Subscribe to List`}
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
                    title: t`Subscription list`,
                    dataIndex: 'name',
                    key: 'name',
                    width: '30%',
                    render: (name: string, record: ContactListWithName) => (
                      <Tooltip title={t`List ID: ${record.list_id}`}>
                        <span style={{ cursor: 'help' }}>{name}</span>
                      </Tooltip>
                    )
                  },
                  {
                    title: t`Status`,
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
                    title: t`Subscribed on`,
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
                    render: (_: unknown, record: ContactListWithName) => (
                      <Button
                        size="small"
                        onClick={() => openStatusModal(record)}
                        loading={
                          updateStatusMutation.isPending && selectedList?.list_id === record.list_id
                        }
                      >
                        {t`Change Status`}
                      </Button>
                    )
                  }
                ]}
              />
            ) : (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={t`This contact is not subscribed to any lists`}
                style={{ margin: '20px 0' }}
              >
                <Button
                  type="primary"
                  onClick={openSubscribeModal}
                  disabled={availableLists.length === 0}
                  icon={<FontAwesomeIcon icon={faPlus} />}
                >
                  {t`Subscribe to List`}
                </Button>
              </Empty>
            )}

            <div className="mt-6">
              <Title level={5} style={{ margin: 0, marginBottom: '16px' }}>
                {t`Activity`}
              </Title>

              <Tabs
                defaultActiveKey="timeline"
                items={[
                  {
                    key: 'timeline',
                    label: t`Timeline`,
                    children: (
                      <div className="pt-4">
                        <ContactTimeline
                          entries={allTimelineEntries}
                          loading={loadingTimeline}
                          timezone={contact?.timezone || workspace.settings.timezone}
                          workspace={workspace}
                          segments={segments}
                          onLoadMore={handleLoadMoreTimeline}
                          hasMore={!!timelineData?.next_cursor}
                          isLoadingMore={isLoadingMoreTimeline}
                        />
                      </div>
                    )
                  },
                  {
                    key: 'messages',
                    label: t`Messages`,
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
          title={t`Change Status for ${selectedList?.name || 'List'}`}
          open={statusModalVisible}
          onCancel={() => setStatusModalVisible(false)}
          footer={null}
        >
          <Form form={statusForm} layout="vertical" onFinish={handleStatusChange}>
            <Form.Item
              name="status"
              label={t`Subscription Status`}
              rules={[{ required: true, message: t`Please select a status` }]}
            >
              <Select options={statusOptions} />
            </Form.Item>
            <Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" loading={updateStatusMutation.isPending}>
                  {t`Update Status`}
                </Button>
                <Button onClick={() => setStatusModalVisible(false)}>{t`Cancel`}</Button>
              </Space>
            </Form.Item>
          </Form>
        </Modal>

        {/* Subscribe to List Modal */}
        <Modal
          title={t`Subscribe to List`}
          open={subscribeModalVisible}
          onCancel={() => setSubscribeModalVisible(false)}
          footer={null}
        >
          <Form form={subscribeForm} layout="vertical" onFinish={handleSubscribe}>
            <Form.Item
              name="list_id"
              label={t`Select List`}
              rules={[{ required: true, message: t`Please select a list` }]}
            >
              <Select
                options={availableLists.map((list) => ({
                  label: list.name,
                  value: list.id
                }))}
                placeholder={t`Select a list`}
              />
            </Form.Item>
            <Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" loading={addToListMutation.isPending}>
                  {t`Subscribe`}
                </Button>
                <Button onClick={() => setSubscribeModalVisible(false)}>{t`Cancel`}</Button>
              </Space>
            </Form.Item>
          </Form>
        </Modal>
      </Drawer>
    </>
  )
}
