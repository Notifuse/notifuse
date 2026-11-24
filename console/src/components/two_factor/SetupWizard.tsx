import { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, App, Button, Checkbox, Form, Input, Spin, Steps } from 'antd'
import { Copy } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useLingui } from '@lingui/react/macro'
import { useBrandName } from '../../hooks/useBrandName'
import {
  twoFactorErrorCode,
  twoFactorService,
  type TwoFactorSetupResponse
} from '../../services/api/twoFactor'
import { RecoveryCodesList } from './RecoveryCodesList'
import { useTwoFactorErrorMessage } from './useTwoFactorErrorMessage'
import { groupKey } from './groupKey'

export interface SetupWizardProps {
  /** The person's email, named in the recovery codes file. */
  email?: string
  /**
   * Replacing the app of an account that has one. The caller checked the proof and started the
   * setup (initialSetup); the proof itself is not kept.
   */
  replacing?: boolean
  /** The setup the caller already started. */
  initialSetup?: TwoFactorSetupResponse
  /** Two-factor authentication is on, or the app replaced: the account's state changed. */
  onEnabled?: () => void
  /** Done: turned on (with the codes saved) or the app replaced. */
  onFinished: () => void
  onCancel: () => void
  /** The setup needs a recent sign-in. */
  onReauthenticate: () => void
  /** True while the recovery codes are on screen and not yet marked as saved. */
  onLockChange?: (locked: boolean) => void
}

/**
 * Turns on two-factor authentication, or replaces the authenticator app: scan the QR code,
 * confirm a code, then (the first time) save the recovery codes.
 */
