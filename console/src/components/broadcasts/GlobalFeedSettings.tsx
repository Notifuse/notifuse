import { useState } from 'react'
import { Switch, Input, Button, Space, Typography, App, Spin, Form, Modal, Alert, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { CheckCircleOutlined, QuestionCircleOutlined } from '@ant-design/icons'
import { Highlight, themes } from 'prism-react-renderer'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  broadcastApi,
  GlobalFeedSettings as GlobalFeedSettingsType,
  RefreshGlobalFeedResponse
} from '../../services/api/broadcast'
import { HeadersEditor } from './HeadersEditor'
import dayjs from '../../lib/dayjs'

const { Text } = Typography

interface GlobalFeedSettingsProps {
  workspaceId: string
  broadcastId?: string
  value?: GlobalFeedSettingsType
  onChange?: (settings: GlobalFeedSettingsType) => void
  globalFeedData?: Record<string, unknown>
  globalFeedFetchedAt?: string
  disabled?: boolean
}

export function GlobalFeedSettings({
  workspaceId,
  broadcastId,
  value,
  onChange,
  globalFeedData,
  globalFeedFetchedAt,
  disabled = false
}: GlobalFeedSettingsProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [localFetchedData, setLocalFetchedData] = useState<Record<string, unknown> | null>(null)
  const [localFetchedAt, setLocalFetchedAt] = useState<string | null>(null)
  const [modalOpen, setModalOpen] = useState(false)

  const settings: GlobalFeedSettingsType = value || {
    enabled: false,
    url: '',
    headers: []
  }

  const refreshMutation = useMutation({
    mutationFn: () => {
      if (!broadcastId) throw new Error('Broadcast ID is required')
      return broadcastApi.refreshGlobalFeed({
        workspace_id: workspaceId,
        broadcast_id: broadcastId,
        url: settings.url,
        headers: settings.headers || []
      })
    },
    onSuccess: (response: RefreshGlobalFeedResponse) => {
      if (response.error) {
        message.error(t`Failed to fetch data: ${response.error}`)
      } else {
        setLocalFetchedData(response.data || null)
        setLocalFetchedAt(response.fetched_at || null)
        queryClient.invalidateQueries({ queryKey: ['broadcasts', workspaceId] })
      }
      setModalOpen(true)
    },
    onError: (error: Error) => {
      message.error(t`Failed to refresh global feed: ${error.message}`)
    }
  })

  const handleChange = (field: keyof GlobalFeedSettingsType, newValue: unknown) => {
    const newSettings = { ...settings, [field]: newValue }
    onChange?.(newSettings)
  }

  const displayData = localFetchedData || globalFeedData
  const displayFetchedAt = localFetchedAt || globalFeedFetchedAt

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <Text strong>{t`Global Data Feed`}</Text>
        <Switch
          checked={settings.enabled}
          onChange={(checked) => handleChange('enabled', checked)}
          disabled={disabled}
        />
      </div>

      {settings.enabled ? (
        <div className="space-y-4">
          <Form.Item
            label={
              <Space size={4}>
                <span>{t`Global feed URL`}</span>
                <Tooltip title={t`The URL must be publicly accessible. A POST request will be sent with broadcast and list information.`}>
                  <QuestionCircleOutlined className="text-gray-400 cursor-help" />
                </Tooltip>
              </Space>
            }
            required
            validateStatus={settings.enabled && !settings.url ? 'error' : undefined}
            help={settings.enabled && !settings.url ? t`URL is required` : undefined}
          >
            <Space.Compact className="w-full">
              <Input
                placeholder="https://api.example.com/broadcast-data"
                value={settings.url}
                onChange={(e) => handleChange('url', e.target.value)}
                disabled={disabled}
              />
              <Button
                type="primary"
                ghost
                onClick={() => refreshMutation.mutate()}
                loading={refreshMutation.isPending}
                disabled={disabled || !settings.url || !broadcastId}
              >
                {t`Test`}
              </Button>
            </Space.Compact>
          </Form.Item>

          <HeadersEditor
            value={settings.headers}
            onChange={(headers) => handleChange('headers', headers)}
            disabled={disabled}
          />

          <Modal
            title={t`Global Feed Response`}
            open={modalOpen}
            onCancel={() => setModalOpen(false)}
            footer={<Button onClick={() => setModalOpen(false)}>{t`Close`}</Button>}
            width={600}
          >
            <div className="py-4">
              {refreshMutation.isPending ? (
                <div className="text-center py-4">
                  <Spin size="small" />
                  <Text type="secondary" className="ml-2">
                    {t`Fetching data...`}
                  </Text>
                </div>
              ) : displayData ? (
                <>
                  <Space className="mb-3">
                    <CheckCircleOutlined className="text-green-500" />
                    <Text type="secondary" className="text-xs">
                      {t`Last fetched:`} {dayjs(displayFetchedAt).format('lll')}
                    </Text>
                  </Space>
                  <Highlight theme={themes.github} code={JSON.stringify(displayData, null, 2)} language="json">
                    {({ className, style, tokens, getLineProps, getTokenProps }) => (
                      <pre
                        className={`${className} p-3 m-0 text-xs leading-relaxed overflow-auto rounded`}
                        style={{
                          ...style,
                          backgroundColor: '#f6f8fa',
                          fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
                          maxHeight: 400
                        }}
                      >
                        {tokens.map((line, i) => (
                          <div key={i} {...getLineProps({ line })}>
                            {line.map((token, key) => (
                              <span key={key} {...getTokenProps({ token })} />
                            ))}
                          </div>
                        ))}
                      </pre>
                    )}
                  </Highlight>
                </>
              ) : (
                <Alert type="error" message={t`No data returned`} />
              )}
            </div>
          </Modal>
        </div>
      ) : (
        <Text type="secondary">
          {t`Enable to fetch global data that will be available to all recipients in this broadcast. The data will be accessible in your templates using the "global_feed" variable.`}
        </Text>
      )}
    </div>
  )
}
