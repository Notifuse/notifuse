import React from 'react'
import { Timeline, Empty, Spin, Button, Tag, Tooltip, Typography, Popover, Collapse } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faCheck,
  faClock,
  faMousePointer,
  faCircleExclamation,
  faTriangleExclamation,
  faArrowRightToBracket,
  faArrowRightFromBracket,
  faBolt
} from '@fortawesome/free-solid-svg-icons'
import { faUser, faFolderOpen, faPaperPlane, faEye } from '@fortawesome/free-regular-svg-icons'
import {
  ContactTimelineEntry,
  ContactListEntityData,
  MessageHistoryEntityData,
  WebhookEventEntityData,
  CustomEventEntityData
} from '../../services/api/contact_timeline'
import type { Workspace } from '../../services/api/types'
import type { Segment } from '../../services/api/segment'
import dayjs from '../../lib/dayjs'
import TemplatePreviewDrawer from '../templates/TemplatePreviewDrawer'
import { getProviderIcon } from '../integrations/EmailProviders'
import { formatValue, formatEventName, getSourceBadge } from '../../utils/formatters'

const { Text } = Typography

interface ContactTimelineProps {
  entries: ContactTimelineEntry[]
  loading?: boolean
  timezone?: string
  workspace?: Workspace
  segments?: Segment[]
  onLoadMore?: () => void
  hasMore?: boolean
  isLoadingMore?: boolean
}

