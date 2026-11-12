import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { Form, Input, Select, App, Switch, Segmented, Tooltip } from 'antd'
import { Info } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'
import { LANGUAGE_OPTIONS } from '../../lib/languages'
import { LogoInput } from './LogoInput'
import { SettingsSaveBar } from './SettingsSaveBar'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { SettingsRow, SettingsRows } from './SettingsRow'
import { KeyValueList, Mono, Panel, Pill } from '../layout'
import { WorkspaceBadge } from '../../layouts/app-shell/WorkspaceSwitcher'
import { WORKSPACE_NAME_MAX_LENGTH } from '../../constants/limits'
import { getEmailTrackingPolicy, TRACKING_DOCS_URL } from '../../lib/emailTracking'
import type { UnknownTrackingConsent } from '../../services/api/workspace'

interface GeneralSettingsProps {
  workspace: Workspace | null
  onWorkspaceUpdate: (workspace: Workspace) => void
  isOwner: boolean
}

interface GeneralSettingsFormValues {
  name: string
  website_url?: string
  logo_url?: string
  timezone: string
  opens_tracking_enabled: boolean
  clicks_tracking_enabled: boolean
  unknown_consent: UnknownTrackingConsent
  custom_endpoint_url?: string
  languages?: string[]
  default_language?: string
}

/**
 * The stored workspace as the form holds it. Both the initial load and Discard
 * read from here, so "restore what was saved" can never drift from "what was
 * loaded".
 */
function toFormValues(workspace: Workspace | null): GeneralSettingsFormValues {
  const tracking = getEmailTrackingPolicy(workspace?.settings)
  return {
    name: workspace?.name || '',
    website_url: workspace?.settings.website_url || '',
    logo_url: workspace?.settings.logo_url || '',
    timezone: workspace?.settings.timezone || 'UTC',
    opens_tracking_enabled: tracking.opens_enabled,
    clicks_tracking_enabled: tracking.clicks_enabled,
    unknown_consent: tracking.unknown_consent,
    custom_endpoint_url: workspace?.settings.custom_endpoint_url || '',
    languages: workspace?.settings.languages || ['en'],
    default_language: workspace?.settings.default_language || 'en'
  }
}

/** The host of a URL, or the fallback when it is not one. */
function hostOf(url: string | undefined, fallback: string): string {
  if (!url) return fallback
  try {
    return new URL(url).hostname
  } catch {
    return fallback
  }
}

const languageLabel = (code: string) =>
  LANGUAGE_OPTIONS.find((option) => option.value === code)?.label ?? code

