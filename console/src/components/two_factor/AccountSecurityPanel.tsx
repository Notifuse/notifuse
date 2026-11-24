import { useState } from 'react'
import { Alert, App, Button, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { signInAgain } from './signInAgain'
import { useAuth } from '../../contexts/AuthContext'
import {
  twoFactorService,
  type TwoFactorProof,
  type TwoFactorSetupResponse
} from '../../services/api/twoFactor'
import dayjs from '../../lib/dayjs'
import { SetupWizard } from './SetupWizard'
import { ProofModal } from './ProofModal'
import { RecoveryCodesList } from './RecoveryCodesList'

type ProofAction = 'replace' | 'regenerate' | 'disable'

export interface AccountSecurityPanelProps {
  /** True while recovery codes are on screen and not yet marked as saved: the wrapper keeps itself open. */
  onLockChange?: (locked: boolean) => void
}

function Section({ title, children, action }: { title: string; children?: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-line-soft py-4 first:pt-0 last:border-b-0 last:pb-0">
      <div className="flex min-w-0 flex-col gap-1">
        <h3 className="m-0 text-sm font-semibold text-ink">{title}</h3>
        {children}
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  )
}

/**
 * Account security: two-factor authentication of the signed-in account, in every workspace it
 * belongs to. Mounted in the drawer the account menu opens, and in the page a session lands on
 * when the instance requires two-factor authentication.
 */
export function AccountSecurityPanel({ onLockChange }: AccountSecurityPanelProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const { user, twoFactor, refreshWorkspaces } = useAuth()
  const [wizard, setWizard] = useState<{ replacing: boolean; setup?: TwoFactorSetupResponse } | null>(null)
  const [proofAction, setProofAction] = useState<ProofAction | null>(null)
  const [newCodes, setNewCodes] = useState<string[] | null>(null)
  const [codesSaved, setCodesSaved] = useState(false)

  const enabled = twoFactor?.enabled === true
  const codesLeft = twoFactor?.recovery_codes_remaining ?? 0
  const addedOn = twoFactor?.enabled_at ? dayjs(twoFactor.enabled_at).format('ll') : ''
  const requiredByInstance = twoFactor?.required_by_instance === true

  // A recent sign-in is needed: this browser signs in again, then comes back here. The
  // other sessions are left alone.
  const reauthenticate = () => {
    signInAgain(user?.email)
  }

  // The account's state after a change. A failure to read it leaves the change made: the
  // next read shows it.
  const refresh = () => refreshWorkspaces().catch(() => undefined)

  const finishWizard = async () => {
    setWizard(null)
    onLockChange?.(false)
    await refresh()
  }

  // Each action checks its proof while the modal is open, so a wrong code is shown there.
  const runProofAction = async (proof: TwoFactorProof) => {
    if (proofAction === 'replace') {
      const setup = await twoFactorService.setup(proof)
      setProofAction(null)
      setWizard({ replacing: true, setup })
      return
    }
    if (proofAction === 'regenerate') {
      const response = await twoFactorService.regenerateRecoveryCodes(proof)
      setProofAction(null)
      setCodesSaved(false)
      setNewCodes(response.recovery_codes)
      onLockChange?.(true)
      void refresh()
      return
    }
    if (proofAction === 'disable') {
      await twoFactorService.disable(proof)
      setProofAction(null)
      message.success(t`Two-factor authentication turned off`)
      void refresh()
    }
  }

  if (wizard) {
    return (
      <SetupWizard
        email={user?.email}
        replacing={wizard.replacing}
        initialSetup={wizard.setup}
        onEnabled={() => void refresh()}
        onFinished={finishWizard}
        onCancel={() => setWizard(null)}
        onReauthenticate={reauthenticate}
        onLockChange={onLockChange}
      />
    )
  }

  if (newCodes) {
    return (
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <h3 className="m-0 text-sm font-semibold text-ink">{t`Save your new recovery codes`}</h3>
          <p className="m-0 text-sm text-ink-2">{t`Your previous codes no longer work. This is the only time Notifuse shows these.`}</p>
        </div>
        <RecoveryCodesList codes={newCodes} email={user?.email} saved={codesSaved} onSavedChange={setCodesSaved} />
        <div className="flex justify-end">
          <Button
            type="primary"
            disabled={!codesSaved}
            onClick={() => {
              setNewCodes(null)
              onLockChange?.(false)
            }}
          >
            {t`Done`}
          </Button>
        </div>
      </div>
    )
  }

  const proofTexts: Record<ProofAction, { title: string; okText: string; danger?: boolean }> = {
    replace: { title: t`Replace your authenticator app`, okText: t`Continue` },
    regenerate: { title: t`Generate new recovery codes`, okText: t`Generate new codes` },
    disable: { title: t`Turn off two-factor authentication`, okText: t`Turn off`, danger: true }
  }

  return (
    <div className="flex flex-col">
      {!enabled ? (
        <Section title={t`Two-factor authentication`}>
          <span data-two-factor-status className="text-xs font-medium text-ink-3">
            {t`Off`}
          </span>
          <p className="m-0 text-sm text-ink-2">
            {t`After the code we email you, Notifuse asks for a second code from an authenticator app on your phone, such as Google Authenticator, 1Password or Bitwarden. Someone who gets into your mailbox still can't sign in.`}
          </p>
          <div className="mt-3 flex items-center gap-3">
            <Button type="primary" onClick={() => setWizard({ replacing: false })}>
              {t`Turn on two-factor authentication`}
            </Button>
            <span className="text-xs text-ink-3">{t`Takes about a minute.`}</span>
          </div>
        </Section>
      ) : (
        <>
          <Section title={t`Two-factor authentication`}>
            <span data-two-factor-status className="text-xs font-medium text-success-fg">
              {t`On`}
            </span>
            <p className="m-0 text-sm text-ink-2">
              {t`After the code we email you, Notifuse asks for a second code from an authenticator app on your phone.`}
            </p>
          </Section>
          <Section
            title={t`Authenticator app`}
            action={<Button onClick={() => setProofAction('replace')}>{t`Replace app`}</Button>}
          >
            {addedOn ? <span className="text-sm text-ink-2">{t`Added on ${addedOn}`}</span> : null}
          </Section>
          <Section
            title={t`Recovery codes`}
            action={<Button onClick={() => setProofAction('regenerate')}>{t`Generate new codes`}</Button>}
          >
            <span className="text-sm text-ink-2">{t`Each one works once, if you lose your phone.`}</span>
            <span data-recovery-codes-left className="text-xs text-ink-3">
              {t`${codesLeft} of 10 left`}
            </span>
            {codesLeft <= 2 ? (
              <div className="mt-2">
                <Alert type="warning" showIcon message={t`Few recovery codes left. Generate new ones.`} />
              </div>
            ) : null}
          </Section>
          <Section
            title={t`Turn off two-factor authentication`}
            action={
              <Tooltip title={requiredByInstance ? t`Your administrator requires it, so you can't turn it off.` : undefined}>
                <Button danger disabled={requiredByInstance} onClick={() => setProofAction('disable')}>
                  {t`Turn off`}
                </Button>
              </Tooltip>
            }
          >
            {requiredByInstance ? (
              <span className="text-sm text-ink-3">{t`Your administrator requires it, so you can't turn it off.`}</span>
            ) : null}
          </Section>
        </>
      )}

      {proofAction ? (
        <ProofModal
          open
          title={proofTexts[proofAction].title}
          okText={proofTexts[proofAction].okText}
          danger={proofTexts[proofAction].danger}
          onSubmit={runProofAction}
          onCancel={() => setProofAction(null)}
        />
      ) : null}
    </div>
  )
}
