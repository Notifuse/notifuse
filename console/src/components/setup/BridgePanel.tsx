import { Form, Input, Switch } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { SetupStatus } from '../../types/setup'
import { LockableItem } from './LockableItem'
import { EnvLockedBody, SetupPanel } from './SetupPanel'
import { type SetupPanelState } from './setupHooks'
import { isFieldRequired, isPanelLocked, type SetupFormValues } from './setupFields'

export function BridgePanel({ status, state }: { status: SetupStatus; state: SetupPanelState }) {
  const { t } = useLingui()
  const form = Form.useFormInstance<SetupFormValues>()
  const locked = isPanelLocked(status, 'bridge')
  const enabled = Form.useWatch('smtp_bridge_enabled', form) === true
  const values: SetupFormValues = { smtp_bridge_enabled: enabled }
  const required = (
    key: 'smtp_bridge_domain' | 'smtp_bridge_tls_cert_base64' | 'smtp_bridge_tls_key_base64',
    text: string
  ) => (isFieldRequired(status, key, values) ? [{ required: true, message: text }] : [])

  const toggle = (
    <Form.Item name="smtp_bridge_enabled" valuePropName="checked" noStyle>
      <Switch aria-label={t`Enable the SMTP bridge`} />
    </Form.Item>
  )

  return (
    <SetupPanel
      id="bridge"
      number={5}
      title={t`SMTP bridge`}
      help={{
        label: t`About the SMTP bridge`,
        content: t`Optional. Receives emails on this server and turns them into transactional notifications. Needs a TLS certificate for its domain; the TLS mode is set with SMTP_BRIDGE_TLS.`
      }}
      state={state}
      extra={locked ? undefined : toggle}
    >
      {locked ? (
        <EnvLockedBody status={status} panel="bridge" />
      ) : enabled ? (
        <>
          <div className="sm:w-1/2">
            <LockableItem
              status={status}
              name="smtp_bridge_domain"
              label={t`Domain`}
              help={t`The bridge listens on the port set by SMTP_BRIDGE_PORT, 587 by default: the port Docker publishes.`}
              rules={required('smtp_bridge_domain', t`SMTP bridge domain is required`)}
            >
              <Input placeholder="smtp.yourcompany.com" />
            </LockableItem>
          </div>
          <div className="grid gap-x-3.5 sm:grid-cols-2">
            <LockableItem
              status={status}
              name="smtp_bridge_tls_cert_base64"
              label={t`TLS certificate (Base64)`}
              help={t`Base64 of the full chain: cat fullchain.pem | base64 -w 0`}
              rules={required('smtp_bridge_tls_cert_base64', t`TLS certificate is required`)}
            >
              <Input.TextArea rows={3} placeholder="LS0tLS1CRUdJTi..." />
            </LockableItem>
            <LockableItem
              status={status}
              name="smtp_bridge_tls_key_base64"
              label={t`TLS private key (Base64)`}
              help={t`Base64 of the private key: cat privkey.pem | base64 -w 0`}
              rules={required('smtp_bridge_tls_key_base64', t`TLS private key is required`)}
            >
              <Input.TextArea rows={3} placeholder="LS0tLS1CRUdJTi..." />
            </LockableItem>
          </div>
        </>
      ) : null}
    </SetupPanel>
  )
}
