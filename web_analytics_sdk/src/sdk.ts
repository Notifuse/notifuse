/**
 * Notifuse Analytics SDK v6.0
 * Ultra-reliable web analytics for tracking TimeScore metrics
 * V3 Session Payload Architecture
 */

import type {
  WebIdentity,
  NotifuseAnalyticsConfig,
  InternalConfig,
  GoalData,
  SessionDebugInfo,
  DeviceInfo,
  HeartbeatTier,
  HeartbeatState,
  GoalType,
} from './types';
import { VALID_GOAL_TYPES } from './types';
import type { SessionAttributes } from './types/session-state';
import { Storage, TabStorage } from './storage/storage';
import { SessionManager } from './core/session';
import { SessionState, SessionStateConfig } from './core/session-state';
import { Sender } from './transport/sender';
import { DeviceDetector } from './detection/device';
import { ScrollTracker } from './events/scroll';
import { NavigationTracker } from './events/navigation';
import { isBot } from './detection/bot';
import { DEFAULT_AD_CLICK_IDS } from './utils/utm';
import { CrossDomainLinker, DEFAULT_CROSS_DOMAIN_PARAM } from './core/cross-domain';
import { parseCustomDimensions } from './utils/custom-dimensions';
import { monotonicNow } from './utils/clock';

// Heartbeat constants
const MIN_HEARTBEAT_INTERVAL = 5000; // 5 seconds minimum
const MIN_HEARTBEAT_MAX_DURATION = 60 * 1000; // 1 minute minimum
const SEND_DEBOUNCE_MS = 100;

/** URL parameter a tracked email link uses to hand over a verified identity. */
const IDENTIFY_PARAM = 'nf_id';

// Default heartbeat tiers
const DEFAULT_HEARTBEAT_TIERS: HeartbeatTier[] = [
  // 0-3 min: High frequency (initial engagement is critical)
  { after: 0, desktopInterval: 10000, mobileInterval: 7000 },
  // 3-5 min: Medium frequency (user is engaged, reduce load)
  { after: 3 * 60 * 1000, desktopInterval: 20000, mobileInterval: 14000 },
  // 5-10 min: Low frequency (long-form content, minimal pings)
  { after: 5 * 60 * 1000, desktopInterval: 30000, mobileInterval: 21000 },
];

// Default configuration
const DEFAULT_CONFIG: Omit<InternalConfig, 'workspace_id' | 'endpoint'> = {
  debug: false,
  sessionTimeout: 30 * 60 * 1000, // 30 minutes
  heartbeatInterval: 10000, // 10 seconds (legacy, used as fallback)
  adClickIds: DEFAULT_AD_CLICK_IDS,
  trackSPA: true,
  trackScroll: true,
  trackClicks: false,
  heartbeatTiers: DEFAULT_HEARTBEAT_TIERS,
  heartbeatMaxDuration: 10 * 60 * 1000, // 10 minutes
  resetHeartbeatOnNavigation: false,
  // Cross-domain tracking
  crossDomains: [],
  crossDomainExpiry: 120, // 2 minutes
  crossDomainStripParams: true,
  crossDomainParam: DEFAULT_CROSS_DOMAIN_PARAM,
};

export class NotifuseAnalyticsSDK {
  private config: InternalConfig | null = null;
  private storage: Storage | null = null;
  private tabStorage: TabStorage | null = null;
  private sessionManager: SessionManager | null = null;
  private sessionState: SessionState | null = null;
  private sender: Sender | null = null;
  private deviceDetector: DeviceDetector | null = null;
  private scrollTracker: ScrollTracker | null = null;
  private navigationTracker: NavigationTracker | null = null;
  private crossDomainLinker: CrossDomainLinker | null = null;
  private deviceInfo: DeviceInfo | null = null;
  private heartbeatTimeout: ReturnType<typeof setTimeout> | null = null;
  private sendDebounceTimeout: ReturnType<typeof setTimeout> | null = null;
  private heartbeatState: HeartbeatState = {
    activeStartTime: 0,
    accumulatedActiveMs: 0,
    isActive: false,
    maxDurationReached: false,
    lastPingTime: 0,
    currentTierIndex: 0,
    pageActiveMs: 0,
    pageStartTime: 0,
  };
  private isMobileDevice = false;
  private isTracking = false;
  private isPaused = false;
  private isInitialized = false;
  private initPromise: Promise<void> | null = null;
  private flushed = false;

