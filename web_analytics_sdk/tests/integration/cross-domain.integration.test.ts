/**
 * Cross-Domain Session Tracking Integration Tests
 *
 * Tests the full cross-domain flow including:
 * - Session continuity across domains (same session_id)
 * - URL decoration with _nf parameter
 * - Parameter reading and stripping
 * - Expired/invalid payload handling
 * - decorateUrl() API
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { CrossDomainLinker, encode } from '../../src/core/cross-domain';
import { SessionManager } from '../../src/core/session';
import { Storage, TabStorage } from '../../src/storage/storage';
import type { InternalConfig } from '../../src/types';

describe('Cross-Domain Integration', () => {
  let storage: Storage;
  let tabStorage: TabStorage;
  let config: InternalConfig;
  let originalLocation: Location;
  let originalHistory: History;

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
    crossDomains: ['blog.example.com', 'shop.example.com'],
    crossDomainExpiry: 120,
    crossDomainStripParams: true,
    crossDomainParam: '_nf',
    ...overrides,
  });

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-15T12:00:00Z'));

    storage = new Storage();
    tabStorage = new TabStorage();
    config = createConfig();

    // Store original location
    originalLocation = window.location;
    originalHistory = window.history;

    // Mock location
    const locationMock = {
      href: 'https://www.example.com/page',
      pathname: '/page',
      search: '',
      hash: '',
      origin: 'https://www.example.com',
      hostname: 'www.example.com',
    };
    Object.defineProperty(window, 'location', {
      value: locationMock,
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

    // Restore location
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
  });

  describe('Session Continuity', () => {
    it('should maintain same session when navigating to configured domain', () => {
      // Create session on source domain
      const sourceManager = new SessionManager(storage, tabStorage, config);
      const sourceSession = sourceManager.getOrCreateSession();

      // Create cross-domain linker
      const linker = new CrossDomainLinker({
        domains: config.crossDomains,
        expiry: config.crossDomainExpiry,
        debug: false,
        paramName: '_nf',
      });
      linker.setIdGetters(
        () => sourceManager.getSessionId()
      );

      // Decorate URL for target domain
      const decoratedUrl = linker.decorateUrl('https://blog.example.com/article');
      expect(decoratedUrl).toContain('_nf=');

      // Extract the _nf parameter
      const url = new URL(decoratedUrl);
      const stmParam = url.searchParams.get('_nf');
      expect(stmParam).toBeTruthy();

      // Simulate arrival on target domain
      Object.defineProperty(window, 'location', {
        value: {
          href: decoratedUrl,
          pathname: '/article',
          search: `?_nf=${stmParam}`,
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      // Read the cross-domain parameter
      const payload = CrossDomainLinker.readParam(config.crossDomainExpiry);
      expect(payload).not.toBeNull();
      expect(payload!.s).toBe(sourceSession.id);

      // Create session on target domain with cross-domain input
      const targetStorage = new Storage(); // Fresh storage (different domain)
      const targetTabStorage = new TabStorage();
      const targetManager = new SessionManager(targetStorage, targetTabStorage, config);

      targetManager.setCrossDomainInput({
        sessionId: payload!.s,
        timestamp: payload!.t,
        expiry: config.crossDomainExpiry,
      });

      const targetSession = targetManager.getOrCreateSession();

      // Verify session continuity
      expect(targetSession.id).toBe(sourceSession.id);
    });
  });

  describe('Expired Payload Handling', () => {
    it('should create new session when cross-domain payload is expired', () => {
      const sessionId = '01901234-5678-7abc-def0-123456789abc';
      const oldTimestamp = Math.floor(Date.now() / 1000) - 300; // 5 minutes ago

      const payload = encode({ s: sessionId, t: oldTimestamp });

      Object.defineProperty(window, 'location', {
        value: {
          href: `https://blog.example.com/page?_nf=${payload}`,
          pathname: '/page',
          search: `?_nf=${payload}`,
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      // readParam should return null for expired payload
      const crossDomainPayload = CrossDomainLinker.readParam(120); // 2 min expiry
      expect(crossDomainPayload).toBeNull();

      // Session manager should create new session
      const manager = new SessionManager(storage, tabStorage, config);
      const session = manager.getOrCreateSession();

      expect(session.id).not.toBe(sessionId);
    });

    it('should reject payload with future timestamp beyond tolerance', () => {
      const sessionId = '01901234-5678-7abc-def0-123456789abc';
      const futureTimestamp = Math.floor(Date.now() / 1000) + 120; // 2 minutes in future

      const payload = encode({ s: sessionId, t: futureTimestamp });

      Object.defineProperty(window, 'location', {
        value: {
          href: `https://blog.example.com/page?_nf=${payload}`,
          pathname: '/page',
          search: `?_nf=${payload}`,
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      const crossDomainPayload = CrossDomainLinker.readParam(120);
      expect(crossDomainPayload).toBeNull();
    });

    it('should accept payload with slight clock skew (within 60s tolerance)', () => {
      const sessionId = '01901234-5678-7abc-def0-123456789abc';
      const slightFutureTimestamp = Math.floor(Date.now() / 1000) + 30; // 30 seconds in future

      const payload = encode({ s: sessionId, t: slightFutureTimestamp });

      Object.defineProperty(window, 'location', {
        value: {
          href: `https://blog.example.com/page?_nf=${payload}`,
          pathname: '/page',
          search: `?_nf=${payload}`,
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      const crossDomainPayload = CrossDomainLinker.readParam(120);
      expect(crossDomainPayload).not.toBeNull();
      expect(crossDomainPayload!.s).toBe(sessionId);
    });
  });

  describe('Invalid Payload Handling', () => {
    it('should create new session for malformed base64', () => {
      Object.defineProperty(window, 'location', {
        value: {
          href: 'https://blog.example.com/page?_nf=not-valid-base64!!!',
          pathname: '/page',
          search: '?_nf=not-valid-base64!!!',
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      const crossDomainPayload = CrossDomainLinker.readParam(120);
      expect(crossDomainPayload).toBeNull();

      const manager = new SessionManager(storage, tabStorage, config);
      const session = manager.getOrCreateSession();

      // Should create fresh session
      expect(session.id).toBeTruthy();
    });

    it('should create new session for invalid UUID format', () => {
      const invalidPayload = encode({
        s: 'not-a-uuid',
        t: Math.floor(Date.now() / 1000),
      });

      Object.defineProperty(window, 'location', {
        value: {
          href: `https://blog.example.com/page?_nf=${invalidPayload}`,
          pathname: '/page',
          search: `?_nf=${invalidPayload}`,
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      const crossDomainPayload = CrossDomainLinker.readParam(120);
      expect(crossDomainPayload).toBeNull();
    });

    it('should create new session for missing fields in payload', () => {
      // Encode partial payload (missing session_id)
      const partialPayload = btoa(JSON.stringify({ t: Date.now() }))
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/, '');

      Object.defineProperty(window, 'location', {
        value: {
          href: `https://blog.example.com/page?_nf=${partialPayload}`,
          pathname: '/page',
          search: `?_nf=${partialPayload}`,
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      const crossDomainPayload = CrossDomainLinker.readParam(120);
      expect(crossDomainPayload).toBeNull();
    });
  });

  describe('Parameter Stripping', () => {
    it('should strip _nf parameter from URL after reading', () => {
      const sessionId = '01901234-5678-7abc-def0-123456789abc';
      const timestamp = Math.floor(Date.now() / 1000);
      const payload = encode({ s: sessionId, t: timestamp });

      const replaceStateSpy = vi.fn();
      Object.defineProperty(window, 'history', {
        value: {
          ...originalHistory,
          replaceState: replaceStateSpy,
        },
        writable: true,
        configurable: true,
      });

      Object.defineProperty(window, 'location', {
        value: {
          href: `https://blog.example.com/page?_nf=${payload}&other=value`,
          pathname: '/page',
          search: `?_nf=${payload}&other=value`,
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      CrossDomainLinker.stripParam();

      expect(replaceStateSpy).toHaveBeenCalledWith(
        undefined, // window.history.state
        '',
        '/page?other=value' // Implementation uses relative path
      );
    });

    it('should preserve other query parameters when stripping', () => {
      const payload = encode({
        s: '01901234-5678-7abc-def0-123456789abc',
        t: Math.floor(Date.now() / 1000),
      });

      const replaceStateSpy = vi.fn();
      Object.defineProperty(window, 'history', {
        value: {
          ...originalHistory,
          replaceState: replaceStateSpy,
        },
        writable: true,
        configurable: true,
      });

      Object.defineProperty(window, 'location', {
        value: {
          href: `https://blog.example.com/page?utm_source=google&_nf=${payload}&page=2`,
          pathname: '/page',
          search: `?utm_source=google&_nf=${payload}&page=2`,
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      CrossDomainLinker.stripParam();

      expect(replaceStateSpy).toHaveBeenCalledWith(
        undefined,
        '',
        '/page?utm_source=google&page=2'
      );
    });

    it('should handle URL with only _nf parameter', () => {
      const payload = encode({
        s: '01901234-5678-7abc-def0-123456789abc',
        t: Math.floor(Date.now() / 1000),
      });

      const replaceStateSpy = vi.fn();
      Object.defineProperty(window, 'history', {
        value: {
          ...originalHistory,
          replaceState: replaceStateSpy,
        },
        writable: true,
        configurable: true,
      });

      Object.defineProperty(window, 'location', {
        value: {
          href: `https://blog.example.com/page?_nf=${payload}`,
          pathname: '/page',
          search: `?_nf=${payload}`,
          hash: '',
          origin: 'https://blog.example.com',
          hostname: 'blog.example.com',
        },
        writable: true,
        configurable: true,
      });

      CrossDomainLinker.stripParam();

      expect(replaceStateSpy).toHaveBeenCalledWith(
        undefined,
        '',
        '/page'
      );
    });
  });

  describe('decorateUrl API', () => {
    it('should decorate URLs for configured domains', () => {
      const linker = new CrossDomainLinker({
        domains: ['blog.example.com', 'shop.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });

      const sessionId = 'session-456';
      linker.setIdGetters(
        () => sessionId
      );

      const decorated = linker.decorateUrl('https://blog.example.com/article');
      expect(decorated).toContain('_nf=');

      // Verify payload can be decoded
      const url = new URL(decorated);
      const stm = url.searchParams.get('_nf');
      const decoded = JSON.parse(atob(stm!.replace(/-/g, '+').replace(/_/g, '/')));
      expect(decoded.s).toBe(sessionId);
    });

    it('should not decorate URLs for unconfigured domains', () => {
      const linker = new CrossDomainLinker({
        domains: ['blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });

      linker.setIdGetters(
        () => 'session-456'
      );

      const decorated = linker.decorateUrl('https://other-site.com/page');
      expect(decorated).toBe('https://other-site.com/page');
      expect(decorated).not.toContain('_nf=');
    });

    it('should not decorate same-origin URLs', () => {
      const linker = new CrossDomainLinker({
        domains: ['www.example.com', 'blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });

      linker.setIdGetters(
        () => 'session-456'
      );

      // Same as current origin
      const decorated = linker.decorateUrl('https://www.example.com/other-page');
      expect(decorated).toBe('https://www.example.com/other-page');
      expect(decorated).not.toContain('_nf=');
    });

    it('should preserve existing query parameters when decorating', () => {
      const linker = new CrossDomainLinker({
        domains: ['blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });

      linker.setIdGetters(
        () => 'session-456'
      );

      const decorated = linker.decorateUrl('https://blog.example.com/page?existing=param&other=value');
      expect(decorated).toContain('existing=param');
      expect(decorated).toContain('other=value');
      expect(decorated).toContain('_nf=');
    });

    it('should preserve hash when decorating', () => {
      const linker = new CrossDomainLinker({
        domains: ['blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });

      linker.setIdGetters(
        () => 'session-456'
      );

      const decorated = linker.decorateUrl('https://blog.example.com/page#section');
      expect(decorated).toContain('#section');
      expect(decorated).toContain('_nf=');
    });
  });

  describe('Click Interception', () => {
    it('should decorate link href on click to configured domain', () => {
      const linker = new CrossDomainLinker({
        domains: ['blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });

      linker.setIdGetters(
        () => 'session-456'
      );

      linker.start();

      // Create a link element
      const link = document.createElement('a');
      link.href = 'https://blog.example.com/article';
      document.body.appendChild(link);

      // Simulate click
      const clickEvent = new MouseEvent('click', {
        bubbles: true,
        cancelable: true,
      });
      link.dispatchEvent(clickEvent);

      // Check that href was decorated
      expect(link.href).toContain('_nf=');

      // Cleanup
      linker.stop();
      document.body.removeChild(link);
    });

    it('should not decorate link to unconfigured domain', () => {
      const linker = new CrossDomainLinker({
        domains: ['blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });

      linker.setIdGetters(
        () => 'session-456'
      );

      linker.start();

      const link = document.createElement('a');
      link.href = 'https://other-site.com/page';
      const originalHref = link.href;
      document.body.appendChild(link);

      const clickEvent = new MouseEvent('click', {
        bubbles: true,
        cancelable: true,
      });
      link.dispatchEvent(clickEvent);

      expect(link.href).toBe(originalHref);
      expect(link.href).not.toContain('_nf=');

      linker.stop();
      document.body.removeChild(link);
    });

    it('should stop intercepting clicks after stop()', () => {
      const linker = new CrossDomainLinker({
        domains: ['blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });

      linker.setIdGetters(
        () => 'session-456'
      );

      linker.start();
      linker.stop();

      const link = document.createElement('a');
      link.href = 'https://blog.example.com/article';
      const originalHref = link.href;
      document.body.appendChild(link);

      const clickEvent = new MouseEvent('click', {
        bubbles: true,
        cancelable: true,
      });
      link.dispatchEvent(clickEvent);

      expect(link.href).toBe(originalHref);

      document.body.removeChild(link);
    });
  });

  describe('Full Flow E2E', () => {
    it('should maintain session through complete cross-domain journey', () => {
      // Step 1: Create session on source domain (www.example.com)
      const sourceManager = new SessionManager(storage, tabStorage, config);
      const sourceSession = sourceManager.getOrCreateSession();
      const originalSessionId = sourceSession.id;

      // Step 2: Create linker and decorate URL
      const linker = new CrossDomainLinker({
        domains: config.crossDomains,
        expiry: config.crossDomainExpiry,
        debug: false,
        paramName: '_nf',
      });
      linker.setIdGetters(
        () => sourceManager.getSessionId()
      );

      const decoratedUrl = linker.decorateUrl('https://blog.example.com/article');

      // Step 3: Simulate navigation to target domain
      const url = new URL(decoratedUrl);
      Object.defineProperty(window, 'location', {
        value: {
          href: decoratedUrl,
          pathname: url.pathname,
          search: url.search,
          hash: url.hash,
          origin: url.origin,
          hostname: url.hostname,
        },
        writable: true,
        configurable: true,
      });

      // Step 4: Read and validate payload on target domain
      const payload = CrossDomainLinker.readParam(config.crossDomainExpiry);
      expect(payload).not.toBeNull();

      // Step 5: Create session on target domain
      const targetStorage = new Storage();
      const targetTabStorage = new TabStorage();
      const targetManager = new SessionManager(targetStorage, targetTabStorage, config);

      targetManager.setCrossDomainInput({
        sessionId: payload!.s,
        timestamp: payload!.t,
        expiry: config.crossDomainExpiry,
      });

      const targetSession = targetManager.getOrCreateSession();

      // Step 6: Verify complete session continuity
      expect(targetSession.id).toBe(originalSessionId);
    });
  });
});
