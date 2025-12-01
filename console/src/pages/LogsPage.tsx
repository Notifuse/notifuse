import { Typography, Tabs } from 'antd'
import { useParams, useSearch } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { MessageHistoryTab } from '../components/messages/MessageHistoryTab'
import { WebhookEventsTab } from '../components/webhooks/WebhookEventsTab'
import { OutgoingWebhooksTab } from '../components/webhooks/OutgoingWebhooksTab'

const { Text } = Typography

export function LogsPage() {
  const { workspaceId } = useParams({ strict: false })
  const search = useSearch({ strict: false }) as { tab?: string }
  const queryClient = useQueryClient()

  if (!workspaceId) {
    return <div>Loading...</div>
  }

  const handleRefreshWebhookEvents = () => {
    queryClient.invalidateQueries({ queryKey: ['webhook-events', workspaceId] })
  }

  return (
    <div className="p-6">
      <div className="mb-6">
        <div className="text-2xl font-medium">Logs</div>
        <Text type="secondary">Monitor message delivery status and webhook events</Text>
      </div>

      <Tabs
        defaultActiveKey={search.tab || 'messages'}
        items={[
          {
            key: 'messages',
            label: 'Message History',
            children: <MessageHistoryTab workspaceId={workspaceId} />
          },
          {
            key: 'incoming-webhooks',
            label: 'Incoming Webhooks',
            children: (
              <WebhookEventsTab workspaceId={workspaceId} onRefresh={handleRefreshWebhookEvents} />
            )
          },
          {
            key: 'outgoing-webhooks',
            label: 'Outgoing Webhooks',
            children: <OutgoingWebhooksTab workspaceId={workspaceId} />
          }
        ]}
      />
    </div>
  )
}
