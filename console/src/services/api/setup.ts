import type { SetupConfig, SetupStatus, PasetoKeys, InitializeResponse } from '../../types/setup'

export const setupApi = {
  /**
   * Get the current installation status
   */
  async getStatus(): Promise<SetupStatus> {
    const response = await fetch('/api/setup.status', {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json'
      }
    })

    if (!response.ok) {
      throw new Error('Failed to fetch setup status')
    }

    return response.json()
  },

  /**
   * Generate new PASETO keys
   */
  async generatePasetoKeys(): Promise<PasetoKeys> {
    const response = await fetch('/api/setup.pasetoKeys', {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json'
      }
    })

    if (!response.ok) {
      throw new Error('Failed to generate PASETO keys')
    }

    return response.json()
  },

  /**
   * Initialize the system with the provided configuration
   */
  async initialize(config: SetupConfig): Promise<InitializeResponse> {
    const response = await fetch('/api/setup.initialize', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(config)
    })

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({ error: 'Unknown error' }))
      throw new Error(errorData.error || 'Failed to initialize setup')
    }

    return response.json()
  }
}
