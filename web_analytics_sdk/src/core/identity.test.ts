/**
 * Tests for verified contact identity (W3)
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { SessionManager } from './session';
import { Storage, TabStorage } from '../storage/storage';
import type { InternalConfig } from '../types';

vi.mock('../utils/uuid', () => ({
  generateUUIDv4: vi.fn(() => 'mock-uuid-v4'),
  generateUUIDv7: vi.fn(() => 'mock-uuid-v7'),
  generateTabId: vi.fn(() => 123456),
}));

vi.mock('../utils/utm', () => ({
  parseUTMParams: vi.fn(() => ({
    source: null, medium: null, campaign: null, term: null, content: null, id: null,
  })),
}));

const createMockStorage = () => {
  const store: Record<string, string> = {};
  return {
    getItem: vi.fn((k: string) => store[k] ?? null),
    setItem: vi.fn((k: string, v: string) => { store[k] = v; }),
    removeItem: vi.fn((k: string) => { delete store[k]; }),
    clear: vi.fn(),
    key: vi.fn((i: number) => Object.keys(store)[i] ?? null),
    get length() { return Object.keys(store).length; },
    _store: store,
  };
};

describe('SessionManager - verified identity', () => {
  let sessionManager: SessionManager;
  let storage: Storage;
  let mockLocalStorage: ReturnType<typeof createMockStorage>;
  let config: InternalConfig;

  beforeEach(() => {
    mockLocalStorage = createMockStorage();
    vi.stubGlobal('localStorage', mockLocalStorage);
    vi.stubGlobal('sessionStorage', createMockStorage());
    vi.stubGlobal('location', { href: 'https://example.com/p', pathname: '/p' });

    storage = new Storage();
    config = {
      workspace_id: 'ws_123',
      endpoint: 'https://api.example.com',
      debug: false,
      sessionTimeout: 30 * 60 * 1000,
      heartbeatInterval: 10000,
      adClickIds: [],
      trackSPA: true,
      trackScroll: true,
      trackClicks: false,
      heartbeatTiers: [{ after: 0, desktopInterval: 10000, mobileInterval: 7000 }],
      heartbeatMaxDuration: 10 * 60 * 1000,
      resetHeartbeatOnNavigation: false,
      crossDomains: [],
      crossDomainExpiry: 120,
      crossDomainStripParams: true,
      crossDomainParam: '_nf',
    } as InternalConfig;
    sessionManager = new SessionManager(storage, new TabStorage(), config);
    sessionManager.getOrCreateSession();
  });

  afterEach(() => vi.unstubAllGlobals());

  it('stores an email with its signature, never the email alone', () => {
    // The server discards an unsigned address, so shipping one would look like
    // identification while silently doing nothing.
    sessionManager.setIdentity({ email: 'Alice@Example.com', hmac: 'abc123' });
    expect(sessionManager.getIdentity()).toEqual({ email: 'Alice@Example.com', hmac: 'abc123' });
  });

  it('keeps the address exactly as given', () => {
    // The customer signed the raw string; lowercasing it here would invalidate
    // every HMAC they mint. Normalization is the server's job, after verifying.
    sessionManager.setIdentity({ email: 'Alice@Example.com', hmac: 'abc123' });
    expect(sessionManager.getIdentity()?.email).toBe('Alice@Example.com');
  });

  it('persists across a reload and a session rollover', () => {
    sessionManager.setIdentity({ email: 'a@b.com', hmac: 'h' });
    const revived = new SessionManager(storage, new TabStorage(), config);
    revived.getOrCreateSession();
    expect(revived.getIdentity()).toEqual({ email: 'a@b.com', hmac: 'h' });
  });

  it('accepts an opaque token from an email-click link', () => {
    sessionManager.setIdentity({ token: 'deadbeefcafe' });
    expect(sessionManager.getIdentity()).toEqual({ token: 'deadbeefcafe' });
  });

  it('clearIdentity() stops future beats carrying it', () => {
    sessionManager.setIdentity({ email: 'a@b.com', hmac: 'h' });
    sessionManager.clearIdentity();
    expect(sessionManager.getIdentity()).toBeNull();
    expect(mockLocalStorage._store['nf_identity']).toBeUndefined();
  });

  it('reset() clears the identity', () => {
    sessionManager.setIdentity({ email: 'a@b.com', hmac: 'h' });
    sessionManager.reset();
    expect(sessionManager.getIdentity()).toBeNull();
  });

  it('a resumed session takes its identity from storage, not the session blob', () => {
    // touch() writes the whole in-memory session back on every beat, so a tab
    // still holding a pre-identification copy would clobber the blob with
    // identity:null. Reading the durable key on resume is what stops an
    // identified visitor silently going anonymous on their next page load.
    sessionManager.setIdentity({ email: 'a@b.com', hmac: 'h' })

    const blob = JSON.parse(mockLocalStorage._store['nf_session'])
    blob.identity = null
    mockLocalStorage._store['nf_session'] = JSON.stringify(blob)

    const resumed = new SessionManager(storage, new TabStorage(), config)
    resumed.getOrCreateSession()
    expect(resumed.getIdentity()).toEqual({ email: 'a@b.com', hmac: 'h' })
  })

  it('purges a legacy nf_user_id left by an older build', () => {
    // That key held an opaque customer string the server no longer accepts;
    // leaving it would be dead state that looks meaningful in devtools.
    mockLocalStorage._store['nf_user_id'] = JSON.stringify('legacy-user-42');
    new SessionManager(storage, new TabStorage(), config).getOrCreateSession();
    expect(mockLocalStorage._store['nf_user_id']).toBeUndefined();
  });

  it('rejects an email without a signature', () => {
    expect(() => sessionManager.setIdentity({ email: 'a@b.com' } as never)).toThrow(
      'Identity requires either a token or an email with its hmac'
    );
  });

  it('rejects an address that fits in 255 characters but not in 255 bytes', () => {
    // The server's bound is Go's len(), which counts UTF-8 bytes, so an
    // SMTPUTF8/IDN address of multibyte characters clears a code-unit check
    // here and is dropped server-side — where the beat still succeeds and the
    // contact is simply never attached. Throwing is the only loud outcome.
    const email = `${'é'.repeat(130)}@b.com`; // 136 UTF-16 units, 266 UTF-8 bytes
    expect(email.length).toBeLessThanOrEqual(255);
    expect(() => sessionManager.setIdentity({ email, hmac: 'h' })).toThrow(
      'Identity email must be 255 bytes or less'
    );
  });

  it('accepts a 255-byte address and rejects a 256-byte one', () => {
    // The common path is all-ASCII, where bytes and characters coincide: the
    // byte count must not shift the boundary that mailboxes actually sit on.
    const at255 = `${'a'.repeat(249)}@b.com`;
    const at256 = `${'a'.repeat(250)}@b.com`;
    expect(() => sessionManager.setIdentity({ email: at255, hmac: 'h' })).not.toThrow();
    expect(sessionManager.getIdentity()?.email).toBe(at255);
    expect(() => sessionManager.setIdentity({ email: at256, hmac: 'h' })).toThrow(
      'Identity email must be 255 bytes or less'
    );
  });

  it('still counts bytes on a page that has stripped TextEncoder', () => {
    // Consent tools and hardened embeds delete globals; the measurement has to
    // survive that rather than blowing up a call that used to work.
    vi.stubGlobal('TextEncoder', undefined);
    expect(() =>
      sessionManager.setIdentity({ email: `${'é'.repeat(130)}@b.com`, hmac: 'h' })
    ).toThrow('Identity email must be 255 bytes or less');
    expect(() => sessionManager.setIdentity({ email: 'a@b.com', hmac: 'h' })).not.toThrow();
  });
});
