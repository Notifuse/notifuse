/**
 * Notifuse Analytics SDK Types
 * V3 Session Payload Architecture
 */
declare global {
    interface Window {
        NotifuseAnalyticsConfig?: NotifuseAnalyticsConfig;
    }
}
interface HeartbeatTier {
    /** Duration threshold in ms. Tier applies when activeTime >= after. */
    after: number;
    /** Interval in ms for desktop. null = stop heartbeat. */
    desktopInterval: number | null;
    /** Interval in ms for mobile. null = stop heartbeat. */
    mobileInterval: number | null;
}
interface NotifuseAnalyticsConfig {
    workspace_id: string;
    endpoint: string;
    debug?: boolean;
    sessionTimeout?: number;
    heartbeatInterval?: number;
    /**
     * REPLACES the recognised ad click ids entirely. Use [] to disable click-id
     * capture. To add one while keeping the defaults, use extraAdClickIds.
     */
    adClickIds?: string[];
    /**
     * Ad click ids to recognise IN ADDITION to the defaults.
     *
     * Exists because adClickIds replaces, and DEFAULT_AD_CLICK_IDS is not
     * reachable from a script tag — the config object is authored before the
     * bundle loads — so adding one id would otherwise mean pasting a copy of the
     * whole list, which then goes stale as ids are added here.
     */
    extraAdClickIds?: string[];
    trackSPA?: boolean;
    trackScroll?: boolean;
    trackClicks?: boolean;
    heartbeatTiers?: HeartbeatTier[];
    heartbeatMaxDuration?: number;
    resetHeartbeatOnNavigation?: boolean;
    /** List of domains to share sessions with (e.g., ['blog.example.com', 'shop.example.com']) */
    crossDomains?: string[];
    /** Cross-domain param expiry in seconds (default: 120) */
    crossDomainExpiry?: number;
    /** Strip the cross-domain param from the URL after reading (default: true) */
    crossDomainStripParams?: boolean;
    /** Cross-domain URL parameter name (default: '_nf') */
    crossDomainParam?: string;
}
interface InternalConfig extends Required<Omit<NotifuseAnalyticsConfig, 'workspace_id' | 'endpoint' | 'heartbeatTiers' | 'crossDomains' | 'crossDomainExpiry' | 'crossDomainStripParams' | 'crossDomainParam' | 'extraAdClickIds'>> {
    /** Folded into adClickIds at init; nothing reads it afterwards. */
    extraAdClickIds?: string[];
    workspace_id: string;
    endpoint: string;
    heartbeatTiers: HeartbeatTier[];
    crossDomains: string[];
    crossDomainExpiry: number;
    crossDomainStripParams: boolean;
    crossDomainParam: string;
}
/**
 * A verified contact identity.
 *
 * Either the customer's server signed the address (identify), or a tracked
 * email link carried an opaque token Notifuse minted. Both are checked against
 * the workspace secret server-side; an unsigned address is discarded, so the
 * SDK never stores one on its own.
 */
type WebIdentity = {
    email: string;
    hmac: string;
    token?: undefined;
} | {
    token: string;
    email?: undefined;
    hmac?: undefined;
};
interface Session {
    id: string;
    workspace_id: string;
    created_at: number;
    updated_at: number;
    last_active_at: number;
    focus_duration_ms: number;
    total_duration_ms: number;
    referrer: string | null;
    landing_page: string;
    utm: UTMParams | null;
    max_scroll_percent: number;
    interaction_count: number;
    sdk_version: string;
    sequence: number;
    dimensions: CustomDimensions;
    identity: WebIdentity | null;
}
interface UTMParams {
    source: string | null;
    medium: string | null;
    campaign: string | null;
    term: string | null;
    content: string | null;
    id: string | null;
    id_from: string | null;
}
interface CustomDimensions {
    [key: number]: string;
}
/**
 * Goal types, mirroring domain.ValidGoalTypes on the server.
 *
 * Keep this list in step with ValidGoalTypes in internal/domain/custom_event.go:
 * a type this SDK allows but the server does not recognise is silently recorded
 * as 'other', which is a confusing way to find out.
 */
declare const VALID_GOAL_TYPES: readonly ["purchase", "subscription", "lead", "signup", "booking", "trial", "other"];
type GoalType = (typeof VALID_GOAL_TYPES)[number];
interface GoalData {
    action: string;
    /**
     * What kind of conversion this is. Required: only your site knows whether a
     * goal is a purchase or a lead, and an untyped goal cannot be used by the
     * goal-based segment conditions. Use 'other' if none of the rest fit.
     */
    type: GoalType;
    value?: number;
    /**
     * Anything else the conversion carries — a currency, an order id, a plan
     * name. These reach the server and are stored on the goal, which is why
     * there are no dedicated fields for them: a declared field that nothing
     * forwards, sends or stores is worse than no field, because callers set it
     * and are never told it went nowhere.
     */
    properties?: Record<string, string>;
}
interface NotifuseAnalyticsAPI {
    init(config: NotifuseAnalyticsConfig): Promise<void>;
    getSessionId(): Promise<string>;
    getConfig(): Readonly<NotifuseAnalyticsConfig> | null;
    getFocusDuration(): Promise<number>;
    getTotalDuration(): Promise<number>;
    trackPageView(url?: string): Promise<void>;
    trackGoal(data: GoalData): Promise<void>;
    setDimension(index: number, value: string): Promise<void>;
    setDimensions(dimensions: Record<number, string>): Promise<void>;
    getDimension(index: number): Promise<string | null>;
    clearDimensions(): Promise<void>;
    /** Set authenticated user ID for tracking */
    identify(email: string, hmac: string): Promise<void>;
    /** Get current user ID */
    getIdentity(): Promise<WebIdentity | null>;
    clearIdentity(): Promise<void>;
    pause(): Promise<void>;
    resume(): Promise<void>;
    reset(): Promise<void>;
    debug(): SessionDebugInfo;
    /** Decorate URL with cross-domain session params (for programmatic navigation) */
    decorateUrl(url: string): Promise<string>;
}
interface SessionDebugInfo {
    session: Session | null;
    config: InternalConfig | null;
    isTracking: boolean;
    actionsCount: number;
    currentPage: string | null;
    pageActiveMs: number;
}

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

declare const _default: NotifuseAnalyticsAPI;

export { _default as default };
export type { GoalData, NotifuseAnalyticsAPI, NotifuseAnalyticsConfig, SessionDebugInfo };
