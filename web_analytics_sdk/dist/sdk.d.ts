/**
 * Notifuse Analytics SDK v6.0
 * Ultra-reliable web analytics for tracking TimeScore metrics
 * V3 Session Payload Architecture
 */
import type { WebIdentity, NotifuseAnalyticsConfig, GoalData, SessionDebugInfo } from './types';
export declare class NotifuseAnalyticsSDK {
    private config;
    private storage;
    private tabStorage;
    private sessionManager;
    private sessionState;
    private sender;
    private deviceDetector;
    private scrollTracker;
    private navigationTracker;
    private crossDomainLinker;
    private deviceInfo;
    private heartbeatTimeout;
    private sendDebounceTimeout;
    private heartbeatState;
    private isMobileDevice;
    private isTracking;
    private isPaused;
    private isInitialized;
    private initPromise;
    private flushed;
    /**
     * Initialize the SDK (called by index.ts from global config or manual init)
     * Returns the init promise so callers can await if needed
     */
    init(userConfig: NotifuseAnalyticsConfig): Promise<void>;
    /**
     * Async initialization logic
     */
    private initializeAsync;
    /**
     * Bind browser events
     */
    private bindEvents;
    /**
     * Visibility change handler
     */
    private onVisibilityChange;
    /**
     * Come back from a hidden, blurred or frozen page.
     *
     * Each of those paths ran flushOnce(), which finalized the current page to
     * send a complete beacon. Reopening it is what makes the visitor's time
     * after they return count towards the page they returned to.
     */
    private resumeTracking;
    /**
     * Window focus handler
     */
    private onFocus;
    /**
     * Window blur handler
     */
    private onBlur;
    /**
     * Page freeze handler (mobile)
     */
    private onFreeze;
    /**
     * Page resume handler (mobile)
     */
    private onResume;
    /**
     * Page unload handler
     */
    private onUnload;
    /**
     * Page show handler (bfcache)
     */
    private onPageShow;
    /**
     * Flush once (deduplicate unload events)
     */
    private flushOnce;
    /**
     * Navigation callback
     */
    private onNavigation;
    /**
     * Start heartbeat with tiered intervals
     */
    private startHeartbeat;
    /**
     * Resume heartbeat after visibility/focus change
     */
    private resumeHeartbeat;
    /**
     * Schedule next heartbeat based on current tier
     */
    private scheduleNextHeartbeat;
    /**
     * Check if we should send a ping right now.
     * Guards against race conditions with visibility changes.
     */
    private shouldSendPing;
    /**
     * Send ping event with SessionState payload
     */
    private sendPingEvent;
    /**
     * Stop heartbeat with optional time accumulation
     */
    private stopHeartbeat;
    /**
     * Get total active time in milliseconds
     */
    private getTotalActiveMs;
    /**
     * Check and update max duration flag
     */
    private checkAndUpdateMaxDuration;
    /**
     * Get current tier based on active time
     */
    private getCurrentTier;
    /**
     * Get current interval based on tier and device type
     */
    private getCurrentInterval;
    /**
     * Reset heartbeat state completely
     */
    private resetHeartbeatState;
    /**
     * Reset page active time only (for SPA navigation)
     */
    private resetPageActiveTime;
    /**
     * Get current page's accumulated focus time in milliseconds.
     * This only counts time when the tab is visible/focused.
     * Used by SessionState to set accurate page duration.
     */
    private getPageActiveMs;
    /**
     * Validate and normalize heartbeat tiers
     */
    private validateTiers;
    /**
     * Schedule a debounced send (for rapid navigations)
     */
    private scheduleDebouncedSend;
    /**
     * Send session payload to server
     * V3: Always sends all actions, always includes attributes.
     * Server uses ReplacingMergeTree to deduplicate events.
     */
    private sendPayload;
    /**
     * Build session attributes from current state
     */
    private buildAttributes;
    /**
     * Get session ID
     */
    getSessionId(): Promise<string>;
    /**
     * Get focus duration in milliseconds
     * In V3, this is calculated from pageview durations in actions[].
     * Current page uses live focus time from heartbeatState.
     */
    getFocusDuration(): Promise<number>;
    /**
     * Get total duration in milliseconds
     */
    getTotalDuration(): Promise<number>;
    /**
     * Track page view
     */
    trackPageView(url?: string): Promise<void>;
    /**
     * Track goal (immediate send)
     */
    trackGoal(data: GoalData): Promise<void>;
    /**
     * Set custom dimension
     */
    setDimension(index: number, value: string): Promise<void>;
    /**
     * Set multiple dimensions
     */
    setDimensions(dimensions: Record<number, string>): Promise<void>;
    /**
     * Get dimension value
     */
    getDimension(index: number): Promise<string | null>;
    /**
     * Clear all dimensions
     */
    clearDimensions(): Promise<void>;
    /**
     * Attach a verified contact identity.
     *
     * The hmac must be minted server-side by the customer over the raw address
     * with their workspace secret. /track is public and unauthenticated, so an
     * unsigned address is discarded — passing one would look like identification
     * while silently doing nothing.
     */
    identify(email: string, hmac: string): Promise<void>;
    /**
     * Current identity, or null when anonymous.
     */
    getIdentity(): Promise<WebIdentity | null>;
    /**
     * Stop future beats carrying the identity.
     *
     * Does NOT anonymize what has already been recorded — see
     * SessionManager.clearIdentity.
     */
    clearIdentity(): Promise<void>;
    /**
     * Pause tracking
     */
    pause(): Promise<void>;
    /**
     * Resume tracking
     */
    resume(): Promise<void>;
    /**
     * Reset session
     */
    /**
     * Roll onto a fresh session id while keeping identity and custom dimensions.
     *
     * Distinct from reset(), which deliberately forgets the visitor. Rotation
     * happens when the inactivity window lapses or the id reaches its absolute
     * age. Whatever the old session accumulated since its last successful beat is
     * sent FIRST, under the old id — without that, every rotation silently drops
     * its own tail, trading a 48h cliff for a small guaranteed loss each time.
     */
    private rotateSession;
    /**
     * Record activity, rotating the session if its window has lapsed.
     *
     * Returns false when a rotation happened, so a caller that was about to open a
     * pageview knows the rotation already did it. Calling this on every beat,
     * navigation and goal is what makes the session window measure activity rather
     * than time since page load.
     */
    private ensureFreshSession;
    reset(): Promise<void>;
    /**
     * Get current configuration (defensive copy)
     */
    getConfig(): Readonly<NotifuseAnalyticsConfig> | null;
    /**
     * Get debug info
     */
    debug(): SessionDebugInfo;
    /**
     * Decorate URL with cross-domain session params
     * Use this for programmatic navigation (window.location.href, window.open)
     */
    decorateUrl(url: string): Promise<string>;
    /**
     * Ensure SDK is initialized (awaits init promise if needed)
     */
    private ensureInitialized;
}
