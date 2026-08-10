import { useEffect, useMemo, useState } from 'react'
import {
  App,
  Button,
  Col,
  Descriptions,
  Divider,
  Form,
  Input,
  InputNumber,
  Row,
  Select,
  Switch,
  Typography
} from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import {
  buildInstallSnippet,
  WebAnalyticsSettings as WebAnalyticsSettingsValues,
  webAnalyticsService
} from '../../services/api/web_analytics'
import { SettingsSectionHeader } from './SettingsSectionHeader'

const DEFAULT_SETTINGS: WebAnalyticsSettingsValues = {
  enabled: false,
  allowed_domains: [],
  bounce_threshold_seconds: 10,
  geo_enabled: true,
  geo_store_city: true,
  geo_store_region: true,
  geo_coordinates_precision: 2
}

/** Slots the backend accepts: custom_1..custom_10. */
const CUSTOM_DIMENSION_SLOTS = Array.from({ length: 10 }, (_, index) => index + 1)

interface WebAnalyticsSettingsProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  canManage: boolean
}

interface WebAnalyticsFormValues {
  enabled: boolean
  allowed_domains?: string[]
  bounce_threshold_seconds?: number
  geo_enabled: boolean
  geo_store_city: boolean
  geo_store_region: boolean
  geo_coordinates_precision: number
  custom_dimension_labels?: Record<string, string>
}