  /**
   * Initialize the SDK (called by index.ts from global config or manual init)
   * Returns the init promise so callers can await if needed
   */
  init(userConfig: NotifuseAnalyticsConfig): Promise<void> {
    // If already initialized or initializing, return existing promise
    if (this.isInitialized) {
      return Promise.resolve();
    }
    if (this.initPromise) {
      return this.initPromise;
    }

    // Store the promise for ensureInitialized to await
    this.initPromise = this.initializeAsync(userConfig);
    return this.initPromise;
  }

  /**
   * Async initialization logic
   */
  private async initializeAsync(userConfig: NotifuseAnalyticsConfig): Promise<void> {
    // Validate required fields
    if (!userConfig.workspace_id) {
      throw new Error('workspace_id is required');
    }
    if (!userConfig.endpoint) {
      throw new Error('endpoint is required');
    }

    // Check for bots
    if (isBot()) {
      console.log('[NotifuseAnalytics] Bot detected, tracking disabled');
      return;
    }

    // Merge config
    this.config = {
      ...DEFAULT_CONFIG,
      ...userConfig,
    } as InternalConfig;

    // Fold extraAdClickIds into the effective list, so everything downstream
    // keeps reading a single config.adClickIds. Applied AFTER the merge, so it
    // extends whichever list won — the defaults, or a caller's replacement.
    const extra = this.config.extraAdClickIds;
    if (extra && extra.length > 0) {
      const effective = [...this.config.adClickIds];
      for (const id of extra) {
        if (!effective.some((known) => known.toLowerCase() === id.toLowerCase())) {
          effective.push(id);
        }
      }
      this.config.adClickIds = effective;
    }

    // Validate and normalize heartbeat tiers
    this.config.heartbeatTiers = this.validateTiers(this.config.heartbeatTiers);

    // Validate heartbeat max duration
    if (this.config.heartbeatMaxDuration !== 0 &&
        this.config.heartbeatMaxDuration < MIN_HEARTBEAT_MAX_DURATION) {
      this.config.heartbeatMaxDuration = MIN_HEARTBEAT_MAX_DURATION;
    }

    // Initialize storage
    this.storage = new Storage();
    this.tabStorage = new TabStorage();

    // Initialize device detector
    this.deviceDetector = new DeviceDetector();
    this.deviceInfo = await this.deviceDetector.detectWithClientHints();

    // Set mobile device flag for heartbeat intervals
    this.isMobileDevice = this.deviceInfo?.device !== 'desktop';

    // Read cross-domain param BEFORE session creation
    const crossDomainPayload = CrossDomainLinker.readParam(
      this.config.crossDomainExpiry,
      this.config.crossDomainParam
    );

    // Initialize session manager
    this.sessionManager = new SessionManager(
      this.storage,
      this.tabStorage,
      this.config
    );

    // Inject cross-domain payload if present
    if (crossDomainPayload) {
      this.sessionManager.setCrossDomainInput({
        sessionId: crossDomainPayload.s,
        timestamp: crossDomainPayload.t,
        expiry: this.config.crossDomainExpiry,
      });
    }

    // Read and strip nf_id BEFORE the session is created.
    //
    // getOrCreateSession snapshots window.location.href as landing_page and
    // persists it, and every later beat re-sends it — so stripping afterwards
    // would leave the workspace-signed credential sitting in the customer's own
    // landing-page reports, which is the exact leak the stripping exists to
    // prevent. The token is applied below, once there is a session to attach it
    // to; only the URL rewrite has to happen first.
    const identifyToken = new URLSearchParams(window.location.search).get(IDENTIFY_PARAM);
    if (identifyToken) {
      CrossDomainLinker.stripParam(IDENTIFY_PARAM);
    }

    // Get or create session (uses cross-domain payload if valid)
    const session = this.sessionManager.getOrCreateSession();

    // Apply URL dimensions (existing values take priority)
    const urlDimensions = parseCustomDimensions(window.location.href);
    if (Object.keys(urlDimensions).length > 0) {
      this.sessionManager.applyUrlDimensions(urlDimensions);
    }

    // Strip _nf param from URL after processing
    if (crossDomainPayload && this.config.crossDomainStripParams) {
      CrossDomainLinker.stripParam(this.config.crossDomainParam);
    }

    // Adopt an identity handed over by a tracked email link.
    //
    // Stripped immediately: the parameter is a workspace-signed credential, and
    // leaving it in the address bar puts it into the customer's own analytics,
    // their server logs, and the Referer of every third-party asset the page
    // loads. Stripping before the first beat also stops it being re-adopted on
    // an SPA navigation.
    //
    // Deliberately NOT propagated through the cross-domain _nf handoff: that
    // would spray the credential across every configured domain. A visitor who
    // crosses domains is simply anonymous there until that origin identifies
    // them itself.
    if (identifyToken) {
      this.sessionManager.setIdentity({ token: identifyToken });
    }

    // Initialize sender
    this.sender = new Sender(
      this.config.endpoint,
      this.storage,
      this.config.debug,
      this.sessionManager.getTabId()
    );

    // Drain anything left over from a previous visit. The `online` event is the
    // only other trigger, and a fresh page load can never observe it — so
    // without this the commonest offline pattern (browse offline, close the tab,
    // reconnect with no page open, come back later) leaves those beats sitting
    // in storage until the TTL silently discards them. Fire-and-forget: a replay
    // must never delay this page's own first beat.
    this.sender.flushQueue().catch(() => {});

    // Initialize scroll tracker
    if (this.config.trackScroll) {
      this.scrollTracker = new ScrollTracker();
      // No milestone callback needed - we just track max scroll
      this.scrollTracker.start();
    }

    // Initialize navigation tracker
    if (this.config.trackSPA) {
      this.navigationTracker = new NavigationTracker();
      this.navigationTracker.setNavigationCallback((url) => this.onNavigation(url));
      this.navigationTracker.start();
    }

    // Initialize cross-domain linker if configured
    if (this.config.crossDomains.length > 0) {
      this.crossDomainLinker = new CrossDomainLinker({
        domains: this.config.crossDomains,
        expiry: this.config.crossDomainExpiry,
        debug: this.config.debug,
        paramName: this.config.crossDomainParam,
      });
      this.crossDomainLinker.setIdGetters(
        () => this.sessionManager?.getSessionId() || ''
      );
      this.crossDomainLinker.start();
    }

    // Initialize SessionState (V3)
    const sessionStateConfig: SessionStateConfig = {
      workspace_id: this.config.workspace_id,
      session_id: session.id,
      created_at: session.created_at,
      tab_id: this.sessionManager.getTabId(),
    };
    this.sessionState = new SessionState(sessionStateConfig);
    this.sessionState.restore(); // Restore from sessionStorage if available

    // Wire up focus time getter for accurate page duration tracking
    this.sessionState.setFocusTimeGetter(() => this.getPageActiveMs());

    // Add initial pageview
    this.sessionState.addPageview(window.location.pathname);

    // Bind events
    this.bindEvents();

    // Start tracking
    this.isTracking = true;
    this.isInitialized = true;

    // Initialize heartbeat state
    const now = monotonicNow();
    this.heartbeatState.pageStartTime = now;
    this.heartbeatState.activeStartTime = now;

    // Start heartbeat
    this.startHeartbeat();

    // Send initial payload (immediate, with attributes)
    await this.sendPayload();

    if (this.config.debug) {
      console.log('[NotifuseAnalytics] Initialized', {
        session_id: session.id,
        device: this.deviceInfo,
      });
    }
  }

