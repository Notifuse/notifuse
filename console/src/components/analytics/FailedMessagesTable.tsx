import React, { useState, useEffect } from 'react'
import { Card, Button } from 'antd'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { MessageHistoryTable } from '../messages/MessageHistoryTable'
import {
  listMessages,
  MessageHistory,
  MessageListParams
} from '../../services/api/messages_history'
import { Workspace } from '../../services/api/types'

interface FailedMessagesTableProps {
  workspace: Workspace
}

export const FailedMessagesTable: React.FC<FailedMessagesTableProps> = ({ workspace }) => {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [messages, setMessages] = useState<MessageHistory[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const buildParams = (): MessageListParams => ({
    limit: 5,
    is_failed: true
  })

  const fetchData = async () => {
    try {
      setLoading(true)
      setError(null)

      const params = buildParams()
      const response = await listMessages(workspace.id, params)
      setMessages(response.messages)
    } catch (err) {
      console.error('Failed to fetch failed messages data:', err)
      setError(err instanceof Error ? err.message : t`Failed to fetch failed messages data`)
    } finally {
      setLoading(false)
    }
  }

  const handleViewMore = () => {
    navigate({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId: workspace.id },
      search: { is_failed: 'true' }
    })
  }

  useEffect(() => {
    fetchData()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspace.id])

  const cardExtra = (
    <Button type="link" size="small" onClick={handleViewMore}>
      {t`View more`}
    </Button>
  )

  return (
    <Card title={t`Recent Failed Messages`} extra={cardExtra}>
      {error ? (
        <div className="text-red-500 p-4">
          <p>{t`Error`}: {error}</p>
        </div>
      ) : (
        <MessageHistoryTable
          messages={messages}
          loading={loading}
          isLoadingMore={false}
          nextCursor={undefined}
          onLoadMore={() => {}}
          show_email={true}
          bordered={false}
          size="small"
          workspace={workspace}
        />
      )}
    </Card>
  )
}
