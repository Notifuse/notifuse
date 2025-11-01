import { api } from './client'
import type {
  SetupConfig,
  SetupStatus,
  InitializeResponse,
  TestSMTPConfig,
  TestSMTPResponse
} from '../../types/setup'

export const setupApi = {
  /**
   * Get the current installation status
   */
  async getStatus(): Promise<SetupStatus> {
    return api.get<SetupStatus>('/api/setup.status')
  },

  /**
   * Initialize the system with the provided configuration
   */
  async initialize(config: SetupConfig): Promise<InitializeResponse> {
    return api.post<InitializeResponse>('/api/setup.initialize', config)
  },

  /**
   * Test SMTP connection with the provided configuration
   */
  async testSmtp(config: TestSMTPConfig): Promise<TestSMTPResponse> {
    return api.post<TestSMTPResponse>('/api/setup.testSmtp', config)
  }
}
