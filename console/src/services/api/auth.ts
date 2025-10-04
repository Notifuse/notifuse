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
  }
  workspaces: Workspace[]
}

/**
 * Check if the current user is the root user
 */
export function isRootUser(userEmail?: string): boolean {
  if (!userEmail || !window.ROOT_EMAIL) {
    return false
  }
  return userEmail === window.ROOT_EMAIL
}

export const authService = {
  signIn: (data: SignInRequest) => api.post<SignInResponse>('/api/user.signin', data),
  verifyCode: (data: VerifyCodeRequest) => api.post<VerifyResponse>('/api/user.verify', data),
  getCurrentUser: () => api.get<GetCurrentUserResponse>('/api/user.me')
}