  /**
   * Bind browser events
   */
  private bindEvents(): void {
    // Visibility change (tab switch, minimize)
    document.addEventListener('visibilitychange', this.onVisibilityChange);

    // Window focus/blur (alt-tab, window switch)
    window.addEventListener('focus', this.onFocus);
    window.addEventListener('blur', this.onBlur);

    // Page lifecycle (mobile freeze/resume)
    document.addEventListener('freeze', this.onFreeze);
    document.addEventListener('resume', this.onResume);

    // Page unload
    window.addEventListener('pagehide', this.onUnload);
    window.addEventListener('beforeunload', this.onUnload);

    // Back-forward cache
    window.addEventListener('pageshow', this.onPageShow);
  }

  /**
   * Visibility change handler
   */
  private onVisibilityChange = (): void => {
    if (document.visibilityState === 'hidden') {
      this.stopHeartbeat(true); // Accumulate active time
      this.flushOnce();
    } else if (document.visibilityState === 'visible') {
      this.resumeTracking();
    }
  };

  /**
   * Come back from a hidden, blurred or frozen page.
   *
   * Each of those paths ran flushOnce(), which finalized the current page to
   * send a complete beacon. Reopening it is what makes the visitor's time
   * after they return count towards the page they returned to.
   */
  private resumeTracking(): void {
    this.flushed = false;
    this.sessionState?.reopenCurrentPage();
    if (!this.isPaused && !this.heartbeatState.maxDurationReached) {
      // Resume heartbeat with fresh timing
      this.resumeHeartbeat();
    }
  }

  /**
   * Window focus handler
   */
  private onFocus = (): void => {
    this.resumeTracking();
  };

  /**
   * Window blur handler
   */
  private onBlur = (): void => {
    this.stopHeartbeat(true); // Accumulate active time
    this.flushOnce();
  };

