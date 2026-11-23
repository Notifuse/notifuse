import { Alert, Button } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useOptionalAuth } from '../../contexts/AuthContext'
import { signInAgain } from './signInAgain'

/**
 * A root screen refused with two_factor_required: the root user has two-factor authentication,
 * but this session was opened without the code. Signing in again fixes it.
 */
export function RootSessionRefused({ title }: { title: string }) {
  const { t } = useLingui()
  const user = useOptionalAuth()?.user
  return (
    <Alert
      type="warning"
      showIcon
      message={title}
      description={
        <div className="flex flex-col gap-3">
          <span>
            {t`Your account has two-factor authentication, but you opened this session without it. System settings, the licence and the platform audit log need a session where you entered your authentication code.`}
          </span>
          <div>
            <Button type="primary" onClick={() => signInAgain(user?.email, null)}>
              {t`Sign in again`}
            </Button>
          </div>
        </div>
      }
    />
  )
}
