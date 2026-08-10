import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  CrossDomainLinker,
  encode,
  decode,
  type CrossDomainPayload,
} from './cross-domain';

describe('CrossDomainLinker', () => {
  describe('encode/decode', () => {
    it('should encode payload to base64url string', () => {
      const payload: CrossDomainPayload = {
        s: '019012ab-cdef-7890-abcd-ef1234567890',
        t: 1704067200,
      };
      const encoded = encode(payload);

      // Should be base64url (no +, /, or = padding)
      expect(encoded).not.toContain('+');
      expect(encoded).not.toContain('/');
      expect(encoded).not.toContain('=');
      expect(typeof encoded).toBe('string');
      expect(encoded.length).toBeGreaterThan(0);
    });

    it('should decode valid base64url payload', () => {
      const original: CrossDomainPayload = {
        s: '019012ab-cdef-7890-abcd-ef1234567890',
        t: 1704067200,
      };
      const encoded = encode(original);
      const decoded = decode(encoded);

      expect(decoded).not.toBeNull();
      expect(decoded?.s).toBe(original.s);
      expect(decoded?.t).toBe(original.t);
    });

    it('should return null for invalid base64', () => {
      const decoded = decode('not-valid-base64!!!');
      expect(decoded).toBeNull();
    });

    it('should return null for malformed JSON', () => {
      // Valid base64 but not JSON
      const encoded = btoa('not json')
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/, '');
      const decoded = decode(encoded);
      expect(decoded).toBeNull();
    });

    it('should return null for missing fields', () => {
      // Missing 's' field
      const partialPayload = { t: 1704067200 };
      const encoded = btoa(JSON.stringify(partialPayload))
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/, '');
      const decoded = decode(encoded);
      expect(decoded).toBeNull();
    });

    it('should return null for invalid UUID format in session_id', () => {
      const payload = { s: 'not-a-uuid', t: 1704067200 };
      const encoded = btoa(JSON.stringify(payload))
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/, '');
      const decoded = decode(encoded);
      expect(decoded).toBeNull();
    });
  });

  describe('decorateUrl', () => {
    let linker: CrossDomainLinker;

    beforeEach(() => {
      linker = new CrossDomainLinker({
        domains: ['blog.example.com', 'shop.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });
      linker.setIdGetters(
        () => '019012ab-cdef-7890-abcd-ef1234567890'
      );
    });

    it('should append _nf param to URL', () => {
      const decorated = linker.decorateUrl('https://blog.example.com/article');
      expect(decorated).toContain('_nf=');
      expect(decorated).toMatch(/https:\/\/blog\.example\.com\/article\?_nf=/);
    });

    it('should preserve existing query params', () => {
      const decorated = linker.decorateUrl('https://blog.example.com/article?page=2&sort=date');
      expect(decorated).toContain('page=2');
      expect(decorated).toContain('sort=date');
      expect(decorated).toContain('_nf=');
    });

    it('should handle URLs with hash', () => {
      const decorated = linker.decorateUrl('https://blog.example.com/article#section');
      expect(decorated).toContain('_nf=');
      expect(decorated).toContain('#section');
      // Hash should come after query params
      expect(decorated).toMatch(/_nf=[^#]+#section/);
    });

    it('should handle relative URLs', () => {
      vi.stubGlobal('location', { origin: 'https://www.example.com', href: 'https://www.example.com/' });
      const decorated = linker.decorateUrl('/page');
      // Should not decorate same-origin relative URLs
      expect(decorated).toBe('/page');
    });

    it('should return original URL if no IDs available', () => {
      linker.setIdGetters(() => '');
      const decorated = linker.decorateUrl('https://blog.example.com/article');
      expect(decorated).toBe('https://blog.example.com/article');
    });
  });

  describe('readParam', () => {
    beforeEach(() => {
      vi.useFakeTimers();
      vi.setSystemTime(new Date('2024-01-01T12:00:00.000Z'));
    });

    afterEach(() => {
      vi.useRealTimers();
      vi.unstubAllGlobals();
    });

    it('should read _nf param from window.location', () => {
      const payload: CrossDomainPayload = {
        s: '019012ab-cdef-7890-abcd-ef1234567890',
        t: Math.floor(Date.now() / 1000),
      };
      const encoded = encode(payload);

      vi.stubGlobal('location', {
        search: `?_nf=${encoded}`,
        href: `https://example.com?_nf=${encoded}`,
      });

      const result = CrossDomainLinker.readParam(120);
      expect(result).not.toBeNull();
      expect(result?.s).toBe(payload.s);
    });

    it('should return null if no _nf param', () => {
      vi.stubGlobal('location', {
        search: '?page=1',
        href: 'https://example.com?page=1',
      });

      const result = CrossDomainLinker.readParam(120);
      expect(result).toBeNull();
    });

    it('should return null for expired timestamp', () => {
      const payload: CrossDomainPayload = {
        s: '019012ab-cdef-7890-abcd-ef1234567890',
        t: Math.floor(Date.now() / 1000) - 300, // 5 minutes ago
      };
      const encoded = encode(payload);

      vi.stubGlobal('location', {
        search: `?_nf=${encoded}`,
        href: `https://example.com?_nf=${encoded}`,
      });

      const result = CrossDomainLinker.readParam(120); // 2 min expiry
      expect(result).toBeNull();
    });

    it('should return null for future timestamp (>60s)', () => {
      const payload: CrossDomainPayload = {
        s: '019012ab-cdef-7890-abcd-ef1234567890',
        t: Math.floor(Date.now() / 1000) + 120, // 2 minutes in future
      };
      const encoded = encode(payload);

      vi.stubGlobal('location', {
        search: `?_nf=${encoded}`,
        href: `https://example.com?_nf=${encoded}`,
      });

      const result = CrossDomainLinker.readParam(120);
      expect(result).toBeNull();
    });

    it('should accept timestamp within clock skew tolerance (60s future)', () => {
      const payload: CrossDomainPayload = {
        s: '019012ab-cdef-7890-abcd-ef1234567890',
        t: Math.floor(Date.now() / 1000) + 30, // 30 seconds in future (within tolerance)
      };
      const encoded = encode(payload);

      vi.stubGlobal('location', {
        search: `?_nf=${encoded}`,
        href: `https://example.com?_nf=${encoded}`,
      });

      const result = CrossDomainLinker.readParam(120);
      expect(result).not.toBeNull();
    });
  });

  describe('stripParam', () => {
    beforeEach(() => {
      vi.stubGlobal('history', {
        state: {},
        replaceState: vi.fn(),
      });
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    it('should remove _nf from URL', () => {
      vi.stubGlobal('location', {
        href: 'https://example.com/page?_nf=abc123',
        pathname: '/page',
        search: '?_nf=abc123',
        hash: '',
      });

      CrossDomainLinker.stripParam();

      expect(window.history.replaceState).toHaveBeenCalledWith(
        {},
        '',
        '/page'
      );
    });

    it('should preserve other query params', () => {
      vi.stubGlobal('location', {
        href: 'https://example.com/page?page=2&_nf=abc123&sort=date',
        pathname: '/page',
        search: '?page=2&_nf=abc123&sort=date',
        hash: '',
      });

      CrossDomainLinker.stripParam();

      expect(window.history.replaceState).toHaveBeenCalledWith(
        {},
        '',
        '/page?page=2&sort=date'
      );
    });

    it('should preserve hash', () => {
      vi.stubGlobal('location', {
        href: 'https://example.com/page?_nf=abc123#section',
        pathname: '/page',
        search: '?_nf=abc123',
        hash: '#section',
      });

      CrossDomainLinker.stripParam();

      expect(window.history.replaceState).toHaveBeenCalledWith(
        {},
        '',
        '/page#section'
      );
    });

    it('should do nothing if no _nf param', () => {
      vi.stubGlobal('location', {
        href: 'https://example.com/page?other=param',
        pathname: '/page',
        search: '?other=param',
        hash: '',
      });

      CrossDomainLinker.stripParam();

      expect(window.history.replaceState).not.toHaveBeenCalled();
    });
  });

  describe('shouldDecorate', () => {
    let linker: CrossDomainLinker;

    beforeEach(() => {
      vi.stubGlobal('location', {
        hostname: 'www.example.com',
        origin: 'https://www.example.com',
        href: 'https://www.example.com/',
      });

      linker = new CrossDomainLinker({
        domains: ['blog.example.com', 'shop.example.com', 'example.io'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    it('should return true for configured domains', () => {
      expect(linker.shouldDecorate('https://blog.example.com/article')).toBe(true);
      expect(linker.shouldDecorate('https://shop.example.com/product')).toBe(true);
      expect(linker.shouldDecorate('https://example.io/page')).toBe(true);
    });

    it('should return false for same-origin', () => {
      expect(linker.shouldDecorate('https://www.example.com/page')).toBe(false);
      expect(linker.shouldDecorate('/relative/path')).toBe(false);
    });

    it('should return false for unconfigured domains', () => {
      expect(linker.shouldDecorate('https://other-site.com/page')).toBe(false);
      expect(linker.shouldDecorate('https://google.com')).toBe(false);
    });

    it('should normalize www. prefix', () => {
      // Current hostname is www.example.com
      // Configured domain is blog.example.com (no www)
      expect(linker.shouldDecorate('https://www.blog.example.com/article')).toBe(true);
    });

    it('should match subdomains of configured domains', () => {
      // example.io is configured, so sub.example.io should match
      expect(linker.shouldDecorate('https://sub.example.io/page')).toBe(true);
    });
  });

  describe('click interception', () => {
    let linker: CrossDomainLinker;
    let mockDocument: { addEventListener: ReturnType<typeof vi.fn>; removeEventListener: ReturnType<typeof vi.fn> };

    beforeEach(() => {
      vi.stubGlobal('location', {
        hostname: 'www.example.com',
        origin: 'https://www.example.com',
        href: 'https://www.example.com/',
      });

      mockDocument = {
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      };
      vi.stubGlobal('document', mockDocument);

      linker = new CrossDomainLinker({
        domains: ['blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });
      linker.setIdGetters(
        () => '019012ab-cdef-7890-abcd-ef1234567890'
      );
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    it('should add click listener on start', () => {
      linker.start();
      expect(mockDocument.addEventListener).toHaveBeenCalledWith(
        'click',
        expect.any(Function),
        true // capture phase
      );
    });

    it('should remove click listener on stop', () => {
      linker.start();
      linker.stop();
      expect(mockDocument.removeEventListener).toHaveBeenCalledWith(
        'click',
        expect.any(Function),
        true
      );
    });

    it('should add submit listener on start', () => {
      linker.start();
      expect(mockDocument.addEventListener).toHaveBeenCalledWith(
        'submit',
        expect.any(Function),
        true
      );
    });

    it('should decorate <a> href on click', () => {
      // Restore real document for this test
      vi.unstubAllGlobals();
      vi.stubGlobal('location', {
        hostname: 'www.example.com',
        origin: 'https://www.example.com',
        href: 'https://www.example.com/',
      });

      const linkerReal = new CrossDomainLinker({
        domains: ['blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });
      linkerReal.setIdGetters(
        () => '019012ab-cdef-7890-abcd-ef1234567890'
      );
      linkerReal.start();

      // Create a link element
      const link = document.createElement('a');
      link.href = 'https://blog.example.com/article';
      document.body.appendChild(link);

      // Simulate click
      const event = new MouseEvent('click', { bubbles: true });
      link.dispatchEvent(event);

      // Check that href was decorated
      expect(link.href).toContain('_nf=');

      // Cleanup
      document.body.removeChild(link);
      linkerReal.stop();
    });

    it('should not decorate same-origin links', () => {
      vi.unstubAllGlobals();
      vi.stubGlobal('location', {
        hostname: 'www.example.com',
        origin: 'https://www.example.com',
        href: 'https://www.example.com/',
      });

      const linkerReal = new CrossDomainLinker({
        domains: ['blog.example.com'],
        expiry: 120,
        debug: false,
        paramName: '_nf',
      });
      linkerReal.setIdGetters(
        () => '019012ab-cdef-7890-abcd-ef1234567890'
      );
      linkerReal.start();

      const link = document.createElement('a');
      link.href = 'https://www.example.com/other-page';
      document.body.appendChild(link);

      const event = new MouseEvent('click', { bubbles: true });
      link.dispatchEvent(event);

      // Should NOT contain _nf
      expect(link.href).not.toContain('_nf=');

      document.body.removeChild(link);
      linkerReal.stop();
    });
  });
});