  /**
   * Page freeze handler (mobile)
   */
  private onFreeze = (): void => {
    this.stopHeartbeat(true); // Accumulate active time
    this.flushOnce();
  };

  /**
   * Page resume handler (mobile)
   */
  private onResume = (): void => {
    this.resumeTracking();
  };

  /**
   * Page unload handler
   */
  private onUnload = (): void => {
    this.flushOnce();
  };

  /**
   * Page show handler (bfcache)
   */
  private onPageShow = (event: PageTransitionEvent): void => {
    if (event.persisted) {
      // Restore SessionState first: it rewrites currentPageIndex from the
      // snapshot, which would undo the reopen below.
      this.sessionState?.restore();
      this.resumeTracking();
      if (this.config?.debug) {
        console.log('[NotifuseAnalytics] Restored from bfcache');
      }
    }
  };

  /**
   * Flush once (deduplicate unload events)
   */
  private flushOnce(): void {
    if (this.flushed) return;
    this.flushed = true;

    if (!this.sessionState || !this.sender) return;

    // Update scroll before finalizing
    if (this.scrollTracker) {
      this.sessionState.updateScroll(this.scrollTracker.getMaxScrollPercent());
    }

    // Finalize current page
    this.sessionState.finalizeForUnload();

    // Build and send via beacon
    const attributes = this.buildAttributes();
    const payload = this.sessionState.buildPayload(attributes, {
      identity: this.sessionManager?.getIdentity() ?? null,
      dimensions: this.sessionManager?.getDimensionsPayload() ?? {},
    });
    this.sender.sendSessionBeacon(payload);

    // Persist final state
    this.sessionState.persist();
  }

  /**
   * Navigation callback
   */
  private onNavigation(url: string): void {
    if (!this.sessionState) return;

    if (this.config?.debug) {
      console.log('[NotifuseAnalytics] Navigation:', url);
    }

    // Update scroll before finalizing page
    if (this.scrollTracker) {
      this.sessionState.updateScroll(this.scrollTracker.getMaxScrollPercent());
    }

    // Add new pageview (this finalizes the previous page)
    this.sessionState.addPageview(window.location.pathname);

    // Reset scroll tracking for new page
    this.scrollTracker?.reset();

    // Reset page timer
    this.resetPageActiveTime();

    // Optionally reset session heartbeat timer
    if (this.config?.resetHeartbeatOnNavigation) {
      this.resetHeartbeatState();
      this.startHeartbeat();
    }

    // Debounced send (navigation can be rapid in SPAs)
    this.scheduleDebouncedSend();

    // Persist state
    this.sessionState.persist();
  }

  /**
   * Start heartbeat with tiered intervals
   */
  private startHeartbeat(): void {
    if (!this.config) return;

    // Don't restart if max duration reached
    if (this.heartbeatState.maxDurationReached) {
      if (this.config.debug) {
        console.log('[NotifuseAnalytics] Heartbeat not started: max duration reached');
      }
      return;
    }

    // Clear existing heartbeat
    this.stopHeartbeat(false); // Don't accumulate time (we're starting fresh)

    // Record when we became active
    const now = monotonicNow();
    this.heartbeatState.activeStartTime = now;
    this.heartbeatState.isActive = true;
    this.heartbeatState.lastPingTime = now;

    // Start the heartbeat loop
    this.scheduleNextHeartbeat();
  }

  /**
   * Resume heartbeat after visibility/focus change
   */
  private resumeHeartbeat(): void {
    if (!this.config) return;

    // Don't restart if max duration reached
    if (this.heartbeatState.maxDurationReached) {
      return;
    }

    // Resume with fresh timing
    const now = monotonicNow();
    this.heartbeatState.activeStartTime = now;
    this.heartbeatState.pageStartTime = now;
    this.heartbeatState.isActive = true;
    this.heartbeatState.lastPingTime = now;
    this.scheduleNextHeartbeat();
  }

