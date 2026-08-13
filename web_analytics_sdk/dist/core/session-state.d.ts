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
import type { Action, CurrentPage, SessionPayload, SessionAttributes } from '../types/session-state';
export interface SessionStateConfig {
    workspace_id: string;
    session_id: string;
    created_at: number;
    /** Identifies this tab as a distinct writer under the shared session id. */
    tab_id?: number;
}
export declare class SessionState {
    private actions;
    private currentPageIndex;
    private seq;
    private readonly tabId;
    private getPageFocusMs;
    private readonly workspaceId;
    private readonly sessionId;
    private readonly createdAt;
    constructor(config: SessionStateConfig);
    /**
     * Set the callback to get current page focus time from SDK's heartbeatState.
     * This is used to track accurate page duration (visible time only).
     */
    setFocusTimeGetter(getter: () => number): void;
    getActions(): Action[];
    /**
     * Get current page info derived from actions[currentPageIndex].
     * Returns null if no current page.
     */
    getCurrentPage(): CurrentPage | null;
    addPageview(path: string): void;
    updateScroll(scrollPercent: number): void;
    addGoal(name: string, goalType: GoalType, value?: number, properties?: Record<string, string>): boolean;
    buildPayload(attributes: SessionAttributes, options?: {
        identity?: WebIdentity | null;
        dimensions?: Record<string, string>;
    }): SessionPayload;
    finalizeForUnload(): void;
    /**
     * Re-open the page that finalizeForUnload() closed.
     *
     * Hiding or blurring the tab finalizes the current page so the beacon can
     * carry a complete payload — but the visitor almost always comes back to
     * that same page. Without this, its duration and scroll stay frozen at the
     * moment they left and every later measurement is dropped on the floor.
     */
    reopenCurrentPage(): void;
    persist(): void;
    restore(): void;
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
    private closePage;
    private finalizeCurrentPageDuration;
    private getNextPageNumber;
}
