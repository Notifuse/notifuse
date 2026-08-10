/**
 * Notifuse Analytics SDK v5.0
 * Ultra-reliable web analytics for tracking TimeScore metrics
 *
 * @example
 * ```html
 * <script>
 * window.NotifuseAnalyticsConfig = {
 *   workspace_id: 'ws_abc123',
 *   endpoint: 'https://your-api.com',
 * };
 * </script>
 * <script async src="notifuse-analytics.min.js"></script>
 * ```
 *
 * Then use the SDK (all methods are async):
 * ```typescript
 * // Track custom dimension programmatically
 * await NotifuseAnalytics.setDimension(1, 'premium-user');
 *
 * // Track goal
 * await NotifuseAnalytics.trackGoal({
 *   action: 'purchase',
 *   value: 99.99,
 *   currency: 'USD',
 * });
 * ```
 *
 * Custom dimensions can also be set via URL parameters:
 * ```
 * https://example.com/page?custom_1=campaign_a&custom_2=variant_b
 * ```
 * URL parameters custom_1 through custom_10 are automatically captured on init.
 * Existing dimension values take priority over URL parameters.
 */

import { NotifuseAnalyticsSDK } from './sdk';
import type {
  NotifuseAnalyticsConfig,
  NotifuseAnalyticsAPI,
  GoalData,
  SessionDebugInfo,
} from './types';

// Create singleton instance
const sdk = new NotifuseAnalyticsSDK();

// Public API wrapper with both auto-init and manual init support
const NotifuseAnalytics: NotifuseAnalyticsAPI = {
  init: (config: NotifuseAnalyticsConfig) => sdk.init(config),
  getSessionId: () => sdk.getSessionId(),
  getConfig: () => sdk.getConfig(),
  getFocusDuration: () => sdk.getFocusDuration(),
  getTotalDuration: () => sdk.getTotalDuration(),
  trackPageView: (url?: string) => sdk.trackPageView(url),
  trackGoal: (data: GoalData) => sdk.trackGoal(data),
  setDimension: (index: number, value: string) => sdk.setDimension(index, value),
  setDimensions: (dimensions: Record<number, string>) => sdk.setDimensions(dimensions),
  getDimension: (index: number) => sdk.getDimension(index),
  clearDimensions: () => sdk.clearDimensions(),
  setUserId: (id: string | null) => sdk.setUserId(id),
  getUserId: () => sdk.getUserId(),
  pause: () => sdk.pause(),
  resume: () => sdk.resume(),
  reset: () => sdk.reset(),
  debug: (): SessionDebugInfo => sdk.debug(),
  decorateUrl: (url: string) => sdk.decorateUrl(url),
};

// Export types
export type {
  NotifuseAnalyticsConfig,
  NotifuseAnalyticsAPI,
  GoalData,
  SessionDebugInfo,
};

// Auto-initialize from global config
if (typeof window !== 'undefined' && window.NotifuseAnalyticsConfig) {
  sdk.init(window.NotifuseAnalyticsConfig);
}

// Default export for UMD/ESM/CJS
export default NotifuseAnalytics;