  /**
   * Schedule next heartbeat based on current tier
   */
  private scheduleNextHeartbeat(): void {
    if (!this.config || !this.heartbeatState.isActive) return;

    // Check max duration BEFORE scheduling
    if (this.checkAndUpdateMaxDuration()) {
      this.stopHeartbeat(true);
      return;
    }

    // Get current interval based on active time
    const interval = this.getCurrentInterval();

    // Null interval means stop (tier config says to stop)
    if (interval === null) {
      this.heartbeatState.maxDurationReached = true;
      this.stopHeartbeat(true);
      if (this.config.debug) {
        console.log('[NotifuseAnalytics] Heartbeat stopped by tier configuration');
      }
      return;
    }

    // Calculate target time with drift compensation
    const targetTime = this.heartbeatState.lastPingTime + interval;
    const now = monotonicNow();
    const delay = Math.max(0, targetTime - now);

    // Schedule next ping
    this.heartbeatTimeout = setTimeout(() => {
      // CRITICAL: Check visibility and state before sending
      if (this.shouldSendPing()) {
        const actualTime = monotonicNow();
        const drift = actualTime - targetTime;

        // Log excessive drift in debug mode
        if (drift > 1000 && this.config?.debug) {
          console.warn(`[NotifuseAnalytics] Heartbeat drift: ${drift}ms`);
        }

        // Update tier index for metadata
        const tierResult = this.getCurrentTier();
        if (tierResult) {
          this.heartbeatState.currentTierIndex = tierResult.index;
        }

        // Send ping with SessionState payload
        this.sendPingEvent();

        // Update last ping time for next calculation
        this.heartbeatState.lastPingTime = actualTime;

        // Schedule next ping
        this.scheduleNextHeartbeat();
      }
    }, delay);
  }

  /**
   * Check if we should send a ping right now.
   * Guards against race conditions with visibility changes.
   */
  private shouldSendPing(): boolean {
    return (
      !this.isPaused &&
      this.isTracking &&
      this.heartbeatState.isActive &&
      !document.hidden &&
      document.visibilityState === 'visible'
    );
  }

  /**
   * Send ping event with SessionState payload
   */
  private sendPingEvent(): void {
    if (!this.sessionState) return;

    // Update scroll from ScrollTracker
    if (this.scrollTracker) {
      this.sessionState.updateScroll(this.scrollTracker.getMaxScrollPercent());
    }

    // Send periodic payload (non-blocking). ensureFreshSession both records the
    // activity this beat represents and rotates a session that has aged out; it
    // sends on its own when it rotates, so only the un-rotated path sends here.
    this.ensureFreshSession()
      .then((same) => (same ? this.sendPayload() : undefined))
      .catch(() => {});
  }

  /**
   * Stop heartbeat with optional time accumulation
   */
  private stopHeartbeat(accumulateTime: boolean = true): void {
    // Accumulate active time before stopping
    if (accumulateTime && this.heartbeatState.isActive) {
      const now = monotonicNow();
      const activeTime = now - this.heartbeatState.activeStartTime;
      this.heartbeatState.accumulatedActiveMs += activeTime;

      // Also accumulate page active time
      const pageTime = now - this.heartbeatState.pageStartTime;
      this.heartbeatState.pageActiveMs += pageTime;
    }

    this.heartbeatState.isActive = false;

    if (this.heartbeatTimeout) {
      clearTimeout(this.heartbeatTimeout);
      this.heartbeatTimeout = null;
    }
  }

  /**
   * Get total active time in milliseconds
   */
  private getTotalActiveMs(): number {
    let total = this.heartbeatState.accumulatedActiveMs;
    if (this.heartbeatState.isActive) {
      total += monotonicNow() - this.heartbeatState.activeStartTime;
    }
    return total;
  }

  /**
   * Check and update max duration flag
   */
  private checkAndUpdateMaxDuration(): boolean {
    if (!this.config || this.config.heartbeatMaxDuration === 0) {
      return false; // Unlimited
    }

    const totalActiveMs = this.getTotalActiveMs();

    if (totalActiveMs >= this.config.heartbeatMaxDuration) {
      this.heartbeatState.maxDurationReached = true;
      if (this.config.debug) {
        const tierResult = this.getCurrentTier();
        console.log(
          `[NotifuseAnalytics] Heartbeat max duration reached ` +
          `(${Math.round(totalActiveMs / 1000)}s active, tier ${tierResult?.index ?? 0})`
        );
      }
      return true;
    }

    return false;
  }

  /**
   * Get current tier based on active time
   */
  private getCurrentTier(): { tier: HeartbeatTier; index: number } | null {
    if (!this.config) return null;

    const totalActiveMs = this.getTotalActiveMs();
    const tiers = this.config.heartbeatTiers;

    // Find the highest tier that applies (tiers sorted by 'after' ascending)
    let currentTier = tiers[0];
    let currentIndex = 0;

    for (let i = 0; i < tiers.length; i++) {
      if (totalActiveMs >= tiers[i].after) {
        currentTier = tiers[i];
        currentIndex = i;
      } else {
        break;
      }
    }

    return { tier: currentTier, index: currentIndex };
  }

  /**
   * Get current interval based on tier and device type
   */
  private getCurrentInterval(): number | null {
    const result = this.getCurrentTier();
    if (!result) return null;

    const { tier } = result;
    return this.isMobileDevice ? tier.mobileInterval : tier.desktopInterval;
  }

