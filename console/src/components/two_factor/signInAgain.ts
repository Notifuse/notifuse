import { handOffSignInEmail } from '../setup/signInHandoff'

/** The MCP connection page an owner signs in from to create a key there. */
export type OAuthAuthorizeReturnPath = `/console/oauth/authorize?request=${string}`

/**
 * Where a sign-in may send the person back to: the account security page, or the MCP connection
 * page of one request. Never any other path or URL from the address bar.
 */
export type SignInReturnTo = 'security' | OAuthAuthorizeReturnPath

/** A request ID as the server mints it (base64url), bounded. */
const OAUTH_REQUEST_ID = /^[A-Za-z0-9_-]{1,128}$/

// The whole string: one path, one parameter, nothing before or after. A dot segment, another
// host, a second parameter or an encoded character fail it.
const OAUTH_RETURN_PATH = /^\/console\/oauth\/authorize\?request=([A-Za-z0-9_-]{1,128})$/

/** The return path to the connection page of a request, or undefined for an ID it cannot carry. */
export function oauthAuthorizeReturnPath(requestId: string): OAuthAuthorizeReturnPath | undefined {
  return OAUTH_REQUEST_ID.test(requestId) ? `/console/oauth/authorize?request=${requestId}` : undefined
}

/** A return point read from the address bar or storage, or undefined when it is not one of the list. */
export function parseSignInReturnTo(value: unknown): SignInReturnTo | undefined {
  if (value === 'security') return value
  if (typeof value !== 'string') return undefined
  const match = OAUTH_RETURN_PATH.exec(value)
  return match ? oauthAuthorizeReturnPath(match[1]) : undefined
}

/** Where the router goes after a sign-in. */
export type SignInDestination =
  | { to: '/console' }
  | { to: '/console/account/security' }
  | { to: '/console/oauth/authorize'; search: { request: string } }

/** The page a return point names, checked again; the console home without one. */
export function signInDestination(returnTo: SignInReturnTo | undefined): SignInDestination {
  if (returnTo === 'security') return { to: '/console/account/security' }
  const match = returnTo ? OAUTH_RETURN_PATH.exec(returnTo) : null
  return match ? { to: '/console/oauth/authorize', search: { request: match[1] } } : { to: '/console' }
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
    return parseSignInReturnTo(value)
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
