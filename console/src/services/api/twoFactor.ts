import { api } from './client'
import { ApiError } from './errors'
import type { SignedInResponse } from './auth'

/** A second factor: a code from the authenticator app, or a recovery code. */
export interface TwoFactorProof {
  code?: string
  recovery_code?: string
}

export interface TwoFactorSetupResponse {
  /** The key to type by hand, base32. Shown once. */
  secret: string
  otpauth_uri: string
}

export interface RecoveryCodesResponse {
  /** Empty when the app was replaced: the existing codes stay valid. */
  recovery_codes: string[]
}

/** Whether a sign-in challenge also accepted the invitation it carried. */
export type InvitationStatus = 'accepted' | 'team_member_limit' | 'invitation_unavailable'

export interface VerifyChallengeResponse extends SignedInResponse {
  invitation_status?: InvitationStatus
}

/**
 * The error codes of two-factor authentication, in the `error` key of the body. None of them
 * is a 401: the client would drop its token.
 */
export type TwoFactorErrorCode =
  | 'two_factor_required'
  | 'reauthentication_required'
  | 'two_factor_invalid_code'
  | 'two_factor_challenge_invalid'
  | 'two_factor_setup_expired'
  | 'two_factor_locked'
  | 'two_factor_totp_locked'
  | 'two_factor_required_by_instance'
  | 'two_factor_required_for_root'

const CODES: ReadonlySet<string> = new Set<TwoFactorErrorCode>([
  'two_factor_required',
  'reauthentication_required',
  'two_factor_invalid_code',
  'two_factor_challenge_invalid',
  'two_factor_setup_expired',
  'two_factor_locked',
  'two_factor_totp_locked',
  'two_factor_required_by_instance',
  'two_factor_required_for_root'
])

/** The two-factor code an API error carries, if any. */
export function twoFactorErrorCode(error: unknown): TwoFactorErrorCode | undefined {
  if (!(error instanceof ApiError)) return undefined
  const code = (error.data as { error?: unknown } | null | undefined)?.error
  return typeof code === 'string' && CODES.has(code) ? (code as TwoFactorErrorCode) : undefined
}

/** Seconds to wait before the next code, for a two_factor_locked answer. */
export function twoFactorRetryAfter(error: unknown): number | undefined {
  if (!(error instanceof ApiError)) return undefined
  const value = (error.data as { retry_after?: unknown } | null | undefined)?.retry_after
  return typeof value === 'number' ? value : undefined
}

export const twoFactorService = {
  setup: (proof: TwoFactorProof = {}) =>
    api.post<TwoFactorSetupResponse>('/api/user.twoFactor.setup', proof),
  enable: (code: string, signOutOtherSessions: boolean) =>
    api.post<RecoveryCodesResponse>('/api/user.twoFactor.enable', {
      code,
      sign_out_other_sessions: signOutOtherSessions
    }),
  disable: (proof: TwoFactorProof) =>
    api.post<{ success: boolean }>('/api/user.twoFactor.disable', proof),
  regenerateRecoveryCodes: (proof: TwoFactorProof) =>
    api.post<RecoveryCodesResponse>('/api/user.twoFactor.regenerateRecoveryCodes', proof),
  verify: (challengeToken: string, proof: TwoFactorProof) =>
    api.post<VerifyChallengeResponse>('/api/user.twoFactor.verify', {
      challenge_token: challengeToken,
      ...proof
    }),
  /** Root only: removes another person's two-factor authentication and signs them out. */
  resetForUser: (target: { user_id: string } | { email: string }, proof: TwoFactorProof) =>
    api.post<{ success: boolean }>('/api/users.resetTwoFactor', { ...target, ...proof })
}
