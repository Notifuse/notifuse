/**
 * Session Payload Types
 *
 * These types define the session payload format for the SDK.
 * The SDK builds a cumulative actions[] array that gets sent to /api/track.
 */
import type { GoalType } from '../types';
export type ActionType = 'pageview' | 'goal';
/**
 * Completed pageview action (user has left the page)
 */
export interface PageviewAction {
    type: 'pageview';
    path: string;
    page_number: number;
    duration: number;
    scroll: number;
    entered_at: number;
    exited_at: number;
}
/**
 * Goal action (conversion event)
 */
export interface GoalAction {
    /** The action discriminator — NOT the goal's own type, which is goal_type. */
    type: 'goal';
    name: string;
    goal_type: GoalType;
    path: string;
    page_number: number;
    timestamp: number;
    value?: number;
    properties?: Record<string, string>;
}
export type Action = PageviewAction | GoalAction;
/**
 * Page currently being viewed (not yet finalized)
 */
export interface CurrentPage {
    path: string;
    page_number: number;
    entered_at: number;
    scroll: number;
}
/**
 * Session attributes (traffic source, device info, etc.)
 * Sent only on first payload of session.
 */
export interface SessionAttributes {
    referrer?: string;
    landing_page: string;
    utm_source?: string;
    utm_medium?: string;
    utm_campaign?: string;
    utm_term?: string;
    utm_content?: string;
    utm_id?: string;
    utm_id_from?: string;
    screen_width?: number;
    screen_height?: number;
    viewport_width?: number;
    viewport_height?: number;
    device?: string;
    browser?: string;
    browser_type?: string;
    os?: string;
    user_agent?: string;
    connection_type?: string;
    language?: string;
    timezone?: string;
}
/**
 * Full session payload sent to /api/track
 *
 * V3 format: No current_page or checkpoint fields.
 * Current page is included in actions[] with duration updated on each send.
 * Server uses ReplacingMergeTree to deduplicate events.
 */
export interface SessionPayload {
    workspace_id: string;
    session_id: string;
    /** The writing tab; 0 when unknown. Part of the child tables' primary keys. */
    tab_id: number;
    /** Verified-identity credentials; the server discards any it cannot check. */
    contact_email?: string;
    contact_email_hmac?: string;
    identify_token?: string;
    actions: Action[];
    attributes?: SessionAttributes;
    created_at: number;
    updated_at: number;
    sdk_version: string;
    /**
     * Monotonic per-session beat counter. The server upserts a session only when
     * the incoming beat_seq is strictly greater than the stored one, so this is
     * what makes retries idempotent and stops a replayed offline beat from
     * overwriting fresher data. Without it every session would freeze on its
     * first beat.
     */
    seq: number;
    user_id?: string | null;
    dimensions?: Record<string, string>;
}
/**
 * Snapshot of SessionState for persistence
 */
export interface SessionStateSnapshot {
    actions: Action[];
    currentPageIndex: number | null;
    seq: number;
}
/**
 * Result from sending a session payload
 */
export interface SendResult {
    success: boolean;
    error?: string;
    queued?: boolean;
}