  /**
   * Reset heartbeat state completely
   */
  private resetHeartbeatState(): void {
    this.stopHeartbeat(false);
    this.heartbeatState = {
      activeStartTime: 0,
      accumulatedActiveMs: 0,
      isActive: false,
      maxDurationReached: false,
      lastPingTime: 0,
      currentTierIndex: 0,
      pageActiveMs: 0,
      pageStartTime: monotonicNow(),
    };
  }

  /**
   * Reset page active time only (for SPA navigation)
   */
  private resetPageActiveTime(): void {
    // Keep session time, reset page time
    this.heartbeatState.pageActiveMs = 0;
    this.heartbeatState.pageStartTime = monotonicNow();
  }

  /**
   * Get current page's accumulated focus time in milliseconds.
   * This only counts time when the tab is visible/focused.
   * Used by SessionState to set accurate page duration.
   */
  private getPageActiveMs(): number {
    let total = this.heartbeatState.pageActiveMs;
    if (this.heartbeatState.isActive) {
      // Add time since last focus start
      total += monotonicNow() - this.heartbeatState.pageStartTime;
    }
    return total;
  }

  /**
   * Validate and normalize heartbeat tiers
   */
  private validateTiers(tiers: HeartbeatTier[]): HeartbeatTier[] {
    if (!tiers || tiers.length === 0) {
      return DEFAULT_HEARTBEAT_TIERS;
    }

    // Sort by 'after' ascending
    const sorted = [...tiers].sort((a, b) => a.after - b.after);

    // Ensure first tier starts at 0
    if (sorted[0].after !== 0) {
      sorted.unshift({
        after: 0,
        desktopInterval: 10000,
        mobileInterval: 7000,
      });
    }

    // Enforce minimum intervals
    return sorted.map((tier) => ({
      ...tier,
      desktopInterval:
        tier.desktopInterval === null
          ? null
          : Math.max(tier.desktopInterval, MIN_HEARTBEAT_INTERVAL),
      mobileInterval:
        tier.mobileInterval === null
          ? null
          : Math.max(tier.mobileInterval, MIN_HEARTBEAT_INTERVAL),
    }));
  }

  /**
   * Schedule a debounced send (for rapid navigations)
   */
  private scheduleDebouncedSend(): void {
    if (this.sendDebounceTimeout) {
      clearTimeout(this.sendDebounceTimeout);
    }

    this.sendDebounceTimeout = setTimeout(async () => {
      this.sendDebounceTimeout = null;
      await this.sendPayload();
    }, SEND_DEBOUNCE_MS);
  }

  /**
   * Send session payload to server
   * V3: Always sends all actions, always includes attributes.
   * Server uses ReplacingMergeTree to deduplicate events.
   */
  private async sendPayload(): Promise<void> {
    if (!this.sessionState || !this.sender) return;

    const attributes = this.buildAttributes();
    const payload = this.sessionState.buildPayload(attributes, {
      identity: this.sessionManager?.getIdentity() ?? null,
      dimensions: this.sessionManager?.getDimensionsPayload() ?? {},
    });

    const result = await this.sender.sendSession(payload);

    if (result.success) {
      // Persist state after successful send
      this.sessionState.persist();
    }
  }

  /**
   * Build session attributes from current state
   */
  private buildAttributes(): SessionAttributes {
    const session = this.sessionManager?.getSession();
    const device = this.deviceInfo;

    return {
      landing_page: session?.landing_page || window.location.href,
      referrer: session?.referrer || undefined,
      utm_source: session?.utm?.source || undefined,
      utm_medium: session?.utm?.medium || undefined,
      utm_campaign: session?.utm?.campaign || undefined,
      utm_term: session?.utm?.term || undefined,
      utm_content: session?.utm?.content || undefined,
      utm_id: session?.utm?.id || undefined,
      utm_id_from: session?.utm?.id_from || undefined,
      screen_width: device?.screen_width,
      screen_height: device?.screen_height,
      viewport_width: device?.viewport_width,
      viewport_height: device?.viewport_height,
      device: device?.device,
      browser: device?.browser,
      browser_type: device?.browser_type || undefined,
      os: device?.os,
      user_agent: device?.user_agent,
      connection_type: device?.connection_type,
      language: device?.language,
      timezone: device?.timezone,
    };
  }

  // Public API

  /**
   * Get session ID
   */
  async getSessionId(): Promise<string> {
    await this.ensureInitialized();
    return this.sessionManager?.getSessionId() || '';
  }

