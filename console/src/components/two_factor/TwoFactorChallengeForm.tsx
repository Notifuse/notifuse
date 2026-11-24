import { useState } from 'react'
import { Alert, Button, Form, Input } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useBrandName } from '../../hooks/useBrandName'
import {
  twoFactorErrorCode,
  twoFactorService,
  type VerifyChallengeResponse
} from '../../services/api/twoFactor'
import { useTwoFactorErrorMessage } from './useTwoFactorErrorMessage'

export interface TwoFactorChallengeFormProps {
  /** The challenge user.verify or workspaces.acceptInvitation answered. */
  challengeToken: string
  /** The second factor passed: the session token and, after an invitation, its outcome. */
  onSignedIn: (response: VerifyChallengeResponse) => Promise<void> | void
  /** Back to the email step: the challenge is gone or the person gives up. */
  onRestart: () => void
}

/**
 * The second step of a sign-in: a code from the authenticator app, or a recovery code. A dead
 * challenge sends the person back to the email step; the other errors stay on this step.
 */
export function TwoFactorChallengeForm({ challengeToken, onSignedIn, onRestart }: TwoFactorChallengeFormProps) {
  const { t } = useLingui()
  const brand = useBrandName()
  const errorMessage = useTwoFactorErrorMessage()
  const [form] = Form.useForm<{ code: string }>()
  const [useRecovery, setUseRecovery] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expired, setExpired] = useState(false)

  const switchMode = (recovery: boolean) => {
    setUseRecovery(recovery)
    setError(null)
    form.resetFields()
  }

  const submit = async ({ code }: { code: string }) => {
    setLoading(true)
    setError(null)
    try {
      const proof = useRecovery ? { recovery_code: code.trim() } : { code: code.replace(/\s/g, '') }
      const response = await twoFactorService.verify(challengeToken, proof)
      await onSignedIn(response)
    } catch (err) {
      const code = twoFactorErrorCode(err)
      if (code === 'two_factor_challenge_invalid') {
        setExpired(true)
      } else if (code === 'two_factor_totp_locked' && !useRecovery) {
        // Only a recovery code gets through now: offer it straight away.
        setUseRecovery(true)
        form.resetFields()
      }
      setError(errorMessage(err, t`Failed to verify code`))
    } finally {
      setLoading(false)
    }
  }

  if (expired) {
    return (
      <div className="flex flex-col gap-4">
        <Alert type="warning" showIcon message={t`This sign-in expired after 10 minutes. Start again with your email.`} />
        <Button type="primary" block onClick={onRestart}>
          {t`Back to sign in`}
        </Button>
      </div>
    )
  }

  return (
    <Form form={form} layout="vertical" onFinish={submit} requiredMark={false}>
      {error ? (
        <div className="mb-4">
          <Alert type="error" showIcon message={error} />
        </div>
      ) : null}
      {useRecovery ? (
        <Form.Item
          name="code"
          label={t`Recovery code`}
          rules={[{ required: true, message: t`Enter one of your recovery codes` }]}
        >
          <Input autoComplete="off" autoFocus spellCheck={false} placeholder="xxxxxx-xxxxxx-xxxxxx-xxxxxx" />
        </Form.Item>
      ) : (
        <Form.Item
          name="code"
          label={t`Authentication code`}
          rules={[
            { required: true, message: t`Enter the 6-digit code from your app` },
            { pattern: /^\s*\d{3}\s?\d{3}\s*$/, message: t`Please enter a valid 6-digit code!` }
          ]}
        >
          <Input
            autoFocus
            inputMode="numeric"
            autoComplete="one-time-code"
            maxLength={7}
            placeholder="000000"
          />
        </Form.Item>
      )}
      <Form.Item>
        <Button type="primary" htmlType="submit" block loading={loading}>
          {t`Verify`}
        </Button>
      </Form.Item>
      <div className="flex justify-between gap-3">
        <Button type="link" onClick={() => switchMode(!useRecovery)}>
          {useRecovery ? t`Use your authenticator app instead` : t`Use a recovery code instead`}
        </Button>
        <Button type="link" onClick={onRestart}>
          {t`Use another email`}
        </Button>
      </div>
      <p className="mt-4 mb-0 text-xs text-ink-3">
        {t`Lost your phone and your recovery codes? Your ${brand} administrator can reset two-factor authentication for your account.`}
      </p>
    </Form>
  )
}
