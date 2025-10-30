import React from 'react'
import { Table, Tag, Tooltip, Button, Spin, Empty, Space } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faPaperPlane,
  faCircleCheck,
  faCircleXmark,
  faArrowPointer,
  faBan,
  faTriangleExclamation,
  faRefresh
} from '@fortawesome/free-solid-svg-icons'
import { faEye, faFaceFrown } from '@fortawesome/free-regular-svg-icons'
import dayjs from '../../lib/dayjs'
import { MessageHistory } from '../../services/api/messages_history'
import TemplatePreviewDrawer from '../templates/TemplatePreviewDrawer'
import { templatesApi } from '../../services/api/template'
import { Workspace } from '../../services/api/types'
import { useQuery } from '@tanstack/react-query'
import type { Broadcast } from '../../services/api/broadcast'
import type { List } from '../../services/api/list'
import { MessageColumnsSelector } from './MessageColumnsSelector'

const STORAGE_KEY = 'message_columns_visibility'

// Template preview button component that handles its own loading state
interface TemplatePreviewButtonProps {
  templateId: string
  templateVersion?: number
  workspace: Workspace
  templateData: Record<string, any>
  messageHistory: MessageHistory
}

const TemplatePreviewButton: React.FC<TemplatePreviewButtonProps> = ({
  templateId,
  templateVersion,
  workspace,
  templateData,
  messageHistory
}) => {
  // Use React Query to fetch the template data
  const { data, isLoading } = useQuery({
    queryKey: ['template', workspace.id, templateId, templateVersion],
    queryFn: async () => {
      const response = await templatesApi.get({
        workspace_id: workspace.id,
        id: templateId,
        version: templateVersion
      })

      if (!response.template) {
        throw new Error('Failed to load template')
      }

      return response.template
    },
    enabled: !!workspace.id && !!templateId,
    staleTime: 60 * 60 * 1000, // 1 hour
    retry: 1
  })

  if (!data || isLoading) {
    return null
  }

  return (
    <TemplatePreviewDrawer
      record={data}
      workspace={workspace}
      templateData={templateData}
      messageHistory={messageHistory}
    >
      <Tooltip title="Preview message">
        <Button type="text" className="opacity-70" icon={<FontAwesomeIcon icon={faEye} />} />
      </Tooltip>
    </TemplatePreviewDrawer>
  )
}

interface MessageHistoryTableProps {
  messages?: MessageHistory[]
  loading: boolean
  isLoadingMore: boolean
  nextCursor?: string
  onLoadMore: () => void
  onRefresh?: () => void
  show_email?: boolean
  bordered?: boolean
  size?: 'small' | 'middle' | 'large'
  workspace: Workspace
  broadcastMap?: Map<string, Broadcast>
  listMap?: Map<string, List>
  visibleColumns?: Record<string, boolean>
  onColumnVisibilityChange?: (key: string, visible: boolean) => void
}