  /**
   * Get focus duration in milliseconds
   * In V3, this is calculated from pageview durations in actions[].
   * Current page uses live focus time from heartbeatState.
   */
  async getFocusDuration(): Promise<number> {
    await this.ensureInitialized();
    if (!this.sessionState) return 0;

    // Sum all pageview durations from actions
    const actions = this.sessionState.getActions();
    let total = 0;
    const currentPage = this.sessionState.getCurrentPage();

    for (let i = 0; i < actions.length; i++) {
      const action = actions[i];
      if (action.type !== 'pageview') continue;

      // Check if this is the current page (by page_number match)
      if (currentPage && action.page_number === currentPage.page_number) {
        // Current page: use live focus time from heartbeatState
        total += this.getPageActiveMs();
      } else {
        // Completed page: use stored duration
        total += action.duration;
      }
    }

    return total;
  }

  /**
   * Get total duration in milliseconds
   */
  async getTotalDuration(): Promise<number> {
    await this.ensureInitialized();
    const session = this.sessionManager?.getSession();
    if (!session) return 0;
    // created_at is a wall-clock stamp, so this subtraction stays on the wall
    // clock — but clamp it, because a backward step would otherwise report a
    // negative session age.
    return Math.max(0, Date.now() - session.created_at);
  }

  /**
   * Track page view
   */
  async trackPageView(url?: string): Promise<void> {
    await this.ensureInitialized();
    if (!this.sessionState) return;

    // Update scroll before navigation
    if (this.scrollTracker) {
      this.sessionState.updateScroll(this.scrollTracker.getMaxScrollPercent());
    }

    // A lapsed window rotates here rather than silently waiting for the next
    // full page load; the rotation opens the pageview on the new session itself.
    if (!(await this.ensureFreshSession(url))) return;

    const path = url || window.location.pathname;
    this.sessionState.addPageview(path);

    // Reset scroll tracking
    this.scrollTracker?.reset();

    // Reset page timer
    this.resetPageActiveTime();

    // Debounced send
    this.scheduleDebouncedSend();

    // Persist state
    this.sessionState.persist();
  }

  /**
   * Track goal (immediate send)
   */
  async trackGoal(data: GoalData): Promise<void> {
    await this.ensureInitialized();
    // Checked here, not before ensureInitialized: a call made before init should
    // still say the SDK is not configured, which is the more useful complaint.
    if (!data || !VALID_GOAL_TYPES.includes(data.type as GoalType)) {
      throw new Error(
        `trackGoal requires a type, one of: ${VALID_GOAL_TYPES.join(', ')}`,
      );
    }
    if (!this.sessionState) return;

    // Rotate first if the window lapsed, so the goal lands on the live session
    // rather than one the server will reject.
    await this.ensureFreshSession();

    // Add goal to SessionState
    this.sessionState.addGoal(data.action, data.type, data.value, data.properties);

    // Cancel any pending debounced send
    if (this.sendDebounceTimeout) {
      clearTimeout(this.sendDebounceTimeout);
      this.sendDebounceTimeout = null;
    }

    // Immediate send for goals (critical for conversion timing)
    await this.sendPayload();

    // Persist state
    this.sessionState.persist();
  }

  /**
   * Set custom dimension
   */
  async setDimension(index: number, value: string): Promise<void> {
    await this.ensureInitialized();
    this.sessionManager?.setDimension(index, value);
  }

  /**
   * Set multiple dimensions
   */
  async setDimensions(dimensions: Record<number, string>): Promise<void> {
    await this.ensureInitialized();
    this.sessionManager?.setDimensions(dimensions);
  }

  /**
   * Get dimension value
   */
  async getDimension(index: number): Promise<string | null> {
    await this.ensureInitialized();
    return this.sessionManager?.getDimension(index) || null;
  }

  /**
   * Clear all dimensions
   */
  async clearDimensions(): Promise<void> {
    await this.ensureInitialized();
    this.sessionManager?.clearDimensions();
  }

  /**
   * Attach a verified contact identity.
   *
   * The hmac must be minted server-side by the customer over the raw address
   * with their workspace secret. /track is public and unauthenticated, so an
   * unsigned address is discarded — passing one would look like identification
   * while silently doing nothing.
   */
  async identify(email: string, hmac: string): Promise<void> {
    await this.ensureInitialized();
    if (typeof email !== 'string' || typeof hmac !== 'string' || !email || !hmac) {
      throw new Error('identify(email, hmac) requires both arguments');
    }
    this.sessionManager?.setIdentity({ email, hmac });
  }

  /**
   * Current identity, or null when anonymous.
   */
  async getIdentity(): Promise<WebIdentity | null> {
    await this.ensureInitialized();
    return this.sessionManager?.getIdentity() ?? null;
  }

