/**
 * URL Dimensions Integration Tests
 *
 * Tests the full URL dimension flow including:
 * - Parsing custom_1 through custom_10 from URL
 * - Applying URL dimensions to session
 * - Priority rule (existing dimensions win)
 * - Persistence across session operations
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { SessionManager } from '../../src/core/session';
import { DEFAULT_CROSS_DOMAIN_PARAM } from '../../src/core/cross-domain';
import { Storage, TabStorage } from '../../src/storage/storage';
import { parseCustomDimensions } from '../../src/utils/custom-dimensions';
import type { InternalConfig } from '../../src/types';

describe('URL Dimensions Integration', () => {
  let storage: Storage;
  let tabStorage: TabStorage;
  let config: InternalConfig;

  const createConfig = (overrides: Partial<InternalConfig> = {}): InternalConfig => ({
    workspace_id: 'ws_test',
    endpoint: 'https://api.example.com',
    debug: false,
    sessionTimeout: 30 * 60 * 1000,
    heartbeatInterval: 10000,
    adClickIds: ['gclid', 'fbclid'],
    trackSPA: true,
    trackScroll: true,
    trackClicks: false,
    heartbeatTiers: [{ after: 0, desktopInterval: 10000, mobileInterval: 7000 }],
    heartbeatMaxDuration: 10 * 60 * 1000,
    resetHeartbeatOnNavigation: false,
    crossDomains: [],
    crossDomainExpiry: 120,
    crossDomainStripParams: true,
    crossDomainParam: DEFAULT_CROSS_DOMAIN_PARAM,
    ...overrides,
  });

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-15T12:00:00Z'));

    storage = new Storage();
    tabStorage = new TabStorage();
    config = createConfig();

    // Mock window.location
    Object.defineProperty(window, 'location', {
      value: {
        href: 'https://www.example.com/page',
        pathname: '/page',
        search: '',
        hash: '',
        origin: 'https://www.example.com',
        hostname: 'www.example.com',
      },
      writable: true,
      configurable: true,
    });

    // Mock document.referrer
    Object.defineProperty(document, 'referrer', {
      value: '',
      writable: true,
      configurable: true,
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
    localStorage.clear();
    sessionStorage.clear();
  });

  describe('Full Flow: URL to Session Dimensions', () => {
    it('should capture custom_* params from URL and apply to session', () => {
      const url = 'https://www.example.com/page?custom_1=campaign_a&custom_3=variant_b&custom_10=source_x';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?custom_1=campaign_a&custom_3=variant_b&custom_10=source_x',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      // Step 1: Parse dimensions from URL (as SDK does)
      const urlDimensions = parseCustomDimensions(window.location.href);
      expect(urlDimensions).toEqual({
        1: 'campaign_a',
        3: 'variant_b',
        10: 'source_x',
      });

      // Step 2: Create session
      const sessionManager = new SessionManager(storage, tabStorage, config);
      sessionManager.getOrCreateSession();

      // Step 3: Apply URL dimensions (as SDK does after session creation)
      sessionManager.applyUrlDimensions(urlDimensions);

      // Step 4: Verify dimensions are set
      expect(sessionManager.getDimension(1)).toBe('campaign_a');
      expect(sessionManager.getDimension(3)).toBe('variant_b');
      expect(sessionManager.getDimension(10)).toBe('source_x');
      expect(sessionManager.getDimension(2)).toBeNull(); // Not in URL

      // Step 5: Verify payload format
      const payload = sessionManager.getDimensionsPayload();
      expect(payload).toEqual({
        custom_1: 'campaign_a',
        custom_3: 'variant_b',
        custom_10: 'source_x',
      });
    });

    it('should preserve existing dimensions when URL has same indices (existing wins)', () => {
      // Pre-set dimensions in storage (simulating previous session)
      localStorage.setItem('nf_dimensions', JSON.stringify({
        1: 'existing_campaign',
        2: 'existing_medium',
      }));

      const url = 'https://www.example.com/page?custom_1=url_campaign&custom_3=url_variant';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?custom_1=url_campaign&custom_3=url_variant',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      // Parse URL dimensions
      const urlDimensions = parseCustomDimensions(window.location.href);

      // Create session (loads existing dimensions from storage)
      const sessionManager = new SessionManager(storage, tabStorage, config);
      sessionManager.getOrCreateSession();

      // Apply URL dimensions
      sessionManager.applyUrlDimensions(urlDimensions);

      // Verify priority rule: existing wins
      expect(sessionManager.getDimension(1)).toBe('existing_campaign'); // NOT overwritten
      expect(sessionManager.getDimension(2)).toBe('existing_medium');   // Preserved
      expect(sessionManager.getDimension(3)).toBe('url_variant');       // New from URL
    });

    it('should persist URL dimensions across page reloads', () => {
      const url = 'https://www.example.com/page?custom_1=campaign_a';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?custom_1=campaign_a',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      // First page load: capture URL dimension
      const urlDimensions = parseCustomDimensions(window.location.href);
      const sessionManager1 = new SessionManager(storage, tabStorage, config);
      sessionManager1.getOrCreateSession();
      sessionManager1.applyUrlDimensions(urlDimensions);

      expect(sessionManager1.getDimension(1)).toBe('campaign_a');

      // Simulate page reload without custom_1 in URL
      Object.defineProperty(window, 'location', {
        value: {
          href: 'https://www.example.com/page',
          pathname: '/page',
          search: '',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      // Advance time but stay within session timeout
      vi.advanceTimersByTime(5 * 60 * 1000); // 5 minutes

      // Second page load: dimension should persist
      const sessionManager2 = new SessionManager(storage, tabStorage, config);
      sessionManager2.getOrCreateSession();

      // No URL dimensions this time
      const urlDimensions2 = parseCustomDimensions(window.location.href);
      expect(Object.keys(urlDimensions2)).toHaveLength(0);

      // But dimension should still be there from previous load
      expect(sessionManager2.getDimension(1)).toBe('campaign_a');
    });
  });

  describe('URL Parameter Edge Cases', () => {
    it('should handle URL-encoded values correctly', () => {
      const url = 'https://www.example.com/page?custom_1=hello%20world&custom_2=a%2Bb%3Dc';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?custom_1=hello%20world&custom_2=a%2Bb%3Dc',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      const urlDimensions = parseCustomDimensions(window.location.href);
      const sessionManager = new SessionManager(storage, tabStorage, config);
      sessionManager.getOrCreateSession();
      sessionManager.applyUrlDimensions(urlDimensions);

      expect(sessionManager.getDimension(1)).toBe('hello world');
      expect(sessionManager.getDimension(2)).toBe('a+b=c');
    });

    it('should ignore out-of-range indices (custom_0, custom_11)', () => {
      const url = 'https://www.example.com/page?custom_0=invalid&custom_1=valid&custom_11=invalid';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?custom_0=invalid&custom_1=valid&custom_11=invalid',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      const urlDimensions = parseCustomDimensions(window.location.href);
      const sessionManager = new SessionManager(storage, tabStorage, config);
      sessionManager.getOrCreateSession();
      sessionManager.applyUrlDimensions(urlDimensions);

      expect(sessionManager.getDimension(1)).toBe('valid');
      expect(urlDimensions[0]).toBeUndefined();
      expect(urlDimensions[11]).toBeUndefined();
    });

    it('should ignore values exceeding 256 characters', () => {
      const longValue = 'a'.repeat(257);
      const url = `https://www.example.com/page?custom_1=${longValue}&custom_2=valid`;

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: `?custom_1=${longValue}&custom_2=valid`,
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      const urlDimensions = parseCustomDimensions(window.location.href);
      const sessionManager = new SessionManager(storage, tabStorage, config);
      sessionManager.getOrCreateSession();
      sessionManager.applyUrlDimensions(urlDimensions);

      expect(sessionManager.getDimension(1)).toBeNull(); // Ignored (too long)
      expect(sessionManager.getDimension(2)).toBe('valid');
    });

    it('should handle empty string values (treated as no value by getDimension API)', () => {
      const url = 'https://www.example.com/page?custom_1=&custom_2=nonempty';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?custom_1=&custom_2=nonempty',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      const urlDimensions = parseCustomDimensions(window.location.href);

      // Empty string IS captured from URL
      expect(urlDimensions[1]).toBe('');

      const sessionManager = new SessionManager(storage, tabStorage, config);
      sessionManager.getOrCreateSession();
      sessionManager.applyUrlDimensions(urlDimensions);

      // But getDimension returns null for empty strings (falsy value)
      // This is existing API behavior: `dimensions[index] || null`
      expect(sessionManager.getDimension(1)).toBeNull();
      expect(sessionManager.getDimension(2)).toBe('nonempty');

      // However, the payload DOES include the empty string
      const payload = sessionManager.getDimensionsPayload();
      expect(payload['custom_1']).toBe('');
      expect(payload['custom_2']).toBe('nonempty');
    });
  });

  describe('Mixed with Other URL Parameters', () => {
    it('should work alongside UTM parameters', () => {
      const url = 'https://www.example.com/page?utm_source=google&utm_medium=cpc&custom_1=ab_test_a&custom_2=cohort_b';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?utm_source=google&utm_medium=cpc&custom_1=ab_test_a&custom_2=cohort_b',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      const urlDimensions = parseCustomDimensions(window.location.href);

      // Should only capture custom_* params
      expect(urlDimensions).toEqual({
        1: 'ab_test_a',
        2: 'cohort_b',
      });

      const sessionManager = new SessionManager(storage, tabStorage, config);
      const session = sessionManager.getOrCreateSession();
      sessionManager.applyUrlDimensions(urlDimensions);

      // UTM should be captured by session separately
      expect(session.utm?.source).toBe('google');
      expect(session.utm?.medium).toBe('cpc');

      // Custom dimensions from URL
      expect(sessionManager.getDimension(1)).toBe('ab_test_a');
      expect(sessionManager.getDimension(2)).toBe('cohort_b');
    });

    it('should work alongside cross-domain _nf parameter', () => {
      // This tests that custom_1-10 are independent of the _nf cross-domain param
      const url = 'https://www.example.com/page?_nf=eyJzIjoiYWJjMTIzIiwidCI6MTcwNTMyMDAwMH0&custom_1=campaign_x';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?_nf=eyJzIjoiYWJjMTIzIiwidCI6MTcwNTMyMDAwMH0&custom_1=campaign_x',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      const urlDimensions = parseCustomDimensions(window.location.href);

      // Should only capture custom_1, not _nf
      expect(urlDimensions).toEqual({ 1: 'campaign_x' });
      expect(urlDimensions['_nf' as unknown as number]).toBeUndefined();
    });
  });

  describe('Programmatic Dimension Override', () => {
    it('should allow setDimension() to override URL-applied dimension', () => {
      const url = 'https://www.example.com/page?custom_1=url_value';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?custom_1=url_value',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      const urlDimensions = parseCustomDimensions(window.location.href);
      const sessionManager = new SessionManager(storage, tabStorage, config);
      sessionManager.getOrCreateSession();
      sessionManager.applyUrlDimensions(urlDimensions);

      expect(sessionManager.getDimension(1)).toBe('url_value');

      // Now override with setDimension()
      sessionManager.setDimension(1, 'programmatic_value');
      expect(sessionManager.getDimension(1)).toBe('programmatic_value');
    });
  });

  describe('Session Reset', () => {
    it('should clear URL dimensions on session reset', () => {
      const url = 'https://www.example.com/page?custom_1=campaign_a';

      Object.defineProperty(window, 'location', {
        value: {
          href: url,
          pathname: '/page',
          search: '?custom_1=campaign_a',
          hash: '',
          origin: 'https://www.example.com',
          hostname: 'www.example.com',
        },
        writable: true,
        configurable: true,
      });

      const urlDimensions = parseCustomDimensions(window.location.href);
      const sessionManager = new SessionManager(storage, tabStorage, config);
      sessionManager.getOrCreateSession();
      sessionManager.applyUrlDimensions(urlDimensions);

      expect(sessionManager.getDimension(1)).toBe('campaign_a');

      // Reset session
      sessionManager.reset();

      // Dimensions should be cleared
      expect(sessionManager.getDimension(1)).toBeNull();
    });
  });
});
