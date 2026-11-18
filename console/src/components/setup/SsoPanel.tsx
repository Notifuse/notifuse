import { useState } from 'react'
import { App, Button, Form, Input, Select, Space, Switch } from 'antd'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { HelpTip } from '../layout'
import type { SetupStatus } from '../../types/setup'
import { LockableItem } from './LockableItem'
import { EnvLockedBody, LockedFieldNote, SetupPanel } from './SetupPanel'
import { defaultRedirectUri, type SetupPanelState } from './setupHooks'
import { envLabel, isFieldLocked, isFieldRequired, isPanelLocked, type SetupFormValues } from './setupFields'

export function SsoPanel({ status, state }: { status: SetupStatus; state: SetupPanelState }) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const form = Form.useFormInstance<SetupFormValues>()
  const locked = isPanelLocked(status, 'sso')
  const [advanced, setAdvanced] = useState(false)

  const enabled = Form.useWatch('oidc_enabled', form) === true
  // Set by the environment, auto-create may be on: the domains field then shows, the server
  // requiring it when the variable is true.
  const autoCreateLocked = isFieldLocked(status, 'oidc_auto_create_users')
  const autoCreate = Form.useWatch('oidc_auto_create_users', form) === true
  const apiEndpoint = Form.useWatch('api_endpoint', form)
  const redirectUri = Form.useWatch('oidc_redirect_uri', form)
  const values: SetupFormValues = { oidc_enabled: enabled, oidc_auto_create_users: autoCreate }
  const required = (key: 'oidc_issuer_url' | 'oidc_client_id' | 'oidc_client_secret', text: string) =>
    isFieldRequired(status, key, values) ? [{ required: true, message: text }] : []

  const derived = defaultRedirectUri(apiEndpoint)
  const copyRedirectUri = async () => {
    try {
      await navigator.clipboard.writeText(redirectUri?.trim() || derived)
      message.success(t`Redirect URI copied`)
    } catch {
      message.error(t`Could not copy the redirect URI`)
    }
  }

  const toggle = (
    <Form.Item name="oidc_enabled" valuePropName="checked" noStyle>
      <Switch aria-label={t`Enable single sign-on`} />
    </Form.Item>
  )

  return (
    <SetupPanel
      id="sso"
      number={4}
      title={t`Single sign-on (OpenID Connect)`}
      help={{
        label: t`About single sign-on`,
        content: t`Optional. Lets people sign in with an identity provider (Google Workspace, Keycloak, Okta…) besides login codes.`
      }}
      state={state}
      extra={locked ? undefined : toggle}
    >
      {locked ? (
        <EnvLockedBody status={status} panel="sso" />
      ) : enabled ? (
        <>
          <div className="grid gap-x-3.5 sm:grid-cols-2">
            <LockableItem
              status={status}
              name="oidc_issuer_url"
              label={t`Issuer URL`}
              help={t`The provider's base URL. Its settings are read from /.well-known/openid-configuration under it.`}
              rules={[
                ...required('oidc_issuer_url', t`Issuer URL is required`),
                { type: 'url', message: t`Must be a valid https URL` }
              ]}
            >
              <Input placeholder="https://accounts.google.com" />
            </LockableItem>
            <LockableItem
              status={status}
              name="oidc_client_id"
              label={t`Client ID`}
              rules={required('oidc_client_id', t`Client ID is required`)}
            >
              <Input placeholder="xxxxxxxx.apps.googleusercontent.com" />
            </LockableItem>
            <LockableItem
              status={status}
              name="oidc_client_secret"
              label={t`Client secret`}
              rules={required('oidc_client_secret', t`Client secret is required`)}
            >
              <Input.Password autoComplete="new-password" />
            </LockableItem>
            <LockableItem
              status={status}
              name="oidc_button_label"
              label={t`Button label`}
              help={t`Text of the button on the sign-in page.`}
            >
              <Input placeholder={t`Sign in with SSO`} />
            </LockableItem>
          </div>
          {isFieldLocked(status, 'oidc_redirect_uri') ? (
            <LockableItem status={status} name="oidc_redirect_uri" label={t`Redirect URI`}>
              <Input />
            </LockableItem>
          ) : (
            <Form.Item
              label={
                <HelpTip title={t`Redirect URI`} label={t`About the redirect URI`}>
                  {t`Register this address with your identity provider. Left empty, it follows the API endpoint.`}
                </HelpTip>
              }
            >
              <Space.Compact block>
                <Form.Item name="oidc_redirect_uri" noStyle rules={[{ type: 'url', message: t`Invalid URL format` }]}>
                  <Input placeholder={derived} aria-label={t`Redirect URI`} />
                </Form.Item>
                <Button onClick={copyRedirectUri}>{t`Copy`}</Button>
              </Space.Compact>
            </Form.Item>
          )}
          <Button
            type="link"
            size="small"
            aria-expanded={advanced}
            icon={
              advanced ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />
            }
            onClick={() => setAdvanced((open) => !open)}
          >
            {t`Advanced`}
          </Button>
          {/* Hidden, not unmounted: a value typed here must still be sent once the section is closed. */}
          <div hidden={!advanced} className="mt-2 sm:w-1/2">
            <LockableItem
              status={status}
              name="oidc_scopes"
              label={t`Scopes`}
              help={t`Separated by spaces. openid is always included. Unverified emails are refused unless OIDC_ALLOW_UNVERIFIED_EMAIL is set.`}
            >
              <Input placeholder="openid email profile" />
            </LockableItem>
          </div>
          <div className="mt-2 flex items-start justify-between gap-4 border-t border-line-soft pt-3">
            <div className="flex min-w-0 flex-col">
              <span className="text-sm font-medium text-ink">{t`Auto-create accounts on first sign-in`}</span>
              <span id="setup-oidc-auto-create-hint" className="text-xs text-ink-3">
                {t`Off (recommended): only invited people can sign in with SSO. On: a verified email in an allowed domain gets an account.`}
              </span>
            </div>
            {isFieldLocked(status, 'oidc_auto_create_users') ? (
              <LockedFieldNote envName={envLabel('oidc_auto_create_users')} />
            ) : (
              <Form.Item name="oidc_auto_create_users" valuePropName="checked" noStyle>
                <Switch
                  aria-label={t`Auto-create accounts on first sign-in`}
                  aria-describedby="setup-oidc-auto-create-hint"
                />
              </Form.Item>
            )}
          </div>
          {autoCreate || autoCreateLocked ? (
            <div className="mt-3 border-l-2 border-primary-300 pl-3.5">
              <LockableItem
                status={status}
                name="oidc_allowed_domains"
                label={t`Allowed email domains`}
                rules={
                  isFieldRequired(status, 'oidc_allowed_domains', values)
                    ? [{ required: true, message: t`At least one domain is required when auto-create is enabled` }]
                    : []
                }
              >
                <Select
                  mode="tags"
                  open={false}
                  suffixIcon={null}
                  tokenSeparators={[',', ';', ' ']}
                  placeholder="example.com"
                  aria-label={t`Allowed email domains`}
                />
              </LockableItem>
            </div>
          ) : null}
        </>
      ) : null}
    </SetupPanel>
  )
}
