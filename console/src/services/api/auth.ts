import { api } from './client'
import {
  SignInRequest,
  SignInResponse,
  VerifyCodeRequest,
  VerifyResponse,
  GetCurrentUserResponse
} from './types'

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
