import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert,
  App,
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
import { SettingsSaveBar } from './SettingsSaveBar'
import { SettingsSectionHeader } from './SettingsSectionHeader'

const DEFAULT_SETTINGS: WebAnalyticsSettingsValues = {
  enabled: false,
  allowed_domains: [],
  identify_from_email_links: false,
  bounce_threshold_seconds: 10,
  geo_enabled: true,
  geo_store_city: true,
  geo_store_region: true,
  geo_coordinates_precision: 2
}

/** Slots the backend accepts: custom_1..custom_10. */
const CUSTOM_DIMENSION_SLOTS = Array.from({ length: 10 }, (_, index) => index + 1)

/**
 * How many coordinate decimals the name toggles actually allow.
 *
 * A coordinate is a place name in another form, so the server never stores one
 * finer than the finest name the workspace agreed to keep: city -> 2 decimals,
 * region only -> 1, neither -> 0. Mirrors EffectiveGeoCoordsPrecision in
 * internal/domain/web_analytics.go, which applies it on every write.
 *
 * The picked precision is a ceiling these toggles can lower, never raise — and
 * it is only ever explained here, never written back down, because the server
 * keeps honouring the stored value the moment the name toggle returns.
 */
function geoPrecisionCeiling(storeCity?: boolean, storeRegion?: boolean): number {
  if (storeCity) return 2
  if (storeRegion) return 1
  return 0
}

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

/**
 * The stored settings as the form holds them. Both the initial load and Discard
 * read from here, so "restore what was saved" can never drift from "what was
 * loaded" — every custom-dimension slot is materialised, blank ones included,
 * because the save path is what strips them again.
 */
function toFormValues(stored?: WebAnalyticsSettingsValues): WebAnalyticsFormValues {
  const settings = { ...DEFAULT_SETTINGS, ...(stored ?? {}) }
  return {
    enabled: settings.enabled,
    allowed_domains: settings.allowed_domains ?? [],
    identify_from_email_links: settings.identify_from_email_links ?? false,
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
  }
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
  identify_from_email_links: boolean
  geo_enabled: boolean
  geo_store_city: boolean
  geo_store_region: boolean
  geo_coordinates_precision: number
  custom_dimension_labels?: Record<string, string>
}

// This panel is readable by any member who can reach /settings, and that is the
// intended behaviour, not a missing gate: a settings *summary* is readable, only
// writes are permission-gated. General and Blog work the same way, so singling this
// one out would make the settings area inconsistent for no reason a user could infer.
// Nothing here is a credential — the workspace secret is never serialised and
// integration credentials are stripped at the API boundary.
//
// If that stance is ever reversed, it has to be backend-first and land with the
// console in the same change: these values arrive in /api/user.me, so hiding the
// panel alone is theatre, and with the settings absent this component falls back to
// DEFAULT_SETTINGS and confidently renders a plausible, wrong configuration.
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
  const geoStoreCity = Form.useWatch('geo_store_city', form)
  const geoStoreRegion = Form.useWatch('geo_store_region', form)
  const geoPrecision = Form.useWatch('geo_coordinates_precision', form)

  useEffect(() => {
    if (!canManage) return
    form.setFieldsValue(toFormValues(stored))
    setFormTouched(false)
  }, [stored, form, canManage])

  // resetFields() would empty the form: the values arrive through
  // setFieldsValue above, so the form has no initialValues to reset to.
  const handleDiscard = useCallback(() => {
    form.setFieldsValue(toFormValues(stored))
    setFormTouched(false)
  }, [form, stored])

  const hasUnsavedChanges = canManage && formTouched

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

  // The picker lists every precision, but the name toggles can put the finer
  // ones out of reach — without this the form states a precision the stored
  // coordinates will not have. Distances mirror the option labels below.
  const precisionCapped =
    geoPrecision !== undefined && geoPrecision > geoPrecisionCeiling(geoStoreCity, geoStoreRegion)
  const precisionCapNotice = !precisionCapped
    ? null
    : geoStoreRegion
      ? t`Store city name is off, so coordinates are stored at regional precision (~11km).`
      : t`Store city name and Store region/state name are off, so coordinates are stored at country precision (~111km).`

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
            {stored?.allowed_domains?.length ? (
              stored.allowed_domains.join(', ')
            ) : (
              // An empty list is not simply permissive: it accepts beats from
              // anywhere, but it also switches OFF email-click identification,
              // because a tracked link only carries an identity token for a
              // listed domain. Saying just "Every domain" reads as "nothing to
              // configure" and hides that second half entirely.
              <>
                {t`Every domain`}
                <div style={{ color: '#faad14', fontSize: 12 }}>
                  {t`Email links cannot identify contacts until a domain is listed.`}
                </div>
              </>
            )}
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
        // The save control floats over the page, so a rejected field can sit
        // anywhere off screen when it is pressed.
        scrollToFirstError
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
          tooltip={t`This list does two things. It filters incoming beats — other origins are silently ignored. It also gates email-click identification: a tracked link only carries an identity token for a domain listed here, so while this is empty no email link can identify a contact. Use *.example.com to cover both example.com and its subdomains — example.com on its own does not match www.example.com.`}
          // Re-runs the rule below the moment collection is switched on, rather
          // than waiting for the save to bounce.
          dependencies={['enabled']}
          rules={[
            ({ getFieldValue }) => ({
              // Read through the form rather than a watched value: this has to
              // hold at submit time whatever the render order was.
              validator: (_, value: string[] | undefined) =>
                !getFieldValue('enabled') || (value?.length ?? 0) > 0
                  ? Promise.resolve()
                  : Promise.reject(
                      new Error(t`List at least one domain to enable web analytics`)
                    )
            })
          ]}
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

        <div className="text-xl font-medium mb-8">{t`Contact identification`}</div>

        <Form.Item
          name="identify_from_email_links"
          label={t`Identify recipients who click a tracked email link`}
          valuePropName="checked"
          tooltip={t`Adds a signed identity to the links of tracked emails, so a recipient who clicks one is recognised on landing without any code on your site. Their visit is then tied to their contact record — timeline entries, goals and automation enrolments. Unlike identify(), which your own server calls, this one is minted by Notifuse for every recipient of every tracked send, which is why it is off by default.`}
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
                {precisionCapNotice && (
                  <div className="mt-2 text-sm text-amber-600">{precisionCapNotice}</div>
                )}
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
        {t`An address that is not a contact yet becomes one, carrying the country, language and timezone the visit reported; an existing contact is never modified. Visitors arriving from a tracked email link are identified automatically, with no code.`}
      </div>

      {/* Last child on purpose — see SettingsSaveBar for why. */}
      <SettingsSaveBar
        dirty={hasUnsavedChanges}
        saving={savingSettings}
        onSave={() => form.submit()}
        onDiscard={handleDiscard}
        leaveWarning={t`Your web analytics settings have not been saved. Leaving this page discards them.`}
      />
    </>
  )
}
