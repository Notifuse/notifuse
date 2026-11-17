import { useState } from 'react'
import { App, Button, Input, Modal } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { EmailProvider } from '../../../services/api/types'
import { emailService } from '../../../services/api/email'

export interface TestEmailTarget {
  provider: EmailProvider
  /**
   * The saved integration, when there is one: the server fills the secrets the provider object
   * carries blank (they are never served) from it. None for a provider not saved yet, whose form
   * still holds what was typed.
   */
  integrationId?: string
  /**
   * Sent from the drawer, with what its form holds, saved or not. A card tests the saved
   * integration, and its dialog says so.
   */
  fromForm?: boolean
}

export interface TestEmailModalProps {
  workspaceId: string
  /** The provider to test; the modal is open while there is one. */
  target: TestEmailTarget | null
  onClose: () => void
}

/** Sends a real email through a provider, saved or not, to the address typed. */
export function TestEmailModal({ workspaceId, target, onClose }: TestEmailModalProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [to, setTo] = useState('')
  const [sending, setSending] = useState(false)

  const close = () => {
    setTo('')
    onClose()
  }

  const send = async () => {
    if (!target || !to) return
    setSending(true)
    try {
      const response = await emailService.testProvider(workspaceId, target.provider, to, target.integrationId)
      if (response.success) {
        message.success(t`Test email sent successfully`)
        close()
      } else {
        message.error(t`Failed to send test email: ${response.error}`)
      }
    } catch (error) {
      console.error('Error testing email provider', error)
      message.error(t`Failed to test email provider`)
    } finally {
      setSending(false)
    }
  }

  return (
    <Modal
      title={t`Send a test email`}
      open={Boolean(target)}
      onCancel={close}
      destroyOnHidden
      footer={[
        <Button key="cancel" onClick={close}>
          {t`Cancel`}
        </Button>,
        <Button key="submit" type="primary" loading={sending} onClick={send} disabled={!to}>
          {t`Send test email`}
        </Button>
      ]}
    >
      <div className="flex flex-col gap-2">
        <label htmlFor="test-email-to" className="text-sm font-medium text-ink">
          {t`Send to`}
        </label>
        <Input
          id="test-email-to"
          type="email"
          placeholder="recipient@example.com"
          value={to}
          onChange={(e) => setTo(e.target.value)}
          onPressEnter={send}
        />
        <div className="text-xs text-ink-3">
          {target?.fromForm
            ? t`A real email, sent through these settings, saved or not.`
            : t`A real email, sent through this integration's saved settings.`}
        </div>
      </div>
    </Modal>
  )
}
