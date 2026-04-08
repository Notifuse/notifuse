import { useState, useEffect, useCallback } from 'react'
import {
  Drawer,
  Form,
  Input,
  InputNumber,
  Switch,
  Button,
  Row,
  Col,
  Typography,
  Divider,
  App,
  Modal,
  Tooltip,
  Space,
  Spin
} from 'antd'
import {
  SettingOutlined,
  ThunderboltOutlined,
  InfoCircleOutlined
} from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { settingsApi } from '../../services/api/settings'
import type { SystemSettingsData } from '../../types/settings'

const { Text, Title } = Typography

export function SystemSettingsDrawer() {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [testingSmtp, setTestingSmtp] = useState(false)
  const [envOverrides, setEnvOverrides] = useState<Record<string, boolean>>({})
  const [originalApiEndpoint, setOriginalApiEndpoint] = useState('')
  const [form] = Form.useForm<SystemSettingsData>()

  const fetchSettings = useCallback(async () => {
    setLoading(true)
    try {
      const response = await settingsApi.get()
      // Convert 0 port values to undefined so optional port fields appear empty
      const settings = {
        ...response.settings,
        smtp_bridge_port: response.settings.smtp_bridge_port || undefined
      }
      form.setFieldsValue(settings)
      setEnvOverrides(response.env_overrides || {})
      setOriginalApiEndpoint(response.settings.api_endpoint || '')
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`Failed to load settings`)
    } finally {
      setLoading(false)
    }
  }, [form, message, t])

  useEffect(() => {
    if (open) {
      fetchSettings()
    }
  }, [open, fetchSettings])

  const bridgeEnabled = Form.useWatch('smtp_bridge_enabled', form)

  const isOverridden = (field: string) => envOverrides[field] === true

  const renderEnvHint = (field: string) => {
    if (!isOverridden(field)) return null
    return (
      <Text type="secondary" style={{ fontSize: 11 }}>
        {t`Set by environment variable`}
      </Text>
    )
  }

  const handleTestSmtp = async () => {
    setTestingSmtp(true)
    try {
      const values = form.getFieldsValue()
      await settingsApi.testSmtp({
        smtp_host: values.smtp_host,
        smtp_port: values.smtp_port,
        smtp_username: values.smtp_username,
        smtp_password: values.smtp_password,
        smtp_use_tls: values.smtp_use_tls,
        smtp_ehlo_hostname: values.smtp_ehlo_hostname
      })
      message.success(t`SMTP connection test successful`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`SMTP connection test failed`)
    } finally {
      setTestingSmtp(false)
    }
  }

  const waitForServerRestart = async (endpoint: string): Promise<void> => {
    const maxAttempts = 60
    const delayMs = 1000

    await new Promise((resolve) => setTimeout(resolve, 2000))

    for (let i = 0; i < maxAttempts; i++) {
      try {
        const response = await fetch(`${endpoint}/api/setup.status?t=${Date.now()}`, {
          method: 'GET'
        })
        if (response.ok) {
          return
        }
      } catch {
        // Expected during restart
      }
      await new Promise((resolve) => setTimeout(resolve, delayMs))
    }

    throw new Error('Server restart timeout')
  }

  const handleSave = () => {
    form.validateFields().then((values) => {
      const newApiEndpoint = values.api_endpoint || ''
      const apiEndpointChanged =
        newApiEndpoint !== originalApiEndpoint && newApiEndpoint !== ''

      const modalContent: string[] = []
      modalContent.push(t`Saving settings will restart the server. You will need to sign in again.`)

      if (apiEndpointChanged) {
        modalContent.push(
          t`The API endpoint has changed. After restart, the console will be available at:` +
            ` ${newApiEndpoint}/console/`
        )
      }

      Modal.confirm({
        title: t`Restart Server?`,
        content: (
          <div>
            {modalContent.map((line, i) => (
              <p key={i}>{line}</p>
            ))}
          </div>
        ),
        okText: t`Save & Restart`,
        cancelText: t`Cancel`,
        onOk: async () => {
          setSaving(true)
          try {
            await settingsApi.update(values)

            const hideMsg = message.loading({
              content: t`Server is restarting...`,
              duration: 0,
              key: 'server-restart'
            })

            const endpointToCheck = apiEndpointChanged
              ? newApiEndpoint
              : originalApiEndpoint || window.location.origin

            try {
              await waitForServerRestart(endpointToCheck)
              message.success({
                content: t`Server restarted successfully!`,
                key: 'server-restart',
                duration: 3
              })

              if (apiEndpointChanged) {
                window.location.href = `${newApiEndpoint}/console/signin`
              } else {
                window.location.href = '/console/signin'
              }
            } catch {
              hideMsg()
              message.error({
                content: t`Server restart timeout. Please refresh the page manually.`,
                key: 'server-restart',
                duration: 0
              })
              setSaving(false)
            }
          } catch (err) {
            message.error(
              err instanceof Error ? err.message : t`Failed to save settings`
            )
            setSaving(false)
          }
        }
      })
    })
  }

  return (
    <>
      <Tooltip title={t`System Settings`}>
        <Button
          type="primary"
          ghost
          icon={<SettingOutlined />}
          onClick={() => setOpen(true)}
          style={{ padding: '4px', lineHeight: 1 }}
        />
      </Tooltip>

      <Drawer
        title={t`System Settings`}
        placement="right"
        width={900}
        onClose={() => setOpen(false)}
        open={open}
        extra={
          <Button type="primary" onClick={handleSave} loading={saving}>
            {t`Save`}
          </Button>
        }
      >
        {loading ? (
          <div style={{ textAlign: 'center', padding: '60px 0' }}>
            <Spin size="large" />
          </div>
        ) : (
          <Form form={form} layout="vertical" disabled={saving}>
            {/* General */}
            <Title level={5}>{t`General`}</Title>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  label={t`Root Email`}
                  name="root_email"
                  rules={[
                    { required: true, message: t`Required` },
                    { type: 'email', message: t`Invalid email` }
                  ]}
                  help={renderEnvHint('root_email')}
                >
                  <Input disabled={isOverridden('root_email')} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  label={t`API Endpoint`}
                  name="api_endpoint"
                  rules={[{ required: true, message: t`Required` }]}
                  help={renderEnvHint('api_endpoint')}
                >
                  <Input
                    disabled={isOverridden('api_endpoint')}
                    placeholder="https://your-domain.com"
                  />
                </Form.Item>
              </Col>
            </Row>

            <Divider />

            {/* SMTP Configuration */}
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 16
              }}
            >
              <Title level={5} style={{ margin: 0 }}>
                {t`System SMTP`}
              </Title>
              <Button
                icon={<ThunderboltOutlined />}
                onClick={handleTestSmtp}
                loading={testingSmtp}
                size="small"
              >
                {t`Test Connection`}
              </Button>
            </div>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  label={t`SMTP Host`}
                  name="smtp_host"
                  rules={[{ required: true, message: t`Required` }]}
                  help={renderEnvHint('smtp_host')}
                >
                  <Input
                    disabled={isOverridden('smtp_host')}
                    placeholder="smtp.example.com"
                  />
                </Form.Item>
              </Col>
              <Col span={6}>
                <Form.Item
                  label={t`SMTP Port`}
                  name="smtp_port"
                  rules={[
                    { required: true, message: t`Required` },
                    {
                      type: 'number',
                      min: 1,
                      max: 65535,
                      message: t`Port must be 1-65535`
                    }
                  ]}
                  help={renderEnvHint('smtp_port')}
                >
                  <InputNumber
                    disabled={isOverridden('smtp_port')}
                    style={{ width: '100%' }}
                    placeholder="587"
                  />
                </Form.Item>
              </Col>
              <Col span={6}>
                <Form.Item
                  label={
                    <Space>
                      {t`Use TLS`}
                      <Tooltip title={t`Enable TLS encryption for SMTP connection`}>
                        <InfoCircleOutlined style={{ color: '#999' }} />
                      </Tooltip>
                    </Space>
                  }
                  name="smtp_use_tls"
                  valuePropName="checked"
                  help={renderEnvHint('smtp_use_tls')}
                >
                  <Switch disabled={isOverridden('smtp_use_tls')} />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  label={t`SMTP Username`}
                  name="smtp_username"
                  help={renderEnvHint('smtp_username')}
                >
                  <Input
                    disabled={isOverridden('smtp_username')}
                    placeholder={t`Optional`}
                    allowClear
                  />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  label={t`SMTP Password`}
                  name="smtp_password"
                  help={renderEnvHint('smtp_password')}
                >
                  <Input.Password
                    disabled={isOverridden('smtp_password')}
                    placeholder={t`Optional`}
                    allowClear
                  />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  label={t`From Email`}
                  name="smtp_from_email"
                  rules={[
                    { required: true, message: t`Required` },
                    { type: 'email', message: t`Invalid email` }
                  ]}
                  help={renderEnvHint('smtp_from_email')}
                >
                  <Input
                    disabled={isOverridden('smtp_from_email')}
                    placeholder="noreply@example.com"
                  />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  label={t`From Name`}
                  name="smtp_from_name"
                  help={renderEnvHint('smtp_from_name')}
                >
                  <Input
                    disabled={isOverridden('smtp_from_name')}
                    placeholder="Notifuse"
                    allowClear
                  />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  label={
                    <Space>
                      {t`EHLO Hostname`}
                      <Tooltip
                        title={t`Custom hostname used in SMTP EHLO/HELO command. Leave empty to use default.`}
                      >
                        <InfoCircleOutlined style={{ color: '#999' }} />
                      </Tooltip>
                    </Space>
                  }
                  name="smtp_ehlo_hostname"
                  help={renderEnvHint('smtp_ehlo_hostname')}
                >
                  <Input
                    disabled={isOverridden('smtp_ehlo_hostname')}
                    placeholder={t`Optional`}
                    allowClear
                  />
                </Form.Item>
              </Col>
            </Row>

            <Divider />

            {/* SMTP Bridge */}
            <Title level={5}>{t`SMTP Bridge`}</Title>
            <Row gutter={16}>
              <Col span={6}>
                <Form.Item
                  label={t`Enabled`}
                  name="smtp_bridge_enabled"
                  valuePropName="checked"
                  help={renderEnvHint('smtp_bridge_enabled')}
                >
                  <Switch disabled={isOverridden('smtp_bridge_enabled')} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  label={t`Bridge Domain`}
                  name="smtp_bridge_domain"
                  rules={[{ required: bridgeEnabled, message: t`Required when bridge is enabled` }]}
                  help={renderEnvHint('smtp_bridge_domain')}
                >
                  <Input
                    disabled={isOverridden('smtp_bridge_domain')}
                    placeholder="bridge.example.com"
                    allowClear
                  />
                </Form.Item>
              </Col>
              <Col span={6}>
                <Form.Item
                  label={t`Bridge Port`}
                  name="smtp_bridge_port"
                  rules={[
                    {
                      type: 'number',
                      min: 1,
                      max: 65535,
                      message: t`Port must be 1-65535`
                    }
                  ]}
                  help={renderEnvHint('smtp_bridge_port')}
                >
                  <InputNumber
                    disabled={isOverridden('smtp_bridge_port')}
                    style={{ width: '100%' }}
                    placeholder="587"
                  />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  label={t`TLS Certificate (Base64)`}
                  name="smtp_bridge_tls_cert_base64"
                  rules={[{ required: bridgeEnabled, message: t`Required when bridge is enabled` }]}
                  help={renderEnvHint('smtp_bridge_tls_cert_base64')}
                >
                  <Input.TextArea
                    disabled={isOverridden('smtp_bridge_tls_cert_base64')}
                    rows={3}
                    placeholder={t`Base64 encoded TLS certificate`}
                    allowClear
                  />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  label={t`TLS Key (Base64)`}
                  name="smtp_bridge_tls_key_base64"
                  rules={[{ required: bridgeEnabled, message: t`Required when bridge is enabled` }]}
                  help={renderEnvHint('smtp_bridge_tls_key_base64')}
                >
                  <Input.TextArea
                    disabled={isOverridden('smtp_bridge_tls_key_base64')}
                    rows={3}
                    placeholder={t`Base64 encoded TLS private key`}
                    allowClear
                  />
                </Form.Item>
              </Col>
            </Row>

            <Divider />

            {/* Features */}
            <Title level={5}>{t`Features`}</Title>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  label={t`Telemetry`}
                  name="telemetry_enabled"
                  valuePropName="checked"
                >
                  <Switch />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  label={t`Check for Updates`}
                  name="check_for_updates"
                  valuePropName="checked"
                >
                  <Switch />
                </Form.Item>
              </Col>
            </Row>
          </Form>
        )}
      </Drawer>
    </>
  )
}
