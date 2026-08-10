import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { SessionManager } from './session';
import { Storage, TabStorage } from '../storage/storage';
import type { InternalConfig } from '../types';

// Mock UUID generation
vi.mock('../utils/uuid', () => ({
  generateUUIDv4: vi.fn(() => 'mock-uuid-v4-' + Math.random().toString(36).slice(2, 10)),
  generateUUIDv7: vi.fn(() => 'mock-uuid-v7-' + Math.random().toString(36).slice(2, 10)),
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

describe('SessionManager - User ID', () => {
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
      href: 'https://example.com/page',
      pathname: '/page',
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
      sessionTimeout: 30 * 60 * 1000,
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

  describe('setUserId / getUserId', () => {
    beforeEach(() => {
      sessionManager.getOrCreateSession();
    });

    it('setUserId() stores value correctly', () => {
      sessionManager.setUserId('user_123');
      expect(sessionManager.getUserId()).toBe('user_123');
    });

    it('getUserId() returns null when no user ID is set', () => {
      expect(sessionManager.getUserId()).toBeNull();
    });

    it('setUserId(null) clears the value', () => {
      sessionManager.setUserId('user_123');
      expect(sessionManager.getUserId()).toBe('user_123');

      sessionManager.setUserId(null);
      expect(sessionManager.getUserId()).toBeNull();
    });

    it('persists user ID to storage', () => {
      sessionManager.setUserId('user_456');

      // Verify it was saved to localStorage
      expect(mockLocalStorage.setItem).toHaveBeenCalledWith(
        'nf_user_id',
        expect.stringContaining('user_456')
      );
    });

    it('loads user ID from storage on session resume', () => {
      // Set user ID and save
      sessionManager.setUserId('persistent_user');

      // Create new SessionManager (simulating page reload)
      storage = new Storage();
      const newSessionManager = new SessionManager(storage, tabStorage, config);
      newSessionManager.getOrCreateSession();

      expect(newSessionManager.getUserId()).toBe('persistent_user');
    });

    it('user ID is included in session', () => {
      sessionManager.setUserId('session_user');
      const session = sessionManager.getSession();

      expect(session?.userId).toBe('session_user');
    });

    it('validates user ID is a string or null', () => {
      expect(() => sessionManager.setUserId(123 as unknown as string)).toThrow(
        'User ID must be a string or null'
      );
    });

    it('validates user ID length (max 256 chars)', () => {
      const longId = 'a'.repeat(257);
      expect(() => sessionManager.setUserId(longId)).toThrow(
        'User ID must be 256 characters or less'
      );
    });
  });

  describe('reset clears user ID', () => {
    it('reset() removes user ID from storage', () => {
      sessionManager.getOrCreateSession();
      sessionManager.setUserId('will_be_cleared');

      sessionManager.reset();

      expect(mockLocalStorage.removeItem).toHaveBeenCalledWith('nf_user_id');
      expect(sessionManager.getUserId()).toBeNull();
    });
  });
});
