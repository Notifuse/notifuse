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

/*
 * Kept out of the JSDoc above on purpose: rollup emits that block into
 * dist/*.d.ts and the unminified bundles, which ship to npm consumers. This
 * note is for maintainers only.
 *
 * Renaming or re-shaping anything in the public API: `npm run type-check`
 * passing does NOT mean the change is complete. These mirror the public surface
 * and none of them is typechecked —
 *   tests/e2e/fixtures/test-page.html   (inline JS)
 *   tests/e2e/*.spec.ts                 (tsconfig excludes tests/)
 *   README.md                           (examples)
 *   the @example block above
 *   ../../docs/web-analytics/*.mdx      (separate repository)
 * Budget a manual grep sweep for the old name.
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
  identify: (email: string, hmac: string) => sdk.identify(email, hmac),
  getIdentity: () => sdk.getIdentity(),
  clearIdentity: () => sdk.clearIdentity(),
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

/**
 * Make installing the bundle idempotent.
 *
 * The same bundle is served at /na.js and /na.<hash>.js, so a site mid-migration
 * (legacy hardcoded tag plus a tag-manager tag) evaluates it twice. The
 * re-entrancy guard inside the SDK is per-instance, so nothing would stop the
 * second copy initialising — and both copies would then share one session id,
 * one persisted snapshot key and one tab_id, clobbering each other's actions and
 * colliding on seq. tab_id cannot separate them: sessionStorage is per tab, not
 * per instance, so the fix has to be here, at the install.
 *
 * Returning the FIRST wrapper matters as much as skipping the second init: the
 * UMD wrapper assigns window.NotifuseAnalytics from this default export, so a
 * fresh object would leave the global pointing at a dead instance and every
 * later trackGoal() would land there. First install wins — a customer may have
 * an older cached hash alongside the new one, and tearing down a running
 * instance to promote a newer bundle is more dangerous than running the older
 * one until the duplicate tag is removed.
 *
 * A plain flag is enough: script evaluation is atomic on a single thread, so
 * there is no race even with async tags.
 */
const INSTALL_KEY = '__notifuseAnalytics';
type InstallHost = Record<string, NotifuseAnalyticsAPI | undefined>;
const host = (typeof window !== 'undefined' ? window : undefined) as unknown as
  | InstallHost
  | undefined;
const alreadyInstalled = host?.[INSTALL_KEY];

if (!alreadyInstalled) {
  if (host) {
    host[INSTALL_KEY] = NotifuseAnalytics;
  }
  // Auto-initialize from global config
  if (typeof window !== 'undefined' && window.NotifuseAnalyticsConfig) {
    sdk.init(window.NotifuseAnalyticsConfig);
  }
} else {
  console.warn(
    '[NotifuseAnalytics] SDK already loaded on this page; ignoring the duplicate ' +
      'install. Remove the extra script tag — two copies would corrupt each ' +
      "other's session data."
  );
}

// Default export for UMD/ESM/CJS
export default alreadyInstalled ?? NotifuseAnalytics;
