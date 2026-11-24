import { useState, type ReactNode } from 'react'
import { Alert, Button, Form, Input, Modal } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { twoFactorErrorCode, twoFactorRetryAfter, type TwoFactorProof } from '../../services/api/twoFactor'
import { useTwoFactorErrorMessage } from './useTwoFactorErrorMessage'

export interface ProofModalProps {
  open: boolean
  title: string
  description?: ReactNode
  okText: string
  danger?: boolean
  /** Runs the action with the proof; a rejection is shown in the modal, which stays open. */
  onSubmit: (proof: TwoFactorProof) => Promise<void>
  onCancel: () => void
}

/**
 * Asks for a second factor before a sensitive action: a code from the authenticator app, or a
 * recovery code. Each action carries its own proof; nothing is remembered between them.
 */
export function ProofModal({ open, title, description, okText, danger, onSubmit, onCancel }: ProofModalProps) {
  const { t } = useLingui()
  const errorMessage = useTwoFactorErrorMessage()
  const [form] = Form.useForm<{ code: string }>()
  const [useRecovery, setUseRecovery] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const close = () => {
    form.resetFields()
    setUseRecovery(false)
    setError(null)
    onCancel()
  }

  const submit = async ({ code }: { code: string }) => {
    setLoading(true)
    setError(null)
    try {
      await onSubmit(useRecovery ? { recovery_code: code.trim() } : { code: code.replace(/\s/g, '') })
      form.resetFields()
      setUseRecovery(false)
    } catch (err) {
      const code = twoFactorErrorCode(err)
      if (code === 'two_factor_locked') {
        // Outside sign-in, a recovery code waits like an authenticator code.
        const minutes = Math.max(1, Math.ceil((twoFactorRetryAfter(err) ?? 60) / 60))
        setError(t`Too many wrong codes. Try again in ${minutes} minutes.`)
      } else {
        if (code === 'two_factor_totp_locked' && !useRecovery) {
          // Only a recovery code gets through now: offered straight away.
          setUseRecovery(true)
          form.resetFields()
        }
        setError(errorMessage(err, t`Something went wrong`))
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      open={open}
      title={title}
      onCancel={close}
      destroyOnHidden
      footer={[
        <Button key="cancel" onClick={close}>
          {t`Cancel`}
        </Button>,
        <Button key="ok" type="primary" danger={danger} loading={loading} onClick={() => form.submit()}>
          {okText}
        </Button>
      ]}
    >
      {description ? <div className="mb-4 text-sm text-ink-2">{description}</div> : null}
      {error ? (
        <div className="mb-4">
          <Alert type="error" showIcon message={error} />
        </div>
      ) : null}
      <Form form={form} layout="vertical" onFinish={submit} requiredMark={false}>
        <Form.Item
          name="code"
          label={useRecovery ? t`Recovery code` : t`Code from your authenticator app`}
          rules={
            useRecovery
              ? [{ required: true, message: t`Enter one of your recovery codes` }]
              : [
                  { required: true, message: t`Enter the 6-digit code from your app` },
                  { pattern: /^\s*\d{3}\s?\d{3}\s*$/, message: t`Please enter a valid 6-digit code!` }
                ]
          }
        >
          {useRecovery ? (
            <Input autoFocus autoComplete="off" spellCheck={false} />
          ) : (
            <Input autoFocus inputMode="numeric" autoComplete="one-time-code" maxLength={7} placeholder="000000" />
          )}
        </Form.Item>
      </Form>
      <Button
        type="link"
        className="app-link-flush"
        onClick={() => {
          setUseRecovery(!useRecovery)
          setError(null)
          form.resetFields()
        }}
      >
        {useRecovery ? t`Use your authenticator app instead` : t`Use a recovery code instead`}
      </Button>
    </Modal>
  )
}
