import { useEffect, useState } from 'react'
import { Alert, Button, Form, Input, InputNumber, Switch, Tooltip } from 'antd'
import { ChevronDown, ChevronRight, Zap } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { Pill } from '../layout'
import { setupApi } from '../../services/api/setup'
import type { SetupStatus } from '../../types/setup'
import { LockableItem } from './LockableItem'
import { EnvLockedBody, SetupPanel } from './SetupPanel'
import { useTlsCaption, type SetupPanelState } from './setupHooks'
import { isFieldLocked, isPanelLocked, type SetupFormValues } from './setupFields'

type TestResult = { ok: true; target: string; at: Date } | { ok: false; message: string }

const DOCS = [
  { name: 'Amazon SES', href: 'https://docs.aws.amazon.com/ses/latest/dg/smtp-credentials.html' },
  { name: 'Mailgun', href: 'https://documentation.mailgun.com/docs/mailgun/user-manual/sending-messages/send-smtp' },
  { name: 'SparkPost', href: 'https://developers.sparkpost.com/api/smtp/' },
  { name: 'Postmark', href: 'https://postmarkapp.com/developer/user-guide/send-email-with-smtp' }
]

export interface SmtpPanelProps {
  status: SetupStatus
  state: SetupPanelState
  /** Told whether the last test failed, so that the summary says so too. */
  onTestFailedChange?: (failed: boolean) => void
}

