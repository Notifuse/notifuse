/**
 * Session management
 * Handles session creation, persistence, and expiry
 */

import type { Session, UTMParams, CustomDimensions, InternalConfig } from '../types';
import { Storage, TabStorage, STORAGE_KEYS } from '../storage/storage';
import { generateUUIDv4, generateUUIDv7 } from '../utils/uuid';
import { parseUTMParams } from '../utils/utm';

const SDK_VERSION = __SDK_VERSION__;
const CLOCK_SKEW_TOLERANCE = 60; // seconds

/**
 * Cross-domain session input (from URL parameters)
 */
export interface CrossDomainInput {
  sessionId: string;
  timestamp: number; // Unix epoch seconds
  expiry: number; // seconds
}

export class SessionManager {
  private storage: Storage;
  private tabStorage: TabStorage;
  private config: InternalConfig;
  private session: Session | null = null;
  private tabId: string;
  private debug: boolean;
  private crossDomainInput: CrossDomainInput | null = null;

  constructor(storage: Storage, tabStorage: TabStorage, config: InternalConfig) {
    this.storage = storage;
    this.tabStorage = tabStorage;
    this.config = config;
    this.debug = config.debug;
    this.tabId = this.getOrCreateTabId();
  }

  /**
   * Set cross-domain input (from URL parameters)
   * Must be called before getOrCreateSession()
   */
  setCrossDomainInput(input: CrossDomainInput): void {
    this.crossDomainInput = input;
  }

  /**
   * Get or create session
   * Priority:
   * 1. Valid cross-domain input (from URL params)
   * 2. Valid existing session in localStorage
   * 3. Create new session
   */
  getOrCreateSession(): Session {
    // Check cross-domain input first (highest priority)
    if (this.crossDomainInput && this.isValidCrossDomain()) {
      const session = this.createSessionFromCrossDomain();
      if (session) {
        return session;
      }
    }

    const stored = this.storage.get<Session>(STORAGE_KEYS.SESSION);

    // Resume existing session if valid
    if (stored && !this.isSessionExpired(stored)) {
      stored.last_active_at = Date.now();
      stored.updated_at = Date.now();
      stored.sequence++;
      this.session = stored;
      this.saveSession();

      if (this.debug) {
        console.log('[NotifuseAnalytics] Resumed session:', stored.id);
      }

      return stored;
    }

    // Create new session
    return this.createSession();
  }

  /**
   * Check if cross-domain input is valid
   */
  private isValidCrossDomain(): boolean {
    if (!this.crossDomainInput) return false;

    const now = Math.floor(Date.now() / 1000);
    const { timestamp, expiry } = this.crossDomainInput;

    // Check if expired
    const age = now - timestamp;
    if (age > expiry) {
      if (this.debug) {
        console.log('[NotifuseAnalytics] Cross-domain input expired:', age, 'seconds old');
      }
      return false;
    }

    // Check if too far in future (clock skew)
    if (timestamp > now + CLOCK_SKEW_TOLERANCE) {
      if (this.debug) {
        console.log('[NotifuseAnalytics] Cross-domain timestamp too far in future');
      }
      return false;
    }

    return true;
  }

  /**
   * Create session from cross-domain input
   */
  private createSessionFromCrossDomain(): Session | null {
    if (!this.crossDomainInput) return null;

    const { sessionId } = this.crossDomainInput;
    const now = Date.now();
    const utm = parseUTMParams(window.location.href, this.config.adClickIds);

    const session: Session = {
      id: sessionId,
      workspace_id: this.config.workspace_id,
      created_at: now,
      updated_at: now,
      last_active_at: now,
      focus_duration_ms: 0,
      total_duration_ms: 0,
      referrer: document.referrer || null,
      landing_page: window.location.href,
      utm: this.hasUTMValues(utm) ? utm : null,
      max_scroll_percent: 0,
      interaction_count: 0,
      sdk_version: SDK_VERSION,
      sequence: 0,
      dimensions: this.loadDimensions(),
      userId: this.loadUserId(),
    };

    this.session = session;
    this.saveSession();

    if (this.debug) {
      console.log('[NotifuseAnalytics] Created session from cross-domain:', session.id);
    }

    return session;
  }

  /**
   * Create a new session
   */
  private createSession(): Session {
    const now = Date.now();
    const utm = parseUTMParams(window.location.href, this.config.adClickIds);

    const session: Session = {
      id: generateUUIDv7(),
      workspace_id: this.config.workspace_id,
      created_at: now,
      updated_at: now,
      last_active_at: now,
      focus_duration_ms: 0,
      total_duration_ms: 0,
      referrer: document.referrer || null,
      landing_page: window.location.href,
      utm: this.hasUTMValues(utm) ? utm : null,
      max_scroll_percent: 0,
      interaction_count: 0,
      sdk_version: SDK_VERSION,
      sequence: 0,
      dimensions: this.loadDimensions(),
      userId: this.loadUserId(),
    };

    this.session = session;
    this.saveSession();

    if (this.debug) {
      console.log('[NotifuseAnalytics] Created session:', session.id);
    }

    return session;
  }

  /**
   * Check if session has expired
   */
  private isSessionExpired(session: Session): boolean {
    return Date.now() - session.last_active_at > this.config.sessionTimeout;
  }

