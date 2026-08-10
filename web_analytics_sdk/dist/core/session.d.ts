/**
 * Session management
 * Handles session creation, persistence, and expiry
 */
import type { Session, CustomDimensions, InternalConfig } from '../types';
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
     * Update session
     */
    updateSession(updates: Partial<Session>): void;
    /**
     * Save session to storage
     */
    private saveSession;
    /**
     * Get tab ID (unique per browser tab)
     */
    getTabId(): string;
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
     * Set user ID for tracking authenticated users
     */
    setUserId(id: string | null): void;
    /**
     * Get current user ID
     */
    getUserId(): string | null;
    /**
     * Load user ID from storage
     */
    private loadUserId;
    /**
     * Save user ID to storage
     */
    private saveUserId;
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
