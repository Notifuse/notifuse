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
 *   type: 'purchase',
 *   value: 99.99,
 *   properties: { currency: 'USD', order_id: 'A-1234' },
 * });
 * ```
 *
 * Custom dimensions can also be set via URL parameters:
 * ```
 * https://example.com/page?custom_1=campaign_a&custom_2=variant_b
 * ```
 *
 * URL parameters custom_1 through custom_10 are automatically captured on init.
 * Existing dimension values take priority over URL parameters.
 */
import type { NotifuseAnalyticsConfig, NotifuseAnalyticsAPI, GoalData, SessionDebugInfo } from './types';
export type { NotifuseAnalyticsConfig, NotifuseAnalyticsAPI, GoalData, SessionDebugInfo, };
declare const _default: NotifuseAnalyticsAPI;
export default _default;
