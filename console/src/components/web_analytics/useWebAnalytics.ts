import { useContext } from 'react'
import { WebAnalyticsContext, type WebAnalyticsContextValue } from './webAnalyticsContext'

export function useWebAnalytics(): WebAnalyticsContextValue {
  const context = useContext(WebAnalyticsContext)
  if (!context) {
    throw new Error('useWebAnalytics must be used inside a WebAnalyticsProvider')
  }
  return context
}