export function ContactTimeline({
  entries,
  loading = false,
  timezone = 'UTC',
  workspace,
  segments = [],
  onLoadMore,
  hasMore = false,
  isLoadingMore = false
}: ContactTimelineProps) {
  // Get color based on operation
  const getOperationColor = (operation: string) => {
    switch (operation) {
      case 'insert':
        return 'green'
      case 'update':
        return 'blue'
      case 'delete':
        return 'red'
      default:
        return 'gray'
    }
  }

  // Get color for contact list status
  const getStatusColor = (status: string) => {
    switch (status?.toLowerCase()) {
      case 'active':
      case 'subscribed':
        return 'green'
      case 'pending':
        return 'orange'
      case 'unsubscribed':
        return 'red'
      case 'bounced':
        return 'volcano'
      case 'complained':
        return 'magenta'
      case 'blacklisted':
        return 'black'
      default:
        return 'blue'
    }
  }

  // Get icon based on entity type
  const getEntityIcon = (entry: ContactTimelineEntry) => {
    const entityType = entry.entity_type
    switch (entityType) {
      case 'contact':
        return faUser
      case 'contact_list':
        return faFolderOpen
      case 'contact_segment':
        // Use kind to determine join vs leave
        if (entry.kind === 'join_segment') {
          return faArrowRightToBracket
        } else if (entry.kind === 'leave_segment') {
          return faArrowRightFromBracket
        }
        return faClock
      case 'message_history':
        if (entry.changes.delivered_at) {
          return faCheck
        } else if (entry.changes.opened_at) {
          return faEye
        } else if (entry.changes.clicked_at) {
          return faMousePointer
        }
        return faPaperPlane
      case 'webhook_event':
        const webhookData = entry.entity_data as any
        const eventType = webhookData?.type
        if (eventType === 'bounce') {
          return faCircleExclamation
        } else if (eventType === 'complaint') {
          return faTriangleExclamation
        } else if (eventType === 'delivered') {
          return faCheck
        }
        return faBolt
      case 'custom_event':
        return faBolt
      default:
        return faClock
    }
  }

  // Format entity type for display
  const formatEntityType = (entityType: string) => {
    return entityType
      .split('_')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
  }

  // Render title with date in standardized format
  const renderTitleWithDate = (entry: ContactTimelineEntry, titleContent: React.ReactNode) => {
    return (
      <div className="flex items-center gap-4 mb-2">
        {titleContent}
        <Tooltip title={`${dayjs(entry.created_at).format('LLLL')} in ${timezone}`}>
          <span>
            <Text type="secondary" className="text-xs cursor-help">
              {dayjs(entry.created_at).fromNow()}
            </Text>
          </span>
        </Tooltip>
      </div>
    )
  }

  // Render contact list subscription message based on status
  const renderContactListMessage = (entry: ContactTimelineEntry) => {
    const statusChange = entry.changes?.status
    const listId = entry.entity_id || 'Unknown List'

    // Extract old and new values if they exist
    const oldStatus =
      typeof statusChange === 'object' && statusChange?.old ? statusChange.old : null
    const newStatus =
      typeof statusChange === 'object' && statusChange?.new ? statusChange.new : statusChange

    // Use entity_data if available to get list name
    const entityData = entry.entity_data as ContactListEntityData | undefined
    const listName = entityData?.name
    const listDisplay = listName ? (
      <>
        <Tooltip title={'ID: ' + listId}>
          <span>
            <Text strong>{listName}</Text>{' '}
          </span>
        </Tooltip>
      </>
    ) : (
      <Text code>{listId}</Text>
    )

    if (entry.operation === 'insert') {
      return (
        <div>
          {renderTitleWithDate(entry, <Text strong>Subscription</Text>)}
          <div className="mb-2">
            <Text>
              Added to list {listDisplay} with status{' '}
              <Tag bordered={false} color={getStatusColor(newStatus)}>
                {newStatus}
              </Tag>
            </Text>
          </div>
        </div>
      )
    } else if (entry.operation === 'update') {
      // Status change - show from/to if old status exists
      return (
        <div>
          {renderTitleWithDate(entry, <Text strong>Subscription</Text>)}
          <div className="mb-2">
            {oldStatus ? (
              <Text>
                Status changed from{' '}
                <Tag bordered={false} color={getStatusColor(oldStatus)}>
                  {oldStatus}
                </Tag>{' '}
                to{' '}
                <Tag bordered={false} color={getStatusColor(newStatus)}>
                  {newStatus}
                </Tag>{' '}
                for list {listDisplay}
              </Text>
            ) : (
              <Text>
                Status changed to{' '}
                <Tag bordered={false} color={getStatusColor(newStatus)}>
                  {newStatus}
                </Tag>{' '}
                for list {listDisplay}
              </Text>
            )}
          </div>
        </div>
      )
    } else if (entry.operation === 'delete') {
      return <div>{renderTitleWithDate(entry, <Text>Removed from list {listDisplay}</Text>)}</div>
    }

    return null
  }

  // Render custom event properties with tiered display approach
  const renderCustomEventProperties = (
    properties: Record<string, any> | undefined,
    timezone: string
  ): React.ReactNode => {
    if (!properties || Object.keys(properties).length === 0) {
      return (
        <Text type="secondary" className="text-xs">
          No properties
        </Text>
      )
    }

    const entries = Object.entries(properties)
    const propertyCount = entries.length

    // Check if all values are primitives (not objects or arrays)
    const allPrimitives = entries.every(
      ([_, value]) => typeof value !== 'object' || value === null
    )

    // Tier 1: Inline display for ≤3 properties with all primitives
    if (propertyCount <= 3 && allPrimitives) {
      return (
        <div className="space-y-1 mt-2">
          {entries.map(([key, value]) => (
            <div key={key} className="text-sm">
              <Text type="secondary" className="font-mono text-xs">
                {key}:
              </Text>{' '}
              {formatValue(value, timezone)}
            </div>
          ))}
        </div>
      )
    }

    // Tier 2: Expandable for >3 properties or complex objects
    const rawJsonContent = (
      <div className="p-2 bg-gray-50 rounded border border-gray-200 max-h-96 overflow-auto">
        <pre className="text-xs m-0 whitespace-pre-wrap break-all">
          {JSON.stringify(properties, null, 2)}
        </pre>
      </div>
    )

    return (
      <div className="mt-2 space-y-2">
        <Collapse
          size="small"
          items={[
            {
              key: '1',
              label: `${propertyCount} properties`,
              children: (
                <div className="space-y-1">
                  {entries.map(([key, value]) => (
                    <div key={key} className="text-sm">
                      <Text type="secondary" className="font-mono text-xs">
                        {key}:
                      </Text>{' '}
                      {formatValue(value, timezone)}
                    </div>
                  ))}
                </div>
              )
            }
          ]}
        />
        <Popover
          content={rawJsonContent}
          title="Raw JSON"
          placement="rightTop"
          trigger="click"
          overlayStyle={{ maxWidth: '600px' }}
        >
          <Button size="small" type="text">
            View Raw JSON
          </Button>
        </Popover>
      </div>
    )
  }

  // Render entity-specific details based on entity type
  const renderEntityDetails = (entry: ContactTimelineEntry) => {
    let tagColor = 'blue'

    switch (entry.entity_type) {
      case 'contact':
        if (entry.operation === 'insert') {
          return <div>{renderTitleWithDate(entry, <Text strong>New contact</Text>)}</div>
        } else if (entry.operation === 'update') {
          return (
            <div>
              {renderTitleWithDate(entry, <Text strong>Contact updated</Text>)}
              <div className="mt-2 space-y-1">
                {Object.entries(entry.changes || {}).map(([key, value]) => {
                  // Handle different value types
                  let displayValue: React.ReactNode

                  if (value === null || value === undefined) {
                    displayValue = (
                      <Text type="secondary" italic>
                        null
                      </Text>
                    )
                  } else if (typeof value === 'object') {
                    // Check if it's an old/new value object
                    if (value.old !== undefined || value.new !== undefined) {
                      const oldVal = value.old
                      const newVal = value.new
                      return (
                        <div key={key} className="text-sm">
                          <Tag color="blue" bordered={false}>
                            {key}
                          </Tag>{' '}
                          changed from <Tag bordered={false}>{String(oldVal)}</Tag> to{' '}
                          <Tag color="green" bordered={false}>
                            {String(newVal)}
                          </Tag>
                        </div>
                      )
                    } else {
                      displayValue = (
                        <Tooltip title={JSON.stringify(value, null, 2)}>
                          <span>
                            <Tag className="cursor-help">JSON Object</Tag>
                          </span>
                        </Tooltip>
                      )
                    }
                  } else if (typeof value === 'boolean') {
                    displayValue = (
                      <Tag color={value ? 'green' : 'red'}>{value ? 'true' : 'false'}</Tag>
                    )
                  } else if (typeof value === 'number') {
                    displayValue = <Text strong>{value.toLocaleString()}</Text>
                  } else if (typeof value === 'string' && value.match(/^\d{4}-\d{2}-\d{2}T/)) {
                    // Likely a date string
                    displayValue = (
                      <Tooltip title={`${dayjs(value).format('LLLL')} in ${timezone}`}>
                        <span>
                          <Text>{dayjs(value).fromNow()}</Text>
                        </span>
                      </Tooltip>
                    )
                  } else {
                    displayValue = <Text>{String(value)}</Text>
                  }

                  return (
                    <div key={key} className="text-sm">
                      <Text type="secondary" className="font-mono">
                        {key}:
                      </Text>{' '}
                      {displayValue}
                    </div>
                  )
                })}
              </div>
            </div>
          )
        } else {
          // Delete or other operations
          return (
            <div>
              {renderTitleWithDate(
                entry,
                <>
                  <Text strong>{formatEntityType(entry.entity_type)}</Text>
                  <Tag color={getOperationColor(entry.operation)}>{entry.operation}</Tag>
                </>
              )}
            </div>
          )
        }

      case 'contact_list':
        return <div>{renderContactListMessage(entry)}</div>

      case 'contact_segment':
        const segmentId = entry.entity_id || 'Unknown Segment'

        // Look up segment from segments prop
        const segment = segments.find((s) => s.id === segmentId)

        const segmentDisplay = segment ? (
          <>
            <Tooltip title={'ID: ' + segmentId}>
              <span>
                <Tag bordered={false} color={segment.color}>
                  {segment.name}
                </Tag>
              </span>
            </Tooltip>
          </>
        ) : (
          <Tag bordered={false} color="blue">
            {segmentId}
          </Tag>
        )

        if (entry.kind === 'join_segment') {
          return (
            <div>
              {renderTitleWithDate(entry, <Text strong>Segment</Text>)}
              <div className="mb-2">
                <Text>Joined segment {segmentDisplay}</Text>
              </div>
            </div>
          )
        } else if (entry.kind === 'leave_segment') {
          return (
            <div>
              {renderTitleWithDate(entry, <Text strong>Segment</Text>)}
              <div className="mb-2">
                <Text>Left segment {segmentDisplay}</Text>
              </div>
            </div>
          )
        }
        return null

      case 'message_history':
        const messageData = entry.entity_data as MessageHistoryEntityData | undefined
        let tag = (
          <Tag bordered={false} color="blue">
            sent
          </Tag>
        )
        if (entry.changes.delivered_at) {
          tag = (
            <Tag bordered={false} color="green">
              delivered
            </Tag>
          )
        } else if (entry.changes.opened_at) {
          tag = (
            <Tag bordered={false} color="cyan">
              opened
            </Tag>
          )
        } else if (entry.changes.clicked_at) {
          tag = (
            <Tag bordered={false} color="geekblue">
              clicked
            </Tag>
          )
        }
        return (
          <div>
            {renderTitleWithDate(
              entry,
              <>
                <Text strong>Email</Text>
                {tag}
              </>
            )}
            {messageData && (
              <div className="mb-2 space-y-1">
                {messageData.template_id && (
                  <div className="flex items-center gap-2">
                    <Text type="secondary" className="text-xs">
                      Template:{' '}
                      {messageData.template_name ? (
                        <Tooltip title={`ID: ${messageData.template_id}`}>
                          <span>
                            <Text strong className="text-xs cursor-help">
                              {messageData.template_name}
                            </Text>
                          </span>
                        </Tooltip>
                      ) : (
                        <Text code className="text-xs">
                          {messageData.template_id}
                        </Text>
                      )}
                      {messageData.template_version && ` (v${messageData.template_version})`}
                    </Text>
                    {workspace && messageData.template_email && (
                      <Tooltip title="Preview email">
                        <span>
                          <TemplatePreviewDrawer
                            record={
                              {
                                id: messageData.template_id,
                                name: messageData.template_name || messageData.template_id,
                                version: messageData.template_version,
                                category: messageData.template_category || 'transactional',
                                channel: messageData.channel,
                                email: messageData.template_email,
                                test_data: messageData.message_data || {}
                              } as any
                            }
                            workspace={workspace}
                            templateData={messageData.message_data}
                          >
                            <Button
                              size="small"
                              type="text"
                              icon={<FontAwesomeIcon icon={faEye} />}
                              className="p-0 h-auto text-xs"
                            />
                          </TemplatePreviewDrawer>
                        </span>
                      </Tooltip>
                    )}
                  </div>
                )}
              </div>
            )}
          </div>
        )

      case 'webhook_event':
        const webhookEventData = entry.entity_data as WebhookEventEntityData
        const eventType = webhookEventData?.type
        const source = webhookEventData?.source || entry.changes?.source?.new
        const bounceType = webhookEventData?.bounce_type
        const bounceCategory = webhookEventData?.bounce_category
        const bounceDiagnostic = webhookEventData?.bounce_diagnostic
        const complaintType = webhookEventData?.complaint_feedback_type
        const webhookTemplateId = webhookEventData?.template_id
        const webhookTemplateVersion = webhookEventData?.template_version

        const isSupabase = source === 'supabase'

        // Set tag color based on event type
        if (eventType === 'bounce') {
          tagColor = 'volcano'
        } else if (eventType === 'complaint') {
          tagColor = 'magenta'
        } else if (eventType === 'delivered') {
          tagColor = 'green'
        } else if (eventType === 'auth_email') {
          tagColor = 'blue'
        } else if (eventType === 'before_user_created') {
          tagColor = 'cyan'
        }

        // Get event type label for display
        const getEventTypeLabel = (type: string) => {
          switch (type) {
            case 'delivered':
              return 'Delivered'
            case 'bounce':
              return 'Bounce'
            case 'complaint':
              return 'Complaint'
            case 'auth_email':
              return 'Auth Email'
            case 'before_user_created':
              return 'User Created'
            default:
              return type
          }
        }

        return (
          <div>
            {renderTitleWithDate(
              entry,
              <>
                {source && getProviderIcon(source, 'small')}
                {eventType && (
                  <Tag color={tagColor} bordered={false}>
                    {getEventTypeLabel(eventType)}
                  </Tag>
                )}
              </>
            )}
            <div className="mb-2 space-y-1">
              {isSupabase && (
                <div>
                  <Text type="secondary" className="text-xs">
                    {eventType === 'auth_email' && 'Authentication email sent via Supabase'}
                    {eventType === 'before_user_created' && 'User created and synced from Supabase'}
                  </Text>
                </div>
              )}
              {!isSupabase && webhookTemplateId && (
                <div>
                  <Text type="secondary" className="text-xs">
                    Template:{' '}
                    {webhookEventData?.template_name ? (
                      <Tooltip title={`ID: ${webhookTemplateId}`}>
                        <span>
                          <Text strong className="text-xs cursor-help">
                            {webhookEventData.template_name}
                          </Text>
                        </span>
                      </Tooltip>
                    ) : (
                      <Text code className="text-xs">
                        {webhookTemplateId}
                      </Text>
                    )}
                    {webhookTemplateVersion && ` (v${webhookTemplateVersion})`}
                  </Text>
                </div>
              )}
              {bounceType && (
                <div>
                  <Text type="secondary" className="text-xs">
                    Bounce Type: <Tag className="text-xs">{bounceType}</Tag>
                  </Text>
                </div>
              )}
              {bounceCategory && (
                <div>
                  <Text type="secondary" className="text-xs">
                    Category: <Tag className="text-xs">{bounceCategory}</Tag>
                  </Text>
                </div>
              )}
              {bounceDiagnostic && (
                <div>
                  <Text type="secondary" className="text-xs">
                    Diagnostic: <Text className="text-xs">{bounceDiagnostic}</Text>
                  </Text>
                </div>
              )}
              {complaintType && (
                <div>
                  <Text type="secondary" className="text-xs">
                    Feedback Type: <Tag className="text-xs">{complaintType}</Tag>
                  </Text>
                </div>
              )}
            </div>
          </div>
        )

      case 'custom_event':
        const customEventData = entry.entity_data as CustomEventEntityData

        return (
          <div>
            {renderTitleWithDate(
              entry,
              <>
                <Tooltip title={customEventData.event_name}>
                  <span>
                    <Tag color="purple">{formatEventName(customEventData.event_name)}</Tag>
                  </span>
                </Tooltip>
                {getSourceBadge(customEventData.source)}
                {entry.operation === 'update' && (
                  <Tag color="orange" bordered={false}>
                    updated
                  </Tag>
                )}
              </>
            )}

            <div className="space-y-1">
              {/* External ID */}
              <div className="mb-1">
                <Text type="secondary" className="text-xs">
                  ID:{' '}
                  <Text code className="text-xs">
                    {customEventData.external_id}
                  </Text>
                </Text>
              </div>

              {/* Occurred time (if different from created_at) */}
              {customEventData.occurred_at !== entry.created_at && (
                <div className="mb-1">
                  <Tooltip
                    title={`${dayjs(customEventData.occurred_at).format('LLLL')} in ${timezone}`}
                  >
                    <Text type="secondary" className="text-xs cursor-help">
                      Occurred: {dayjs(customEventData.occurred_at).fromNow()}
                    </Text>
                  </Tooltip>
                </div>
              )}

              {/* Properties */}
              {renderCustomEventProperties(customEventData.properties, timezone)}
            </div>
          </div>
        )

      default:
        return (
          <div>
            {renderTitleWithDate(
              entry,
              <>
                <Text strong>{formatEntityType(entry.entity_type)}</Text>
                <Tag color={getOperationColor(entry.operation)}>{entry.operation}</Tag>
              </>
            )}
            {entry.entity_id && (
              <div className="mb-2">
                <Text type="secondary" className="text-xs">
                  Entity ID:{' '}
                  <Text code className="text-xs">
                    {entry.entity_id}
                  </Text>
                </Text>
              </div>
            )}
          </div>
        )
    }
  }

  if (loading && entries.length === 0) {
    return (
      <div className="flex justify-center items-center py-8">
        <Spin size="large" />
      </div>
    )
  }

  if (!loading && entries.length === 0) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="No timeline events found for this contact"
      />
    )
  }

  return (
    <div>
      <Timeline
        className="contact-timeline"
        items={entries.map((entry) => ({
          //   color: getOperationColor(entry.operation),
          dot: (
            <Popover
              content={
                <pre className="text-xs max-w-lg max-h-96 overflow-auto bg-gray-50 p-2 rounded">
                  {JSON.stringify(entry, null, 2)}
                </pre>
              }
              title="Raw Entry Data"
              trigger="hover"
              placement="right"
            >
              <div className="cursor-pointer">
                <FontAwesomeIcon icon={getEntityIcon(entry)} />
              </div>
            </Popover>
          ),
          children: renderEntityDetails(entry)
        }))}
      />

      {hasMore && onLoadMore && (
        <div className="text-center mt-4">
          <Button onClick={onLoadMore} loading={isLoadingMore} type="dashed" block>
            {isLoadingMore ? 'Loading...' : 'Load More Events'}
          </Button>
        </div>
      )}
    </div>
  )
}