export function GeneralSettings({ workspace, onWorkspaceUpdate, isOwner }: GeneralSettingsProps) {
  const { t } = useLingui()
  const [savingSettings, setSavingSettings] = useState(false)
  const [formTouched, setFormTouched] = useState(false)
  const [form] = Form.useForm<GeneralSettingsFormValues>()
  const { message } = App.useApp()

  useEffect(() => {
    // Only set form values if user is owner (form will be rendered)
    if (!isOwner) return

    // Set form values from workspace data whenever workspace changes
    form.setFieldsValue(toFormValues(workspace))
    setFormTouched(false)
  }, [workspace, form, isOwner])

  // resetFields() would empty the form: the values arrive through
  // setFieldsValue above, so the form has no initialValues to reset to.
  const handleDiscard = useCallback(() => {
    form.setFieldsValue(toFormValues(workspace))
    setFormTouched(false)
  }, [form, workspace])

  const handleSaveSettings = async (values: GeneralSettingsFormValues) => {
    if (!workspace) return

    setSavingSettings(true)
    try {
      await workspaceService.update({
        ...workspace,
        name: values.name,
        settings: {
          ...workspace.settings,
          website_url: values.website_url,
          logo_url: (values.logo_url as string | null | undefined) || null,
          cover_url: workspace?.settings.cover_url || null,
          timezone: values.timezone,
          // The single switch stays in step for readers that only know it.
          email_tracking_enabled: values.opens_tracking_enabled || values.clicks_tracking_enabled,
          email_tracking: {
            opens_enabled: values.opens_tracking_enabled,
            clicks_enabled: values.clicks_tracking_enabled,
            unknown_consent: values.unknown_consent
          },
          // Emptied means emptied. workspaces.update keeps every setting its body leaves
          // out, and undefined is dropped by JSON.stringify — so sending it restored the
          // very domain the operator was migrating off. An empty string is a value the
          // body carries, and both the server and every reader here treat it as unset.
          custom_endpoint_url: values.custom_endpoint_url || '',
          languages: values.languages || ['en'],
          default_language: values.default_language || 'en'
        }
      })

      // Refresh the workspace data
      const response = await workspaceService.get(workspace.id)

      // Update the parent component with the new workspace data
      onWorkspaceUpdate(response.workspace)

      setFormTouched(false)
      message.success(t`Workspace settings updated successfully`)
    } catch (error: unknown) {
      console.error('Failed to update workspace settings', error)
      // Extract the actual error message from the API response
      const errorMessage = (error as Error)?.message || t`Failed to update workspace settings`
      message.error(errorMessage)
    } finally {
      setSavingSettings(false)
    }
  }

  const handleFormChange = () => {
    setFormTouched(true)
  }

  const trackingPolicy = getEmailTrackingPolicy(workspace?.settings)
  const storedDomain = workspace?.settings.custom_endpoint_url || ''
  const watchedLanguages = Form.useWatch('languages', form)
  const watchedDomain = Form.useWatch('custom_endpoint_url', form)
  const watchedName = Form.useWatch('name', form)

  const header = (
    <SettingsSectionHeader
      title={t`General`}
      description={t`Workspace identity, languages and email tracking`}
    />
  )

  const consentLabel = (
    <span className="inline-flex items-center gap-1">
      {t`Contacts without recorded consent`}
      <Tooltip
        title={t`A contact's own consent always wins. This choice applies to contacts whose consent to open and click tracking is not recorded. Some regulators, such as the Italian Garante and the French CNIL, expect tracking to require consent.`}
      >
        <Info size={13} aria-label={t`About consent`} className="text-ink-3" />
      </Tooltip>
    </span>
  )
  const consentHelp = (
    <>
      {t`A contact's own consent always wins.`}{' '}
      <a href={TRACKING_DOCS_URL} target="_blank" rel="noopener noreferrer">
        {t`Learn more`}
      </a>
    </>
  )

  const websiteHelp = t`Linked from the notification center and available to templates; also used to find your logo`
  const logoHelp = t`A square image, shown next to the workspace name`
  const domainHelp = t`Used in email links, the notification center and web pages. Leave empty to use the API endpoint`

  const domainPill = storedDomain ? (
    <Pill tone="neutral" dot={false}>
      {t`In use`}
    </Pill>
  ) : null

  const dnsRecord = (domain: string | undefined) => (
    <div data-dns-record className="mt-1 flex flex-col gap-2 rounded-md bg-subtle p-3">
      <div className="text-sm font-medium text-ink">{t`DNS record`}</div>
      <KeyValueList
        labelWidth={80}
        items={[
          { label: t`Type`, value: <Mono copy>CNAME</Mono> },
          { label: t`Name`, value: <Mono copy>{hostOf(domain, 'blog.yourdomain.com')}</Mono> },
          {
            label: t`Value`,
            value: (
              <Mono copy>
                {hostOf(window.API_ENDPOINT || 'http://localhost:3000', 'your-api-endpoint.com')}
              </Mono>
            )
          }
        ]}
      />
      <div className="text-xs text-ink-3">
        {t`Configure a custom domain for email links, notification center, and web publications. DNS verification will be performed before saving to ensure you control this domain.`}{' '}
        {t`DNS verification prevents domain squatting`}
      </div>
    </div>
  )

  const onOff = (enabled: boolean) => (enabled ? t`On` : t`Off`)
  const readValue = (value: ReactNode) => <div className="text-sm text-ink">{value}</div>

  if (!isOwner) {
    // The same rows, with the values as text for members who cannot change them.
    const settings = workspace?.settings
    const languages = settings?.languages?.length ? settings.languages : ['en']
    return (
      <>
        {header}
        <Panel title={t`Workspace`}>
          <SettingsRows>
            <SettingsRow label={t`Name`}>{readValue(workspace?.name || t`Not set`)}</SettingsRow>
            <SettingsRow label={t`Website`} help={websiteHelp}>
              {readValue(
                settings?.website_url ? (
                  <a href={settings.website_url} target="_blank" rel="noopener noreferrer">
                    {settings.website_url}
                  </a>
                ) : (
                  t`Not set`
                )
              )}
            </SettingsRow>
            <SettingsRow label={t`Logo`} help={logoHelp}>
              {settings?.logo_url ? (
                <img
                  src={settings.logo_url}
                  alt={t`Workspace logo`}
                  className="size-10 rounded-md border border-line bg-surface object-contain"
                />
              ) : (
                <WorkspaceBadge workspace={{ name: workspace?.name ?? '' }} large />
              )}
            </SettingsRow>
          </SettingsRows>
        </Panel>
        <Panel title={t`Language & timezone`}>
          <SettingsRows>
            <SettingsRow label={t`Timezone`} help={t`Scheduled sends and statistics use it`}>
              {readValue(settings?.timezone || 'UTC')}
            </SettingsRow>
            <SettingsRow
              label={t`Supported languages`}
              help={t`One version of each template per language`}
            >
              <div className="flex flex-wrap gap-1">
                {languages.map((code) => (
                  <Pill key={code} tone="neutral" dot={false}>
                    {languageLabel(code)}
                  </Pill>
                ))}
              </div>
            </SettingsRow>
            <SettingsRow
              label={t`Default language`}
              help={t`For contacts without a known language`}
            >
              {readValue(languageLabel(settings?.default_language || 'en'))}
            </SettingsRow>
          </SettingsRows>
        </Panel>
        <Panel title={t`Email tracking`}>
          <SettingsRows>
            <SettingsRow
              label={t`Opens`}
              help={t`An invisible image in each email records when it is opened`}
            >
              {readValue(onOff(trackingPolicy.opens_enabled))}
            </SettingsRow>
            <SettingsRow
              label={t`Clicks`}
              help={t`Links go through a redirect that records each click`}
            >
              {readValue(onOff(trackingPolicy.clicks_enabled))}
            </SettingsRow>
            <SettingsRow label={consentLabel} help={consentHelp}>
              {readValue(
                trackingPolicy.unknown_consent === 'no_track' ? t`Not tracked` : t`Tracked`
              )}
            </SettingsRow>
          </SettingsRows>
        </Panel>
        <Panel title={t`Custom domain`} extra={domainPill}>
          <SettingsRows>
            <SettingsRow label={t`Domain`} help={domainHelp}>
              {readValue(storedDomain || t`Default (API endpoint)`)}
            </SettingsRow>
          </SettingsRows>
        </Panel>
      </>
    )
  }

  return (
    <>
      {header}

      <Form
        form={form}
        layout="vertical"
        onFinish={handleSaveSettings}
        onValuesChange={handleFormChange}
        // The save control floats over the page, so a rejected field can sit
        // anywhere off screen when it is pressed.
        scrollToFirstError
      >
        <div className="flex flex-col gap-3">
          <Panel title={t`Workspace`}>
            <SettingsRows>
              <SettingsRow label={t`Name`} htmlFor="name">
                <Form.Item
                  name="name"
                  rules={[
                    { required: true, message: t`Please enter workspace name` },
                    {
                      max: WORKSPACE_NAME_MAX_LENGTH,
                      message: t`Workspace name must be at most ${WORKSPACE_NAME_MAX_LENGTH} characters long`
                    }
                  ]}
                >
                  <Input
                    placeholder={t`Enter workspace name`}
                    maxLength={WORKSPACE_NAME_MAX_LENGTH}
                    showCount
                  />
                </Form.Item>
              </SettingsRow>

              <SettingsRow label={t`Website`} help={websiteHelp} htmlFor="website_url">
                <Form.Item
                  name="website_url"
                  rules={[{ type: 'url' as const, message: t`Please enter a valid URL` }]}
                >
                  <Input placeholder="https://example.com" />
                </Form.Item>
              </SettingsRow>

              <SettingsRow label={t`Logo`} help={logoHelp}>
                <Form.Item
                  name="logo_url"
                  rules={[{ type: 'url' as const, message: t`Please enter a valid URL` }]}
                >
                  <LogoInput workspaceName={watchedName || workspace?.name} />
                </Form.Item>
              </SettingsRow>
            </SettingsRows>
          </Panel>

          <Panel title={t`Language & timezone`}>
            <SettingsRows>
              <SettingsRow
                label={t`Timezone`}
                help={t`Scheduled sends and statistics use it`}
                htmlFor="timezone"
              >
                <Form.Item
                  name="timezone"
                  rules={[{ required: true, message: t`Please select a timezone` }]}
                >
                  <Select options={TIMEZONE_OPTIONS} showSearch optionFilterProp="label" />
                </Form.Item>
              </SettingsRow>

              <SettingsRow
                label={t`Supported languages`}
                help={t`One version of each template per language`}
                htmlFor="languages"
              >
                <Form.Item name="languages">
                  <Select
                    mode="multiple"
                    options={LANGUAGE_OPTIONS}
                    showSearch
                    optionFilterProp="label"
                    placeholder={t`Select supported languages`}
                    onChange={(values: string[]) => {
                      const currentDefault = form.getFieldValue('default_language')
                      if (currentDefault && !values.includes(currentDefault)) {
                        form.setFieldsValue({ default_language: values[0] || 'en' })
                      }
                      handleFormChange()
                    }}
                  />
                </Form.Item>
              </SettingsRow>

              <SettingsRow
                label={t`Default language`}
                help={t`For contacts without a known language`}
                htmlFor="default_language"
              >
                <Form.Item name="default_language">
                  <Select
                    options={(watchedLanguages?.length ? watchedLanguages : ['en']).map(
                      (code: string) => {
                        const opt = LANGUAGE_OPTIONS.find((o) => o.value === code)
                        return opt || { label: code, value: code }
                      }
                    )}
                    showSearch
                    optionFilterProp="label"
                  />
                </Form.Item>
              </SettingsRow>
            </SettingsRows>
          </Panel>

          <Panel title={t`Email tracking`}>
            <SettingsRows>
              <SettingsRow
                label={t`Opens`}
                help={t`An invisible image in each email records when it is opened`}
                htmlFor="opens_tracking_enabled"
              >
                <Form.Item name="opens_tracking_enabled" valuePropName="checked">
                  <Switch />
                </Form.Item>
              </SettingsRow>

              <SettingsRow
                label={t`Clicks`}
                help={t`Links go through a redirect that records each click`}
                htmlFor="clicks_tracking_enabled"
              >
                <Form.Item name="clicks_tracking_enabled" valuePropName="checked">
                  <Switch />
                </Form.Item>
              </SettingsRow>

              <SettingsRow label={consentLabel} help={consentHelp}>
                <Form.Item name="unknown_consent">
                  <Segmented
                    options={[
                      { label: t`Track`, value: 'track' },
                      { label: t`Don't track`, value: 'no_track' }
                    ]}
                  />
                </Form.Item>
              </SettingsRow>
            </SettingsRows>
          </Panel>

          <Panel title={t`Custom domain`} extra={domainPill}>
            <SettingsRows>
              <SettingsRow label={t`Domain`} help={domainHelp} htmlFor="custom_endpoint_url">
                <Form.Item
                  name="custom_endpoint_url"
                  rules={[{ type: 'url' as const, message: t`Please enter a valid URL` }]}
                >
                  <Input placeholder="https://api.yourdomain.com" />
                </Form.Item>
                {dnsRecord(watchedDomain)}
              </SettingsRow>
            </SettingsRows>
          </Panel>
        </div>
      </Form>

      {/* Last child on purpose — see SettingsSaveBar for why. */}
      <SettingsSaveBar
        dirty={isOwner && formTouched}
        saving={savingSettings}
        onSave={() => form.submit()}
        onDiscard={handleDiscard}
        leaveWarning={t`Your workspace settings have not been saved. Leaving this page discards them.`}
      />
    </>
  )
}
