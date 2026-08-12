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

// Mirrors the server's WebTrackMaxIdentifyTokenLength (internal/domain/web_analytics.go).
// That bound is derived, not chosen: a token is the hex of (12-byte GCM nonce ‖
// ciphertext ‖ 16-byte tag) over a JSON payload carrying the address, and
// encoding/json can spend six characters on a single address byte, so
// 2 * (28 + 29 + 6 * 255) = 3174. Real mints land well below it — a plain
// 255-character address produces 624 characters, an address of all "&" produces
// 3054 — but every one of those is over the 512 this file used to assert, which
// is the silent identity loss these boundary tests exist to catch.
const MAX_TOKEN_BYTES = 3174;

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

  it('accepts a multibyte address the contacts table can store', () => {
    // Deliberately inverted from the byte-counting version of this test. The
    // bound mirrors contacts.email VARCHAR(255), and Postgres counts VARCHAR in
    // CHARACTERS — so this address is a perfectly storable contact, and the
    // server now resolves it. Refusing it here would be the client reproducing
    // the very silent identity loss these bounds exist to prevent.
    const email = `${'é'.repeat(130)}@b.com`; // 136 characters, 266 UTF-8 bytes
    expect([...email].length).toBeLessThanOrEqual(255);
    expect(() => sessionManager.setIdentity({ email, hmac: 'h' })).not.toThrow();
    expect(sessionManager.getIdentity()?.email).toBe(email);
  });

  it('rejects an address longer than the column, counted in characters', () => {
    // The bound must still bound: 256 characters cannot be stored, multibyte or
    // not, so both of these are refused for the same reason.
    const ascii256 = `${'a'.repeat(250)}@b.com`;
    const multibyte256 = `${'é'.repeat(250)}@b.com`;
    expect([...ascii256].length).toBe(256);
    expect([...multibyte256].length).toBe(256);
    expect(() => sessionManager.setIdentity({ email: ascii256, hmac: 'h' })).toThrow(
      'Identity email must be 255 characters or less'
    );
    expect(() => sessionManager.setIdentity({ email: multibyte256, hmac: 'h' })).toThrow(
      'Identity email must be 255 characters or less'
    );
  });

  it('accepts a 255-character address and rejects a 256-character one', () => {
    // The common path is all-ASCII, where characters and bytes coincide: the
    // unit change must not shift the boundary real mailboxes sit on.
    const at255 = `${'a'.repeat(249)}@b.com`;
    const at256 = `${'a'.repeat(250)}@b.com`;
    expect(() => sessionManager.setIdentity({ email: at255, hmac: 'h' })).not.toThrow();
    expect(sessionManager.getIdentity()?.email).toBe(at255);
    expect(() => sessionManager.setIdentity({ email: at256, hmac: 'h' })).toThrow(
      'Identity email must be 255 characters or less'
    );
  });

  it('counts an astral character once, as the server does', () => {
    // String#length would report 2 per emoji (UTF-16 surrogate pair) and reject
    // an address the server counts as half that length.
    const email = `${'𝔞'.repeat(200)}@b.com`; // 206 code points, 400 UTF-16 units
    expect(email.length).toBeGreaterThan(255);
    expect([...email].length).toBeLessThanOrEqual(255);
    expect(() => sessionManager.setIdentity({ email, hmac: 'h' })).not.toThrow();
  });

  it('rejects an hmac the server would refuse to read', () => {
    // ResolveWebIdentity bounds the hmac BEFORE it compares it, and an
    // over-long one costs the identity rather than the beat: the beat still
    // succeeds, the visit is still recorded, the contact is just never
    // attached. Only the caller of identify() can fix that, so it throws.
    expect(() =>
      sessionManager.setIdentity({ email: 'a@b.com', hmac: 'a'.repeat(65) })
    ).toThrow('Identity hmac must be 64 bytes or less');
  });

  it('accepts a 64-byte hmac and rejects a 65-byte one', () => {
    // 64 is the exact length of the credential the server computes — a
    // hex-encoded SHA-256 — not a generous ceiling, so a boundary off by one
    // rejects every signature the customer can legitimately mint.
    const at64 = 'a'.repeat(64);
    expect(() => sessionManager.setIdentity({ email: 'a@b.com', hmac: at64 })).not.toThrow();
    expect(sessionManager.getIdentity()).toEqual({ email: 'a@b.com', hmac: at64 });
    expect(() =>
      sessionManager.setIdentity({ email: 'a@b.com', hmac: 'a'.repeat(65) })
    ).toThrow('Identity hmac must be 64 bytes or less');
  });

  it('drops an over-long nf_id token instead of throwing', () => {
    // The only caller that supplies a token is the nf_id URL read on page
    // load, so the value is a stranger's, not a developer's: anyone can link
    // to the customer's site with ?nf_id=<megabytes>. Throwing there rejects
    // init()'s promise before the sender is even constructed, which turns a
    // crafted link into a dead SDK and an unhandled rejection in the host
    // page. There is no developer mistake to report either — the server
    // refuses to MINT a token over this bound, so no real link carries one.
    expect(() => sessionManager.setIdentity({ token: 'a'.repeat(MAX_TOKEN_BYTES + 1) })).not.toThrow();
    expect(sessionManager.getIdentity()).toBeNull();
    expect(mockLocalStorage._store['nf_identity']).toBeUndefined();
  });

  it('accepts a token at the server bound and drops a one-over one without losing the stored identity', () => {
    // The bound has to be the server's exactly. Lower, and a token the server
    // would have read is dropped here instead — the same silent no-attach, just
    // moved client-side. And dropping the over-long one must not clear what is
    // already stored: a crafted link would otherwise log a visitor out of their
    // own analytics identity.
    const atBound = 'a'.repeat(MAX_TOKEN_BYTES);
    sessionManager.setIdentity({ token: atBound });
    expect(sessionManager.getIdentity()).toEqual({ token: atBound });

    sessionManager.setIdentity({ token: 'a'.repeat(MAX_TOKEN_BYTES + 1) });
    expect(sessionManager.getIdentity()).toEqual({ token: atBound });
  });

  it('accepts the token an ordinary 255-character address mints', () => {
    // 624 characters: hex of the nonce, the tag and a JSON payload of
    // {"e":"<255 chars>","x":<10 digits>,"v":1}. Nothing about it is exotic —
    // it is what BuildWebIdentifyToken emits for the longest address the
    // contacts table can hold — yet it sat 112 characters over the bound this
    // file used to assert, so every such link identified nobody.
    const minted = 'a'.repeat(624);
    sessionManager.setIdentity({ token: minted });
    expect(sessionManager.getIdentity()).toEqual({ token: minted });
  });

  it('measures the token in bytes, like the server', () => {
    // The server's gate is Go's len() over the raw parameter, so the unit is
    // bytes on both ends. No genuine token can tell the difference — a mint is
    // hex, where a character is a byte — but a crafted nf_id can, and one the
    // server will refuse to read must not be stored and re-sent on every beat.
    const multibyte = 'é'.repeat(MAX_TOKEN_BYTES / 2 + 1); // 1588 characters, 3176 bytes
    expect(multibyte.length).toBeLessThanOrEqual(MAX_TOKEN_BYTES);
    sessionManager.setIdentity({ token: multibyte });
    expect(sessionManager.getIdentity()).toBeNull();
  });

  it('refuses a stored credential the server would not read, and lets the visitor recover', () => {
    // A visitor handed an oversized token before the bound existed has it
    // sitting in localStorage. Nothing else ever revisits the stored value, so
    // without a check here every beat would keep carrying a credential the
    // server refuses to read — permanently, across SDK upgrades.
    mockLocalStorage._store['nf_identity'] = JSON.stringify({ token: 'a'.repeat(MAX_TOKEN_BYTES + 1) });

    const revived = new SessionManager(storage, new TabStorage(), config);
    revived.getOrCreateSession();

    expect(revived.getIdentity()).toBeNull();
    // Dropped from storage, not merely ignored, so the next page load starts clean.
    expect(mockLocalStorage._store['nf_identity']).toBeUndefined();
  });

  it('keeps a stored credential that is exactly at the bound', () => {
    // The recovery path must not become a slow erasure of valid identities.
    const atBound = 'a'.repeat(MAX_TOKEN_BYTES);
    mockLocalStorage._store['nf_identity'] = JSON.stringify({ token: atBound });

    const revived = new SessionManager(storage, new TabStorage(), config);
    revived.getOrCreateSession();

    expect(revived.getIdentity()).toEqual({ token: atBound });
  });

  it('refuses a stored hmac pair that overshoots the signature length', () => {
    mockLocalStorage._store['nf_identity'] = JSON.stringify({
      email: 'a@b.com',
      hmac: 'f'.repeat(65),
    });

    const revived = new SessionManager(storage, new TabStorage(), config);
    revived.getOrCreateSession();

    expect(revived.getIdentity()).toBeNull();
  });

  it('still measures correctly on a page that has stripped TextEncoder', () => {
    // Consent tools and hardened embeds delete globals; the measurement has to
    // survive that rather than blowing up a call that used to work. The address
    // no longer needs TextEncoder at all (characters, not bytes), but the hmac
    // and token bounds still do, so the fallback has to hold for them.
    vi.stubGlobal('TextEncoder', undefined);
    expect(() =>
      sessionManager.setIdentity({ email: `${'a'.repeat(250)}@b.com`, hmac: 'h' })
    ).toThrow('Identity email must be 255 characters or less');
    expect(() => sessionManager.setIdentity({ email: 'a@b.com', hmac: 'h' })).not.toThrow();
    expect(() =>
      sessionManager.setIdentity({ email: 'a@b.com', hmac: 'f'.repeat(65) })
    ).toThrow('Identity hmac must be 64 bytes or less');
  });
});
