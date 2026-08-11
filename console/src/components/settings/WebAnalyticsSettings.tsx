import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
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
  Switch
} from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import {
  buildInstallSnippet,
  resolveTrackingEndpoint,
  WebAnalyticsSettings as WebAnalyticsSettingsValues,
  webAnalyticsService
} from '../../services/api/web_analytics'
import { CodeSnippet } from '../common/CodeSnippet'
import { SettingsSectionHeader } from './SettingsSectionHeader'

const DEFAULT_SETTINGS: WebAnalyticsSettingsValues = {
  enabled: false,
  allowed_domains: [],
  contact_bridge_enabled: false,
  bounce_threshold_seconds: 10,
  geo_enabled: true,
  geo_store_city: true,
  geo_store_region: true,
  geo_coordinates_precision: 2
}

/** Slots the backend accepts: custom_1..custom_10. */
const CUSTOM_DIMENSION_SLOTS = Array.from({ length: 10 }, (_, index) => index + 1)

interface ToggleRowProps {
  name: string
  title: string
  description: string
}

/** Label + description on the left, switch aligned on the right. */
function ToggleRow({ name, title, description }: ToggleRowProps) {
  return (
    <div className="flex items-start justify-between gap-6">
      <div>
        <div className="font-medium">{title}</div>
        <div className="text-sm text-gray-500">{description}</div>
      </div>
      <Form.Item name={name} valuePropName="checked" noStyle>
        <Switch aria-label={title} />
      </Form.Item>
    </div>
  )
}

interface WebAnalyticsSettingsProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  canManage: boolean
}

interface WebAnalyticsFormValues {
  enabled: boolean
  allowed_domains?: string[]
  bounce_threshold_seconds?: number
  contact_bridge_enabled: boolean
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
      contact_bridge_enabled: settings.contact_bridge_enabled ?? false,
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
  const endpoint = useMemo(() => resolveTrackingEndpoint(workspace), [workspace])

  const handleSaveSettings = async () => {
    if (!workspace) return

    // The nested geo fields are unmounted while geo tracking is off, and
    // onFinish only reports mounted fields — reading the whole store keeps the
    // hidden city/region/precision choices instead of resetting them to the
    // defaults on the next save.
    const values = form.getFieldsValue(true) as WebAnalyticsFormValues

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

  const identifySnippet =
    'NotifuseAnalytics.identify("alice@example.com", "<hmac from your server>")'

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

        <div className="text-xl font-medium mb-8">{t`Contact timeline`}</div>

        <Form.Item
          name="contact_bridge_enabled"
          label={t`Record web goals on the contact timeline`}
          valuePropName="checked"
          tooltip={t`Only goals from visitors identified with identify() are recorded, and only when the address already belongs to a contact. Recorded goals can trigger automations and change segment membership. Obtaining consent to store web activity against a contact is your responsibility.`}
        >
          <Switch />
        </Form.Item>

        <Divider className="!my-8" />

        <div className="text-xl font-medium mb-8">{t`Geographic data collection`}</div>

        <div className="space-y-4">
          <ToggleRow
            name="geo_enabled"
            title={t`Enable geo-location tracking`}
            description={t`Track visitor country, region, city, and coordinates`}
          />

          {geoEnabled && (
            <div className="ml-6 space-y-4 border-l-2 border-gray-100 pl-4">
              <ToggleRow
                name="geo_store_city"
                title={t`Store city name`}
                description={t`Record the city of visitors`}
              />

              <ToggleRow
                name="geo_store_region"
                title={t`Store region/state name`}
                description={t`Record the region or state of visitors`}
              />

              <div>
                <div className="font-medium">{t`Coordinates precision`}</div>
                <div className="mb-2 text-sm text-gray-500">{t`Lower precision = more privacy`}</div>
                <Form.Item name="geo_coordinates_precision" noStyle>
                  <Select
                    aria-label={t`Coordinates precision`}
                    className="w-full"
                    options={[
                      { value: 0, label: t`Country level (~111km precision)` },
                      { value: 1, label: t`Regional (~11km precision)` },
                      { value: 2, label: t`City level (~1km precision)` }
                    ]}
                  />
                </Form.Item>
              </div>
            </div>
          )}
        </div>

        <Alert
          className="!mt-6"
          type="info"
          title={t`IP addresses are never stored — only used for geo lookup. Country is always included when geo tracking is enabled.`}
        />

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
      <CodeSnippet code={snippet} language="markup" />

      <div className="text-xl font-medium mb-2 mt-8">{t`Identify a visitor`}</div>
      <div className="text-gray-500 mb-4">
        {t`Call identify() once you know who the visitor is. The signature must be computed on your server with your workspace secret key — the tracking endpoint is public, so an unsigned address is ignored.`}
      </div>
      <CodeSnippet code={identifySnippet} language="javascript" />
      <div className="text-gray-500">
        {t`The address must already belong to a contact: identifying someone who is not one records nothing, by design. Visitors arriving from a tracked email link are identified automatically, with no code.`}
      </div>
    </>
  )
}
