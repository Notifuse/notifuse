import { useCallback } from 'react'
import { useLingui } from '@lingui/react/macro'
import { twoFactorErrorCode, twoFactorRetryAfter } from '../../services/api/twoFactor'

/**
 * The sentence a person reads for a two-factor error. Declared as a hook so that `t` is the
 * useLingui() binding the macro rewrites.
 */
export function useTwoFactorErrorMessage() {
  const { t } = useLingui()
  return useCallback(
    (error: unknown, fallback: string): string => {
      switch (twoFactorErrorCode(error)) {
        case 'two_factor_invalid_code':
          return t`That code didn't work. Try the code your app shows now.`
        case 'two_factor_locked': {
          const minutes = Math.max(1, Math.ceil((twoFactorRetryAfter(error) ?? 60) / 60))
          return t`Too many wrong codes. Try again in ${minutes} minutes, or use a recovery code.`
        }
        case 'two_factor_totp_locked':
          return t`Authenticator codes are blocked on your account after too many wrong attempts. Use a recovery code, or ask your Notifuse administrator to reset two-factor authentication.`
        case 'two_factor_challenge_invalid':
          return t`This sign-in expired after 10 minutes. Start again with your email.`
        case 'two_factor_setup_expired':
          return t`This setup expired after 15 minutes. Scan the new QR code below.`
        case 'reauthentication_required':
          return t`Sign in again to continue: turning on two-factor authentication needs a recent sign-in.`
        case 'two_factor_required_by_instance':
          return t`Your administrator requires it, so you can't turn it off.`
        case 'two_factor_required':
          return t`This needs a session where you entered your authentication code.`
        case 'two_factor_required_for_root':
          return t`Turn on two-factor authentication for your own account first, so you don't lock yourself out.`
        default:
          return error instanceof Error && error.message ? error.message : fallback
      }
    },
    [t]
  )
}
