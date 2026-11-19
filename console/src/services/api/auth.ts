import { api } from './client'
import type { Workspace } from './workspace'

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
  workspaces: Workspace[]
}

/** Split ROOT_EMAIL / admin allowlist (comma, semicolon, or whitespace). */
function parseEmailAllowlist(raw: string | undefined | null): string[] {
  if (!raw) return []
  return String(raw)
    .split(/[,;\s]+/)
    .map((s) => s.trim().toLowerCase())
    .filter((s) => s.includes('@'))
}

/**
 * Check if the current user is the root / Mail Studio super admin.
 * ROOT_EMAIL may be a single address or a comma-separated list.
 * Matching is case-insensitive so portal SSO emails still unlock full chrome.
 */
export function isRootUser(userEmail?: string): boolean {
  if (!userEmail) return false
  const email = userEmail.trim().toLowerCase()
  if (!email.includes('@')) return false
  const roots = parseEmailAllowlist(window.ROOT_EMAIL)
  if (roots.includes(email)) return true
  const extras = parseEmailAllowlist(window.MAIL_STUDIO_ADMIN_EMAILS)
  return extras.includes(email)
}

export interface LogoutResponse {
  message: string
}

export interface UpdateLanguageResponse {
  message: string
}

export const authService = {
  signIn: (data: SignInRequest) => api.post<SignInResponse>('/api/user.signin', data),
  verifyCode: (data: VerifyCodeRequest) => api.post<VerifyResponse>('/api/user.verify', data),
  getCurrentUser: () => api.get<GetCurrentUserResponse>('/api/user.me'),
  logout: () => api.post<LogoutResponse>('/api/user.logout', {}),
  updateLanguage: (language: string) =>
    api.post<UpdateLanguageResponse>('/api/user.updateLanguage', { language })
}
