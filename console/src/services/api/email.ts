import { api } from './client'
import type { EmailProvider } from './workspace'
import type { TestEmailProviderResponse } from './template'

export const emailService = {
  /**
   * Test an email provider configuration by sending a test email
   * @param workspaceId The ID of the workspace
   * @param provider The email provider configuration to test
   * @param to The recipient email address for the test
   * @returns A response indicating success or failure
   */
  testProvider: (
    workspaceId: string,
    provider: EmailProvider,
    to: string
  ): Promise<TestEmailProviderResponse> => {
    return api.post<TestEmailProviderResponse>('/api/email.testProvider', {
      provider,
      to,
      workspace_id: workspaceId
    })
  }
}
