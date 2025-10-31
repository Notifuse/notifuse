import { useEffect } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import {
  Button,
  Input,
  Radio,
  Form,
  InputNumber,
  App,
  Divider,
  Row,
  Col,
  Alert,
  Collapse,
  Switch
} from 'antd'
import { ApiOutlined, CheckOutlined, CopyOutlined, ArrowRightOutlined } from '@ant-design/icons'
import { setupApi } from '../services/api/setup'
import type { SetupConfig } from '../types/setup'

const { TextArea } = Input

declare global {
  interface Window {
    IS_INSTALLED?: boolean
    API_ENDPOINT?: string
  }
}

export default function SetupWizard() {
  const navigate = useNavigate()

  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [testing, setTesting] = useState(false)
  const [statusLoading, setStatusLoading] = useState(true)
  const [keyMode, setKeyMode] = useState<'generate' | 'existing'>('generate')
  const [setupComplete, setSetupComplete] = useState(false)
  const [apiEndpoint, setApiEndpoint] = useState('')
  const [generatedKeys, setGeneratedKeys] = useState<{
    public_key: string
    private_key: string
  } | null>(null)
  const [configStatus, setConfigStatus] = useState<{
    smtp_configured: boolean
    paseto_configured: boolean
    api_endpoint_configured: boolean
    root_email_configured: boolean
  }>({
    smtp_configured: false,
    paseto_configured: false,
    api_endpoint_configured: false,
    root_email_configured: false
  })
  const { message } = App.useApp()

  useEffect(() => {
    // Get API endpoint from window object
    setApiEndpoint((window as any).API_ENDPOINT || '')

    // Fetch setup status
    const fetchStatus = async () => {
      try {
        const status = await setupApi.getStatus()
        // console.log('status', status)
        if (status.is_installed) {
          navigate({ to: '/signin' })
          return
        }
        setConfigStatus({
          smtp_configured: status.smtp_configured,
          paseto_configured: status.paseto_configured,
          api_endpoint_configured: status.api_endpoint_configured,
          root_email_configured: status.root_email_configured
        })
      } catch (error) {
        message.error('Failed to fetch setup status')
      } finally {
        setStatusLoading(false)
      }
    }
    fetchStatus()
  }, [navigate, message])

  const handleTestConnection = async () => {
    try {
      await form.validateFields(['smtp_host', 'smtp_port'])
      setTesting(true)

      const values = form.getFieldsValue()
      const testConfig = {
        smtp_host: values.smtp_host,
        smtp_port: values.smtp_port,
        smtp_username: values.smtp_username || '',
        smtp_password: values.smtp_password || ''
      }

      const result = await setupApi.testSmtp(testConfig)
      setTesting(false)
      message.success(result.message || 'SMTP connection successful!')
    } catch (error) {
      setTesting(false)
      message.error(error instanceof Error ? error.message : 'Failed to test SMTP connection')
    }
  }

  const handleSubmit = async (values: any) => {
    setLoading(true)

    // console.log('values', values)
    try {
      // Only include fields that are not configured via environment variables
      const setupConfig: SetupConfig = {}

      // Root email (only if not configured via env)
      if (!configStatus.root_email_configured) {
        setupConfig.root_email = values.root_email
      }

      // API endpoint (only if not configured via env)
      if (!configStatus.api_endpoint_configured) {
        setupConfig.api_endpoint = values.api_endpoint
      }

      // PASETO keys (only if not configured via env)
      if (!configStatus.paseto_configured) {
        setupConfig.generate_paseto_keys = keyMode === 'generate'
        if (keyMode === 'existing') {
          setupConfig.paseto_public_key = values.paseto_public_key
          setupConfig.paseto_private_key = values.paseto_private_key
        }
      }

      // SMTP configuration (only if not configured via env)
      if (!configStatus.smtp_configured) {
        setupConfig.smtp_host = values.smtp_host
        setupConfig.smtp_port = values.smtp_port
        setupConfig.smtp_username = values.smtp_username || ''
        setupConfig.smtp_password = values.smtp_password || ''
        setupConfig.smtp_from_email = values.smtp_from_email
        setupConfig.smtp_from_name = values.smtp_from_name || 'Notifuse'
      }

      // Telemetry and check for updates settings
      setupConfig.telemetry_enabled = values.telemetry_enabled || false
      setupConfig.check_for_updates = values.check_for_updates || false

      const result = await setupApi.initialize(setupConfig)

      // Subscribe to newsletter if checked (fail silently)
      if (values.subscribe_newsletter && values.root_email) {
        try {
          const contact: any = {
            email: values.root_email
          }

          // Only include custom fields if values are available
          const endpoint = values.api_endpoint || apiEndpoint
          if (endpoint) {
            contact.custom_string_1 = endpoint
          }

          if (values.check_for_updates !== undefined) {
            contact.custom_string_2 = values.check_for_updates ? 'true' : 'false'
          }

          if (values.telemetry_enabled !== undefined) {
            contact.custom_string_3 = values.telemetry_enabled ? 'true' : 'false'
          }

          await fetch('https://email.notifuse.com/subscribe', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json'
            },
            body: JSON.stringify({
              workspace_id: 'notifuse',
              contact,
              list_ids: ['newsletter']
            })
          })
        } catch (error) {
          // Fail silently - don't block setup if newsletter subscription fails
          console.error('Newsletter subscription failed:', error)
        }
      }

      // If keys were generated and returned, show them to the user
      if (result.paseto_keys) {
        setGeneratedKeys(result.paseto_keys)
      }

      // Show setup complete screen immediately with keys
      setSetupComplete(true)

      // Keep loading state active while server restarts
      // Show loading message for server restart
      const hideRestartMessage = message.loading({
        content: 'Server is restarting with new configuration...',
        duration: 0, // Don't auto-dismiss
        key: 'server-restart'
      })

      // Wait for server to restart
      try {
        await waitForServerRestart()

        // Success - server is back up
        message.success({
          content: 'Server restarted successfully! You can now sign in.',
          key: 'server-restart',
          duration: 3
        })

        // Don't redirect automatically - let user click the button
        setLoading(false)
      } catch (error) {
        hideRestartMessage()
        message.error({
          content: 'Server restart timeout. Please refresh the page manually.',
          key: 'server-restart',
          duration: 0
        })
        setLoading(false)
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'Failed to complete setup')
      setLoading(false)
    }
  }

  /**
   * Wait for the server to restart after setup completion
   * Polls the health endpoint until server is back online
   */
  const waitForServerRestart = async (): Promise<void> => {
    const maxAttempts = 60 // 60 seconds max wait
    const delayMs = 1000 // Check every second

    // Wait for server to start shutting down
    await new Promise((resolve) => setTimeout(resolve, 2000))

    // Poll health endpoint
    for (let i = 0; i < maxAttempts; i++) {
      try {
        const response = await fetch('/api/setup.status', {
          method: 'GET',
          cache: 'no-cache',
          headers: {
            'Cache-Control': 'no-cache'
          }
        })

        if (response.ok) {
          // Server is back!
          console.log(`Server restarted successfully after ${i + 1} attempts`)
          return
        }
      } catch (error) {
        // Expected during restart - server is down
        console.log(`Waiting for server... attempt ${i + 1}/${maxAttempts}`)
      }

      await new Promise((resolve) => setTimeout(resolve, delayMs))
    }

    throw new Error('Server restart timeout')
  }

  const handleCopyKey = (key: string, keyType: string) => {
    navigator.clipboard.writeText(key)
    message.success(`${keyType} copied to clipboard!`)
  }

  const handleDone = () => {
    // Force a full page reload to fetch fresh config from /config.js
    // This ensures window.IS_INSTALLED is properly set from the backend
    window.location.href = '/signin'
  }

  if (statusLoading) {
    return (
      <App>
        <div className="min-h-screen bg-gray-50 flex items-center justify-center">
          <div className="text-center">
            <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900" />
            <p className="mt-4 text-gray-600">Loading setup...</p>
          </div>
        </div>
      </App>
    )
  }

  return (
    <App>
      <div className="min-h-screen bg-gray-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8">
        <div className="sm:mx-auto sm:w-full sm:max-w-3xl">
          {/* Logo */}
          <div className="text-center mb-8">
            <img src="/logo.png" alt="Notifuse" className="mx-auto" width={120} />
          </div>

          <div className="bg-white py-8 px-4 shadow sm:rounded-lg sm:px-10">
            {setupComplete ? (
              <div className="space-y-6">
                <div className="text-center">
                  <CheckOutlined
                    style={{ fontSize: '48px', color: '#52c41a', marginBottom: '16px' }}
                  />
                  <h2 className="text-3xl font-bold text-gray-900 mb-2">Setup Complete!</h2>
                  <p className="text-gray-600">
                    Your Notifuse instance has been successfully configured.
                  </p>
                </div>

                {generatedKeys && (
                  <div className="mt-8">
                    <div className="mb-6">
                      <Alert
                        message="Save Your PASETO Keys"
                        description="These keys are critical for your installation. Save them securely - they cannot be recovered if lost. You'll need them if you migrate or restore your installation."
                        type="warning"
                      />
                    </div>

                    <Row gutter={16}>
                      <Col span={12}>
                        <label className="block text-sm font-semibold text-gray-700 mb-2">
                          Public Key
                        </label>
                        <TextArea
                          readOnly
                          value={generatedKeys.public_key}
                          rows={6}
                          style={{ fontFamily: 'monospace', fontSize: '12px' }}
                        />
                        <div className="mt-2">
                          <Button
                            type="primary"
                            ghost
                            block
                            icon={<CopyOutlined />}
                            onClick={() => handleCopyKey(generatedKeys.public_key, 'Public key')}
                          >
                            Copy
                          </Button>
                        </div>
                      </Col>

                      <Col span={12}>
                        <label className="block text-sm font-semibold text-gray-700 mb-2">
                          Private Key
                        </label>
                        <TextArea
                          readOnly
                          value={generatedKeys.private_key}
                          rows={6}
                          style={{ fontFamily: 'monospace', fontSize: '12px' }}
                        />
                        <div className="mt-2">
                          <Button
                            type="primary"
                            ghost
                            block
                            icon={<CopyOutlined />}
                            onClick={() => handleCopyKey(generatedKeys.private_key, 'Private key')}
                          >
                            Copy
                          </Button>
                        </div>
                      </Col>
                    </Row>
                  </div>
                )}

                <div className="mt-8 text-center">
                  <Button
                    type="primary"
                    size="large"
                    block
                    onClick={handleDone}
                    loading={loading}
                    icon={!loading && <ArrowRightOutlined />}
                    iconPosition="end"
                    disabled={loading}
                  >
                    {loading ? 'Waiting for server restart...' : 'Go to Sign In'}
                  </Button>
                </div>
              </div>
            ) : (
              <div className="space-y-6">
                <div className="text-center">
                  <h2 className="text-3xl font-bold text-gray-900">Setup</h2>
                </div>

                <Form
                  form={form}
                  layout="vertical"
                  onFinish={handleSubmit}
                  initialValues={{
                    smtp_port: 587,
                    smtp_from_name: 'Notifuse',
                    subscribe_newsletter: true,
                    telemetry_enabled: true,
                    check_for_updates: true
                  }}
                >
                  {(!configStatus.root_email_configured ||
                    !configStatus.api_endpoint_configured) && (
                    <div className="mt-12">
                      {!configStatus.root_email_configured && (
                        <Form.Item
                          label="Root Email"
                          name="root_email"
                          rules={[
                            { required: true, message: 'Admin email is required' },
                            { type: 'email', message: 'Invalid email format' }
                          ]}
                          tooltip="This email will be used for the root administrator account"
                        >
                          <Input placeholder="admin@example.com" />
                        </Form.Item>
                      )}
                      {!configStatus.api_endpoint_configured && (
                        <Form.Item
                          label="API Endpoint"
                          name="api_endpoint"
                          rules={[
                            { required: true, message: 'API endpoint is required' },
                            { type: 'url', message: 'Invalid URL format' }
                          ]}
                          tooltip="Public URL where this Notifuse instance is accessible"
                        >
                          <Input placeholder="https://notifuse.example.com" />
                        </Form.Item>
                      )}
                    </div>
                  )}

                  {/* Newsletter Subscription */}
                  <Form.Item
                    name="subscribe_newsletter"
                    valuePropName="checked"
                    label="Subscribe to the newsletter (new features...)"
                    style={{ marginTop: 24 }}
                  >
                    <Switch />
                  </Form.Item>

                  {/* SMTP Configuration Section */}
                  {!configStatus.smtp_configured && (
                    <>
                      <Divider orientation="center" style={{ marginTop: 32, marginBottom: 24 }}>
                        SMTP Configuration
                      </Divider>

                      <div className="text-center mb-4">
                        <p className="text-sm text-gray-600">
                          See docs for:
                          <a
                            href="https://docs.aws.amazon.com/ses/latest/dg/smtp-credentials.html"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-blue-600 hover:underline pl-2"
                          >
                            Amazon SES
                          </a>
                          {' • '}
                          <a
                            href="https://documentation.mailgun.com/docs/mailgun/user-manual/sending-messages/send-smtp"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-blue-600 hover:underline"
                          >
                            Mailgun
                          </a>
                          {' • '}
                          <a
                            href="https://developers.sparkpost.com/api/smtp/"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-blue-600 hover:underline"
                          >
                            SparkPost
                          </a>
                          {' • '}
                          <a
                            href="https://postmarkapp.com/developer/user-guide/send-email-with-smtp"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-blue-600 hover:underline"
                          >
                            Postmark
                          </a>
                        </p>
                      </div>

                      <Row gutter={16}>
                        <Col span={12}>
                          <Form.Item
                            label="SMTP Host"
                            name="smtp_host"
                            rules={[{ required: true, message: 'SMTP host is required' }]}
                          >
                            <Input placeholder="smtp.example.com" />
                          </Form.Item>
                        </Col>
                        <Col span={12}>
                          <Form.Item
                            label="SMTP Port"
                            name="smtp_port"
                            rules={[{ required: true, message: 'SMTP port is required' }]}
                            tooltip="Common ports: 587 (TLS), 465 (SSL), 25 (unencrypted)"
                          >
                            <InputNumber
                              min={1}
                              max={65535}
                              placeholder="587"
                              style={{ width: '100%' }}
                            />
                          </Form.Item>
                        </Col>
                      </Row>

                      <Row gutter={16}>
                        <Col span={12}>
                          <Form.Item label="SMTP Username" name="smtp_username">
                            <Input placeholder="user@example.com" />
                          </Form.Item>
                        </Col>
                        <Col span={12}>
                          <Form.Item label="SMTP Password" name="smtp_password">
                            <Input.Password placeholder="••••••••" />
                          </Form.Item>
                        </Col>
                      </Row>

                      <Row gutter={16}>
                        <Col span={12}>
                          <Form.Item
                            label="From Email"
                            name="smtp_from_email"
                            rules={[
                              { required: true, message: 'From email is required' },
                              { type: 'email', message: 'Invalid email format' }
                            ]}
                          >
                            <Input placeholder="notifications@example.com" />
                          </Form.Item>
                        </Col>
                        <Col span={12}>
                          <Form.Item label="From Name" name="smtp_from_name">
                            <Input placeholder="Notifuse" />
                          </Form.Item>
                        </Col>
                      </Row>

                      <div className="text-right">
                        <Button
                          onClick={handleTestConnection}
                          loading={testing}
                          icon={<ApiOutlined />}
                        >
                          Test Connection
                        </Button>
                      </div>
                    </>
                  )}

                  {/* Advanced Settings Collapse */}
                  <Collapse
                    ghost
                    style={{ marginTop: 32 }}
                    items={[
                      {
                        key: 'advanced',
                        label: 'Advanced Settings',
                        children: (
                          <>
                            {/* PASETO Keys Section */}
                            {!configStatus.paseto_configured && (
                              <>
                                <Divider orientation="center" style={{ marginBottom: 24 }}>
                                  PASETO Keys
                                </Divider>

                                <div className="mb-4">
                                  <Alert
                                    description={
                                      <>
                                        <strong>Important:</strong> PASETO keys are used to generate
                                        and validate API keys for your workspaces. Save them
                                        securely after setup - they cannot be recovered if lost.
                                        Losing these keys will force you to regenerate all workspace
                                        API keys, which will break existing integrations.
                                        <br />
                                        <br />
                                        💡 Need to generate PASETO keys? Use our online tool:{' '}
                                        <a
                                          href="https://paseto.notifuse.com"
                                          target="_blank"
                                          rel="noopener noreferrer"
                                          className="underline font-medium hover:text-blue-900"
                                        >
                                          paseto.notifuse.com
                                        </a>
                                      </>
                                    }
                                    type="warning"
                                    showIcon={false}
                                  />
                                </div>

                                <Form.Item>
                                  <Radio.Group
                                    value={keyMode}
                                    onChange={(e) => setKeyMode(e.target.value)}
                                    block
                                  >
                                    <Radio.Button value="generate">
                                      Generate New Keys Automatically
                                    </Radio.Button>
                                    <Radio.Button value="existing">Use Existing Keys</Radio.Button>
                                  </Radio.Group>
                                </Form.Item>

                                {keyMode === 'existing' && (
                                  <>
                                    <Form.Item
                                      label="Public Key"
                                      name="paseto_public_key"
                                      rules={[
                                        { required: true, message: 'Public key is required' }
                                      ]}
                                    >
                                      <TextArea
                                        rows={3}
                                        placeholder="Paste your base64-encoded PASETO public key here..."
                                        style={{ fontFamily: 'monospace', fontSize: '12px' }}
                                      />
                                    </Form.Item>

                                    <Form.Item
                                      label="Private Key"
                                      name="paseto_private_key"
                                      rules={[
                                        { required: true, message: 'Private key is required' }
                                      ]}
                                    >
                                      <TextArea
                                        rows={3}
                                        placeholder="Paste your base64-encoded PASETO private key here..."
                                        style={{ fontFamily: 'monospace', fontSize: '12px' }}
                                      />
                                    </Form.Item>
                                  </>
                                )}
                              </>
                            )}

                            {/* Telemetry Setting */}
                            <Divider
                              orientation="center"
                              style={{ marginTop: 32, marginBottom: 24 }}
                            >
                              Privacy Settings
                            </Divider>

                            <Row gutter={16}>
                              <Col span={12}>
                                <Form.Item
                                  name="telemetry_enabled"
                                  valuePropName="checked"
                                  label="Enable Anonymous Telemetry"
                                  tooltip="Help us improve Notifuse by sending anonymous usage statistics. No personal data or message content is collected."
                                >
                                  <Switch />
                                </Form.Item>
                              </Col>
                              <Col span={12}>
                                <Form.Item
                                  name="check_for_updates"
                                  valuePropName="checked"
                                  label="Check for Updates"
                                  tooltip="Periodically check for new Notifuse versions and security updates. A popup will list new versions available."
                                >
                                  <Switch />
                                </Form.Item>
                              </Col>
                            </Row>
                          </>
                        )
                      }
                    ]}
                  />

                  {/* Submit Button */}
                  <Divider style={{ marginTop: 32, marginBottom: 24 }} />

                  <Button
                    type="primary"
                    htmlType="submit"
                    loading={loading}
                    size="large"
                    icon={<CheckOutlined />}
                    iconPosition="end"
                    block
                  >
                    {loading ? 'Setting up...' : 'Complete Setup'}
                  </Button>
                </Form>
              </div>
            )}
          </div>
        </div>
      </div>
    </App>
  )
}
