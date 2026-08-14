import React from 'react'
import { useLingui } from '@lingui/react/macro'
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
  faBolt,
  faPlay,
  faFlagCheckered,
  faReply,
  faRobot,
  faGlobe
} from '@fortawesome/free-solid-svg-icons'
import { faUser, faFolderOpen, faPaperPlane, faEye } from '@fortawesome/free-regular-svg-icons'
import {
  ContactTimelineEntry,
  ContactListEntityData,
  MessageHistoryEntityData,
  InboundWebhookEventEntityData,
  CustomEventEntityData,
  AutomationEventEntityData
} from '../../services/api/contact_timeline'
import type { Workspace } from '../../services/api/types'
import type { Segment } from '../../services/api/segment'
import dayjs from '../../lib/dayjs'
import TemplatePreviewDrawer from '../templates/TemplatePreviewDrawer'
import { getProviderIcon } from '../integrations/EmailProviders'
import { formatValue, formatEventName, getSourceBadge } from '../../utils/formatters'
import { WebPageDrawer } from './WebPageDrawer'

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
  const { t } = useLingui()

  // The page a "View website" button opened, null when the drawer is closed.
  const [previewURL, setPreviewURL] = React.useState<string | null>(null)

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

  // Get standardized color for action tags across all event types
  const getActionTagColor = (action: string): string => {
    const colorMap: Record<string, string> = {
      // Positive actions
      created: 'green',
      subscribed: 'green',
      joined: 'green',
      delivered: 'green',
      // Neutral/info actions
      updated: 'blue',
      sent: 'blue',
      'status changed': 'blue',
      'auth email': 'blue',
      // Warning actions
      left: 'orange',
      pending: 'orange',
      // Negative actions
      deleted: 'red',
      removed: 'red',
      bounce: 'volcano',
      complaint: 'magenta',
      // Engagement actions
      opened: 'cyan',
      clicked: 'geekblue',
      'user created': 'cyan',
      // Automation actions
      started: 'blue',
      ended: 'green',
      completed: 'green',
      exited: 'orange',
      failed: 'red'
    }
    return colorMap[action.toLowerCase()] || 'default'
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
        if (entry.kind === 'segment.joined') {
          return faArrowRightToBracket
        } else if (entry.kind === 'segment.left') {
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
      case 'inbound_webhook_event': {
        const webhookData = entry.entity_data as InboundWebhookEventEntityData | undefined
        const eventType = webhookData?.type
        if (eventType === 'bounce') {
          return faCircleExclamation
        } else if (eventType === 'complaint') {
          return faTriangleExclamation
        } else if (eventType === 'delivered') {
          return faCheck
        } else if (eventType === 'reply') {
          return faReply
        } else if (eventType === 'auto_reply') {
          return faRobot
        }
        return faBolt
      }
      case 'custom_event':
        return faBolt
      case 'web_session':
        return faGlobe
      case 'web_page':
        // The same eye the email preview and the email-open row use: one icon
        // for "someone looked at this", rather than a second variant of it.
        return faEye
      case 'automation':
        if (entry.kind === 'automation.start') return faPlay
        if (entry.kind === 'automation.end') return faFlagCheckered
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

  // Render unified event header with category, action tag, and timestamp
  // The web navigation rows are written by the analytics projection, not by a
  // database trigger, but they carry the same {field: {new: value}} envelope —
  // segment conditions on contact_timeline read changes->'<key>'->>'new', so any
  // other shape would be unreachable from a segment.
  const webChange = <T,>(entry: ContactTimelineEntry, key: string): T | undefined => {
    const change = entry.changes?.[key]
    if (typeof change === 'object' && change !== null && 'new' in change) {
      return (change as { new: T }).new
    }
    return undefined
  }

  // Rebuilds the address a pageview refers to. web_pages stores paths only, so
  // the host comes from the visit's landing domain, which the projection carries
  // alongside the path.
  //
  // The workspace's own website URL is the fallback, for rows projected before
  // the domain was recorded. It is only a fallback: a workspace can track a site
  // that is not the one configured in its settings, and guessing wrong there
  // sends someone to a page that was never visited.
  const webPageURL = (entry: ContactTimelineEntry): string | null => {
    const path = webChange<string>(entry, 'path')
    if (!path) return null

    const domain = webChange<string>(entry, 'landing_domain')
    const origin = domain ? `https://${domain}` : workspace?.settings?.website_url
    if (!origin) return null

    try {
      return new URL(path, origin).toString()
    } catch {
      return null
    }
  }

  // Engaged time, so a value under a second is real rather than a rounding
  // artefact and must not display as "0s" — Math.round takes everything below
  // 500ms to zero, and a bounce is exactly the visit that lands there.
  const formatWebDuration = (durationMs: number) => {
    const totalSeconds = Math.max(1, Math.round(durationMs / 1000))
    if (totalSeconds < 60) return t`${totalSeconds}s`
    const minutes = Math.floor(totalSeconds / 60)
    const seconds = totalSeconds % 60
    if (minutes < 60) {
      return seconds === 0 ? t`${minutes}m` : t`${minutes}m ${seconds}s`
    }
    // A long visit reads as "2h 7m", not "127m" — matching the web analytics
    // dashboard's own formatter.
    const hours = Math.floor(minutes / 60)
    const remainingMinutes = minutes % 60
    return remainingMinutes === 0 ? t`${hours}h` : t`${hours}h ${remainingMinutes}m`
  }

  const renderEventHeader = (
    entry: ContactTimelineEntry,
    category: string | null,
    actionLabel: string,
    actionColor?: string,
    prefixContent?: React.ReactNode,
    // Sits between the action tag and the timestamp, for facts that belong to
    // the headline rather than to the detail below it — the size of a visit,
    // say, which is the first thing anyone scanning the timeline wants.
    suffixContent?: React.ReactNode
  ) => {
    const color = actionColor || getActionTagColor(actionLabel)
    return (
      <div className="flex items-center gap-2 mb-2 flex-wrap">
        {prefixContent}
        {category && <Text strong>{category}</Text>}
        {/* Empty when the category already says everything: a web session that
            "visited" adds a word, not a fact. */}
        {actionLabel && (
          <Tag variant="filled" color={color}>
            {actionLabel}
          </Tag>
        )}
        {suffixContent}
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
    const listId = entry.entity_id || t`Unknown List`

    // Extract old and new values if they exist
    const oldStatus =
      typeof statusChange === 'object' && statusChange !== null && 'old' in statusChange ? String(statusChange.old) : null
    const newStatus =
      typeof statusChange === 'object' && statusChange !== null && 'new' in statusChange ? String(statusChange.new) : String(statusChange)

    // Use entity_data if available to get list name
    const entityData = entry.entity_data as ContactListEntityData | undefined
    const listName = entityData?.name
    const listDisplay = listName ? (
      <Tooltip title={t`ID: ${listId}`}>
        <span>
          <Text strong>{listName}</Text>
        </span>
      </Tooltip>
    ) : (
      <Text code>{listId}</Text>
    )

    // Map operations to action labels
    const subscriptionActionMap: Record<string, string> = {
      insert: t`subscribed`,
      update: t`status changed`,
      delete: t`removed`
    }
    const actionLabel = subscriptionActionMap[entry.operation] || entry.operation

    if (entry.operation === 'insert') {
      return (
        <div>
          {renderEventHeader(entry, t`Subscription`, actionLabel)}
          <div className="text-sm">
            <Text type="secondary">{t`List:`}</Text> {listDisplay}{' '}
            <Text type="secondary">{t`with status`}</Text>{' '}
            <Tag variant="filled" color={getStatusColor(newStatus)}>
              {newStatus}
            </Tag>
          </div>
        </div>
      )
    } else if (entry.operation === 'update') {
      return (
        <div>
          {renderEventHeader(entry, t`Subscription`, actionLabel)}
          <div className="text-sm">
            <Text type="secondary">{t`List:`}</Text> {listDisplay}
            {oldStatus ? (
              <>
                {' — '}
                <Text type="secondary">{oldStatus}</Text>
                <Text type="secondary"> → </Text>
                <Tag variant="filled" color={getStatusColor(newStatus)}>
                  {newStatus}
                </Tag>
              </>
            ) : (
              <>
                {' → '}
                <Tag variant="filled" color={getStatusColor(newStatus)}>
                  {newStatus}
                </Tag>
              </>
            )}
          </div>
        </div>
      )
    } else if (entry.operation === 'delete') {
      return (
        <div>
          {renderEventHeader(entry, t`Subscription`, actionLabel)}
          <div className="text-sm">
            <Text type="secondary">{t`List:`}</Text> {listDisplay}
          </div>
        </div>
      )
    }

    return null
  }

  // Render custom event properties with tiered display approach
  const renderCustomEventProperties = (
    properties: Record<string, unknown> | undefined,
    timezone: string
  ): React.ReactNode => {
    if (!properties || Object.keys(properties).length === 0) {
      return (
        <Text type="secondary" className="text-xs">
          {t`No properties`}
        </Text>
      )
    }

    const entries = Object.entries(properties)
    const propertyCount = entries.length

    // Check if all values are primitives (not objects or arrays)
    const allPrimitives = entries.every(
      ([, value]) => typeof value !== 'object' || value === null
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
              label: t`${propertyCount} properties`,
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
          title={t`Raw JSON`}
          placement="rightTop"
          trigger="click"
          styles={{ root: { maxWidth: '600px' } }}
        >
          <Button size="small" type="text">
            {t`View Raw JSON`}
          </Button>
        </Popover>
      </div>
    )
  }

  // Render entity-specific details based on entity type
  const renderEntityDetails = (entry: ContactTimelineEntry) => {
    switch (entry.entity_type) {
      case 'contact': {
        // Map operations to action labels
        const contactActionMap: Record<string, string> = {
          insert: t`created`,
          update: t`updated`,
          delete: t`deleted`
        }
        const contactAction = contactActionMap[entry.operation] || entry.operation

        if (entry.operation === 'update') {
          return (
            <div>
              {renderEventHeader(entry, t`Contact`, contactAction)}
              <div className="space-y-1">
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
                    if (value !== null && ('old' in value || 'new' in value)) {
                      const oldVal = 'old' in value ? value.old : undefined
                      const newVal = 'new' in value ? value.new : undefined
                      return (
                        <div key={key} className="text-sm">
                          <Text type="secondary" className="font-mono text-xs">
                            {key}:
                          </Text>{' '}
                          <Text type="secondary">{String(oldVal)}</Text>
                          <Text type="secondary"> → </Text>
                          <Text>{String(newVal)}</Text>
                        </div>
                      )
                    } else {
                      displayValue = (
                        <Tooltip title={JSON.stringify(value, null, 2)}>
                          <span>
                            <Tag className="cursor-help">{t`JSON Object`}</Tag>
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
                      <Text type="secondary" className="font-mono text-xs">
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
          // Insert or delete - just header, no details needed
          return <div>{renderEventHeader(entry, t`Contact`, contactAction)}</div>
        }
      }

      case 'contact_list':
        return <div>{renderContactListMessage(entry)}</div>

      case 'contact_segment': {
        const segmentId = entry.entity_id || t`Unknown Segment`

        // Look up segment from segments prop
        const segment = segments.find((s) => s.id === segmentId)

        const segmentDisplay = segment ? (
          <Tooltip title={t`ID: ${segmentId}`}>
            <span>
              <Tag variant="filled" color={segment.color}>
                {segment.name}
              </Tag>
            </span>
          </Tooltip>
        ) : (
          <Tag variant="filled" color="blue">
            {segmentId}
          </Tag>
        )

        // Map kind to action labels
        const segmentActionLabel = entry.kind === 'segment.joined' ? t`joined` : t`left`

        return (
          <div>
            {renderEventHeader(entry, t`Segment`, segmentActionLabel)}
            <div className="text-sm">
              {segmentDisplay}
            </div>
          </div>
        )
      }

      case 'message_history': {
        const messageData = entry.entity_data as MessageHistoryEntityData | undefined

        // Determine email action based on changes
        let emailAction = t`sent`
        if (entry.changes.delivered_at) {
          emailAction = t`delivered`
        } else if (entry.changes.opened_at) {
          emailAction = t`opened`
        } else if (entry.changes.clicked_at) {
          emailAction = t`clicked`
        }

        return (
          <div>
            {renderEventHeader(entry, t`Email`, emailAction)}
            {messageData && messageData.template_id && (
              <div className="text-sm flex items-center gap-2">
                <Text type="secondary">{t`Template:`}</Text>{' '}
                {messageData.template_name ? (
                  <Tooltip title={t`ID: ${messageData.template_id}`}>
                    <span>
                      <Text strong className="cursor-help">
                        {messageData.template_name}
                      </Text>
                    </span>
                  </Tooltip>
                ) : (
                  <Text code>{messageData.template_id}</Text>
                )}
                {messageData.template_version && (
                  <Text type="secondary">(v{messageData.template_version})</Text>
                )}
                {workspace && messageData.template_email && (
                  <Tooltip title={t`Preview email`}>
                    <span>
                      <TemplatePreviewDrawer
                        record={{
                          id: messageData.template_id,
                          name: messageData.template_name || messageData.template_id,
                          version: messageData.template_version,
                          category: messageData.template_category || 'transactional',
                          channel: messageData.channel as 'email' | 'web',
                          email: messageData.template_email as unknown as Parameters<typeof TemplatePreviewDrawer>[0]['record']['email'],
                          test_data: messageData.message_data || {},
                          created_at: '',
                          updated_at: ''
                        }}
                        workspace={workspace}
                        templateData={messageData.message_data}
                      >
                        <Button
                          size="small"
                          type="text"
                          icon={<FontAwesomeIcon icon={faEye} />}
                          className="p-0 h-auto"
                        />
                      </TemplatePreviewDrawer>
                    </span>
                  </Tooltip>
                )}
              </div>
            )}
          </div>
        )
      }

      case 'inbound_webhook_event': {
        const webhookEventData = entry.entity_data as InboundWebhookEventEntityData
        const eventType = webhookEventData?.type
        const sourceChange = entry.changes?.source
        const source = webhookEventData?.source || (typeof sourceChange === 'object' && sourceChange !== null && 'new' in sourceChange ? sourceChange.new as string : undefined)
        const bounceType = webhookEventData?.bounce_type
        const bounceCategory = webhookEventData?.bounce_category
        const bounceDiagnostic = webhookEventData?.bounce_diagnostic
        const complaintType = webhookEventData?.complaint_feedback_type
        const webhookTemplateId = webhookEventData?.template_id
        const webhookTemplateVersion = webhookEventData?.template_version

        const isSupabase = source === 'supabase'

        // Map event types to labels and colors
        const webhookEventLabels: Record<string, string> = {
          delivered: t`Delivered`,
          bounce: t`Bounce`,
          complaint: t`Complaint`,
          reply: t`Replied`,
          auto_reply: t`Auto-reply`,
          auth_email: t`Auth Email`,
          before_user_created: t`User Created`
        }
        const webhookActionLabel = webhookEventLabels[eventType || ''] || eventType || t`Event`

        // Provider icon as prefix content
        const providerPrefix = source ? getProviderIcon(source, 'small') : undefined

        return (
          <div>
            {renderEventHeader(entry, t`Webhook`, webhookActionLabel, undefined, providerPrefix)}
            <div className="space-y-1">
              {isSupabase && (
                <div className="text-sm">
                  <Text type="secondary">
                    {eventType === 'auth_email' && t`Authentication email sent via Supabase`}
                    {eventType === 'before_user_created' && t`User created and synced from Supabase`}
                  </Text>
                </div>
              )}
              {!isSupabase && webhookTemplateId && (
                <div className="text-sm">
                  <Text type="secondary">{t`Template:`}</Text>{' '}
                  {webhookEventData?.template_name ? (
                    <Tooltip title={t`ID: ${webhookTemplateId}`}>
                      <span>
                        <Text strong className="cursor-help">
                          {webhookEventData.template_name}
                        </Text>
                      </span>
                    </Tooltip>
                  ) : (
                    <Text code>{webhookTemplateId}</Text>
                  )}
                  {webhookTemplateVersion && (
                    <Text type="secondary"> (v{webhookTemplateVersion})</Text>
                  )}
                </div>
              )}
              {bounceType && (
                <div className="text-sm">
                  <Text type="secondary">{t`Type:`}</Text> {bounceType}
                  {bounceCategory && (
                    <>
                      {' | '}
                      <Text type="secondary">{t`Category:`}</Text> {bounceCategory}
                    </>
                  )}
                </div>
              )}
              {bounceDiagnostic && (
                <div className="text-sm">
                  <Text type="secondary">{t`Diagnostic:`}</Text> {bounceDiagnostic}
                </div>
              )}
              {complaintType && (
                <div className="text-sm">
                  <Text type="secondary">{t`Feedback:`}</Text> {complaintType}
                </div>
              )}
            </div>
          </div>
        )
      }

      case 'custom_event': {
        const customEventData = entry.entity_data as CustomEventEntityData | undefined
        // Falls back to entry.kind when entity_data is absent, which is every time: the
        // timeline query joins message and webhook entities but never custom events.
        // The kind the trigger writes is 'custom_event.<name>', and formatting it whole
        // rendered a cart conversion as "Custom Event Add To Cart".
        const eventName =
          customEventData?.event_name || entry.kind.replace(/^custom_event\./, '')
        const externalId = customEventData?.external_id || entry.entity_id
        // Get goal fields from entity_data or from changes (for timeline entries)
        const goalTypeChange = entry.changes?.goal_type
        const goalValueChange = entry.changes?.goal_value
        const goalNameChange = entry.changes?.goal_name
        const goalType = customEventData?.goal_type || (typeof goalTypeChange === 'object' && goalTypeChange !== null && 'new' in goalTypeChange ? goalTypeChange.new as string : undefined)
        const goalValue = customEventData?.goal_value ?? (typeof goalValueChange === 'object' && goalValueChange !== null && 'new' in goalValueChange ? goalValueChange.new as number : undefined)
        const goalName = customEventData?.goal_name || (typeof goalNameChange === 'object' && goalNameChange !== null && 'new' in goalNameChange ? goalNameChange.new as string : undefined)

        // Custom events use the event name as the primary identifier (no category label)
        const formattedEventName = formatEventName(eventName)

        return (
          <div>
            {/* Custom header: event name tag serves as primary identifier */}
            <div className="flex items-center gap-2 mb-2 flex-wrap">
              <Tooltip title={eventName}>
                <span>
                  <Tag color="purple" variant="filled">{formattedEventName}</Tag>
                </span>
              </Tooltip>
              {goalValue !== undefined && goalValue !== null && (
                <Tag color="cyan" variant="filled">
                  {goalType === 'purchase' || goalType === 'subscription' ? '$' : ''}
                  {goalValue.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                </Tag>
              )}
              {customEventData?.source && getSourceBadge(customEventData.source)}
              {entry.operation === 'update' && (
                <Tag color="orange" variant="filled">
                  {t`updated`}
                </Tag>
              )}
              <Tooltip title={`${dayjs(entry.created_at).format('LLLL')} in ${timezone}`}>
                <span>
                  <Text type="secondary" className="text-xs cursor-help">
                    {dayjs(entry.created_at).fromNow()}
                  </Text>
                </span>
              </Tooltip>
            </div>

            <div className="space-y-1">
              {/* Goal name */}
              {goalName && (
                <div className="text-sm">
                  <Text type="secondary">{t`Goal:`}</Text> {goalName}
                </div>
              )}

              {/* External ID */}
              {externalId && (
                <div className="text-sm">
                  <Text type="secondary">{t`ID:`}</Text>{' '}
                  <span className="font-mono">{externalId}</span>
                </div>
              )}

              {/* Occurred time (if different from created_at) */}
              {customEventData?.occurred_at && customEventData.occurred_at !== entry.created_at && (
                <div className="text-sm">
                  <Tooltip
                    title={`${dayjs(customEventData.occurred_at).format('LLLL')} in ${timezone}`}
                  >
                    <span>
                      <Text type="secondary" className="cursor-help">
                        {t`Occurred:`} {dayjs(customEventData.occurred_at).fromNow()}
                      </Text>
                    </span>
                  </Tooltip>
                </div>
              )}

              {/* Properties */}
              {customEventData?.properties && renderCustomEventProperties(customEventData.properties, timezone)}
            </div>
          </div>
        )
      }

      case 'web_session': {
        const pageviews = webChange<number>(entry, 'pageview_count')
        const durationMs = webChange<number>(entry, 'duration_ms')
        const landingPath = webChange<string>(entry, 'landing_path')
        const exitPath = webChange<string>(entry, 'exit_path')
        const referrerDomain = webChange<string>(entry, 'referrer_domain')
        // The UTM chain, in the order a campaign is read: which source, through
        // which medium, for which campaign, with which creative. Each part is
        // labelled on hover, because "instagram / social / holiday-sale /
        // video-hero-v2" is unreadable without knowing which slot is which.
        const utmChain: Array<{ label: string; value: string }> = [
          { label: t`UTM source`, value: webChange<string>(entry, 'utm_source') || '' },
          { label: t`UTM medium`, value: webChange<string>(entry, 'utm_medium') || '' },
          { label: t`UTM campaign`, value: webChange<string>(entry, 'utm_campaign') || '' },
          { label: t`UTM content`, value: webChange<string>(entry, 'utm_content') || '' }
        ].filter((part) => part.value !== '')
        const device = webChange<string>(entry, 'device')
        const country = webChange<string>(entry, 'country')
        const goalCount = webChange<number>(entry, 'goal_count')

        // The shape of the visit, on the headline: it is what anyone scanning
        // the timeline reads first, and it is a summary rather than a detail.
        const sessionTags = (
          <>
            {pageviews !== undefined && (
              <Tag variant="filled" color="blue">
                {pageviews === 1 ? t`1 page` : t`${pageviews} pages`}
              </Tag>
            )}
            {durationMs !== undefined && durationMs > 0 && (
              <Tag variant="filled" color="cyan">{formatWebDuration(durationMs)}</Tag>
            )}
            {goalCount !== undefined && goalCount > 0 && (
              <Tag variant="filled" color="purple">
                {goalCount === 1 ? t`1 goal` : t`${goalCount} goals`}
              </Tag>
            )}
          </>
        )

        return (
          <div>
            {renderEventHeader(entry, t`Web session`, '', 'geekblue', undefined, sessionTags)}
            <div className="space-y-1 text-sm">
              {/* One property per line. Run together, an entry and an exit page
                  read as a single path with a stray word in it, and paths are
                  long enough to wrap anyway.

                  Independent guards: landing_path is TEXT NOT NULL DEFAULT '',
                  so a visit that arrived without one used to lose its exit page
                  as well. */}
              {landingPath && (
                <div className="break-all">
                  <Text type="secondary">{t`Entry:`}</Text> {landingPath}
                </div>
              )}
              {exitPath && exitPath !== landingPath && (
                <div className="break-all">
                  <Text type="secondary">{t`Exit:`}</Text> {exitPath}
                </div>
              )}
              {referrerDomain && (
                <div className="break-all">
                  <Text type="secondary">{t`Referrer:`}</Text> {referrerDomain}
                </div>
              )}
              {utmChain.length > 0 && (
                <div className="break-all">
                  <Text type="secondary">{t`Source:`}</Text>{' '}
                  {utmChain.map((part, index) => (
                    <React.Fragment key={part.label}>
                      {index > 0 && <Text type="secondary"> / </Text>}
                      <Tooltip title={part.label}>
                        <span className="cursor-help">{part.value}</span>
                      </Tooltip>
                    </React.Fragment>
                  ))}
                </div>
              )}
              {(device || country) && (
                <div className="text-xs">
                  <Text type="secondary">{[device, country].filter(Boolean).join(' \u00b7 ')}</Text>
                </div>
              )}
            </div>
          </div>
        )
      }

      case 'web_page': {
        const path = webChange<string>(entry, 'path')
        const durationMs = webChange<number>(entry, 'duration_ms')
        const maxScroll = webChange<number>(entry, 'max_scroll')
        const isLanding = webChange<boolean>(entry, 'is_landing')
        const isExit = webChange<boolean>(entry, 'is_exit')
        const url = webPageURL(entry)

        return (
          <div>
            {renderEventHeader(entry, t`Page view`, '', 'blue')}
            <div className="space-y-1 text-sm">
              <div className="flex items-center gap-1">
                <span className="break-all">{path || t`Unknown page`}</span>
                {url && (
                  <Tooltip title={t`View website`}>
                    <Button
                      size="small"
                      type="text"
                      className="p-0 h-auto"
                      aria-label={t`View website`}
                      icon={<FontAwesomeIcon icon={faEye} />}
                      onClick={() => setPreviewURL(url)}
                    />
                  </Tooltip>
                )}
              </div>
              <div className="flex items-center gap-2 flex-wrap">
                {durationMs !== undefined && durationMs > 0 && (
                  <Tag variant="filled" color="cyan">{formatWebDuration(durationMs)}</Tag>
                )}
                {maxScroll !== undefined && maxScroll > 0 && (
                  <Tag variant="filled" color="geekblue">{t`${maxScroll}% scrolled`}</Tag>
                )}
                {isLanding && <Tag variant="filled" color="green">{t`entry page`}</Tag>}
                {isExit && <Tag variant="filled" color="orange">{t`exit page`}</Tag>}
              </div>
            </div>
          </div>
        )
      }

      case 'automation': {
        const automationData = entry.entity_data as AutomationEventEntityData | undefined
        const isStart = entry.kind === 'automation.start'
        const exitReasonChange = entry.changes?.exit_reason
        const exitReason = typeof exitReasonChange === 'object' && exitReasonChange !== null && 'new' in exitReasonChange
          ? exitReasonChange.new as string
          : undefined

        // Determine action label based on event kind and exit reason
        let actionLabel = isStart ? t`started` : t`ended`
        if (!isStart && exitReason) {
          if (exitReason === 'failed') actionLabel = t`failed`
          else if (exitReason.includes('exited') || exitReason.includes('deleted')) actionLabel = t`exited`
          else if (exitReason === 'completed') actionLabel = t`completed`
        }

        return (
          <div>
            {renderEventHeader(entry, t`Automation`, actionLabel)}
            <div className="text-sm">
              <Text type="secondary">{t`Automation:`}</Text>{' '}
              {automationData?.name ? (
                <Tooltip title={t`ID: ${entry.entity_id}`}>
                  <span>
                    <Text strong className="cursor-help">{automationData.name}</Text>
                  </span>
                </Tooltip>
              ) : (
                <Text code>{entry.entity_id || t`Unknown`}</Text>
              )}
              {!isStart && exitReason && exitReason !== 'completed' && (
                <Tag className="ml-2" variant="filled">{exitReason}</Tag>
              )}
            </div>
          </div>
        )
      }

      default:
        return (
          <div>
            {renderEventHeader(entry, formatEntityType(entry.entity_type), entry.operation)}
            {entry.entity_id && (
              <div className="text-sm">
                <Text type="secondary">{t`Entity ID:`}</Text>{' '}
                <Text code>{entry.entity_id}</Text>
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
        description={t`No timeline events found for this contact`}
      />
    )
  }

  return (
    <div>
      <Timeline
        className="contact-timeline"
        items={entries.map((entry) => ({
          icon: (
            <Popover
              content={
                <pre className="text-xs max-w-lg max-h-96 overflow-auto bg-gray-50 p-2 rounded">
                  {JSON.stringify(entry, null, 2)}
                </pre>
              }
              title={t`Raw Entry Data`}
              trigger="hover"
              placement="right"
            >
              <div className="cursor-pointer">
                <FontAwesomeIcon icon={getEntityIcon(entry)} />
              </div>
            </Popover>
          ),
          content: renderEntityDetails(entry)
        }))}
      />

      {hasMore && onLoadMore && (
        <div className="text-center mt-4">
          <Button onClick={onLoadMore} loading={isLoadingMore} type="dashed" block>
            {isLoadingMore ? t`Loading...` : t`Load More Events`}
          </Button>
        </div>
      )}

      <WebPageDrawer
        open={previewURL !== null}
        url={previewURL}
        onClose={() => setPreviewURL(null)}
      />
    </div>
  )
}