  /**
   * Check if UTM has any values
   */
  private hasUTMValues(utm: UTMParams): boolean {
    return Boolean(
      utm.source || utm.medium || utm.campaign || utm.term || utm.content || utm.id
    );
  }

  /**
   * Get current session
   */
  getSession(): Session | null {
    return this.session;
  }

  /**
   * Update session
   */
  updateSession(updates: Partial<Session>): void {
    if (!this.session) return;

    Object.assign(this.session, updates, {
      updated_at: Date.now(),
      last_active_at: Date.now(),
    });

    this.saveSession();
  }

  /**
   * Save session to storage
   */
  private saveSession(): void {
    if (!this.session) return;
    this.storage.set(STORAGE_KEYS.SESSION, this.session);
  }

  /**
   * Get tab ID (unique per browser tab)
   */
  getTabId(): string {
    return this.tabId;
  }

  /**
   * Get or create tab ID
   */
  private getOrCreateTabId(): string {
    let tabId = this.tabStorage.get<string>(STORAGE_KEYS.TAB_ID);
    if (!tabId) {
      tabId = generateUUIDv4();
      this.tabStorage.set(STORAGE_KEYS.TAB_ID, tabId);
    }
    return tabId;
  }

  /**
   * Get session ID
   */
  getSessionId(): string {
    return this.session?.id || '';
  }

  // Custom Dimensions

  /**
   * Set a custom dimension (1-10)
   */
  setDimension(index: number, value: string): void {
    if (index < 1 || index > 10) {
      throw new Error('Dimension index must be between 1 and 10');
    }

    if (typeof value !== 'string') {
      throw new Error('Dimension value must be a string');
    }

    if (value.length > 256) {
      throw new Error('Dimension value must be 256 characters or less');
    }

    if (!this.session) return;

    this.session.dimensions[index] = value;
    this.saveDimensions();
    this.saveSession();

    if (this.debug) {
      console.log(`[NotifuseAnalytics] Set dimension custom_${index}:`, value);
    }
  }

  /**
   * Set multiple dimensions
   */
  setDimensions(dimensions: Record<number, string>): void {
    for (const [index, value] of Object.entries(dimensions)) {
      this.setDimension(Number(index), value);
    }
  }

  /**
   * Get a dimension value
   */
  getDimension(index: number): string | null {
    if (!this.session) return null;
    return this.session.dimensions[index] || null;
  }

  /**
   * Clear all dimensions
   */
  clearDimensions(): void {
    if (!this.session) return;
    this.session.dimensions = {};
    this.saveDimensions();
    this.saveSession();
  }

  /**
   * Get all dimensions as payload fields
   */
  getDimensionsPayload(): Record<string, string> {
    if (!this.session) return {};

    const payload: Record<string, string> = {};
    for (const [index, value] of Object.entries(this.session.dimensions)) {
      payload[`custom_${index}`] = value;
    }
    return payload;
  }

  /**
   * Load dimensions from storage
   */
  private loadDimensions(): CustomDimensions {
    return this.storage.get<CustomDimensions>(STORAGE_KEYS.DIMENSIONS) || {};
  }

  /**
   * Save dimensions to storage
   */
  private saveDimensions(): void {
    if (!this.session) return;
    this.storage.set(STORAGE_KEYS.DIMENSIONS, this.session.dimensions);
  }

  // User ID

  /**
   * Set user ID for tracking authenticated users
   */
  setUserId(id: string | null): void {
    if (id !== null && typeof id !== 'string') {
      throw new Error('User ID must be a string or null');
    }

    if (id !== null && id.length > 256) {
      throw new Error('User ID must be 256 characters or less');
    }

    if (!this.session) return;

    this.session.userId = id;
    this.saveUserId();
    this.saveSession();

    if (this.debug) {
      console.log('[NotifuseAnalytics] Set user ID:', id);
    }
  }

  /**
   * Get current user ID
   */
  getUserId(): string | null {
    if (!this.session) return null;
    return this.session.userId;
  }

  /**
   * Load user ID from storage
   */
  private loadUserId(): string | null {
    return this.storage.get<string>(STORAGE_KEYS.USER_ID) || null;
  }

  /**
   * Save user ID to storage
   */
  private saveUserId(): void {
    if (!this.session) return;
    if (this.session.userId === null) {
      this.storage.remove(STORAGE_KEYS.USER_ID);
    } else {
      this.storage.set(STORAGE_KEYS.USER_ID, this.session.userId);
    }
  }

  /**
   * Apply dimensions from URL parameters
   * Only sets dimensions that don't already have values (existing wins)
   */
  applyUrlDimensions(urlDimensions: CustomDimensions): void {
    if (!this.session) return;

    let changed = false;
    for (const [index, value] of Object.entries(urlDimensions)) {
      const numIndex = Number(index);
      if (!this.session.dimensions[numIndex]) {
        this.session.dimensions[numIndex] = value;
        changed = true;

        if (this.debug) {
          console.log(`[NotifuseAnalytics] Set dimension custom_${numIndex} from URL:`, value);
        }
      }
    }

    if (changed) {
      this.saveDimensions();
      this.saveSession();
    }
  }

  /**
   * Reset session (clear and create new)
   */
  reset(): Session {
    this.storage.remove(STORAGE_KEYS.SESSION);
    this.storage.remove(STORAGE_KEYS.DIMENSIONS);
    this.storage.remove(STORAGE_KEYS.USER_ID);
    this.session = null;
    return this.createSession();
  }
}
