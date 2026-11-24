import { useState } from 'react'
import { App, Button, Modal } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useBrandName } from '../../hooks/useBrandName'
import { useOptionalAuth } from '../../contexts/AuthContext'
import { twoFactorErrorCode, twoFactorService, type TwoFactorProof } from '../../services/api/twoFactor'
import { signInAgain } from './signInAgain'
import { ProofModal } from './ProofModal'
import { useTwoFactorErrorMessage } from './useTwoFactorErrorMessage'

export interface ResetTwoFactorButtonProps {
  /** The person, by id (a member) or by email (anyone, workspace or not). */
  target: { user_id: string } | { email: string }
  /** Their email, named in the confirmation. */
  email: string
  disabled?: boolean
  onDone?: () => void
}

/**
 * Root only: removes the two-factor authentication of someone who lost both their phone and
 * their recovery codes. A root user with two-factor authentication confirms with a code.
 */
export function ResetTwoFactorButton({ target, email, disabled, onDone }: ResetTwoFactorButtonProps) {
  const { t } = useLingui()
  const brand = useBrandName()
  const { message } = App.useApp()
  const auth = useOptionalAuth()
  const twoFactor = auth?.twoFactor
  const errorMessage = useTwoFactorErrorMessage()
  const [open, setOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)

  const consequences = (
    <div className="flex flex-col gap-3">
      <ul className="m-0 flex flex-col gap-1 pl-5">
        <li>{t`Their authenticator app and recovery codes stop working.`}</li>
        <li>{t`They're signed out everywhere.`}</li>
        <li>{t`They get an email about the reset.`}</li>
        <li>{t`If ${brand} requires two-factor authentication, they set it up again at their next sign-in.`}</li>
      </ul>
      <p className="m-0 text-ink-2">
        {t`Before you reset it, check that the request really comes from ${email}, for example by a call. Attackers try this with administrators.`}
      </p>
    </div>
  )

  const reset = async (proof: TwoFactorProof) => {
    await twoFactorService.resetForUser(target, proof)
    setOpen(false)
    message.success(t`Two-factor authentication reset for ${email}`)
    onDone?.()
  }

  const title = t`Reset two-factor authentication for ${email}?`

  if (twoFactor?.enabled) {
    return (
      <>
        <Button danger disabled={disabled} onClick={() => setOpen(true)}>
          {t`Reset`}
        </Button>
        {open ? (
          <ProofModal
            open
            title={title}
            description={consequences}
            okText={t`Reset`}
            danger
            onSubmit={reset}
            onCancel={() => setOpen(false)}
          />
        ) : null}
      </>
    )
  }

  // A root user without two-factor authentication: a recent sign-in instead of a code.
  return (
    <>
      <Button danger disabled={disabled} onClick={() => setOpen(true)}>
        {t`Reset`}
      </Button>
      <Modal
        open={open}
        title={title}
        okText={t`Reset`}
        okButtonProps={{ danger: true, loading: confirming }}
        cancelText={t`Cancel`}
        onCancel={() => setOpen(false)}
        onOk={async () => {
          setConfirming(true)
          try {
            await reset({})
          } catch (err) {
            if (twoFactorErrorCode(err) === 'reauthentication_required') {
              // Without two-factor authentication, the root user proves it is them by a
              // recent sign-in.
              message.warning(t`Sign in again to reset two-factor authentication: it needs a recent sign-in.`)
              signInAgain(auth?.user?.email, null)
              return
            }
            message.error(errorMessage(err, t`Failed to reset two-factor authentication`))
          } finally {
            setConfirming(false)
          }
        }}
        destroyOnHidden
      >
        {consequences}
      </Modal>
    </>
  )
}
