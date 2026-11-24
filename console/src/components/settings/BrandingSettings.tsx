import { useState } from 'react'
import { Alert, Button, Col, Form, Input, Row, Switch, Typography, type FormInstance } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { InstanceLogo } from '../brand/InstanceLogo'
import { LicenceGateNotice } from '../license/LicenceGateNotice'
import { useLicense } from '../../hooks/useLicense'
import { previewBranding } from '../brand/brandingPreview'
import {
  MAX_BRAND_NAME_CODE_POINTS,
  validateBrandName,
  validateBrandingUrl,
  type BrandingUrlError,
  type BrandNameError
} from '../brand/brandingValidation'
import type { SystemSettingsData } from '../../types/settings'

const { Title, Text } = Typography

/** The brand values as stored, read when the form loaded: what an unlicensed save may only clear. */
export interface StoredBranding {
  name: string
  logoUrl: string
  iconUrl: string
  hideHelpMenu: boolean
}

interface BrandingSettingsProps {
  form: FormInstance<SystemSettingsData>
  stored: StoredBranding
  /** Set when the stored brand cannot be read; a save that sets or clears the brand replaces it. */
  brandingError?: string
  /** The workspace the licence notice links its button to. */
  workspaceId?: string
  /** Called when a button sets the brand fields (Clear, Reset branding), which antd does not count as touching them. */
  onBrandingEdited?: () => void
}

const trimmed = (value: unknown) => (typeof value === 'string' ? value.trim() : '')

