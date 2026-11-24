import { api } from './client'
import type {
  SystemSettingsResponse,
  SystemSettingsData,
  SystemSettingsUpdateResponse
} from '../../types/settings'
import type { TestSMTPConfig, TestSMTPResponse } from '../../types/setup'

export const settingsApi = {
  async get(): Promise<SystemSettingsResponse> {
    return api.get<SystemSettingsResponse>('/api/settings.get')
  },

  async update(settings: SystemSettingsData): Promise<SystemSettingsUpdateResponse> {
    return api.post<SystemSettingsUpdateResponse>('/api/settings.update', settings)
  },

  async testSmtp(config: TestSMTPConfig): Promise<TestSMTPResponse> {
    return api.post<TestSMTPResponse>('/api/settings.testSmtp', config)
  }
}
