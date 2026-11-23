/** Where a sign-in may send the person back to: a fixed list, never a URL from the address bar. */
export type SignInReturnTo = 'security'

export const SIGN_IN_RETURN_PATHS: Record<SignInReturnTo, string> = {
  security: '/console/account/security'
}

/**
 * Signs this browser in again, then back to the account security page (or the console): the
 * step needs a recent sign-in, or one where the second factor was entered. Only this
 * browser's token goes; the account's other sessions stay.
 */
export function signInAgain(email?: string, returnTo: SignInReturnTo | null = 'security'): void {
  localStorage.removeItem('auth_token')
  const params = new URLSearchParams()
  if (returnTo) params.set('return_to', returnTo)
  if (email) params.set('email', email)
  window.location.assign(`/console/signin?${params.toString()}`)
}
