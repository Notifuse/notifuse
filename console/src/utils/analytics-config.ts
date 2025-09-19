import { analyticsService } from '../services/api/analytics'

// Configuration utility for the analytics service
export const configureAnalytics = (maxConcurrency: number = 1) => {
  analyticsService.configure({
    maxConcurrency,
    baseUrl: window.API_ENDPOINT || 'http://localhost:3000'
  })
}

// Helper function to query analytics with workspace context
export const queryAnalytics = async (query: any, workspaceId: string) => {
  return analyticsService.query(query, workspaceId)
}

// Get analytics service status for debugging
export const getAnalyticsStatus = () => {
  return analyticsService.getQueueStatus()
}

// Default configuration - can be called during app initialization
export const initializeAnalytics = () => {
  configureAnalytics(1) // Default to 1 concurrent request
}
