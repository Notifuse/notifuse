/**
 * SessionState - Manages cumulative actions[] array for V3 session payload
 *
 * Key concepts:
 * - actions[]: Array of pageviews and goals (pages added immediately with duration=0)
 * - currentPageIndex: Index into actions[] for the page user is currently viewing
 * - Focus time: Duration is tracked via callback to SDK's heartbeatState.pageActiveMs
 *
 * Changes in V3:
 * - No separate currentPage field - page is in actions[] from the start
 * - No checkpoint - always send all actions, server uses ReplacingMergeTree
 * - No attributesSent optimization - always include attributes
 */

import type { GoalType, WebIdentity } from '../types';
import type {
  Action,
  PageviewAction,
  GoalAction,
  CurrentPage,
  SessionPayload,
  SessionAttributes,
  SessionStateSnapshot,
} from '../types/session-state';

const STORAGE_KEY = 'nf_session_state';
const SDK_VERSION = __SDK_VERSION__;
const MAX_ACTIONS = 1000; // Match server limit from Phase 2

export interface SessionStateConfig {
  workspace_id: string;
  session_id: string;
  created_at: number;
  /** Identifies this tab as a distinct writer under the shared session id. */
  tab_id?: number;
}

export class SessionState {
  private actions: Action[] = [];
  private currentPageIndex: number | null = null;
  private seq = 0;
  private readonly tabId: number;
  private getPageFocusMs: (() => number) | null = null;

  private readonly workspaceId: string;
  private readonly sessionId: string;
  private readonly createdAt: number;

  constructor(config: SessionStateConfig) {
    this.workspaceId = config.workspace_id;
    this.sessionId = config.session_id;
    this.createdAt = config.created_at;
    this.tabId = config.tab_id ?? 0;
  }

  // === Focus Time Callback ===

  /**
   * Set the callback to get current page focus time from SDK's heartbeatState.
   * This is used to track accurate page duration (visible time only).
   */
  setFocusTimeGetter(getter: () => number): void {
    this.getPageFocusMs = getter;
  }

  // === Getters ===

  getActions(): Action[] {
    return [...this.actions];
  }

  /**
   * Get current page info derived from actions[currentPageIndex].
   * Returns null if no current page.
   */
  getCurrentPage(): CurrentPage | null {
    if (this.currentPageIndex === null) return null;
    const action = this.actions[this.currentPageIndex];
    if (!action || action.type !== 'pageview') return null;

    return {
      path: action.path,
      page_number: action.page_number,
      entered_at: action.entered_at,
      scroll: action.scroll,
    };
  }

  // === Page Tracking ===

  addPageview(path: string): void {
    // Check MAX_ACTIONS limit (pageviews now consume action slots)
    if (this.actions.length >= MAX_ACTIONS) {
      console.warn(
        `[SessionState] MAX_ACTIONS (${MAX_ACTIONS}) reached, pageview not added`,
      );
      return;
    }

    const now = Date.now();

    // Finalize previous page if exists (update its duration)
    if (this.currentPageIndex !== null) {
      this.finalizeCurrentPageDuration(now);
    }

    // Create new page action with duration=0
    const pageNumber = this.getNextPageNumber();
    const pageview: PageviewAction = {
      type: 'pageview',
      path,
      page_number: pageNumber,
      duration: 0, // Initial duration, updated on each send
      scroll: 0,
      entered_at: now,
      exited_at: now, // Will be updated on each send
    };

    // Add to actions and set as current
    this.actions.push(pageview);
    this.currentPageIndex = this.actions.length - 1;

    // Warn if approaching limit
    if (this.actions.length >= MAX_ACTIONS * 0.9) {
      console.warn(
        `[SessionState] Approaching MAX_ACTIONS limit (${this.actions.length}/${MAX_ACTIONS})`,
      );
    }
  }

  updateScroll(scrollPercent: number): void {
    if (this.currentPageIndex === null) return;

    const action = this.actions[this.currentPageIndex];
    if (!action || action.type !== 'pageview') return;

    // Clamp to 0-100
    const clamped = Math.max(0, Math.min(100, scrollPercent));

    // Only update if higher (track max)
    if (clamped > action.scroll) {
      action.scroll = clamped;
    }
  }

  // === Goal Tracking ===

  addGoal(
    name: string,
    goalType: GoalType,
    value?: number,
    properties?: Record<string, string>,
  ): boolean {
    // Check MAX_ACTIONS limit
    if (this.actions.length >= MAX_ACTIONS) {
      console.warn(
        `[SessionState] MAX_ACTIONS (${MAX_ACTIONS}) reached, goal not added`,
      );
      return false;
    }

    // Get current page info from actions
    const currentPage =
      this.currentPageIndex !== null
        ? (this.actions[this.currentPageIndex] as PageviewAction)
        : null;

    const goal: GoalAction = {
      type: 'goal',
      name,
      goal_type: goalType,
      path: currentPage?.path || '/',
      page_number: currentPage?.page_number || 1,
      timestamp: Date.now(),
    };

    if (value !== undefined) {
      goal.value = value;
    }

    if (properties) {
      goal.properties = properties;
    }

    this.actions.push(goal);

    // Warn if approaching limit
    if (this.actions.length >= MAX_ACTIONS * 0.9) {
      console.warn(
        `[SessionState] Approaching MAX_ACTIONS limit (${this.actions.length}/${MAX_ACTIONS})`,
      );
    }

    return true;
  }

