import { useState } from 'react'
import {
  Switch,
  Input,
  Button,
  Space,
  Typography,
  App,
  Spin,
  Alert,
  Form,
  Modal,
  Tooltip
} from 'antd'
import { useLingui } from '@lingui/react/macro'
import { CheckCircleOutlined, QuestionCircleOutlined } from '@ant-design/icons'
import { Highlight, themes } from 'prism-react-renderer'
import { useMutation } from '@tanstack/react-query'
import {
  broadcastApi,
  RecipientFeedSettings as RecipientFeedSettingsType,
  TestRecipientFeedResponse
} from '../../services/api/broadcast'
import { HeadersEditor } from './HeadersEditor'

const { Text, Paragraph } = Typography

interface RecipientFeedSettingsProps {
  workspaceId: string
  broadcastId?: string
  value?: RecipientFeedSettingsType
  onChange?: (settings: RecipientFeedSettingsType) => void
  disabled?: boolean
}

export function RecipientFeedSettings({
  workspaceId,
  broadcastId,
  value,
  onChange,
  disabled = false
}: RecipientFeedSettingsProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [testEmail, setTestEmail] = useState('')
  const [testResult, setTestResult] = useState<TestRecipientFeedResponse | null>(null)
  const [modalOpen, setModalOpen] = useState(false)

  const settings: RecipientFeedSettingsType = value || {
    enabled: false,
    url: '',
    headers: []
  }

  const testMutation = useMutation({
    mutationFn: () => {
      if (!broadcastId) throw new Error('Broadcast ID is required')
      return broadcastApi.testRecipientFeed({
        workspace_id: workspaceId,
        broadcast_id: broadcastId,
        contact_email: testEmail || undefined,
        url: settings.url,
        headers: settings.headers || []
      })
    },
    onSuccess: (response: TestRecipientFeedResponse) => {
      if (response.error) {
        message.error(t`Test failed: ${response.error}`)
        setTestResult(response)
      } else {
        message.success(t`Recipient feed test completed successfully`)
        setTestResult(response)
      }
    },
    onError: (error: Error) => {
      message.error(t`Failed to test recipient feed: ${error.message}`)
    }
  })

  const handleChange = (field: keyof RecipientFeedSettingsType, newValue: unknown) => {
    const newSettings = { ...settings, [field]: newValue }
    onChange?.(newSettings)
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <Text strong>{t`Per-Recipient Data Feed`}</Text>
        <Switch
          checked={settings.enabled}
          onChange={(checked) => handleChange('enabled', checked)}
          disabled={disabled}
        />
      </div>

      {settings.enabled ? (
        <div className="space-y-4">
          <Alert
            description={t`This feature makes an HTTP request for each recipient before sending their email. This can significantly slow down broadcast processing. If the feed request fails after retries, the broadcast will pause automatically.`}
            type="info"
            className="!mb-6"
          />

          <Form.Item
            label={
              <Space size={4}>
                <span>{t`Per-recipient feed URL`}</span>
                <Tooltip title={t`The URL must use HTTPS. A POST request will be sent with contact information for each recipient.`}>
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
                placeholder="https://api.example.com/recipient-data"
                value={settings.url}
                onChange={(e) => handleChange('url', e.target.value)}
                disabled={disabled}
              />
              <Button
                type="primary"
                ghost
                onClick={() => setModalOpen(true)}
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
            title={t`Test Recipient Feed`}
            open={modalOpen}
            onCancel={() => {
              setModalOpen(false)
              setTestResult(null)
            }}
            footer={null}
            width={600}
          >
            <div className="py-4">
              <Space.Compact className="w-full mb-2">
                <Input
                  placeholder={t`Contact email (leave empty for random)`}
                  value={testEmail}
                  onChange={(e) => setTestEmail(e.target.value)}
                  style={{ flex: 1 }}
                />
                <Button
                  type="primary"
                  onClick={() => testMutation.mutate()}
                  loading={testMutation.isPending}
                >
                  {t`Run Test`}
                </Button>
              </Space.Compact>
              <Paragraph type="secondary" className="text-xs mb-3">
                {t`Leave empty to use a random contact from the broadcast audience.`}
              </Paragraph>

              {testMutation.isPending ? (
                <div className="text-center py-4">
                  <Spin size="small" />
                  <Text type="secondary" className="ml-2">
                    {t`Testing feed...`}
                  </Text>
                </div>
              ) : testResult ? (
                <>
                  {testResult.error ? (
                    <Alert
                      message={t`Test Failed`}
                      description={testResult.error}
                      type="error"
                      className="mb-2"
                    />
                  ) : (
                    <Space className="mb-3">
                      <CheckCircleOutlined className="text-green-500" />
                      <Text type="secondary" className="text-xs">
                        {t`Tested with:`} {testResult.contact_email}
                      </Text>
                    </Space>
                  )}
                  {testResult.data && (
                    <Highlight
                      theme={themes.github}
                      code={JSON.stringify(testResult.data, null, 2)}
                      language="json"
                    >
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
                  )}
                </>
              ) : null}
            </div>
          </Modal>
        </div>
      ) : (
        <Text type="secondary">
          {t`Enable to fetch personalized data for each recipient before sending their email. The data will be accessible in your templates using the "recipient_feed" variable.`}
        </Text>
      )}
    </div>
  )
}
