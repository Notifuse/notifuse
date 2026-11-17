import { api, ApiError } from './client'
import type { Workspace } from './workspace'
import type { Entitlements } from '../../types/license'

// Authentication types
export interface SignInRequest {
  email: string
}

export interface SignInResponse {
  message: string
  code?: string
}

export interface VerifyCodeRequest {
  email: string
  code: string
}

export interface VerifyResponse {
  token: string
}

export interface GetCurrentUserResponse {
  user: {
    id: string
    email: string
    timezone: string
    language: string
  }
  // The backend may return null (not []) when the user has no workspaces,
  // e.g. a freshly installed root account. Callers must normalize to [].
  workspaces: Workspace[] | null
  // What the deployment is licensed for, and whether the console is currently read-only.
  //
  // OPTIONAL because a server older than this bundle does not send them, and because
  // /api/licence.get — the other source — is root-only, so this is the only path by which a
  // non-root console can learn the deployment's licence state without first being refused a
  // write. Absent means "not told", never "unlicensed": LicenseProvider falls back to the
  // root-only endpoint and then to the 402s themselves rather than assuming anything.
  entitlements?: Entitlements
  // Whether this console asks updates.notifuse.com for the releases it is missing: true for the
  // root user and workspace owners when CHECK_FOR_UPDATES is on. Absent (an older server, an
  // API key) means no.
  check_for_updates?: boolean
  // The release this server was built as, sent with that question. Not window.VERSION, which
  // the VERSION environment variable can change.
  version?: string
}

/**
 * Parse a ROOT_EMAIL setting into a list of non-empty emails. Entries may be
 * separated by commas, semicolons, or whitespace (mirrors the backend parser and
 * the tag input's separators). Case is preserved (matching is case-sensitive).
 */
export function parseRootEmails(setting?: string): string[] {
  if (!setting) {
    return []
  }
  return setting.split(/[\s,;]+/).filter(Boolean)
}

/**
 * Check if the given user is one of the configured root users.
 * ROOT_EMAIL may hold multiple comma/semicolon-separated emails.
 */
export function isRootUser(userEmail?: string): boolean {
  if (!userEmail) {
    return false
  }
  return parseRootEmails(window.ROOT_EMAIL).includes(userEmail)
}

export interface LogoutResponse {
  message: string
}

export interface UpdateLanguageResponse {
  message: string
}

/**
 * Complete the OIDC Authorization-Code login by exchanging the one-time code for the
 * internal session JWT. The code is read from the URL fragment by the caller
 * (SignInPage) and passed here in the REQUEST BODY — no cookie is used, so this works
 * across split console/API origins (the shared api.post() can't carry the dedicated
 * cross-origin fetch). Returns the JWT, identical in shape to verifyCode().
 */
async function oidcExchange(code: string): Promise<VerifyResponse> {
  let defaultOrigin = window.location.origin
  if (defaultOrigin.includes('notifusedev.com')) {
    defaultOrigin = 'https://localapi.notifuse.com:4000'
  }
  const apiEndpoint = window.API_ENDPOINT?.trim().replace(/\/+$/, '') || defaultOrigin

  const response = await fetch(`${apiEndpoint}/api/user.oidc.exchange`, {
    method: 'POST',
    // NOTE: no `credentials` — the code is in the body, not a cookie, so this
    // succeeds cross-origin under the existing CORS (* origin + Bearerless) posture.
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code })
  })

  if (!response.ok) {
    const errorData = await response.json().catch(() => null)
    throw new ApiError(errorData?.error || 'OIDC exchange failed', response.status, errorData)
  }
  return response.json() as Promise<VerifyResponse>
}

export const authService = {
  signIn: (data: SignInRequest) => api.post<SignInResponse>('/api/user.signin', data),
  verifyCode: (data: VerifyCodeRequest) => api.post<VerifyResponse>('/api/user.verify', data),
  oidcExchange,
  getCurrentUser: () => api.get<GetCurrentUserResponse>('/api/user.me'),
  logout: () => api.post<LogoutResponse>('/api/user.logout', {}),
  updateLanguage: (language: string) =>
    api.post<UpdateLanguageResponse>('/api/user.updateLanguage', { language })
}