export function MessageHistoryTable({
  messages = [],
  loading,
  isLoadingMore,
  nextCursor,
  onLoadMore,
  onRefresh,
  show_email = true,
  bordered = false,
  size = 'small',
  workspace,
  broadcastMap = new Map(),
  listMap = new Map(),
  visibleColumns = {},
  onColumnVisibilityChange
}: MessageHistoryTableProps) {
  // Format date using dayjs
  const formatDate = (dateString: string | undefined): string => {
    if (!dateString) return '-'
    return `${dayjs(dateString).format('lll')} in ${workspace.settings.timezone}`
  }

  // All available columns with their metadata for the selector
  const allColumns = [
    { key: 'id', title: 'Message ID' },
    { key: 'external_id', title: 'External ID' },
    { key: 'contact_email', title: 'Contact Email' },
    { key: 'template_id', title: 'Template' },
    { key: 'broadcast_id', title: 'Broadcast' },
    { key: 'list_ids', title: 'List IDs' },
    { key: 'events', title: 'Events' },
    { key: 'error', title: 'Error' },
    { key: 'created_at', title: 'Created At' }
  ]

  // Define base columns
  const baseColumns = [
    {
      title: 'Message ID',
      dataIndex: 'id',
      key: 'id',
      hidden: visibleColumns.id === false,
      render: (id: string) => {
        return (
          <Tooltip title={id}>
            <span className="text-xs text-gray-500">{id.substring(0, 8) + '...'}</span>
          </Tooltip>
        )
      }
    },
    {
      title: 'External ID',
      dataIndex: 'external_id',
      key: 'external_id',
      hidden: visibleColumns.external_id === false,
      render: (externalId: string | undefined) => {
        if (!externalId) {
          return <span className="text-xs text-gray-400">-</span>
        }
        return (
          <Tooltip title={externalId}>
            <span className="text-xs text-gray-500">
              {externalId.length > 12 ? externalId.substring(0, 12) + '...' : externalId}
            </span>
          </Tooltip>
        )
      }
    },
    {
      title: 'Template',
      key: 'template_id',
      hidden: visibleColumns.template_id === false,
      render: (record: MessageHistory) => {
        return (
          <>
            <span className="text-xs">{record.template_id}</span>
            <span className="text-xs text-gray-500 pl-2">v{record.template_version}</span>
          </>
        )
      }
    },
    {
      title: 'Broadcast',
      dataIndex: 'broadcast_id',
      key: 'broadcast_id',
      hidden: visibleColumns.broadcast_id === false,
      render: (broadcastId: string | undefined) => {
        if (!broadcastId) {
          return <span className="text-xs text-gray-400">-</span>
        }

        const broadcast = broadcastMap.get(broadcastId)
        if (!broadcast) {
          return (
            <Tooltip title={broadcastId}>
              <span className="text-xs text-gray-500">{broadcastId.substring(0, 8)}...</span>
            </Tooltip>
          )
        }

        // Get list names from the broadcast audience
        const listNames =
          broadcast.audience.lists
            ?.map((listId) => listMap.get(listId)?.name || listId)
            .join(', ') || ''

        const tooltipContent = (
          <div>
            <div>
              <strong>ID:</strong> {broadcastId}
            </div>
            {listNames && (
              <div>
                <strong>Lists:</strong> {listNames}
              </div>
            )}
          </div>
        )

        return (
          <Tooltip title={tooltipContent}>
            <span className="text-xs cursor-help">{broadcast.name}</span>
          </Tooltip>
        )
      }
    },
    {
      title: 'List IDs',
      key: 'list_ids',
      hidden: visibleColumns.list_ids === false,
      render: (record: MessageHistory) => {
        if (!record.list_ids || record.list_ids.length === 0) {
          return <span className="text-xs text-gray-400">-</span>
        }

        // Get list names from listMap
        const listTags = record.list_ids.map((listId) => {
          const list = listMap.get(listId)
          const listName = list?.name || listId

          return (
            <Tag key={listId} bordered={false} color="blue" className="text-xs">
              {listName}
            </Tag>
          )
        })

        return <div className="flex flex-wrap gap-1">{listTags}</div>
      }
    },
    {
      title: 'Events',
      key: 'events',
      hidden: visibleColumns.events === false,
      render: (record: MessageHistory) => {
        const events = []
        if (record.sent_at)
          events.push(
            <Tooltip key="sent" title={formatDate(record.sent_at)}>
              <Tag bordered={false} color="blue">
                <FontAwesomeIcon icon={faPaperPlane} className="opacity-70" /> Sent
              </Tag>
            </Tooltip>
          )
        if (record.delivered_at)
          events.push(
            <Tooltip key="delivered" title={formatDate(record.delivered_at)}>
              <Tag bordered={false} color="green">
                <FontAwesomeIcon icon={faCircleCheck} className="opacity-70" /> Delivered
              </Tag>
            </Tooltip>
          )
        if (record.failed_at)
          events.push(
            <Tooltip key="failed" title={formatDate(record.failed_at)}>
              <Tag bordered={false} color="red">
                <FontAwesomeIcon icon={faCircleXmark} className="opacity-70" /> Failed
              </Tag>
            </Tooltip>
          )
        if (record.opened_at)
          events.push(
            <Tooltip key="opened" title={formatDate(record.opened_at)}>
              <Tag bordered={false} color="cyan">
                <FontAwesomeIcon icon={faEye} className="opacity-70" /> Opened
              </Tag>
            </Tooltip>
          )
        if (record.clicked_at)
          events.push(
            <Tooltip key="clicked" title={formatDate(record.clicked_at)}>
              <Tag bordered={false} color="geekblue">
                <FontAwesomeIcon icon={faArrowPointer} className="opacity-70" /> Clicked
              </Tag>
            </Tooltip>
          )
        if (record.bounced_at)
          events.push(
            <Tooltip key="bounced" title={formatDate(record.bounced_at)}>
              <Tag bordered={false} color="volcano">
                <FontAwesomeIcon icon={faTriangleExclamation} className="opacity-70" /> Bounced
              </Tag>
            </Tooltip>
          )
        if (record.complained_at)
          events.push(
            <Tooltip key="complained" title={formatDate(record.complained_at)}>
              <Tag bordered={false} color="red">
                <FontAwesomeIcon icon={faFaceFrown} className="opacity-70" /> Complained
              </Tag>
            </Tooltip>
          )
        if (record.unsubscribed_at)
          events.push(
            <Tooltip key="unsubscribed" title={formatDate(record.unsubscribed_at)}>
              <Tag bordered={false} color="red">
                <FontAwesomeIcon icon={faBan} className="opacity-70" /> Unsubscribed
              </Tag>
            </Tooltip>
          )
        return <div className="flex items-center gap-1">{events}</div>
      }
    },
    {
      title: 'Error',
      key: 'error',
      hidden: visibleColumns.error === false,
      render: (record: MessageHistory) => {
        return (
          <div className="text-xs">
            {record.error && (
              <Tooltip title={record.error}>{record.error.substring(0, 50)}...</Tooltip>
            )}
          </div>
        )
      }
    },
    {
      title: 'Created At',
      dataIndex: 'created_at',
      key: 'created_at',
      hidden: visibleColumns.created_at === false,
      render: (date: string) => {
        return <Tooltip title={formatDate(date)}>{dayjs(date).fromNow()}</Tooltip>
      }
    }
  ]

  // Email column to conditionally add
  const emailColumn = {
    title: 'Contact Email',
    dataIndex: 'contact_email',
    key: 'contact_email',
    hidden: visibleColumns.contact_email === false,
    render: (email: string) => <span className="text-xs">{email}</span>
  }

  // Add actions column
  const actionsColumn = {
    title: (
      <Space size="small">
        {onRefresh && (
          <Tooltip title="Refresh">
            <Button
              type="text"
              size="small"
              icon={<FontAwesomeIcon icon={faRefresh} />}
              onClick={onRefresh}
              className="opacity-70 hover:opacity-100"
            />
          </Tooltip>
        )}
        {onColumnVisibilityChange && (
          <MessageColumnsSelector
            columns={allColumns.map((col) => ({
              ...col,
              visible: visibleColumns[col.key] !== false
            }))}
            onColumnVisibilityChange={onColumnVisibilityChange}
            storageKey={STORAGE_KEY}
          />
        )}
      </Space>
    ),
    key: 'actions',
    width: 100,
    align: 'right' as const,
    render: (record: MessageHistory) => {
      if (!record.template_id) {
        return null
      }

      return (
        <div className="flex justify-end">
          <TemplatePreviewButton
            templateId={record.template_id}
            templateVersion={record.template_version}
            workspace={workspace}
            templateData={record.message_data.data || {}}
            messageHistory={record}
          />
        </div>
      )
    }
  }

  // Build columns array based on show_email prop and add actions column
  const allTableColumns = show_email
    ? [emailColumn, ...baseColumns, actionsColumn]
    : [...baseColumns, actionsColumn]

  // Filter out hidden columns
  const columns = allTableColumns.filter((col) => !col.hidden)

  if (loading && !isLoadingMore) {
    return (
      <div className="loading-container" style={{ padding: '40px 0', textAlign: 'center' }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>Loading message history...</div>
      </div>
    )
  }

  if (!messages || messages.length === 0) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="No messages found"
        style={{ margin: '40px 0' }}
      />
    )
  }

  return (
    <>
      <Table
        dataSource={messages}
        columns={columns}
        rowKey="id"
        pagination={false}
        size={size}
        className={bordered ? 'border border-gray-300 rounded' : ''}
      />

      {nextCursor && (
        <div className="flex justify-center mt-4 mb-8">
          <Button size="small" onClick={onLoadMore} loading={isLoadingMore}>
            Load More
          </Button>
        </div>
      )}
    </>
  )
}
