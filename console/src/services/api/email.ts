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
    to: string,
    /**
     * The saved integration being tested, when there is one. Credentials are not
     * served to clients, so the provider object above carries blanks for them;
     * the server fills those from this integration. Omit it when testing a
     * provider that has not been saved — the form still holds what was typed.
     */
    integrationId?: string
  ): Promise<TestEmailProviderResponse> => {
    return api.post<TestEmailProviderResponse>('/api/email.testProvider', {
      provider,
      to,
      workspace_id: workspaceId,
      ...(integrationId ? { integration_id: integrationId } : {})
    })
  }
}