export function BrandingSettings({
  form,
  stored,
  brandingError,
  workspaceId,
  onBrandingEdited
}: BrandingSettingsProps) {
  const { t } = useLingui()
  const { has } = useLicense()
  const licensed = has('branding')

  const name = trimmed(Form.useWatch('branding_name', form))
  const logoUrl = trimmed(Form.useWatch('branding_logo_url', form))
  const iconUrl = trimmed(Form.useWatch('branding_icon_url', form))
  const hideHelpMenu = Form.useWatch('branding_hide_help_menu', form) === true

  // The URL whose preview failed to load. The error stays under the field but never blocks the
  // save: the administrator's own browser may be the one that cannot reach the image.
  const [failedLogo, setFailedLogo] = useState('')
  const [failedIcon, setFailedIcon] = useState('')

  const nameMessages: Record<BrandNameError, string> = {
    too_long: t`Use at most ${MAX_BRAND_NAME_CODE_POINTS} characters.`,
    control: t`Remove the control or invisible characters.`
  }
  const urlMessages: Record<BrandingUrlError, string> = {
    too_long: t`This URL is too long.`,
    control: t`Remove the control characters from this URL.`,
    space: t`Remove the spaces from this URL.`,
    backslash: t`Remove the backslashes from this URL.`,
    fragment: t`Remove the part after # from this URL.`,
    not_https: t`Use an absolute https:// URL.`,
    userinfo: t`Remove the user name or password from this URL.`,
    port: t`Use the default HTTPS port.`,
    ipv6: t`Write the IPv6 address in its short form, without a zone.`,
    private: t`This address is private or reserved. Use a public one.`,
    unicode_host: t`Use the ASCII form of the domain (it starts with xn--).`,
    ipv4: t`Write the IPv4 address as four numbers from 0 to 255.`,
    not_public: t`Use a public domain name.`,
    invalid: t`This URL is not valid.`
  }

  const urlRule = {
    validator: (_rule: unknown, value: unknown) => {
      const error = validateBrandingUrl(trimmed(value))
      return error ? Promise.reject(new Error(urlMessages[error])) : Promise.resolve()
    }
  }

  // Without the licence, an empty field cannot be filled, and a filled one can only be cleared:
  // the server refuses anything else with 402. A value typed before the licence turned out to be
  // missing (a 402 on save) counts as filled, so it can be cleared too.
  const textLocked = (storedValue: string, current: string) =>
    !licensed && storedValue === '' && current === ''
  const clearOnly = (storedValue: string, current: string) =>
    !licensed && (storedValue !== '' || current !== '')

  const clearField = (field: keyof SystemSettingsData) => {
    form.setFieldValue(field, '')
    onBrandingEdited?.()
    void form.validateFields([field, 'branding_name'])
  }

  const resetIdentity = () => {
    form.setFieldsValue({
      branding_name: '',
      branding_logo_url: '',
      branding_icon_url: '',
      branding_hide_help_menu: false
    })
    onBrandingEdited?.()
    void form.validateFields(['branding_name', 'branding_logo_url', 'branding_icon_url'])
  }

  const clearButton = (field: keyof SystemSettingsData, label: string, value: string) => (
    <Button
      type="text"
      size="small"
      onClick={() => clearField(field)}
      disabled={!value}
      aria-label={t`Clear ${label}`}
    >
      {t`Clear`}
    </Button>
  )

  const preview = previewBranding({ name, logoUrl, iconUrl })

  const imageField = (
    field: 'branding_logo_url' | 'branding_icon_url',
    label: string,
    help: string,
    storedValue: string,
    failed: string,
    setFailed: (url: string) => void,
    value: string
  ) => (
    <Form.Item
      label={label}
      name={field}
      rules={[urlRule]}
      extra={
        <>
          <div>{help}</div>
          {value && failed === value ? (
            <div>
              <Text type="warning">{t`This image did not load in your browser. You can still save it.`}</Text>
            </div>
          ) : null}
        </>
      }
    >
      <Input
        placeholder="https://cdn.example.com/brand.png"
        readOnly={clearOnly(storedValue, value)}
        disabled={textLocked(storedValue, value)}
        addonAfter={clearButton(field, label, value)}
        prefix={
          value && !validateBrandingUrl(value) ? (
            <img
              src={value}
              alt=""
              referrerPolicy="no-referrer"
              onLoad={() => setFailed('')}
              onError={() => setFailed(value)}
              style={{ width: 20, height: 20, objectFit: 'contain' }}
            />
          ) : null
        }
      />
    </Form.Item>
  )

  return (
    <section aria-labelledby="branding-settings-title">
      <Title level={5} id="branding-settings-title">
        {t`Branding`}
      </Title>
      <div className="mb-4">
        <Text type="secondary">
          {t`This identity applies to the whole instance and to everyone who uses it.`}
        </Text>
      </div>

      {brandingError ? (
        <div className="mb-4">
          <Alert
            type="warning"
            showIcon
            message={t`The saved branding cannot be read`}
            description={t`To replace it, save new branding values or clear them. Saving other settings leaves it as it is.`}
          />
        </div>
      ) : null}

      <LicenceGateNotice feature="branding" workspaceId={workspaceId} />

      <Row gutter={16}>
        <Col span={12}>
          <Form.Item
            label={t`Brand name`}
            name="branding_name"
            dependencies={['branding_logo_url', 'branding_icon_url']}
            extra={t`Shown in the browser tab, in the menu when there is no logo, and in the emails this instance sends.`}
            rules={[
              {
                validator: (_rule, value) => {
                  const error = validateBrandName(trimmed(value))
                  return error ? Promise.reject(new Error(nameMessages[error])) : Promise.resolve()
                }
              },
              ({ getFieldValue }) => ({
                validator: (_rule, value) => {
                  const needsName =
                    trimmed(getFieldValue('branding_logo_url')) !== '' ||
                    trimmed(getFieldValue('branding_icon_url')) !== ''
                  return needsName && trimmed(value) === ''
                    ? Promise.reject(new Error(t`A brand name is required when a logo or an icon is set.`))
                    : Promise.resolve()
                }
              })
            ]}
          >
            <Input
              count={{
                show: true,
                max: MAX_BRAND_NAME_CODE_POINTS,
                strategy: (text) => Array.from(text).length
              }}
              readOnly={clearOnly(stored.name, name)}
              disabled={textLocked(stored.name, name)}
              addonAfter={clearButton('branding_name', t`Brand name`, name)}
            />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item
            label={t`Hide help menu`}
            name="branding_hide_help_menu"
            valuePropName="checked"
            extra={t`Removes the Help menu, whose links lead to Notifuse, from the sidebar.`}
          >
            <Switch disabled={!licensed && !hideHelpMenu} />
          </Form.Item>
        </Col>
      </Row>

      <Row gutter={16}>
        <Col span={12}>
          {imageField(
            'branding_logo_url',
            t`Main logo URL`,
            t`A transparent PNG works best. Shown in the menu, on the sign-in page and while the console loads.`,
            stored.logoUrl,
            failedLogo,
            setFailedLogo,
            logoUrl
          )}
        </Col>
        <Col span={12}>
          {imageField(
            'branding_icon_url',
            t`Brand icon URL`,
            t`A square PNG works best. Shown in the folded menu and as the browser icon.`,
            stored.iconUrl,
            failedIcon,
            setFailedIcon,
            iconUrl
          )}
        </Col>
      </Row>

      <div className="mb-2 flex items-center justify-between">
        <Text strong>{t`Preview`}</Text>
        <Button size="small" onClick={resetIdentity}>
          {t`Reset branding`}
        </Button>
      </div>
      <div className="flex flex-wrap items-center gap-4 rounded-md border border-line bg-fill p-3" data-testid="branding-preview">
        <div className="flex h-12 w-[228px] items-center rounded-md bg-surface px-5" aria-label={t`Open menu`}>
          <InstanceLogo key={`full-${preview.logo_url}`} variant="full" maxWidth={150} maxHeight={24} branding={preview} />
        </div>
        <div className="flex h-12 w-14 items-center justify-center rounded-md bg-surface" aria-label={t`Folded menu`}>
          <InstanceLogo key={`compact-${preview.icon_url}`} variant="compact" maxWidth={24} maxHeight={24} branding={preview} />
        </div>
        <div className="flex h-8 max-w-[260px] items-center gap-2 rounded-t-md bg-surface px-3 text-xs text-ink-2" aria-label={t`Browser tab`}>
          <img
            src={preview.custom ? preview.icon_url || '/console/neutral-icon.png' : '/console/favicon-32x32.png'}
            alt=""
            referrerPolicy="no-referrer"
            className="size-4 shrink-0 object-contain"
          />
          <span className="truncate">{`Console | ${preview.name}`}</span>
        </div>
      </div>
    </section>
  )
}