export function WebAnalyticsSettings({
  workspace,
  onWorkspaceUpdate,
  canManage
}: WebAnalyticsSettingsProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [form] = Form.useForm<WebAnalyticsFormValues>()
  const [savingSettings, setSavingSettings] = useState(false)
  const [formTouched, setFormTouched] = useState(false)

  const stored = workspace?.settings?.web_analytics
  const geoEnabled = Form.useWatch('geo_enabled', form)

  useEffect(() => {
    if (!canManage) return

    const settings = { ...DEFAULT_SETTINGS, ...(stored ?? {}) }
    form.setFieldsValue({
      enabled: settings.enabled,
      allowed_domains: settings.allowed_domains ?? [],
      bounce_threshold_seconds: settings.bounce_threshold_seconds,
      geo_enabled: settings.geo_enabled,
      geo_store_city: settings.geo_store_city,
      geo_store_region: settings.geo_store_region,
      geo_coordinates_precision: settings.geo_coordinates_precision,
      custom_dimension_labels: Object.fromEntries(
        CUSTOM_DIMENSION_SLOTS.map((slot) => [
          `custom_${slot}`,
          settings.custom_dimension_labels?.[`custom_${slot}`] ?? ''
        ])
      )
    })
    setFormTouched(false)
  }, [stored, form, canManage])

  // The tracking snippet must point at the domain the SDK will actually beat to.
  const endpoint = useMemo(() => {
    return (
      workspace?.settings?.custom_endpoint_url ||
      window.API_ENDPOINT?.trim().replace(/\/+$/, '') ||
      window.location.origin
    )
  }, [workspace])

  const handleSaveSettings = async (values: WebAnalyticsFormValues) => {
    if (!workspace) return

    setSavingSettings(true)
    try {
      // Empty slots would otherwise be stored as blank labels and shadow the
      // raw custom_N name everywhere the dimension is listed.
      const labels = Object.entries(values.custom_dimension_labels ?? {}).filter(
        ([, label]) => label.trim() !== ''
      )

      await webAnalyticsService.setSettings(workspace.id, {
        ...DEFAULT_SETTINGS,
        ...values,
        custom_dimension_labels: labels.length > 0 ? Object.fromEntries(labels) : undefined,
        // Attribution rules are edited on the Web Analytics filters tab; keep
        // whatever it saved instead of dropping it on every settings save.
        filters: stored?.filters
      })

      const response = await workspaceService.get(workspace.id)
      onWorkspaceUpdate(response.workspace)

      setFormTouched(false)
      message.success(t`Web analytics settings updated successfully`)
    } catch (error: unknown) {
      console.error('Failed to update web analytics settings', error)
      const errorMessage = (error as Error)?.message || t`Failed to update web analytics settings`
      message.error(errorMessage)
    } finally {
      setSavingSettings(false)
    }
  }

  const snippet = workspace ? buildInstallSnippet(endpoint, workspace.id) : ''

  if (!canManage) {
    return (
      <>
        <SettingsSectionHeader
          title={t`Web Analytics`}
          description={t`Website traffic tracking settings`}
        />

        <Descriptions
          bordered
          column={1}
          size="small"
          styles={{ label: { width: '200px', fontWeight: '500' } }}
        >
          <Descriptions.Item label={t`Web Analytics`}>
            {stored?.enabled ? (
              <span style={{ color: '#52c41a' }}>
                <CheckCircleOutlined style={{ marginRight: '8px' }} />
                {t`Enabled`}
              </span>
            ) : (
              <span style={{ color: '#ff4d4f' }}>
                <CloseCircleOutlined style={{ marginRight: '8px' }} />
                {t`Disabled`}
              </span>
            )}
          </Descriptions.Item>

          <Descriptions.Item label={t`Allowed domains`}>
            {stored?.allowed_domains?.length ? stored.allowed_domains.join(', ') : t`Every domain`}
          </Descriptions.Item>

          <Descriptions.Item label={t`Bounce threshold`}>
            {t`${
              stored?.bounce_threshold_seconds ?? DEFAULT_SETTINGS.bounce_threshold_seconds
            } seconds`}
          </Descriptions.Item>

          <Descriptions.Item label={t`Visitor locations`}>
            {stored?.geo_enabled ? t`Resolved` : t`Not resolved`}
          </Descriptions.Item>
        </Descriptions>
      </>
    )
  }

  return (
    <>
      <SettingsSectionHeader
        title={t`Web Analytics`}
        description={t`Track website traffic with a first-party script. Beats are stored in this workspace's database, so no data leaves your infrastructure.`}
      />

      <Form
        form={form}
        layout="vertical"
        onFinish={handleSaveSettings}
        onValuesChange={() => setFormTouched(true)}
      >
        <Form.Item
          name="enabled"
          label={t`Enable web analytics`}
          valuePropName="checked"
          tooltip={t`When disabled, incoming beats are rejected and the dashboards stop updating.`}
        >
          <Switch />
        </Form.Item>

        <Form.Item
          name="allowed_domains"
          label={t`Allowed domains`}
          tooltip={t`Beats from other origins are silently ignored. Wildcards like *.example.com are supported; empty allows every domain.`}
        >
          <Select
            mode="tags"
            open={false}
            suffixIcon={null}
            tokenSeparators={[',', ' ']}
            placeholder="example.com, *.example.com"
          />
        </Form.Item>

        <Row gutter={24}>
          <Col span={12}>
            <Form.Item
              name="bounce_threshold_seconds"
              label={t`Bounce threshold (seconds)`}
              tooltip={t`Sessions with less engaged time than this count as bounces.`}
              rules={[{ required: true, message: t`Please enter a bounce threshold` }]}
            >
              <InputNumber min={1} max={600} className="w-full" />
            </Form.Item>
          </Col>
        </Row>

        <Divider className="!my-8" />

        <div className="text-xl font-medium mb-8">{t`Geolocation privacy`}</div>

        <Form.Item
          name="geo_enabled"
          label={t`Resolve visitor locations (country)`}
          valuePropName="checked"
          tooltip={t`Locations are resolved from the IP address on the server; the IP itself is never stored.`}
        >
          <Switch />
        </Form.Item>

        <Row gutter={24}>
          <Col span={8}>
            <Form.Item name="geo_store_region" label={t`Store region`} valuePropName="checked">
              <Switch disabled={!geoEnabled} />
            </Form.Item>
          </Col>

          <Col span={8}>
            <Form.Item name="geo_store_city" label={t`Store city`} valuePropName="checked">
              <Switch disabled={!geoEnabled} />
            </Form.Item>
          </Col>

          <Col span={8}>
            <Form.Item
              name="geo_coordinates_precision"
              label={t`Coordinate precision`}
              tooltip={t`Number of decimals kept. Two decimals is roughly one kilometer.`}
              rules={[{ required: true, message: t`Please enter a coordinate precision` }]}
            >
              <InputNumber min={0} max={2} disabled={!geoEnabled} className="w-full" />
            </Form.Item>
          </Col>
        </Row>

        <Divider className="!my-8" />

        <div className="text-xl font-medium mb-2">{t`Custom dimension labels`}</div>
        <div className="text-gray-500 mb-8">
          {t`Naming a slot renames it everywhere it appears: dashboards, the explore picker and attribution rules.`}
        </div>

        <Row gutter={24}>
          {CUSTOM_DIMENSION_SLOTS.map((slot) => (
            <Col span={12} key={slot}>
              <Form.Item
                name={['custom_dimension_labels', `custom_${slot}`]}
                label={`custom_${slot}`}
              >
                <Input placeholder={t`Label`} />
              </Form.Item>
            </Col>
          ))}
        </Row>

        <Form.Item>
          <Button type="primary" htmlType="submit" loading={savingSettings} disabled={!formTouched}>
            {t`Save Changes`}
          </Button>
        </Form.Item>
      </Form>

      <Divider className="!my-8" />

      <div className="text-xl font-medium mb-2">{t`Install`}</div>
      <div className="text-gray-500 mb-4">
        {t`Paste this snippet before the closing </head> tag of your website.`}
      </div>
      <Typography.Paragraph copyable={{ text: snippet }}>
        <pre className="overflow-x-auto rounded bg-gray-50 p-3 text-xs">{snippet}</pre>
      </Typography.Paragraph>
    </>
  )
}