export function SmtpPanel({ status, state, onTestFailedChange }: SmtpPanelProps) {
  const { t } = useLingui()
  const form = Form.useFormInstance<SetupFormValues>()
  const locked = isPanelLocked(status, 'smtp')
  const [advanced, setAdvanced] = useState(false)
  const [testing, setTesting] = useState(false)
  const [result, setResult] = useState<TestResult | null>(null)

  const host = Form.useWatch('smtp_host', form)
  const port = Form.useWatch('smtp_port', form)
  const username = Form.useWatch('smtp_username', form)
  const password = Form.useWatch('smtp_password', form)
  const useTls = Form.useWatch('smtp_use_tls', form)
  const tlsCaption = useTlsCaption(useTls, port)

  // A result describes the values it tested: editing them makes it stale.
  useEffect(() => {
    setResult(null)
  }, [host, port, username, password, useTls])

  const testFailed = !locked && result !== null && !result.ok
  useEffect(() => {
    onTestFailedChange?.(testFailed)
  }, [testFailed, onTestFailedChange])

  // setup.testSmtp tests only the values in its body (it applies env credentials, nothing else):
  // with one of these set by the environment it would test another connection than the real one.
  const testUnreliable = (['smtp_host', 'smtp_port', 'smtp_use_tls', 'smtp_ehlo_hostname'] as const).some((key) =>
    isFieldLocked(status, key)
  )

  const testConnection = async () => {
    try {
      await form.validateFields(['smtp_host', 'smtp_port'])
    } catch {
      return
    }
    const values = form.getFieldsValue(true) as SetupFormValues
    setTesting(true)
    try {
      await setupApi.testSmtp({
        smtp_host: values.smtp_host ?? '',
        smtp_port: values.smtp_port ?? 587,
        smtp_username: values.smtp_username ?? '',
        smtp_password: values.smtp_password ?? '',
        smtp_use_tls: values.smtp_use_tls ?? true,
        smtp_ehlo_hostname: values.smtp_ehlo_hostname || undefined
      })
      setResult({ ok: true, target: `${values.smtp_host}:${values.smtp_port}`, at: new Date() })
    } catch (error) {
      setResult({ ok: false, message: error instanceof Error ? error.message : t`Failed to test SMTP connection` })
    } finally {
      setTesting(false)
    }
  }

  // Both or neither: alone, the mailer connects without logging in and the server can refuse to send.
  const pairRule = (other: 'smtp_username' | 'smtp_password', message: string) => ({
    validator: (_rule: unknown, value: unknown) => {
      const mine = typeof value === 'string' && value !== ''
      const theirs = isFieldLocked(status, other) || Boolean(form.getFieldValue(other))
      return mine || !theirs ? Promise.resolve() : Promise.reject(new Error(message))
    }
  })

  const testButton = (
    <Button
      size="small"
      icon={<Zap size={13} aria-hidden="true" />}
      onClick={testConnection}
      loading={testing}
      disabled={testUnreliable}
    >
      {result && !result.ok ? t`Test again` : t`Test connection`}
    </Button>
  )

  const time = result?.ok ? result.at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : ''
  const target = result?.ok ? result.target : ''
  const footer = locked ? undefined : (
    <>
      <span aria-live="polite">
        {result?.ok ? <Pill tone="success">{t`Connected to ${target} · ${time}`}</Pill> : null}
      </span>
      {testUnreliable ? (
        <Tooltip
          title={t`Some connection settings are set by the environment: the test would not check the connection the server will use.`}
        >
          <span>{testButton}</span>
        </Tooltip>
      ) : (
        testButton
      )}
    </>
  )

  return (
    <SetupPanel
      id="smtp"
      number={2}
      title={t`System email delivery (SMTP)`}
      help={{
        label: t`About system email delivery`,
        content: (
          <>
            {t`Used for sign-in codes and invitations. Workspaces set up their own email providers later.`}{' '}
            {t`SMTP guides:`}{' '}
            {DOCS.map((doc, index) => (
              <span key={doc.name}>
                {index > 0 ? ' · ' : ''}
                <a href={doc.href} target="_blank" rel="noopener noreferrer">
                  {doc.name}
                </a>
              </span>
            ))}
          </>
        )
      }}
      state={state}
      footer={footer}
    >
      {locked ? (
        <EnvLockedBody status={status} panel="smtp" />
      ) : (
        <>
          {result && !result.ok ? (
            <div className="mb-3">
              <Alert type="error" showIcon title={result.message} />
            </div>
          ) : null}
          <div className="grid gap-x-3.5 sm:grid-cols-[minmax(0,1fr)_120px_170px]">
            <LockableItem
              status={status}
              name="smtp_host"
              label={t`Host`}
              rules={[{ required: true, message: t`SMTP host is required` }]}
            >
              <Input placeholder="smtp.example.com" />
            </LockableItem>
            <LockableItem
              status={status}
              name="smtp_port"
              label={t`Port`}
              help={t`Common ports: 587 (STARTTLS), 465 (implicit TLS), 25 (unencrypted).`}
              rules={[{ required: true, message: t`SMTP port is required` }]}
            >
              <InputNumber min={1} max={65535} placeholder="587" style={{ width: '100%' }} />
            </LockableItem>
            <LockableItem
              status={status}
              name="smtp_use_tls"
              label={t`Use TLS`}
              valuePropName="checked"
              extra={isFieldLocked(status, 'smtp_use_tls') ? undefined : tlsCaption}
            >
              <Switch aria-label={t`Use TLS`} />
            </LockableItem>
          </div>
          <div className="grid gap-x-3.5 sm:grid-cols-2">
            <LockableItem
              status={status}
              name="smtp_username"
              label={t`Username`}
              dependencies={['smtp_password']}
              rules={[pairRule('smtp_password', t`Enter the username that goes with this password, or clear both.`)]}
            >
              <Input placeholder={t`Optional`} autoComplete="off" />
            </LockableItem>
            <LockableItem
              status={status}
              name="smtp_password"
              label={t`Password`}
              dependencies={['smtp_username']}
              rules={[pairRule('smtp_username', t`Enter the password that goes with this username, or clear both.`)]}
            >
              <Input.Password placeholder={t`Optional`} autoComplete="new-password" />
            </LockableItem>
          </div>
          <div className="grid gap-x-3.5 sm:grid-cols-2">
            <LockableItem
              status={status}
              name="smtp_from_email"
              label={t`From email`}
              rules={[
                { required: true, message: t`From email is required` },
                { type: 'email', message: t`Invalid email format` }
              ]}
            >
              <Input placeholder="noreply@example.com" />
            </LockableItem>
            <LockableItem status={status} name="smtp_from_name" label={t`From name`}>
              <Input placeholder="Notifuse" />
            </LockableItem>
          </div>
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
              name="smtp_ehlo_hostname"
              label={t`EHLO hostname`}
              help={t`The name this server gives when it connects to the SMTP server. Empty means the SMTP host.`}
            >
              <Input placeholder={t`Defaults to the SMTP host`} />
            </LockableItem>
          </div>
        </>
      )}
    </SetupPanel>
  )
}