  /**
   * Stop future beats carrying the identity.
   *
   * Does NOT anonymize what has already been recorded — see
   * SessionManager.clearIdentity.
   */
  async clearIdentity(): Promise<void> {
    await this.ensureInitialized();
    this.sessionManager?.clearIdentity();
  }

  /**
   * Pause tracking
   */
  async pause(): Promise<void> {
    await this.ensureInitialized();
    this.isPaused = true;
    this.stopHeartbeat(true); // Accumulate time
  }

  /**
   * Resume tracking
   */
  async resume(): Promise<void> {
    await this.ensureInitialized();
    this.isPaused = false;

    // Fold any in-flight time into the page total before restarting.
    this.stopHeartbeat(true);

    // Clearing the cap is what lets resume() restart tracking after
    // heartbeatMaxDuration. Only the cap counter is cleared: accumulatedActiveMs
    // drives when to stop pinging, while pageActiveMs is the page's measured
    // engagement and must survive a pause/resume.
    this.heartbeatState.accumulatedActiveMs = 0;
    this.heartbeatState.currentTierIndex = 0;
    this.heartbeatState.maxDurationReached = false;

    this.sessionState?.reopenCurrentPage();
    this.resumeHeartbeat();
  }

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
  private async rotateSession(path?: string): Promise<void> {
    if (!this.sessionManager || !this.config || !this.sessionState) return;

    this.sessionState.finalizeForUnload();
    await this.sendPayload();

    const session = this.sessionManager.getOrCreateSession();
    this.sessionState = new SessionState({
      workspace_id: this.config.workspace_id,
      session_id: session.id,
      created_at: session.created_at,
      tab_id: this.sessionManager.getTabId(),
    });
    this.sessionState.setFocusTimeGetter(() => this.getPageActiveMs());
    this.sessionState.addPageview(path || window.location.pathname);

    this.scrollTracker?.reset();
    this.resetHeartbeatState();
    this.startHeartbeat();
  }

  /**
   * Record activity, rotating the session if its window has lapsed.
   *
   * Returns false when a rotation happened, so a caller that was about to open a
   * pageview knows the rotation already did it. Calling this on every beat,
   * navigation and goal is what makes the session window measure activity rather
   * than time since page load.
   */
  private async ensureFreshSession(path?: string): Promise<boolean> {
    if (!this.sessionManager) return true;
    if (this.sessionManager.touch()) return true;
    await this.rotateSession(path);
    return false;
  }

  async reset(): Promise<void> {
    await this.ensureInitialized();
    if (!this.sessionManager || !this.config) return;

    // Create new session
    this.sessionManager.reset();
    const session = this.sessionManager.getOrCreateSession();

    // Reinitialize SessionState with new session
    const sessionStateConfig: SessionStateConfig = {
      workspace_id: this.config.workspace_id,
      session_id: session.id,
      created_at: session.created_at,
      tab_id: this.sessionManager.getTabId(),
    };
    this.sessionState = new SessionState(sessionStateConfig);

    // Wire up focus time getter for accurate page duration tracking
    this.sessionState.setFocusTimeGetter(() => this.getPageActiveMs());

    this.sessionState.addPageview(window.location.pathname);

    // Reset scroll and heartbeat
    this.scrollTracker?.reset();
    this.resetHeartbeatState();
    this.startHeartbeat();

    // Send initial payload for new session
    await this.sendPayload();
  }

  /**
   * Get current configuration (defensive copy)
   */
  getConfig(): Readonly<NotifuseAnalyticsConfig> | null {
    if (!this.config) return null;
    return { ...this.config };
  }

  /**
   * Get debug info
   */
  debug(): SessionDebugInfo {
    return {
      session: this.sessionManager?.getSession() || null,
      config: this.config,
      isTracking: this.isTracking,
      actionsCount: this.sessionState?.getActions().length || 0,
      currentPage: this.sessionState?.getCurrentPage()?.path || null,
      pageActiveMs: this.getPageActiveMs(),
    };
  }

  /**
   * Decorate URL with cross-domain session params
   * Use this for programmatic navigation (window.location.href, window.open)
   */
  async decorateUrl(url: string): Promise<string> {
    await this.ensureInitialized();
    if (!this.crossDomainLinker) {
      return url; // Return unchanged if cross-domain not configured
    }
    return this.crossDomainLinker.decorateUrl(url);
  }

  /**
   * Ensure SDK is initialized (awaits init promise if needed)
   */
  private async ensureInitialized(): Promise<void> {
    if (this.isInitialized) return;

    if (!this.initPromise) {
      throw new Error('Notifuse Analytics not configured. Set window.NotifuseAnalyticsConfig before loading the SDK.');
    }

    await this.initPromise;
  }
}
