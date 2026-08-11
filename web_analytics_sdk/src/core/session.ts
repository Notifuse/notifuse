/**
 * Session management
 * Handles session creation, persistence, and expiry
 */

import type { Session, UTMParams, CustomDimensions, InternalConfig, WebIdentity } from '../types';
import { Storage, TabStorage, STORAGE_KEYS } from '../storage/storage';
import { generateTabId, generateUUIDv7 } from '../utils/uuid';
import { parseUTMParams } from '../utils/utm';

const SDK_VERSION = __SDK_VERSION__;
const CLOCK_SKEW_TOLERANCE = 60; // seconds

// Absolute lifetime of one session id, independent of activity.
//
// The server rejects any session id whose embedded timestamp is older than 48h
// (WebSessionIDMaxAge) and the SDK has no path back from that 400 — it never
// rotates in response — so a pinned tab or a kiosk display goes silent forever.
// Rotating at half the server bound leaves a full day of headroom for a beat
// sitting in the offline queue to still be replayed against the id it was
// minted under. Deliberately not configurable: raising it re-opens the cliff.
const SESSION_MAX_AGE_MS = 24 * 60 * 60 * 1000;

/**
 * Length of a string in UTF-8 bytes.
 *
 * TextEncoder exists in every browser the SDK targets, but the SDK is dropped
 * into pages it does not control and globals do get stripped there, so a
 * missing one degrades to hand-counting rather than throwing a ReferenceError
 * on a call that used to work.
 */
function utf8Length(value: string): number {
  if (typeof TextEncoder !== 'undefined') {
    return new TextEncoder().encode(value).length;
  }
  // for..of walks code points, so a surrogate pair arrives once as its combined
  // code point and is counted as the single 4-byte character it encodes to.
  let bytes = 0;
  for (const char of value) {
    const code = char.codePointAt(0)!;
    if (code <= 0x7f) bytes += 1;
    else if (code <= 0x7ff) bytes += 2;
    else if (code <= 0xffff) bytes += 3;
    else bytes += 4;
  }
  return bytes;
}

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
  private tabId: number;
  private debug: boolean;
  private crossDomainInput: CrossDomainInput | null = null;

  constructor(storage: Storage, tabStorage: TabStorage, config: InternalConfig) {
    this.storage = storage;
    this.tabStorage = tabStorage;
    this.config = config;
    this.debug = config.debug;
    this.tabId = this.getOrCreateTabId();
    // One-time migration, not part of session creation: a resumed session would
    // otherwise leave the dead key sitting in storage indefinitely.
    this.storage.remove(STORAGE_KEYS.LEGACY_USER_ID);
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
      // Identity comes from its own key, not from the session blob. touch()
      // writes the whole in-memory session back on every beat, so a second tab
      // still holding a pre-identification copy would otherwise clobber the
      // blob with identity:null and every later page load would resume
      // anonymous — with the credential still sitting intact in storage.
      stored.identity = this.loadIdentity();
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
      identity: this.loadIdentity(),
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
      identity: this.loadIdentity(),
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
    const now = Date.now();
    if (now - session.last_active_at > this.config.sessionTimeout) return true;
    return now - session.created_at > SESSION_MAX_AGE_MS;
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
  touch(): boolean {
    if (!this.session) return false;
    if (this.isSessionExpired(this.session)) return false;

    const now = Date.now();
    this.session.last_active_at = now;
    this.session.updated_at = now;
    this.saveSession();
    return true;
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
  getTabId(): number {
    return this.tabId;
  }

  /**
   * Get or create tab ID
   */
  private getOrCreateTabId(): number {
    const stored = this.tabStorage.get<unknown>(STORAGE_KEYS.TAB_ID);
    // Anything that is not a safe integer is replaced, which also migrates the
    // UUID string an earlier build wrote here — sending that as a BIGINT would
    // be rejected outright.
    if (typeof stored === 'number' && Number.isSafeInteger(stored) && stored > 0) {
      return stored;
    }
    const tabId = generateTabId();
    this.tabStorage.set(STORAGE_KEYS.TAB_ID, tabId);
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
   * Attach a verified contact identity to this visitor.
   *
   * The address is stored EXACTLY as given: the customer's server signed that
   * raw string, so lowercasing it here would invalidate every HMAC they mint.
   * Normalization happens server-side, after the signature is checked.
   */
  setIdentity(identity: WebIdentity): void {
    if (!identity || (!identity.token && !(identity.email && identity.hmac))) {
      throw new Error('Identity requires either a token or an email with its hmac');
    }
    // Bytes, not characters: the server's gate is Go's len() over the RAW
    // address, so a multibyte SMTPUTF8/IDN address that clears a code-unit
    // check here is dropped server-side in silence — the beat succeeds, the
    // visit is recorded, the contact is simply never attached — which is
    // exactly the outcome this throw exists to prevent. It mirrors that raw
    // gate closely rather than agreeing with it exactly: the server applies
    // the same bound a second time to the lowercased address, and a couple of
    // Latin code points (U+023A, U+023E) grow from 2 bytes to 3 when
    // lowercased, so an address accepted here at 255 bytes can still be
    // dropped at 256.
    if (identity.email && utf8Length(identity.email) > 255) {
      throw new Error('Identity email must be 255 bytes or less');
    }
    if (!this.session) return;

    this.session.identity = identity;
    this.storage.set(STORAGE_KEYS.IDENTITY, identity);
    this.saveSession();

    if (this.debug) {
      console.log('[NotifuseAnalytics] Identity set');
    }
  }

  getIdentity(): WebIdentity | null {
    if (!this.session) return null;
    return this.session.identity;
  }

  /**
   * Stop future beats carrying the identity.
   *
   * This does NOT anonymize the session already recorded: the server keeps a
   * contact_email once set, deliberately, so a beat that simply has not read
   * its stored identity yet cannot un-attribute a visit. Erasure is a
   * contact-deletion operation, not a client-side one.
   */
  clearIdentity(): void {
    this.storage.remove(STORAGE_KEYS.IDENTITY);
    if (!this.session) return;
    this.session.identity = null;
    this.saveSession();
  }

  /**
   * Load the stored identity, discarding anything an older build left behind.
   */
  private loadIdentity(): WebIdentity | null {
    const stored = this.storage.get<WebIdentity>(STORAGE_KEYS.IDENTITY);
    if (!stored) return null;
    if (stored.token || (stored.email && stored.hmac)) return stored;
    return null;
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
    this.storage.remove(STORAGE_KEYS.IDENTITY);
    this.session = null;
    return this.createSession();
  }
}
