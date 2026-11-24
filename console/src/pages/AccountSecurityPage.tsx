import { useEffect, useState } from 'react'
import { Alert, Button } from 'antd'
import { ArrowLeft, LogOut } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { useBrandName } from '../hooks/useBrandName'
import { InstanceLogo } from '../components/brand/InstanceLogo'
import { getInstanceBranding } from '../components/brand/instanceBranding'
import { useNavigate } from '@tanstack/react-router'
import { useAuth } from '../contexts/AuthContext'
import { AccountSecurityPanel } from '../components/two_factor/AccountSecurityPanel'
import { RootSessionRefused } from '../components/two_factor/RootSessionRefused'

/**
 * The account security page, outside any workspace: where a session lands when the instance
 * requires two-factor authentication and it has not passed it, and where a sign-in comes back
 * to after a step that needed a recent one.
 */
export function AccountSecurityPage() {
  const { t } = useLingui()
  const brand = useBrandName()
  const navigate = useNavigate()
  const { user, workspaces, twoFactorRequired, rootSessionWithoutCode, refreshWorkspaces } = useAuth()

  // Read the account again on arrival: a session sent here by a refused call (the requirement
  // switched on meanwhile) would otherwise show the state it had before.
  useEffect(() => {
    void refreshWorkspaces().catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  // True while new recovery codes are on screen and not yet marked as saved: the ways out of
  // the page (back to a workspace, logout) wait, as the drawer stays open, so the person does
  // not leave with codes they never saved.
  const [codesPending, setCodesPending] = useState(false)
  const firstWorkspace = twoFactorRequired || rootSessionWithoutCode ? undefined : workspaces[0]
  const workspaceName = firstWorkspace?.name ?? ''

  return (
    <div className="min-h-screen bg-canvas">
      <header className="flex h-12 items-center justify-between border-b border-line-soft bg-surface px-4">
        <div className="flex items-center gap-3">
          {getInstanceBranding().custom ? (
            <InstanceLogo variant="full" maxWidth={150} maxHeight={24} />
          ) : (
            <span className="text-sm font-semibold text-ink">Notifuse</span>
          )}
          {!codesPending && !twoFactorRequired && !rootSessionWithoutCode && !firstWorkspace ? (
            <Button type="text" icon={<ArrowLeft size={14} aria-hidden="true" />} onClick={() => navigate({ to: '/console' })}>
              {t`Back to ${brand}`}
            </Button>
          ) : null}
          {!codesPending && firstWorkspace ? (
            <Button
              type="text"
              icon={<ArrowLeft size={14} aria-hidden="true" />}
              onClick={() =>
                navigate({ to: '/console/workspace/$workspaceId', params: { workspaceId: firstWorkspace.id } })
              }
            >
              {t`Back to ${workspaceName}`}
            </Button>
          ) : null}
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm text-ink-2">{user?.email}</span>
          {codesPending ? null : (
            <Button type="text" icon={<LogOut size={14} aria-hidden="true" />} onClick={() => navigate({ to: '/console/logout' })}>
              {t`Logout`}
            </Button>
          )}
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-2xl flex-col gap-4 px-4 py-8">
        <div className="flex flex-col gap-1">
          <h1 className="m-0 text-lg font-semibold text-ink">{t`Account security`}</h1>
          <p className="m-0 text-sm text-ink-2">{t`Settings for your ${brand} account, in every workspace you belong to.`}</p>
        </div>
        {rootSessionWithoutCode && !twoFactorRequired ? (
          <RootSessionRefused title={t`Sign in again to open your workspaces`} />
        ) : null}
        {twoFactorRequired ? (
          <Alert
            type="warning"
            showIcon
            message={t`Your ${brand} administrator requires two-factor authentication.`}
            description={t`Turn it on to open your workspaces. Until then, this page is the only one you can use.`}
          />
        ) : null}
        <div className="rounded-lg border border-line-soft bg-surface p-5">
          <AccountSecurityPanel onLockChange={setCodesPending} />
        </div>
      </main>
    </div>
  )
}
