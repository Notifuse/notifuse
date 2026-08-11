/**
 * Session management
 * Handles session creation, persistence, and expiry
 */
import type { Session, CustomDimensions, InternalConfig, WebIdentity } from '../types';
import { Storage, TabStorage } from '../storage/storage';
/**
 * Cross-domain session input (from URL parameters)
 */
export interface CrossDomainInput {
    sessionId: string;
    timestamp: number;
    expiry: number;
}
export declare class SessionManager {
    private storage;
    private tabStorage;
    private config;
    private session;
    private tabId;
    private debug;
    private crossDomainInput;
    constructor(storage: Storage, tabStorage: TabStorage, config: InternalConfig);
    /**
     * Set cross-domain input (from URL parameters)
     * Must be called before getOrCreateSession()
     */
    setCrossDomainInput(input: CrossDomainInput): void;
    /**
     * Get or create session
     * Priority:
     * 1. Valid cross-domain input (from URL params)
     * 2. Valid existing session in localStorage
     * 3. Create new session
     */
    getOrCreateSession(): Session;
    /**
     * Check if cross-domain input is valid
     */
    private isValidCrossDomain;
    /**
     * Create session from cross-domain input
     */
    private createSessionFromCrossDomain;
    /**
     * Create a new session
     */
    private createSession;
    /**
     * Check if session has expired
     */
    private isSessionExpired;
    /**
     * Check if UTM has any values
     */
    private hasUTMValues;
    /**
     * Get current session
     */
    getSession(): Session | null;
    /**
     * Record activity on the current session, returning false when it has expired
     * and the caller must rotate.
     *
     * Two bugs live here if nothing calls this. last_active_at would only ever be
     * written at page load, so the inactivity window would measure time since load
     * rather than since activity — fragmenting a long read into several sessions.
     * And the expiry check would run once per load and never again, so a tab left
     * open would never rotate, eventually crossing the server's id bound and being
     * rejected permanently.
     */
    touch(): boolean;
    /**
     * Save session to storage
     */
    private saveSession;
    /**
     * Get tab ID (unique per browser tab)
     */
    getTabId(): number;
    /**
     * Get or create tab ID
     */
    private getOrCreateTabId;
    /**
     * Get session ID
     */
    getSessionId(): string;
    /**
     * Set a custom dimension (1-10)
     */
    setDimension(index: number, value: string): void;
    /**
     * Set multiple dimensions
     */
    setDimensions(dimensions: Record<number, string>): void;
    /**
     * Get a dimension value
     */
    getDimension(index: number): string | null;
    /**
     * Clear all dimensions
     */
    clearDimensions(): void;
    /**
     * Get all dimensions as payload fields
     */
    getDimensionsPayload(): Record<string, string>;
    /**
     * Load dimensions from storage
     */
    private loadDimensions;
    /**
     * Save dimensions to storage
     */
    private saveDimensions;
    /**
     * Attach a verified contact identity to this visitor.
     *
     * The address is stored EXACTLY as given: the customer's server signed that
     * raw string, so lowercasing it here would invalidate every HMAC they mint.
     * Normalization happens server-side, after the signature is checked.
     */
    setIdentity(identity: WebIdentity): void;
    getIdentity(): WebIdentity | null;
    /**
     * Stop future beats carrying the identity.
     *
     * This does NOT anonymize the session already recorded: the server keeps a
     * contact_email once set, deliberately, so a beat that simply has not read
     * its stored identity yet cannot un-attribute a visit. Erasure is a
     * contact-deletion operation, not a client-side one.
     */
    clearIdentity(): void;
    /**
     * Load the stored identity, discarding anything an older build left behind.
     */
    private loadIdentity;
    /**
     * Apply dimensions from URL parameters
     * Only sets dimensions that don't already have values (existing wins)
     */
    applyUrlDimensions(urlDimensions: CustomDimensions): void;
    /**
     * Reset session (clear and create new)
     */
    reset(): Session;
}
