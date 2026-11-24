import { useState } from 'react'
import { Alert, Form, Input, Switch, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useBrandName } from '../../hooks/useBrandName'
import { useOptionalAuth } from '../../contexts/AuthContext'
import type { SystemTwoFactorSettings } from '../../types/settings'
import { ResetTwoFactorButton } from '../two_factor/ResetTwoFactorButton'

const { Title } = Typography

export interface SystemSecuritySettingsProps {
  twoFactor?: SystemTwoFactorSettings
}

/**
 * System settings › Security: the instance's two-factor requirement, who has not turned it on,
 * and the reset of a person's two-factor authentication by email.
 */
export function SystemSecuritySettings({ twoFactor }: SystemSecuritySettingsProps) {
  const { t } = useLingui()
  const brand = useBrandName()
  const auth = useOptionalAuth()
  const [resetEmail, setResetEmail] = useState('')
  const fromEnv = twoFactor?.from_env === true
  // Turning it on from an account without it would lock the root user out at the next
  // sign-in: the server refuses it (two_factor_required_for_root), the switch says why first.
  // A session that passed a second factor (an authenticator code, single sign-on or the root
  // signature) may switch it on; the server refuses any other.
  const rootWithout = auth ? !auth.sessionTwoFactorPassed : true
  const required = Form.useWatch('require_two_factor')
  const usersWithout = twoFactor?.users_without ?? []
  const trimmedEmail = resetEmail.trim()

  return (
    <div className="flex flex-col gap-4">
      <Title level={5} style={{ margin: 0 }}>
        {t`Security`}
      </Title>

      <div className="flex items-start justify-between gap-6">
        <div className="flex flex-col gap-1">
          <span className="text-sm font-semibold text-ink">{t`Require two-factor authentication for everyone`}</span>
          <span className="text-sm text-ink-2">
            {t`People who haven't turned it on can only open their account page until they do. Single sign-on users and API keys aren't affected.`}
          </span>
        </div>
        <Form.Item name="require_two_factor" valuePropName="checked" noStyle>
          <Switch disabled={fromEnv || (rootWithout && !required)} aria-label={t`Require two-factor authentication for everyone`} />
        </Form.Item>
      </div>

      {fromEnv ? (
        <Alert type="info" showIcon message={t`Set by the REQUIRE_TWO_FACTOR environment variable. Change it there and restart ${brand}.`} />
      ) : rootWithout && !required ? (
        <Alert
          type="warning"
          showIcon
          message={t`Turn on two-factor authentication for your own account first, so you don't lock yourself out.`}
          description={t`Open Account security from your account menu.`}
        />
      ) : null}

      {usersWithout.length > 0 ? (
        <div className="flex flex-col gap-2">
          <span className="text-sm font-semibold text-ink">{t`People without two-factor authentication`}</span>
          <span className="text-sm text-ink-2">
            {t`They'll be asked to turn it on the next time they open ${brand}. Let them know before you switch this on.`}
          </span>
          <ul data-users-without-two-factor className="m-0 max-h-40 list-none overflow-y-auto rounded-md border border-line-soft p-0">
            {usersWithout.map((email) => (
              <li key={email} className="border-b border-line-soft px-3 py-1.5 text-sm text-ink last:border-b-0">
                {email}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="flex flex-col gap-2">
        <span className="text-sm font-semibold text-ink">{t`Reset two-factor authentication`}</span>
        <span className="text-sm text-ink-2">
          {t`For a person who lost both their phone and their recovery codes, including someone who belongs to no workspace.`}
        </span>
        <div className="flex gap-2">
          <Input
            type="email"
            value={resetEmail}
            onChange={(e) => setResetEmail(e.target.value)}
            placeholder="name@example.com"
            aria-label={t`Email of the person`}
          />
          <ResetTwoFactorButton
            target={{ email: trimmedEmail }}
            email={trimmedEmail}
            disabled={!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmedEmail)}
            onDone={() => setResetEmail('')}
          />
        </div>
      </div>
    </div>
  )
}
