import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { SessionManager } from './session';
import { Storage, TabStorage } from '../storage/storage';
import type { InternalConfig, Session } from '../types';

// Mock UUID generation
vi.mock('../utils/uuid', () => ({
  generateUUIDv4: vi.fn(() => 'mock-uuid-v4-' + Math.random().toString(36).slice(2, 10)),
  generateUUIDv7: vi.fn(() => 'mock-uuid-v7-' + Math.random().toString(36).slice(2, 10)),
  // Distinct safe integers, so tests can assert two tabs are different writers.
  generateTabId: vi.fn(() => Math.floor(Math.random() * 900000) + 100000),
}));

// Mock UTM parsing
vi.mock('../utils/utm', () => ({
  parseUTMParams: vi.fn(() => ({
    source: null,
    medium: null,
    campaign: null,
    term: null,
    content: null,
    id: null,
    id_from: null,
  })),
  DEFAULT_AD_CLICK_IDS: ['gclid', 'fbclid'],
}));

describe('SessionManager', () => {
  let storage: Storage;
  let tabStorage: TabStorage;
  let config: InternalConfig;
  let sessionManager: SessionManager;

  // Mock storage
  const createMockStorage = () => {
    const store: Record<string, string> = {};
    return {
      getItem: vi.fn((key: string) => store[key] ?? null),
      setItem: vi.fn((key: string, value: string) => {
        store[key] = value;
      }),
      removeItem: vi.fn((key: string) => {
        delete store[key];
      }),
      clear: vi.fn(),
      key: vi.fn((index: number) => Object.keys(store)[index] ?? null),
      get length() {
        return Object.keys(store).length;
      },
      _store: store,
    };
  };

  let mockLocalStorage: ReturnType<typeof createMockStorage>;
  let mockSessionStorage: ReturnType<typeof createMockStorage>;

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-15T12:00:00.000Z'));

    mockLocalStorage = createMockStorage();
    mockSessionStorage = createMockStorage();
    vi.stubGlobal('localStorage', mockLocalStorage);
    vi.stubGlobal('sessionStorage', mockSessionStorage);

    // Mock window.location
    vi.stubGlobal('location', {
      href: 'https://example.com/page?utm_source=google',
      pathname: '/page',
      hostname: 'example.com',
    });

    // Mock document.referrer
    Object.defineProperty(document, 'referrer', {
      value: 'https://google.com/search',
      writable: true,
      configurable: true,
    });

    storage = new Storage();
    tabStorage = new TabStorage();
    config = {
      workspace_id: 'ws_123',
      endpoint: 'https://api.example.com',
      debug: false,
      sessionTimeout: 30 * 60 * 1000, // 30 minutes
      heartbeatInterval: 10000,
      adClickIds: ['gclid', 'fbclid'],
      trackSPA: true,
      trackScroll: true,
      trackClicks: false,
      heartbeatTiers: [
        { after: 0, desktopInterval: 10000, mobileInterval: 7000 },
      ],
      heartbeatMaxDuration: 10 * 60 * 1000,
      resetHeartbeatOnNavigation: false,
      crossDomains: [],
      crossDomainExpiry: 120,
      crossDomainStripParams: true,
      crossDomainParam: '_nf',
    };

    sessionManager = new SessionManager(storage, tabStorage, config);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  describe('session creation', () => {
    it('generates UUIDv7 for session.id', async () => {
      const { generateUUIDv7 } = await import('../utils/uuid');
      const session = sessionManager.getOrCreateSession();
      expect(generateUUIDv7).toHaveBeenCalled();
      expect(session.id).toMatch(/^mock-uuid-v7-/);
    });

    it('sets workspace_id from config', () => {
      const session = sessionManager.getOrCreateSession();
      expect(session.workspace_id).toBe('ws_123');
    });

    it('captures timestamps (created_at, updated_at, last_active_at)', () => {
      const now = Date.now();
      const session = sessionManager.getOrCreateSession();

      expect(session.created_at).toBe(now);
      expect(session.updated_at).toBe(now);
      expect(session.last_active_at).toBe(now);
    });

    it('captures document.referrer', () => {
      const session = sessionManager.getOrCreateSession();
      expect(session.referrer).toBe('https://google.com/search');
    });

    // A session is minted whenever the inactivity window has lapsed, including
    // in place on a tab the visitor left open — where document.referrer is
    // whichever of the site's own pages linked them here. Recording it would
    // credit the visit to the site itself.
    it('drops a referrer from the page the visitor is already on', () => {
      Object.defineProperty(document, 'referrer', {
        value: 'https://example.com/compare/',
        writable: true,
        configurable: true,
      });

      const session = sessionManager.getOrCreateSession();
      expect(session.referrer).toBeNull();
    });

    it('keeps a referrer from another host of the same site', () => {
      Object.defineProperty(document, 'referrer', {
        value: 'https://docs.example.com/guide',
        writable: true,
        configurable: true,
      });

      const session = sessionManager.getOrCreateSession();
      expect(session.referrer).toBe('https://docs.example.com/guide');
    });

    it('drops a self-referral whatever the case of its host', () => {
      Object.defineProperty(document, 'referrer', {
        value: 'https://EXAMPLE.com/compare/',
        writable: true,
        configurable: true,
      });

      const session = sessionManager.getOrCreateSession();
      expect(session.referrer).toBeNull();
    });

    // The Android Google Search app: a non-http referrer that the default
    // channel rules match on, so the scheme must not cost it its place.
    it('keeps a non-http referrer', () => {
      Object.defineProperty(document, 'referrer', {
        value: 'android-app://com.google.android.googlequicksearchbox/',
        writable: true,
        configurable: true,
      });

      const session = sessionManager.getOrCreateSession();
      expect(session.referrer).toBe('android-app://com.google.android.googlequicksearchbox/');
    });

    it('keeps a referrer it cannot parse', () => {
      Object.defineProperty(document, 'referrer', {
        value: 'not a url',
        writable: true,
        configurable: true,
      });

      const session = sessionManager.getOrCreateSession();
      expect(session.referrer).toBe('not a url');
    });

    it('captures window.location.href as landing_page', () => {
      const session = sessionManager.getOrCreateSession();
      expect(session.landing_page).toBe('https://example.com/page?utm_source=google');
    });

    it('initializes numeric fields to 0', () => {
      const session = sessionManager.getOrCreateSession();

      expect(session.focus_duration_ms).toBe(0);
      expect(session.max_scroll_percent).toBe(0);
      expect(session.interaction_count).toBe(0);
    });

    it('sets sdk_version to the build version', () => {
      const session = sessionManager.getOrCreateSession();
      expect(session.sdk_version).toBe('39.0');
    });

    it('sets sequence to 0', () => {
      const session = sessionManager.getOrCreateSession();
      expect(session.sequence).toBe(0);
    });

    it('loads dimensions from storage', () => {
      mockLocalStorage._store['nf_dimensions'] = JSON.stringify({ 1: 'value1', 2: 'value2' });
      storage = new Storage();
      sessionManager = new SessionManager(storage, tabStorage, config);

      const session = sessionManager.getOrCreateSession();
      expect(session.dimensions).toEqual({ 1: 'value1', 2: 'value2' });
    });
  });

  describe('session resume', () => {
    it('resumes session when not expired', () => {
      const existingSession: Session = {
        id: 'existing-session-id',
        workspace_id: 'ws_123',
        created_at: Date.now() - 5 * 60 * 1000, // 5 minutes ago
        updated_at: Date.now() - 1 * 60 * 1000,
        last_active_at: Date.now() - 1 * 60 * 1000, // 1 minute ago
        focus_duration_ms: 5000,
        total_duration_ms: 10000,
        referrer: 'https://google.com',
        landing_page: 'https://example.com',
        utm: null,
        max_scroll_percent: 50,
        interaction_count: 5,
        sdk_version: '39.0',
        sequence: 3,
        dimensions: {},
      identity: null,
      };

      mockLocalStorage._store['nf_session'] = JSON.stringify(existingSession);
      storage = new Storage();
      sessionManager = new SessionManager(storage, tabStorage, config);

      const session = sessionManager.getOrCreateSession();
      expect(session.id).toBe('existing-session-id');
    });

    it('increments sequence on resume', () => {
      const existingSession: Session = {
        id: 'existing-session-id',
        workspace_id: 'ws_123',
        created_at: Date.now() - 5 * 60 * 1000,
        updated_at: Date.now() - 1 * 60 * 1000,
        last_active_at: Date.now() - 1 * 60 * 1000,
        focus_duration_ms: 0,
        total_duration_ms: 0,
        referrer: null,
        landing_page: 'https://example.com',
        utm: null,
        max_scroll_percent: 0,
        interaction_count: 0,
        sdk_version: '39.0',
        sequence: 3,
        dimensions: {},
      identity: null,
      };

      mockLocalStorage._store['nf_session'] = JSON.stringify(existingSession);
      storage = new Storage();
      sessionManager = new SessionManager(storage, tabStorage, config);

      const session = sessionManager.getOrCreateSession();
      expect(session.sequence).toBe(4);
    });

    it('updates last_active_at on resume', () => {
      const oldTime = Date.now() - 5 * 60 * 1000;
      const existingSession: Session = {
        id: 'existing-session-id',
        workspace_id: 'ws_123',
        created_at: oldTime,
        updated_at: oldTime,
        last_active_at: oldTime,
        focus_duration_ms: 0,
        total_duration_ms: 0,
        referrer: null,
        landing_page: 'https://example.com',
        utm: null,
        max_scroll_percent: 0,
        interaction_count: 0,
        sdk_version: '39.0',
        sequence: 0,
        dimensions: {},
      identity: null,
      };

      mockLocalStorage._store['nf_session'] = JSON.stringify(existingSession);
      storage = new Storage();
      sessionManager = new SessionManager(storage, tabStorage, config);

      const session = sessionManager.getOrCreateSession();
      expect(session.last_active_at).toBe(Date.now());
    });

    it('creates new session when expired (>sessionTimeout)', () => {
      const expiredTime = Date.now() - 35 * 60 * 1000; // 35 minutes ago
      const existingSession: Session = {
        id: 'old-session-id',
        workspace_id: 'ws_123',
        created_at: expiredTime,
        updated_at: expiredTime,
        last_active_at: expiredTime, // Expired
        focus_duration_ms: 0,
        total_duration_ms: 0,
        referrer: null,
        landing_page: 'https://example.com',
        utm: null,
        max_scroll_percent: 0,
        interaction_count: 0,
        sdk_version: '39.0',
        sequence: 5,
        dimensions: {},
      identity: null,
      };

      mockLocalStorage._store['nf_session'] = JSON.stringify(existingSession);
      storage = new Storage();
      sessionManager = new SessionManager(storage, tabStorage, config);

      const session = sessionManager.getOrCreateSession();
      expect(session.id).not.toBe('old-session-id');
      expect(session.sequence).toBe(0); // New session
    });
  });

  describe('custom dimensions', () => {
    beforeEach(() => {
      sessionManager.getOrCreateSession();
    });

    it('setDimension(index, value) validates index 1-10', () => {
      expect(() => sessionManager.setDimension(1, 'valid')).not.toThrow();
      expect(() => sessionManager.setDimension(10, 'valid')).not.toThrow();
    });

    it('setDimension() throws for index < 1', () => {
      expect(() => sessionManager.setDimension(0, 'value')).toThrow(
        'Dimension index must be between 1 and 10'
      );
    });

    it('setDimension() throws for index > 10', () => {
      expect(() => sessionManager.setDimension(11, 'value')).toThrow(
        'Dimension index must be between 1 and 10'
      );
    });

    it('setDimension() throws for value > 256 chars', () => {
      const longValue = 'a'.repeat(257);
      expect(() => sessionManager.setDimension(1, longValue)).toThrow(
        'Dimension value must be 256 characters or less'
      );
    });

    it('setDimension() throws for non-string value', () => {
      expect(() => sessionManager.setDimension(1, 123 as unknown as string)).toThrow(
        'Dimension value must be a string'
      );
    });

    it('setDimensions({1: "a", 2: "b"}) sets multiple', () => {
      sessionManager.setDimensions({ 1: 'value1', 2: 'value2' });

      expect(sessionManager.getDimension(1)).toBe('value1');
      expect(sessionManager.getDimension(2)).toBe('value2');
    });

    it('getDimension(index) returns value or null', () => {
      sessionManager.setDimension(1, 'test');
      expect(sessionManager.getDimension(1)).toBe('test');
      expect(sessionManager.getDimension(2)).toBeNull();
    });

    it('clearDimensions() empties dimensions', () => {
      sessionManager.setDimension(1, 'value1');
      sessionManager.setDimension(2, 'value2');
      sessionManager.clearDimensions();

      expect(sessionManager.getDimension(1)).toBeNull();
      expect(sessionManager.getDimension(2)).toBeNull();
    });

    it('getDimensionsPayload() returns {custom_1: "val", custom_2: "val2", ...}', () => {
      sessionManager.setDimension(1, 'first');
      sessionManager.setDimension(3, 'third');

      const payload = sessionManager.getDimensionsPayload();
      expect(payload).toEqual({
        custom_1: 'first',
        custom_3: 'third',
      });
    });
  });

  describe('applyUrlDimensions', () => {
    beforeEach(() => {
      sessionManager.getOrCreateSession();
    });

    it('sets dimensions from URL params when session has no dimensions', () => {
      sessionManager.applyUrlDimensions({ 1: 'campaign_a', 3: 'variant_b' });

      expect(sessionManager.getDimension(1)).toBe('campaign_a');
      expect(sessionManager.getDimension(3)).toBe('variant_b');
    });

    it('does not overwrite existing dimensions (priority rule)', () => {
      sessionManager.setDimension(1, 'existing_value');

      sessionManager.applyUrlDimensions({ 1: 'url_value', 2: 'new_value' });

      expect(sessionManager.getDimension(1)).toBe('existing_value');
      expect(sessionManager.getDimension(2)).toBe('new_value');
    });

    it('handles partial overlap correctly', () => {
      sessionManager.setDimension(2, 'existing_2');
      sessionManager.setDimension(5, 'existing_5');

      sessionManager.applyUrlDimensions({ 1: 'url_1', 2: 'url_2', 3: 'url_3', 5: 'url_5' });

      expect(sessionManager.getDimension(1)).toBe('url_1');
      expect(sessionManager.getDimension(2)).toBe('existing_2'); // Not overwritten
      expect(sessionManager.getDimension(3)).toBe('url_3');
      expect(sessionManager.getDimension(5)).toBe('existing_5'); // Not overwritten
    });

    it('persists changes to storage', () => {
      sessionManager.applyUrlDimensions({ 1: 'persisted_value' });

      // Verify it was saved to localStorage
      expect(mockLocalStorage.setItem).toHaveBeenCalledWith(
        'nf_dimensions',
        expect.stringContaining('persisted_value')
      );
    });

    it('does not save if no new dimensions were applied', () => {
      sessionManager.setDimension(1, 'existing');
      const callCountBefore = mockLocalStorage.setItem.mock.calls.length;

      sessionManager.applyUrlDimensions({ 1: 'ignored' });

      // Should not have additional calls since dimension 1 already exists
      const callsAfter = mockLocalStorage.setItem.mock.calls.slice(callCountBefore);
      const dimensionCalls = callsAfter.filter(call => call[0] === 'nf_dimensions');
      expect(dimensionCalls).toHaveLength(0);
    });

    it('does nothing if no session exists', () => {
      const freshSessionManager = new SessionManager(storage, tabStorage, config);
      // Don't call getOrCreateSession()

      // Should not throw
      expect(() => freshSessionManager.applyUrlDimensions({ 1: 'test' })).not.toThrow();
    });

    it('handles empty object gracefully', () => {
      sessionManager.applyUrlDimensions({});
      // Should not throw and no dimensions should be set
      expect(sessionManager.getDimension(1)).toBeNull();
    });
  });

  describe('tab ID', () => {
    it('getTabId() returns a stable safe integer', () => {
      // Was a UUIDv4: tab_id is now part of a BIGINT primary key column.
      const id = sessionManager.getTabId();
      expect(Number.isSafeInteger(id)).toBe(true);
      expect(sessionManager.getTabId()).toBe(id);
    });

    it('getTabId() returns same ID within tab session', () => {
      const tabId1 = sessionManager.getTabId();
      const tabId2 = sessionManager.getTabId();

      expect(tabId1).toBe(tabId2);
    });

    it('getTabId() uses sessionStorage', () => {
      sessionManager.getTabId();
      expect(mockSessionStorage.setItem).toHaveBeenCalledWith(
        'nf_tab_id',
        expect.any(String)
      );
    });
  });

  describe('reset', () => {
    it('removes session from storage', () => {
      sessionManager.getOrCreateSession();
      sessionManager.reset();

      expect(mockLocalStorage.removeItem).toHaveBeenCalledWith('nf_session');
    });

    it('removes dimensions from storage', () => {
      sessionManager.getOrCreateSession();
      sessionManager.setDimension(1, 'test');
      sessionManager.reset();

      expect(mockLocalStorage.removeItem).toHaveBeenCalledWith('nf_dimensions');
    });

    it('creates new session with new ID', () => {
      const session1 = sessionManager.getOrCreateSession();
      const session1Id = session1.id;

      const session2 = sessionManager.reset();

      expect(session2.id).not.toBe(session1Id);
    });
  });

  describe('tab identity (W0.1)', () => {
    it('mints a positive safe integer, not a UUID', () => {
      // tab_id lands in a BIGINT column that is part of the web_pages and
      // web_goals primary keys. A UUID would cost 16 bytes on the highest-volume
      // partitioned table and widen its PK index, for uniqueness far beyond what
      // "distinct among one session's tabs" needs.
      const tabId = sessionManager.getTabId();
      expect(typeof tabId).toBe('number');
      expect(Number.isSafeInteger(tabId)).toBe(true);
      expect(tabId).toBeGreaterThan(0);
    });

    it('is stable within a tab and persisted to sessionStorage', () => {
      const first = sessionManager.getTabId();
      expect(sessionManager.getTabId()).toBe(first);
      // sessionStorage is the correct lifetime: it survives a reload — so the
      // tab keeps numbering its pages from where it left off — and dies with the
      // tab, so a new tab is a genuinely new writer.
      expect(JSON.parse(mockSessionStorage._store['nf_tab_id'])).toBe(first);
    });

    it('differs between tabs, which is what makes them disjoint writers', () => {
      // A second tab means a second sessionStorage: that is exactly why tab_id
      // lives there while the session id lives in the shared localStorage.
      vi.stubGlobal('sessionStorage', createMockStorage());
      const otherTab = new SessionManager(storage, new TabStorage(), config);
      expect(otherTab.getTabId()).not.toBe(sessionManager.getTabId());
    });

    it('replaces a legacy UUID tab id rather than sending it as a number', () => {
      mockSessionStorage._store['nf_tab_id'] = JSON.stringify('8a9c1a1e-6f0e-4d17-9d5a-6b1f6e2d3c4b');
      const fresh = new SessionManager(storage, new TabStorage(), config);
      expect(Number.isSafeInteger(fresh.getTabId())).toBe(true);
    });
  });

  describe('session lifetime (W0.3)', () => {
    const makeStored = (overrides: Partial<Session>): Session => ({
      id: 'stored-id',
      workspace_id: 'ws_123',
      created_at: Date.now(),
      updated_at: Date.now(),
      last_active_at: Date.now(),
      focus_duration_ms: 0,
      total_duration_ms: 0,
      referrer: null,
      landing_page: 'https://example.com',
      utm: null,
      max_scroll_percent: 0,
      interaction_count: 0,
      sdk_version: '39.0',
      sequence: 1,
      dimensions: {},
      identity: null,
      ...overrides,
    });

    const load = (session: Session) => {
      mockLocalStorage._store['nf_session'] = JSON.stringify(session);
      storage = new Storage();
      return new SessionManager(storage, tabStorage, config);
    };

    it('expires a session past the absolute age cap even when activity is recent', () => {
      // The server rejects a session id older than 48h outright, and nothing in
      // the SDK reacts to that 400 by rotating — so a pinned tab goes silent
      // forever. Rotating at 24h leaves a full day of headroom for a queued beat
      // to still be replayed against the id it was minted under.
      const sm = load(
        makeStored({
          id: 'ancient',
          created_at: Date.now() - 25 * 60 * 60 * 1000,
          last_active_at: Date.now() - 1000,
        })
      );
      expect(sm.getOrCreateSession().id).not.toBe('ancient');
    });

    it('keeps a 23h-old session that is still active', () => {
      const sm = load(
        makeStored({
          id: 'still-fine',
          created_at: Date.now() - 23 * 60 * 60 * 1000,
          last_active_at: Date.now() - 1000,
        })
      );
      expect(sm.getOrCreateSession().id).toBe('still-fine');
    });

    it('touch() refreshes last_active_at and persists it', () => {
      // Without this the 30-minute window measures time since page LOAD, not
      // since activity: a visitor reading one long article is handed a brand new
      // session on their next navigation, splitting engaged time and attribution.
      const sm = load(makeStored({ id: 'active' }));
      sm.getOrCreateSession();
      sm.getSession()!.last_active_at = Date.now() - 5 * 60 * 1000;

      expect(sm.touch()).toBe(true);
      expect(Date.now() - sm.getSession()!.last_active_at).toBeLessThan(1000);
      expect(JSON.parse(mockLocalStorage._store['nf_session']).last_active_at).toBe(
        sm.getSession()!.last_active_at
      );
    });

    it('touch() reports expiry rather than silently continuing', () => {
      // The expiry check runs once at init today, so a window that lapses while
      // the tab stays open is never noticed. touch() is what lets the caller
      // rotate at the moment it actually happens.
      const sm = load(makeStored({ id: 'lapsing' }));
      sm.getOrCreateSession();
      sm.getSession()!.last_active_at = Date.now() - 31 * 60 * 1000;
      expect(sm.touch()).toBe(false);
    });

    it('a rotated session inherits identity and dimensions', () => {
      mockLocalStorage._store['nf_identity'] = JSON.stringify({ email: 'user@example.com', hmac: 'sig' });
      mockLocalStorage._store['nf_dimensions'] = JSON.stringify({ 1: 'pro' });
      const sm = load(
        makeStored({ id: 'ancient', created_at: Date.now() - 25 * 60 * 60 * 1000 })
      );

      const next = sm.getOrCreateSession();
      expect(next.id).not.toBe('ancient');
      expect(next.identity).toEqual({ email: 'user@example.com', hmac: 'sig' });
      expect(next.dimensions).toEqual({ 1: 'pro' });
    });
  });

  describe('getters', () => {
    it('getSessionId() returns session.id', () => {
      const session = sessionManager.getOrCreateSession();
      expect(sessionManager.getSessionId()).toBe(session.id);
    });

    it('getSession() returns current session', () => {
      const session = sessionManager.getOrCreateSession();
      expect(sessionManager.getSession()).toBe(session);
    });
  });

  describe('cross-domain session', () => {
    const getValidCrossDomainInput = () => ({
      sessionId: 'cross-domain-session-id-1234-567890abcdef',
      timestamp: Math.floor(Date.now() / 1000), // Evaluated at test time (with fake timers)
      expiry: 120,
    });

    it('should resume session from valid cross-domain input', () => {
      const input = getValidCrossDomainInput();
      sessionManager.setCrossDomainInput(input);
      const session = sessionManager.getOrCreateSession();

      expect(session.id).toBe(input.sessionId);
    });

    it('drops a self-referral on the cross-domain path too', () => {
      Object.defineProperty(document, 'referrer', {
        value: 'https://example.com/compare/',
        writable: true,
        configurable: true,
      });

      const input = getValidCrossDomainInput();
      sessionManager.setCrossDomainInput(input);
      const session = sessionManager.getOrCreateSession();

      expect(session.id).toBe(input.sessionId);
      expect(session.referrer).toBeNull();
    });

    it('should ignore expired cross-domain input', () => {
      const expiredInput = {
        ...getValidCrossDomainInput(),
        timestamp: Math.floor(Date.now() / 1000) - 300, // 5 minutes ago
        expiry: 120, // 2 minute expiry
      };

      sessionManager.setCrossDomainInput(expiredInput);
      const session = sessionManager.getOrCreateSession();

      // Should create new session, not use cross-domain
      expect(session.id).not.toBe(expiredInput.sessionId);
    });

    it('should ignore cross-domain input with future timestamp (>60s)', () => {
      const futureInput = {
        ...getValidCrossDomainInput(),
        timestamp: Math.floor(Date.now() / 1000) + 120, // 2 minutes in future
      };

      sessionManager.setCrossDomainInput(futureInput);
      const session = sessionManager.getOrCreateSession();

      // Should create new session, not use cross-domain
      expect(session.id).not.toBe(futureInput.sessionId);
    });

    it('should accept cross-domain input within clock skew tolerance (60s future)', () => {
      const slightlyFutureInput = {
        ...getValidCrossDomainInput(),
        timestamp: Math.floor(Date.now() / 1000) + 30, // 30 seconds in future
      };

      sessionManager.setCrossDomainInput(slightlyFutureInput);
      const session = sessionManager.getOrCreateSession();

      // Should use cross-domain input
      expect(session.id).toBe(slightlyFutureInput.sessionId);
    });

    it('should fallback to localStorage if cross-domain invalid', () => {
      // Store an existing session in localStorage
      const existingSession: Session = {
        id: 'existing-local-session-id',
        workspace_id: 'ws_123',
        created_at: Date.now() - 5 * 60 * 1000,
        updated_at: Date.now() - 1 * 60 * 1000,
        last_active_at: Date.now() - 1 * 60 * 1000,
        focus_duration_ms: 0,
        total_duration_ms: 0,
        referrer: null,
        landing_page: 'https://example.com',
        utm: null,
        max_scroll_percent: 0,
        interaction_count: 0,
        sdk_version: '39.0',
        sequence: 3,
        dimensions: {},
      identity: null,
      };

      mockLocalStorage._store['nf_session'] = JSON.stringify(existingSession);
      storage = new Storage();
      sessionManager = new SessionManager(storage, tabStorage, config);

      // Set expired cross-domain input
      const expiredInput = {
        ...getValidCrossDomainInput(),
        timestamp: Math.floor(Date.now() / 1000) - 300,
      };
      sessionManager.setCrossDomainInput(expiredInput);

      const session = sessionManager.getOrCreateSession();

      // Should resume from localStorage, not cross-domain
      expect(session.id).toBe('existing-local-session-id');
    });

    it('should prefer cross-domain input over localStorage when valid', () => {
      // Store an existing session in localStorage
      const existingSession: Session = {
        id: 'existing-local-session-id',
        workspace_id: 'ws_123',
        created_at: Date.now() - 5 * 60 * 1000,
        updated_at: Date.now() - 1 * 60 * 1000,
        last_active_at: Date.now() - 1 * 60 * 1000,
        focus_duration_ms: 0,
        total_duration_ms: 0,
        referrer: null,
        landing_page: 'https://example.com',
        utm: null,
        max_scroll_percent: 0,
        interaction_count: 0,
        sdk_version: '39.0',
        sequence: 3,
        dimensions: {},
      identity: null,
      };

      mockLocalStorage._store['nf_session'] = JSON.stringify(existingSession);
      storage = new Storage();
      sessionManager = new SessionManager(storage, tabStorage, config);

      // Set valid cross-domain input
      const input = getValidCrossDomainInput();
      sessionManager.setCrossDomainInput(input);

      const session = sessionManager.getOrCreateSession();

      // Should use cross-domain, not localStorage
      expect(session.id).toBe(input.sessionId);
    });
  });
});
