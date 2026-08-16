/**
 * Notifuse Analytics SDK Types
 * V3 Session Payload Architecture
 */

// Global window declaration for NotifuseAnalyticsConfig
declare global {
  interface Window {
    NotifuseAnalyticsConfig?: NotifuseAnalyticsConfig;
  }
}

// Heartbeat Tier Configuration
export interface HeartbeatTier {
  /** Duration threshold in ms. Tier applies when activeTime >= after. */
  after: number;
  /** Interval in ms for desktop. null = stop heartbeat. */
  desktopInterval: number | null;
  /** Interval in ms for mobile. null = stop heartbeat. */
  mobileInterval: number | null;
}

// Heartbeat State
export interface HeartbeatState {
  /** When current active period started */
  activeStartTime: number;
  /** Total accumulated active time in ms (from previous periods) */
  accumulatedActiveMs: number;
  /** Whether heartbeat is currently active */
  isActive: boolean;
  /** Whether max duration has been reached */
  maxDurationReached: boolean;
  /** Timestamp of last ping sent (for drift compensation) */
  lastPingTime: number;
  /** Current tier index (for metadata) */
  currentTierIndex: number;
  /** Page-specific active time (resets on SPA navigation) */
  pageActiveMs: number;
  /** When current page started */
  pageStartTime: number;
}

// Configuration
export interface NotifuseAnalyticsConfig {
  // Required
  workspace_id: string;
  endpoint: string;

  // Optional
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

  // Features
  trackSPA?: boolean;
  trackScroll?: boolean;
  trackClicks?: boolean;

  // Heartbeat (tiered intervals)
  heartbeatTiers?: HeartbeatTier[];
  heartbeatMaxDuration?: number;
  resetHeartbeatOnNavigation?: boolean;

  // Cross-domain tracking (URL decoration)
  /** List of domains to share sessions with (e.g., ['blog.example.com', 'shop.example.com']) */
  crossDomains?: string[];
  /** Cross-domain param expiry in seconds (default: 120) */
  crossDomainExpiry?: number;
  /** Strip the cross-domain param from the URL after reading (default: true) */
  crossDomainStripParams?: boolean;
  /** Cross-domain URL parameter name (default: '_nf') */
  crossDomainParam?: string;
}

export interface InternalConfig extends Required<Omit<NotifuseAnalyticsConfig, 'workspace_id' | 'endpoint' | 'heartbeatTiers' | 'crossDomains' | 'crossDomainExpiry' | 'crossDomainStripParams' | 'crossDomainParam' | 'extraAdClickIds'>> {
  // Note for the next field of this shape: a config option consumed only during
  // init must be added to the `Omit<...>` list above. `Required<>` otherwise
  // forces DEFAULT_CONFIG and every test fixture to carry a field that is dead
  // by the time anything reads the config.
  /** Folded into adClickIds at init; nothing reads it afterwards. */
  extraAdClickIds?: string[];
  workspace_id: string;
  endpoint: string;
  heartbeatTiers: HeartbeatTier[];
  // Cross-domain (optional, with defaults)
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
export type WebIdentity =
  | { email: string; hmac: string; token?: undefined }
  | { token: string; email?: undefined; hmac?: undefined };

// Session
export interface Session {
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

export interface UTMParams {
  source: string | null;
  medium: string | null;
  campaign: string | null;
  term: string | null;
  content: string | null;
  id: string | null;
  id_from: string | null;
}

export interface CustomDimensions {
  [key: number]: string;
}

// Device
export interface DeviceInfo {
  screen_width: number;
  screen_height: number;
  viewport_width: number;
  viewport_height: number;
  device: 'desktop' | 'mobile' | 'tablet';
  browser: string;
  browser_type: string | null;
  os: string;
  user_agent: string;
  connection_type: string;
  timezone: string;
  language: string;
}

// Goal
/**
 * Goal types, mirroring domain.ValidGoalTypes on the server.
 *
 * Keep this list in step with ValidGoalTypes in internal/domain/custom_event.go:
 * a type this SDK allows but the server does not recognise is silently recorded
 * as 'other', which is a confusing way to find out.
 */
export const VALID_GOAL_TYPES = [
  'purchase',
  'subscription',
  'lead',
  'signup',
  'booking',
  'trial',
  'other',
] as const;

export type GoalType = (typeof VALID_GOAL_TYPES)[number];

export interface GoalData {
  action: string;
  /**
   * What kind of conversion this is. Required: only your site knows whether a
   * goal is a purchase or a lead, and an untyped goal cannot be used by the
   * goal-based segment conditions. Use 'other' if none of the rest fit.
   */
  // Making this REQUIRED was a one-time, pre-release allowance — 38.0 had not
  // tagged, so no trackGoal caller existed anywhere. Do not cite it as
  // precedent: after release, adding a required field to a public SDK type
  // breaks every customer's site on their next bundle refresh. A new field must
  // be optional, with the server supplying the default.
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

// Public API
export interface NotifuseAnalyticsAPI {
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

// Debug info (V3)
export interface SessionDebugInfo {
  session: Session | null;
  config: InternalConfig | null;
  isTracking: boolean;
  actionsCount: number;
  currentPage: string | null;
  pageActiveMs: number; // V3: Focus time for current page
}