  // === Payload Building ===

  buildPayload(
    attributes: SessionAttributes,
    options?: { identity?: WebIdentity | null; dimensions?: Record<string, string> }
  ): SessionPayload {
    // Update current page's duration and exited_at before building payload
    if (this.currentPageIndex !== null) {
      const action = this.actions[this.currentPageIndex];
      if (action && action.type === 'pageview') {
        // Get focus time from SDK (or 0 if no getter set)
        this.closePage(action, Date.now());
      }
    }

    const payload: SessionPayload = {
      workspace_id: this.workspaceId,
      session_id: this.sessionId,
      tab_id: this.tabId,
      actions: [...this.actions],
      // Always include attributes (no optimization)
      attributes,
      created_at: this.createdAt,
      updated_at: Date.now(),
      sdk_version: SDK_VERSION,
      seq: ++this.seq,
    };

    // Add user_id if provided in options
    // Spread the credential the visitor actually holds. An unsigned address is
    // never stored client-side, so there is no shape here that the server would
    // silently discard.
    if (options && 'identity' in options) {
      const identity = options.identity;
      if (identity?.token) {
        payload.identify_token = identity.token;
      } else if (identity?.email && identity.hmac) {
        payload.contact_email = identity.email;
        payload.contact_email_hmac = identity.hmac;
      }
    }

    // Add dimensions if provided in options
    if (options && 'dimensions' in options) {
      payload.dimensions = options.dimensions;
    }

    return payload;
  }

  // === Unload Handling ===

  finalizeForUnload(): void {
    if (this.currentPageIndex === null) return;

    const action = this.actions[this.currentPageIndex];
    if (action && action.type === 'pageview') {
      // Update final duration and exit time
      this.closePage(action, Date.now());
    }

    // Clear current page index
    this.currentPageIndex = null;
  }

  /**
   * Re-open the page that finalizeForUnload() closed.
   *
   * Hiding or blurring the tab finalizes the current page so the beacon can
   * carry a complete payload — but the visitor almost always comes back to
   * that same page. Without this, its duration and scroll stay frozen at the
   * moment they left and every later measurement is dropped on the floor.
   */
  reopenCurrentPage(): void {
    if (this.currentPageIndex !== null) return;

    for (let i = this.actions.length - 1; i >= 0; i--) {
      if (this.actions[i].type === 'pageview') {
        this.currentPageIndex = i;
        return;
      }
    }
  }

  // === Persistence ===

  persist(): void {
    try {
      const snapshot: SessionStateSnapshot = {
        actions: this.actions,
        currentPageIndex: this.currentPageIndex,
        seq: this.seq,
      };

      // Include session ID for validation on restore
      const data = {
        session_id: this.sessionId,
        ...snapshot,
      };

      sessionStorage.setItem(STORAGE_KEY, JSON.stringify(data));
    } catch (e) {
      // sessionStorage may be unavailable (private mode, quota exceeded)
      console.warn('[SessionState] Failed to persist:', e);
    }
  }

  restore(): void {
    try {
      const stored = sessionStorage.getItem(STORAGE_KEY);
      if (!stored) return;

      const data = JSON.parse(stored);

      // Validate session ID matches
      if (data.session_id !== this.sessionId) {
        // Different session, clear old data
        sessionStorage.removeItem(STORAGE_KEY);
        return;
      }

      // Restore state
      this.actions = data.actions || [];
      this.currentPageIndex = data.currentPageIndex ?? null;
      // Never restart the counter on reload: a lower seq would be ignored by
      // the server and the rest of the session silently lost.
      this.seq = typeof data.seq === 'number' ? data.seq : 0;
    } catch (e) {
      // Corrupted data, ignore
      console.warn('[SessionState] Failed to restore:', e);
      sessionStorage.removeItem(STORAGE_KEY);
    }
  }

  // === Private Helpers ===

  /**
   * Update the current page's duration when navigating away.
   * Uses focus time from SDK's heartbeatState via callback.
   */
  /**
   * Write a page's final duration and exit stamp, both clamped.
   *
   * The two clamps guard one failure: a backward wall-clock step. A negative
   * duration or an exited_at before entered_at is rejected by the server, and
   * since completed pages are never recomputed the bad value rides along in
   * every later beat of the session — so one clock glitch becomes permanent,
   * total loss for that visitor unless it is caught here.
   */
  private closePage(action: PageviewAction, exitTime: number): void {
    const focus = this.getPageFocusMs ? this.getPageFocusMs() : 0;
    action.duration = focus > 0 ? focus : 0;
    action.exited_at = exitTime > action.entered_at ? exitTime : action.entered_at;
  }

  private finalizeCurrentPageDuration(exitTime: number): void {
    if (this.currentPageIndex === null) return;

    const action = this.actions[this.currentPageIndex];
    if (!action || action.type !== 'pageview') return;

    // Get focus time from SDK (or 0 if no getter set)
    this.closePage(action, exitTime);
  }

  private getNextPageNumber(): number {
    // Find highest page_number in actions
    let maxPageNumber = 0;

    for (const action of this.actions) {
      if (action.page_number > maxPageNumber) {
        maxPageNumber = action.page_number;
      }
    }

    return maxPageNumber + 1;
  }
}
