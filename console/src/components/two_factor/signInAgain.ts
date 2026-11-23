import { handOffSignInEmail } from '../setup/signInHandoff'

/** Where a sign-in may send the person back to: a fixed list, never a URL from the address bar. */
export type SignInReturnTo = 'security'

export const SIGN_IN_RETURN_PATHS: Record<SignInReturnTo, string> = {
  security: '/console/account/security'
}

// Single sign-on leaves the console for the identity provider and comes back without the query
// string: the return point waits in this tab's session storage.
const RETURN_KEY = 'notifuse_signin_return_to'

export function rememberReturnTo(returnTo: SignInReturnTo | undefined): void {
  try {
    if (returnTo) sessionStorage.setItem(RETURN_KEY, returnTo)
  } catch {
    // No storage: the sign-in lands on the console home.
  }
}

/** Read once after a sign-in: the return point single sign-on carried across. */
export function takeReturnTo(): SignInReturnTo | undefined {
  try {
    const value = sessionStorage.getItem(RETURN_KEY)
    sessionStorage.removeItem(RETURN_KEY)
    return value === 'security' ? value : undefined
  } catch {
    return undefined
  }
}

/**
 * Signs this browser in again, then back to the account security page (or the console): the
 * step needs a recent sign-in, or one where the second factor was entered. Only this
 * browser's token goes; the account's other sessions stay. The email is prefilled, not sent:
 * the person chooses between a code and single sign-on.
 */
export function signInAgain(email?: string, returnTo: SignInReturnTo | null = 'security'): void {
  localStorage.removeItem('auth_token')
  handOffSignInEmail(email)
  const params = new URLSearchParams()
  if (returnTo) params.set('return_to', returnTo)
  const query = params.toString()
  window.location.assign(`/console/signin${query ? `?${query}` : ''}`)
}