export function SetupWizard({
  email,
  replacing = false,
  initialSetup,
  onFinished,
  onCancel,
  onReauthenticate,
  onLockChange,
  onEnabled
}: SetupWizardProps) {
  const { t } = useLingui()
  const brand = useBrandName()
  const { message } = App.useApp()
  const errorMessage = useTwoFactorErrorMessage()
  const [step, setStep] = useState(0)
  const [setup, setSetup] = useState<TwoFactorSetupResponse | null>(initialSetup ?? null)
  const [expired, setExpired] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [codeError, setCodeError] = useState<string | null>(null)
  const [enabling, setEnabling] = useState(false)
  const [signOutOthers, setSignOutOthers] = useState(true)
  const [codes, setCodes] = useState<string[]>([])
  const [saved, setSaved] = useState(false)
  const [form] = Form.useForm<{ code: string }>()
  const started = useRef(false)

  const start = useCallback(
    async (afterExpiry: boolean) => {
      setLoadError(null)
      setSetup(null)
      try {
        setSetup(await twoFactorService.setup({}))
        setExpired(afterExpiry)
        setStep(0)
        form.resetFields()
      } catch (err) {
        if (twoFactorErrorCode(err) === 'reauthentication_required') {
          onReauthenticate()
          return
        }
        setLoadError(errorMessage(err, t`Failed to start the setup`))
      }
    },
    [form, onReauthenticate, errorMessage, t]
  )

  useEffect(() => {
    if (started.current) return
    started.current = true
    if (!initialSetup) void start(false)
  }, [start, initialSetup])

  // Locked while the recovery codes are on screen, until Done: closing the drawer then would
  // skip what Done does.
  useEffect(() => {
    onLockChange?.(step === 2)
  }, [step, onLockChange])

  const enable = async ({ code }: { code: string }) => {
    setEnabling(true)
    setCodeError(null)
    try {
      const response = await twoFactorService.enable(code.replace(/\s/g, ''), signOutOthers)
      onEnabled?.()
      if (response.recovery_codes.length > 0) {
        setCodes(response.recovery_codes)
        setStep(2)
      } else {
        message.success(t`Authenticator app replaced`)
        onFinished()
      }
    } catch (err) {
      if (twoFactorErrorCode(err) === 'two_factor_setup_expired') {
        if (replacing) {
          // The proof that started this replacement is used up: a new one is needed.
          setSetup(null)
          setStep(0)
          setLoadError(t`This setup expired after 15 minutes. Start the replacement again.`)
          return
        }
        await start(true)
        return
      }
      setCodeError(
        twoFactorErrorCode(err) === 'two_factor_invalid_code'
          ? t`That code didn't work. Check that your phone's clock is set automatically, then enter the code the app shows now.`
          : errorMessage(err, t`Failed to turn on two-factor authentication`)
      )
    } finally {
      setEnabling(false)
    }
  }

  const copyKey = async () => {
    if (!setup) return
    try {
      await navigator.clipboard.writeText(setup.secret)
      message.success(t`Key copied`)
    } catch {
      message.error(t`Could not copy the key. Type it by hand.`)
    }
  }

  const account = email ?? ''

  return (
    <div className="flex flex-col gap-6">
      <Steps
        size="small"
        current={step}
        items={[{ title: t`Scan` }, { title: t`Confirm` }, ...(replacing ? [] : [{ title: t`Recovery codes` }])]}
      />

      {loadError ? (
        <div className="flex flex-col gap-3">
          <Alert type="error" showIcon message={loadError} />
          <div className="flex justify-end gap-2">
            <Button onClick={onCancel}>{t`Cancel`}</Button>
            {replacing ? null : <Button onClick={() => void start(false)}>{t`Try again`}</Button>}
          </div>
        </div>
      ) : null}

      {step === 0 ? (
        setup ? (
          <div className="flex flex-col gap-4">
            {expired ? (
              <Alert type="warning" showIcon message={t`This setup expired after 15 minutes. Scan the new QR code below.`} />
            ) : null}
            <div className="flex flex-col gap-1">
              <h3 className="m-0 text-sm font-semibold text-ink">{t`Scan this QR code with your authenticator app`}</h3>
              <p className="m-0 text-sm text-ink-2">
                {t`Use Google Authenticator, 1Password, Bitwarden, Microsoft Authenticator or any app that shows 6-digit codes.`}
              </p>
            </div>
            <div className="flex justify-center rounded-md border border-line-soft bg-surface p-4">
              <QRCodeSVG value={setup.otpauth_uri} size={176} marginSize={2} title={t`QR code for your authenticator app`} />
            </div>
            <div className="flex flex-col gap-1.5">
              <span className="text-sm text-ink-2">{t`Can't scan it? Enter this key`}</span>
              <div className="flex items-center gap-2">
                <code data-setup-key className="flex-1 rounded-md bg-fill px-3 py-2 font-mono text-sm text-ink">
                  {groupKey(setup.secret)}
                </code>
                <Button icon={<Copy size={14} aria-hidden="true" />} onClick={copyKey}>
                  {t`Copy`}
                </Button>
              </div>
              <span className="text-xs text-ink-3">{t`Account name in the app: ${brand} (${account})`}</span>
            </div>
            <div className="flex justify-end gap-2">
              <Button onClick={onCancel}>{t`Cancel`}</Button>
              <Button type="primary" onClick={() => setStep(1)}>
                {t`Next`}
              </Button>
            </div>
          </div>
        ) : loadError ? null : (
          <div className="flex justify-center p-6">
            <Spin />
          </div>
        )
      ) : null}

      {step === 1 ? (
        <Form form={form} layout="vertical" onFinish={enable} requiredMark={false}>
          <h3 className="mt-0 mb-3 text-sm font-semibold text-ink">{t`Enter the code shown in the app`}</h3>
          {codeError ? (
            <div className="mb-4">
              <Alert type="error" showIcon message={codeError} />
            </div>
          ) : null}
          <Form.Item
            name="code"
            label={t`6-digit code`}
            rules={[
              { required: true, message: t`Enter the 6-digit code from your app` },
              { pattern: /^\s*\d{3}\s?\d{3}\s*$/, message: t`Please enter a valid 6-digit code!` }
            ]}
          >
            <Input autoFocus inputMode="numeric" autoComplete="one-time-code" maxLength={7} placeholder="000000" />
          </Form.Item>
          <Form.Item>
            <Checkbox checked={signOutOthers} onChange={(e) => setSignOutOthers(e.target.checked)}>
              {t`Sign me out of my other sessions`}
            </Checkbox>
          </Form.Item>
          <div className="flex justify-end gap-2">
            <Button onClick={() => setStep(0)}>{t`Back`}</Button>
            <Button type="primary" htmlType="submit" loading={enabling}>
              {replacing ? t`Replace app` : t`Turn on`}
            </Button>
          </div>
        </Form>
      ) : null}

      {step === 2 ? (
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <h3 className="m-0 text-sm font-semibold text-ink">{t`Save your recovery codes`}</h3>
            <p className="m-0 text-sm text-ink-2">
              {t`If you lose your phone, each code lets you sign in once. This is the only time ${brand} shows them.`}
            </p>
          </div>
          <RecoveryCodesList codes={codes} email={email} saved={saved} onSavedChange={setSaved} />
          <div className="flex justify-end">
            <Button type="primary" disabled={!saved} onClick={onFinished}>
              {t`Done`}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  )
}
