import * as UAParser from 'ua-parser-js';

/**
 * Notifuse Analytics SDK Types
 * V3 Session Payload Architecture
 */
// Goal
/**
 * Goal types, mirroring domain.ValidGoalTypes on the server.
 *
 * Keep this list in step with ValidGoalTypes in internal/domain/custom_event.go:
 * a type this SDK allows but the server does not recognise is silently recorded
 * as 'other', which is a confusing way to find out.
 */
const VALID_GOAL_TYPES = [
    'purchase',
    'subscription',
    'lead',
    'signup',
    'booking',
    'trial',
    'other',
];

/**
 * Storage module with localStorage + memory fallback
 * Handles Safari Private Mode gracefully
 */
const STORAGE_PREFIX = 'nf_';
class Storage {
    constructor() {
        this.useMemory = false;
        this.memory = new Map();
        this.testStorage();
    }
    /**
     * Test localStorage availability
     * Safari Private Mode throws QuotaExceededError even on empty storage
     */
    testStorage() {
        try {
            const testKey = STORAGE_PREFIX + 'test';
            localStorage.setItem(testKey, 'test');
            localStorage.removeItem(testKey);
        }
        catch {
            this.useMemory = true;
        }
    }
    /**
     * Get a value from storage
     */
    get(key) {
        const fullKey = STORAGE_PREFIX + key;
        if (this.useMemory) {
            const value = this.memory.get(fullKey);
            if (value === undefined)
                return null;
            try {
                return JSON.parse(value);
            }
            catch {
                return null;
            }
        }
        try {
            const value = localStorage.getItem(fullKey);
            if (value === null)
                return null;
            return JSON.parse(value);
        }
        catch {
            // Fallback to memory if localStorage fails mid-session
            this.useMemory = true;
            return this.memory.get(fullKey) ? JSON.parse(this.memory.get(fullKey)) : null;
        }
    }
    /**
     * Set a value in storage
     */
    set(key, value) {
        const fullKey = STORAGE_PREFIX + key;
        const data = JSON.stringify(value);
        if (this.useMemory) {
            this.memory.set(fullKey, data);
            return;
        }
        try {
            localStorage.setItem(fullKey, data);
        }
        catch {
            // Quota exceeded mid-session - switch to memory
            this.useMemory = true;
            this.memory.set(fullKey, data);
        }
    }
    /**
     * Remove a value from storage
     */
    remove(key) {
        const fullKey = STORAGE_PREFIX + key;
        if (this.useMemory) {
            this.memory.delete(fullKey);
            return;
        }
        try {
            localStorage.removeItem(fullKey);
        }
        catch {
            this.memory.delete(fullKey);
        }
    }
    /**
     * Clear all SDK storage
     */
    clear() {
        if (this.useMemory) {
            this.memory.clear();
            return;
        }
        try {
            // Only remove nf_ prefixed keys
            const keysToRemove = [];
            for (let i = 0; i < localStorage.length; i++) {
                const key = localStorage.key(i);
                if (key?.startsWith(STORAGE_PREFIX)) {
                    keysToRemove.push(key);
                }
            }
            keysToRemove.forEach(key => localStorage.removeItem(key));
        }
        catch {
            this.memory.clear();
        }
    }
    /**
     * Check if using memory fallback
     */
    isUsingMemory() {
        return this.useMemory;
    }
}
// Session storage for tab-specific data
class TabStorage {
    constructor() {
        this.memory = new Map();
        this.useMemory = false;
        this.testStorage();
    }
    testStorage() {
        try {
            const testKey = STORAGE_PREFIX + 'test';
            sessionStorage.setItem(testKey, 'test');
            sessionStorage.removeItem(testKey);
        }
        catch {
            this.useMemory = true;
        }
    }
    get(key) {
        const fullKey = STORAGE_PREFIX + key;
        if (this.useMemory) {
            const value = this.memory.get(fullKey);
            if (value === undefined)
                return null;
            try {
                return JSON.parse(value);
            }
            catch {
                return null;
            }
        }
        try {
            const value = sessionStorage.getItem(fullKey);
            if (value === null)
                return null;
            return JSON.parse(value);
        }
        catch {
            this.useMemory = true;
            return null;
        }
    }
    set(key, value) {
        const fullKey = STORAGE_PREFIX + key;
        const data = JSON.stringify(value);
        if (this.useMemory) {
            this.memory.set(fullKey, data);
            return;
        }
        try {
            sessionStorage.setItem(fullKey, data);
        }
        catch {
            this.useMemory = true;
            this.memory.set(fullKey, data);
        }
    }
}
// Storage keys
const STORAGE_KEYS = {
    SESSION: 'session',
    PENDING_QUEUE: 'pending',
    TAB_ID: 'tab_id',
    DIMENSIONS: 'dimensions',
    IDENTITY: 'identity',
    /** Written by builds before verified identity; purged on init. */
    LEGACY_USER_ID: 'user_id',
};

/**
 * UUID generation utilities
 * Uses crypto APIs for secure random generation (available in all ES2017+ browsers)
 */
/**
 * Generate a UUIDv4
 * Uses native crypto.randomUUID() when available (2-3x faster),
 * falls back to crypto.getRandomValues() for older browsers
 */
/**
 * Generate a UUIDv7 (time-sortable)
 * Format: timestamp (48 bits) + version (4 bits) + random (12 bits) + variant (2 bits) + random (62 bits)
 */
function generateUUIDv7() {
    const timestamp = Date.now();
    // Convert timestamp to hex (48 bits = 12 hex chars)
    const timestampHex = timestamp.toString(16).padStart(12, '0');
    // Generate random bytes using crypto API (available in all ES2017+ browsers)
    const randomBytes = new Uint8Array(10);
    crypto.getRandomValues(randomBytes);
    const randomHex = Array.from(randomBytes, (b) => b.toString(16).padStart(2, '0')).join('');
    // Build UUIDv7
    // Format: xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx
    return [
        timestampHex.slice(0, 8), // First 8 hex chars of timestamp
        timestampHex.slice(8, 12), // Next 4 hex chars
        '7' + randomHex.slice(0, 3), // Version 7 + 3 random hex
        ((parseInt(randomHex.slice(3, 4), 16) & 0x3) | 0x8).toString(16) +
            randomHex.slice(4, 7), // Variant + 3 random hex
        randomHex.slice(7, 19), // 12 random hex chars
    ].join('-');
}
/**
 * Random identifier for one browser tab, as a JS-safe integer.
 *
 * Lands in a BIGINT column that forms part of the web_pages and web_goals
 * primary keys, so it only has to be unique among one session's tabs — a UUID
 * would add 16 bytes to the highest-volume partitioned table and widen its PK
 * index for uniqueness nobody needs. 53 bits keeps the value an exact float64
 * integer, so it survives JSON round-tripping without precision loss.
 */
function generateTabId() {
    if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
        const buf = new Uint32Array(2);
        crypto.getRandomValues(buf);
        // 21 high bits + 32 low bits = 53.
        return (buf[0] % 0x200000) * 0x100000000 + buf[1] + 1;
    }
    return Math.floor(Math.random() * Number.MAX_SAFE_INTEGER) + 1;
}

/**
 * UTM and Ad Click ID parsing utilities
 */
// Default ad click ID parameters to track
const DEFAULT_AD_CLICK_IDS = [
    'gclid', // Google Ads
    'fbclid', // Facebook/Meta Ads
    'msclkid', // Microsoft Ads
    'dclid', // DoubleClick
    'twclid', // Twitter/X Ads
    'ttclid', // TikTok Ads
    'li_fat_id', // LinkedIn Ads
    'wbraid', // Google Ads (iOS)
    'gbraid', // Google Ads (cross-device)
    'epik', // Pinterest Ads
    'ScCid', // Snapchat Ads (canonical spelling; matched case-insensitively)
    'rdt_cid', // Reddit Ads
    'qclid', // Quora Ads
];
/**
 * Parse UTM parameters from URL
 */
function parseUTMParams(url, adClickIds = DEFAULT_AD_CLICK_IDS) {
    const params = new URL(url).searchParams;
    // Ad networks are inconsistent about the casing of their click ids (Snapchat
    // documents ScCid, plenty of links carry sccid) and URLSearchParams.get is
    // case-sensitive, so an exact lookup silently misses them.
    const byLowerKey = new Map();
    for (const [key, value] of params) {
        const lower = key.toLowerCase();
        if (!byLowerKey.has(lower))
            byLowerKey.set(lower, value);
    }
    // Find ad click ID
    let utm_id = null;
    let utm_id_from = null;
    // Iterating adClickIds rather than the URL's parameters is deliberate: it is
    // what keeps priority OUR order instead of whatever order the network wrote
    // them in. gclid must still win over fbclid when both are present.
    for (const param of adClickIds) {
        const value = byLowerKey.get(param.toLowerCase());
        if (value) {
            utm_id = value;
            // The canonical spelling, not the one seen in the URL: the seeded
            // attribution rules compare utm_id_from with an exact equality, so
            // reporting 'sccid' would attribute the click to nothing.
            utm_id_from = param;
            break; // Use first match
        }
    }
    return {
        source: params.get('utm_source'),
        medium: params.get('utm_medium'),
        campaign: params.get('utm_campaign'),
        term: params.get('utm_term'),
        content: params.get('utm_content'),
        id: utm_id,
        id_from: utm_id_from,
    };
}

/**
 * Session management
 * Handles session creation, persistence, and expiry
 */
const SDK_VERSION$1 = "39.0";
const CLOCK_SKEW_TOLERANCE$1 = 60; // seconds
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
function utf8Length(value) {
    if (typeof TextEncoder !== 'undefined') {
        return new TextEncoder().encode(value).length;
    }
    // for..of walks code points, so a surrogate pair arrives once as its combined
    // code point and is counted as the single 4-byte character it encodes to.
    let bytes = 0;
    for (const char of value) {
        const code = char.codePointAt(0);
        if (code <= 0x7f)
            bytes += 1;
        else if (code <= 0x7ff)
            bytes += 2;
        else if (code <= 0xffff)
            bytes += 3;
        else
            bytes += 4;
    }
    return bytes;
}
/**
 * Length of a string in Unicode code points.
 *
 * Not String#length, which counts UTF-16 units and so reports 2 for a single
 * astral character: the server counts code points, and a client bound that
 * disagrees rejects addresses the server would have accepted.
 */
function charLength(value) {
    // Spreading walks code points, pairing surrogates back into one character.
    return [...value].length;
}
// Mirrors of the server's ResolveWebIdentity gates (WebTrackMaxEmailLength,
// WebTrackMaxHMACLength, WebTrackMaxIdentifyTokenLength). A credential that
// overshoots any of them is discarded server-side in silence — the beat still
// succeeds and the visit is still recorded, the contact is just never attached
// — so the SDK refuses it up front instead of sending something it knows will
// be ignored.
// Counted in CHARACTERS, not bytes: the server compares this bound with
// utf8.RuneCountInString because it mirrors a VARCHAR(255) column, and Postgres
// counts VARCHAR in characters. Counting bytes here would reject an SMTPUTF8
// address of 134 characters and 256 bytes that the contacts table stores
// perfectly happily — the client refusing what the server would have accepted,
// which is the same silent identity loss these bounds exist to prevent, only
// one layer up.
const MAX_IDENTITY_EMAIL_CHARS = 255;
const MAX_IDENTITY_HMAC_BYTES = 64;
// Must equal WebTrackMaxIdentifyTokenLength exactly, and it is the one of the
// three that is derived rather than declared, so it moves when the address
// bound moves. Server-side it reads 2 * (28 + 29 + 6 * 255): a token is the hex
// of a 12-byte GCM nonce, the ciphertext and a 16-byte tag, over a JSON payload
// of {"e":"<address>","x":<expiry>,"v":1}, where encoding/json may spend six
// characters on one address byte.
//
// A LOWER value here is not the safe direction. Every gate in this file exists
// because the server drops an over-long credential in silence — the beat still
// succeeds, the visit is still recorded, the contact is simply never attached —
// and a bound below the server's reproduces exactly that outcome one layer up,
// on tokens the server would have read: at 512 the SDK discarded the mint of
// any address past ~199 characters, including the ordinary 624-character token
// of a 255-character address.
const MAX_IDENTITY_TOKEN_BYTES = 3174;
/**
 * Whether every credential in an identity is inside the bounds the server will
 * actually read.
 *
 * Shared by the accept path and the load path so the two cannot drift: a value
 * rejected on the way in must not become acceptable simply by having been
 * written to storage by an older build.
 */
function identityWithinBounds(identity) {
    if (identity.email && charLength(identity.email) > MAX_IDENTITY_EMAIL_CHARS)
        return false;
    if (identity.hmac && utf8Length(identity.hmac) > MAX_IDENTITY_HMAC_BYTES)
        return false;
    if (identity.token && utf8Length(identity.token) > MAX_IDENTITY_TOKEN_BYTES)
        return false;
    return true;
}
/**
 * The referring URL, unless it is a page of the site the visitor is already on.
 *
 * A session is minted with document.referrer, and one is minted whenever the
 * inactivity window has lapsed — rotating in place on a tab left open, or on the
 * visitor's next internal click. In both cases document.referrer is one of the
 * site's own pages, and recording it would replace the visit's real acquisition
 * source with the site itself: the referrers report lists your own domain, and a
 * session that ought to read as direct reads as a referral instead.
 *
 * Exact hostname match, which URL and location both give lowercased. Another
 * host of the same site (docs.acme.com -> www.acme.com) is a genuine referral
 * and is kept. The server applies the same rule to referrer_domain against
 * landing_domain, which is what covers visitors still running an older build.
 */
function externalReferrer() {
    const referrer = document.referrer;
    if (!referrer)
        return null;
    try {
        if (new URL(referrer).hostname === window.location.hostname)
            return null;
    }
    catch {
        // Kept rather than dropped: document.referrer is always an absolute URL, so
        // a parse failure here is an edge case in the parser, not evidence that the
        // referrer is internal — and dropping it would lose a real source.
    }
    return referrer;
}
class SessionManager {
    constructor(storage, tabStorage, config) {
        this.session = null;
        this.crossDomainInput = null;
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
    setCrossDomainInput(input) {
        this.crossDomainInput = input;
    }
    /**
     * Get or create session
     * Priority:
     * 1. Valid cross-domain input (from URL params)
     * 2. Valid existing session in localStorage
     * 3. Create new session
     */
    getOrCreateSession() {
        // Check cross-domain input first (highest priority)
        if (this.crossDomainInput && this.isValidCrossDomain()) {
            const session = this.createSessionFromCrossDomain();
            if (session) {
                return session;
            }
        }
        const stored = this.storage.get(STORAGE_KEYS.SESSION);
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
    isValidCrossDomain() {
        if (!this.crossDomainInput)
            return false;
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
        if (timestamp > now + CLOCK_SKEW_TOLERANCE$1) {
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
    createSessionFromCrossDomain() {
        if (!this.crossDomainInput)
            return null;
        const { sessionId } = this.crossDomainInput;
        const now = Date.now();
        const utm = parseUTMParams(window.location.href, this.config.adClickIds);
        const session = {
            id: sessionId,
            workspace_id: this.config.workspace_id,
            created_at: now,
            updated_at: now,
            last_active_at: now,
            focus_duration_ms: 0,
            total_duration_ms: 0,
            referrer: externalReferrer(),
            landing_page: window.location.href,
            utm: this.hasUTMValues(utm) ? utm : null,
            max_scroll_percent: 0,
            interaction_count: 0,
            sdk_version: SDK_VERSION$1,
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
    createSession() {
        const now = Date.now();
        const utm = parseUTMParams(window.location.href, this.config.adClickIds);
        const session = {
            id: generateUUIDv7(),
            workspace_id: this.config.workspace_id,
            created_at: now,
            updated_at: now,
            last_active_at: now,
            focus_duration_ms: 0,
            total_duration_ms: 0,
            referrer: externalReferrer(),
            landing_page: window.location.href,
            utm: this.hasUTMValues(utm) ? utm : null,
            max_scroll_percent: 0,
            interaction_count: 0,
            sdk_version: SDK_VERSION$1,
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
    isSessionExpired(session) {
        const now = Date.now();
        if (now - session.last_active_at > this.config.sessionTimeout)
            return true;
        return now - session.created_at > SESSION_MAX_AGE_MS;
    }
    /**
     * Check if UTM has any values
     */
    hasUTMValues(utm) {
        return Boolean(utm.source || utm.medium || utm.campaign || utm.term || utm.content || utm.id);
    }
    /**
     * Get current session
     */
    getSession() {
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
    touch() {
        if (!this.session)
            return false;
        if (this.isSessionExpired(this.session))
            return false;
        const now = Date.now();
        this.session.last_active_at = now;
        this.session.updated_at = now;
        this.saveSession();
        return true;
    }
    /**
     * Save session to storage
     */
    saveSession() {
        if (!this.session)
            return;
        this.storage.set(STORAGE_KEYS.SESSION, this.session);
    }
    /**
     * Get tab ID (unique per browser tab)
     */
    getTabId() {
        return this.tabId;
    }
    /**
     * Get or create tab ID
     */
    getOrCreateTabId() {
        const stored = this.tabStorage.get(STORAGE_KEYS.TAB_ID);
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
    getSessionId() {
        return this.session?.id || '';
    }
    // Custom Dimensions
    /**
     * Set a custom dimension (1-10)
     */
    setDimension(index, value) {
        if (index < 1 || index > 10) {
            throw new Error('Dimension index must be between 1 and 10');
        }
        if (typeof value !== 'string') {
            throw new Error('Dimension value must be a string');
        }
        if (value.length > 256) {
            throw new Error('Dimension value must be 256 characters or less');
        }
        if (!this.session)
            return;
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
    setDimensions(dimensions) {
        for (const [index, value] of Object.entries(dimensions)) {
            this.setDimension(Number(index), value);
        }
    }
    /**
     * Get a dimension value
     */
    getDimension(index) {
        if (!this.session)
            return null;
        return this.session.dimensions[index] || null;
    }
    /**
     * Clear all dimensions
     */
    clearDimensions() {
        if (!this.session)
            return;
        this.session.dimensions = {};
        this.saveDimensions();
        this.saveSession();
    }
    /**
     * Get all dimensions as payload fields
     */
    getDimensionsPayload() {
        if (!this.session)
            return {};
        const payload = {};
        for (const [index, value] of Object.entries(this.session.dimensions)) {
            payload[`custom_${index}`] = value;
        }
        return payload;
    }
    /**
     * Load dimensions from storage
     */
    loadDimensions() {
        return this.storage.get(STORAGE_KEYS.DIMENSIONS) || {};
    }
    /**
     * Save dimensions to storage
     */
    saveDimensions() {
        if (!this.session)
            return;
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
    setIdentity(identity) {
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
        if (identity.email && charLength(identity.email) > MAX_IDENTITY_EMAIL_CHARS) {
            throw new Error('Identity email must be 255 characters or less');
        }
        // ResolveWebIdentity bounds the hmac before it will even run the signature
        // comparison, so an over-long one is the same silent no-attach as an
        // over-long address, and it has the same author: identify() is the only
        // caller that supplies this pair. 64 is not a generous ceiling but the
        // exact length of the credential the server computes — a hex-encoded
        // SHA-256 — so a signature that overshoots it was mis-derived.
        //
        // Bytes, to mirror Go's len(), but unlike the address the unit cannot
        // change the answer here: hex is ASCII, where the two counts coincide, and
        // an hmac with multibyte characters in it fails the comparison whatever its
        // length. The same goes for the token below.
        if (identity.hmac && utf8Length(identity.hmac) > MAX_IDENTITY_HMAC_BYTES) {
            throw new Error('Identity hmac must be 64 bytes or less');
        }
        // Dropped rather than thrown, alone among these checks, because the token
        // is the one credential a stranger supplies: it is read from the nf_id URL
        // parameter during init(), so anyone can link to the customer's site with
        // ?nf_id=<megabytes>. init() runs before the sender is constructed and
        // caches its own promise, so a throw there would not just skip the
        // identity — it would leave the SDK dead for the rest of the page load and
        // surface as an unhandled rejection inside code the customer never wrote.
        // Nothing is lost by dropping quietly instead: BuildWebIdentifyToken
        // refuses to mint a token over this bound, so no genuine link can carry
        // one, and there is no caller mistake here for a throw to report.
        if (identity.token && utf8Length(identity.token) > MAX_IDENTITY_TOKEN_BYTES) {
            if (this.debug) {
                console.warn('[NotifuseAnalytics] Ignored an oversized identify token');
            }
            return;
        }
        if (!this.session)
            return;
        this.session.identity = identity;
        this.storage.set(STORAGE_KEYS.IDENTITY, identity);
        this.saveSession();
        if (this.debug) {
            console.log('[NotifuseAnalytics] Identity set');
        }
    }
    getIdentity() {
        if (!this.session)
            return null;
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
    clearIdentity() {
        this.storage.remove(STORAGE_KEYS.IDENTITY);
        if (!this.session)
            return;
        this.session.identity = null;
        this.saveSession();
    }
    /**
     * Load the stored identity, discarding anything an older build left behind.
     */
    loadIdentity() {
        const stored = this.storage.get(STORAGE_KEYS.IDENTITY);
        if (!stored)
            return null;
        if (!(stored.token || (stored.email && stored.hmac)))
            return null;
        // Bounds are re-checked on the way out, not just on the way in. A visitor
        // who was handed an oversized token before this check existed has it sitting
        // in localStorage, and every beat would keep carrying a credential the
        // server refuses to read — permanently, since nothing else ever revisits the
        // stored value. Dropping the key lets that visitor recover on this page load
        // instead of staying broken across SDK upgrades.
        if (!identityWithinBounds(stored)) {
            this.storage.remove(STORAGE_KEYS.IDENTITY);
            return null;
        }
        return stored;
    }
    /**
     * Apply dimensions from URL parameters
     * Only sets dimensions that don't already have values (existing wins)
     */
    applyUrlDimensions(urlDimensions) {
        if (!this.session)
            return;
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
    reset() {
        this.storage.remove(STORAGE_KEYS.SESSION);
        this.storage.remove(STORAGE_KEYS.DIMENSIONS);
        this.storage.remove(STORAGE_KEYS.IDENTITY);
        this.session = null;
        return this.createSession();
    }
}

/**
 * SessionState - Manages cumulative actions[] array for V3 session payload
 *
 * Key concepts:
 * - actions[]: Array of pageviews and goals (pages added immediately with duration=0)
 * - currentPageIndex: Index into actions[] for the page user is currently viewing
 * - Focus time: Duration is tracked via callback to SDK's heartbeatState.pageActiveMs
 *
 * Changes in V3:
 * - No separate currentPage field - page is in actions[] from the start
 * - No checkpoint - always send all actions, server uses ReplacingMergeTree
 * - No attributesSent optimization - always include attributes
 */
const STORAGE_KEY = 'nf_session_state';
const SDK_VERSION = "39.0";
const MAX_ACTIONS = 1000; // Match server limit from Phase 2
class SessionState {
    constructor(config) {
        this.actions = [];
        this.currentPageIndex = null;
        this.seq = 0;
        this.getPageFocusMs = null;
        this.workspaceId = config.workspace_id;
        this.sessionId = config.session_id;
        this.createdAt = config.created_at;
        this.tabId = config.tab_id ?? 0;
    }
    // === Focus Time Callback ===
    /**
     * Set the callback to get current page focus time from SDK's heartbeatState.
     * This is used to track accurate page duration (visible time only).
     */
    setFocusTimeGetter(getter) {
        this.getPageFocusMs = getter;
    }
    // === Getters ===
    getActions() {
        return [...this.actions];
    }
    /**
     * Get current page info derived from actions[currentPageIndex].
     * Returns null if no current page.
     */
    getCurrentPage() {
        if (this.currentPageIndex === null)
            return null;
        const action = this.actions[this.currentPageIndex];
        if (!action || action.type !== 'pageview')
            return null;
        return {
            path: action.path,
            page_number: action.page_number,
            entered_at: action.entered_at,
            scroll: action.scroll,
        };
    }
    // === Page Tracking ===
    addPageview(path) {
        // Check MAX_ACTIONS limit (pageviews now consume action slots)
        if (this.actions.length >= MAX_ACTIONS) {
            console.warn(`[SessionState] MAX_ACTIONS (${MAX_ACTIONS}) reached, pageview not added`);
            return;
        }
        const now = Date.now();
        // Finalize previous page if exists (update its duration)
        if (this.currentPageIndex !== null) {
            this.finalizeCurrentPageDuration(now);
        }
        // Create new page action with duration=0
        const pageNumber = this.getNextPageNumber();
        const pageview = {
            type: 'pageview',
            path,
            page_number: pageNumber,
            duration: 0, // Initial duration, updated on each send
            scroll: 0,
            entered_at: now,
            exited_at: now, // Will be updated on each send
        };
        // Add to actions and set as current
        this.actions.push(pageview);
        this.currentPageIndex = this.actions.length - 1;
        // Warn if approaching limit
        if (this.actions.length >= MAX_ACTIONS * 0.9) {
            console.warn(`[SessionState] Approaching MAX_ACTIONS limit (${this.actions.length}/${MAX_ACTIONS})`);
        }
    }
    updateScroll(scrollPercent) {
        if (this.currentPageIndex === null)
            return;
        const action = this.actions[this.currentPageIndex];
        if (!action || action.type !== 'pageview')
            return;
        // Clamp to 0-100
        const clamped = Math.max(0, Math.min(100, scrollPercent));
        // Only update if higher (track max)
        if (clamped > action.scroll) {
            action.scroll = clamped;
        }
    }
    // === Goal Tracking ===
    addGoal(name, goalType, value, properties) {
        // Check MAX_ACTIONS limit
        if (this.actions.length >= MAX_ACTIONS) {
            console.warn(`[SessionState] MAX_ACTIONS (${MAX_ACTIONS}) reached, goal not added`);
            return false;
        }
        // Get current page info from actions
        const currentPage = this.currentPageIndex !== null
            ? this.actions[this.currentPageIndex]
            : null;
        const goal = {
            type: 'goal',
            name,
            goal_type: goalType,
            path: currentPage?.path || '/',
            page_number: currentPage?.page_number || 1,
            timestamp: Date.now(),
        };
        if (value !== undefined) {
            goal.value = value;
        }
        if (properties) {
            goal.properties = properties;
        }
        this.actions.push(goal);
        // Warn if approaching limit
        if (this.actions.length >= MAX_ACTIONS * 0.9) {
            console.warn(`[SessionState] Approaching MAX_ACTIONS limit (${this.actions.length}/${MAX_ACTIONS})`);
        }
        return true;
    }
    // === Payload Building ===
    buildPayload(attributes, options) {
        // Update current page's duration and exited_at before building payload
        if (this.currentPageIndex !== null) {
            const action = this.actions[this.currentPageIndex];
            if (action && action.type === 'pageview') {
                // Get focus time from SDK (or 0 if no getter set)
                this.closePage(action, Date.now());
            }
        }
        const payload = {
            workspace_id: this.workspaceId,
            session_id: this.sessionId,
            tab_id: this.tabId,
            actions: [...this.actions],
            // Always include attributes (no optimization)
            attributes,
            created_at: this.createdAt,
            updated_at: Date.now(),
            sdk_version: SDK_VERSION,
            seq: ++this.seq,
        };
        // Add user_id if provided in options
        // Spread the credential the visitor actually holds. An unsigned address is
        // never stored client-side, so there is no shape here that the server would
        // silently discard.
        if (options && 'identity' in options) {
            const identity = options.identity;
            if (identity?.token) {
                payload.identify_token = identity.token;
            }
            else if (identity?.email && identity.hmac) {
                payload.contact_email = identity.email;
                payload.contact_email_hmac = identity.hmac;
            }
        }
        // Add dimensions if provided in options
        if (options && 'dimensions' in options) {
            payload.dimensions = options.dimensions;
        }
        return payload;
    }
    // === Unload Handling ===
    finalizeForUnload() {
        if (this.currentPageIndex === null)
            return;
        const action = this.actions[this.currentPageIndex];
        if (action && action.type === 'pageview') {
            // Update final duration and exit time
            this.closePage(action, Date.now());
        }
        // Clear current page index
        this.currentPageIndex = null;
    }
    /**
     * Re-open the page that finalizeForUnload() closed.
     *
     * Hiding or blurring the tab finalizes the current page so the beacon can
     * carry a complete payload — but the visitor almost always comes back to
     * that same page. Without this, its duration and scroll stay frozen at the
     * moment they left and every later measurement is dropped on the floor.
     */
    reopenCurrentPage() {
        if (this.currentPageIndex !== null)
            return;
        for (let i = this.actions.length - 1; i >= 0; i--) {
            if (this.actions[i].type === 'pageview') {
                this.currentPageIndex = i;
                return;
            }
        }
    }
    // === Persistence ===
    persist() {
        try {
            const snapshot = {
                actions: this.actions,
                currentPageIndex: this.currentPageIndex,
                seq: this.seq,
            };
            // Include session ID for validation on restore
            const data = {
                session_id: this.sessionId,
                ...snapshot,
            };
            sessionStorage.setItem(STORAGE_KEY, JSON.stringify(data));
        }
        catch (e) {
            // sessionStorage may be unavailable (private mode, quota exceeded)
            console.warn('[SessionState] Failed to persist:', e);
        }
    }
    restore() {
        try {
            const stored = sessionStorage.getItem(STORAGE_KEY);
            if (!stored)
                return;
            const data = JSON.parse(stored);
            // Validate session ID matches
            if (data.session_id !== this.sessionId) {
                // Different session, clear old data
                sessionStorage.removeItem(STORAGE_KEY);
                return;
            }
            // Restore state
            this.actions = data.actions || [];
            this.currentPageIndex = data.currentPageIndex ?? null;
            // Never restart the counter on reload: a lower seq would be ignored by
            // the server and the rest of the session silently lost.
            this.seq = typeof data.seq === 'number' ? data.seq : 0;
        }
        catch (e) {
            // Corrupted data, ignore
            console.warn('[SessionState] Failed to restore:', e);
            sessionStorage.removeItem(STORAGE_KEY);
        }
    }
    // === Private Helpers ===
    /**
     * Update the current page's duration when navigating away.
     * Uses focus time from SDK's heartbeatState via callback.
     */
    /**
     * Write a page's final duration and exit stamp, both clamped.
     *
     * The two clamps guard one failure: a backward wall-clock step. A negative
     * duration or an exited_at before entered_at is rejected by the server, and
     * since completed pages are never recomputed the bad value rides along in
     * every later beat of the session — so one clock glitch becomes permanent,
     * total loss for that visitor unless it is caught here.
     */
    closePage(action, exitTime) {
        const focus = this.getPageFocusMs ? this.getPageFocusMs() : 0;
        action.duration = focus > 0 ? focus : 0;
        action.exited_at = exitTime > action.entered_at ? exitTime : action.entered_at;
    }
    finalizeCurrentPageDuration(exitTime) {
        if (this.currentPageIndex === null)
            return;
        const action = this.actions[this.currentPageIndex];
        if (!action || action.type !== 'pageview')
            return;
        // Get focus time from SDK (or 0 if no getter set)
        this.closePage(action, exitTime);
    }
    getNextPageNumber() {
        // Find highest page_number in actions
        let maxPageNumber = 0;
        for (const action of this.actions) {
            if (action.page_number > maxPageNumber) {
                maxPageNumber = action.page_number;
            }
        }
        return maxPageNumber + 1;
    }
}

/**
 * V3 Session Payload Transport
 * Handles sending session payloads to the server with offline support
 */
const QUEUE_TTL_MS = 24 * 60 * 60 * 1000; // 24 hours
const TIMEOUT_MS = 10000; // 10 seconds
class Sender {
    constructor(endpoint, storage, debug = false, tabId = 0) {
        this.isFlushing = false;
        this.endpoint = endpoint;
        this.storage = storage;
        this.debug = debug;
        this.queueKey = `${STORAGE_KEYS.PENDING_QUEUE}_${tabId}`;
        // Listen for online event to flush queue
        if (typeof window !== 'undefined') {
            window.addEventListener('online', () => this.handleOnline());
        }
    }
    /**
     * Stringify payload with sent_at timestamp injected at send time.
     * CRITICAL: Call this at every HTTP send point, not when building/caching payload.
     */
    stringifyWithSentAt(payload) {
        return JSON.stringify({
            ...payload,
            sent_at: Date.now(),
        });
    }
    /**
     * Check if browser is offline
     */
    isOffline() {
        return typeof navigator !== 'undefined' && navigator.onLine === false;
    }
    /**
     * Classify a failed send by whether retrying this exact payload could ever
     * succeed.
     *
     * The server answers 200 for everything it wants the client NOT to retry —
     * unknown workspace, feature disabled, disallowed domain, bot user-agent — so
     * a 4xx means the payload itself is unacceptable and will stay unacceptable;
     * retrying it forever only poisons the queue. 408 and 429 are the exceptions:
     * they are about timing, not content. 5xx and network failures are transient
     * by definition, and in a cumulative-snapshot model the retry is one
     * idempotent re-POST that supersedes everything before it.
     */
    classifyStatus(status) {
        if (status >= 200 && status < 300)
            return 'ok';
        if (status === 408 || status === 429)
            return 'retryable';
        if (status >= 400 && status < 500)
            return 'permanent';
        return 'retryable';
    }
    /**
     * Write the payload to durable storage before any send is attempted.
     *
     * Persist-then-send is the only ordering that survives the tab dying
     * mid-flight, and it is safe precisely because duplicates are free: the
     * server's `EXCLUDED.beat_seq > beat_seq` guard makes a replayed beat a
     * no-op, while a dropped one costs everything since the last success.
     */
    persist(payload) {
        const id = `${payload.session_id}:${payload.seq}`;
        const queue = this.getQueue().filter((item) => item.id !== id);
        queue.push({ id, payload, queuedAt: Date.now() });
        this.saveQueue(queue);
        return id;
    }
    /** Remove one settled beat, leaving concurrently-added ones untouched. */
    dequeue(id) {
        this.saveQueue(this.getQueue().filter((item) => item.id !== id));
    }
    /**
     * One HTTP attempt, always bounded by a timeout — without one a single hung
     * connection stalls a drain indefinitely.
     */
    async attempt(payload) {
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);
        try {
            const response = await fetch(`${this.endpoint}/track`, {
                method: 'POST',
                headers: { 'Content-Type': 'text/plain;charset=UTF-8' },
                body: this.stringifyWithSentAt(payload), // Fresh sent_at at send time
                keepalive: true,
                signal: controller.signal,
            });
            if (response.ok) {
                const data = await Promise.resolve(response.json?.()).catch(() => undefined);
                return { outcome: 'ok', data };
            }
            const status = typeof response.status === 'number' ? response.status : 500;
            return {
                outcome: this.classifyStatus(status),
                error: `HTTP ${response.status}: ${response.statusText}`,
            };
        }
        catch (error) {
            // The abort is our own timeout firing, not a server signal; keep the
            // friendlier label callers already match on.
            const aborted = error instanceof Error && error.name === 'AbortError';
            return {
                outcome: 'retryable',
                error: aborted
                    ? 'Request timeout'
                    : error instanceof Error
                        ? error.message
                        : 'Unknown error',
            };
        }
        finally {
            clearTimeout(timeoutId);
        }
    }
    /**
     * Get pending queue from storage
     */
    getQueue() {
        return this.storage.get(this.queueKey) || [];
    }
    /**
     * Save queue to storage (with size limit)
     */
    saveQueue(queue) {
        const trimmed = queue.slice(-100);
        this.storage.set(this.queueKey, trimmed);
    }
    /**
     * Drain the durable queue.
     *
     * Each item is removed only once its own send has settled in a way that means
     * it will never succeed again — a 2xx, or a permanent rejection. The previous
     * implementation blanked the whole queue before sending anything, so a tab
     * closed mid-drain took every un-sent item with it.
     */
    async flushQueue() {
        if (this.isFlushing)
            return;
        this.isFlushing = true;
        try {
            const now = Date.now();
            for (const item of this.getQueue()) {
                if (now - item.queuedAt > QUEUE_TTL_MS) {
                    this.dequeue(item.id);
                    continue;
                }
                if (this.isOffline())
                    break;
                const result = await this.attempt(item.payload);
                if (result.outcome !== 'retryable') {
                    this.dequeue(item.id);
                }
            }
        }
        finally {
            this.isFlushing = false;
        }
    }
    /**
     * Flush queue when back online.
     *
     * The `online` edge is only one of the triggers: a fresh page load can never
     * observe it, so the SDK also calls flushQueue() at init. Without that, the
     * commonest offline pattern — browse offline, close the tab, reconnect with
     * no page open — leaves the queue untouched until its TTL discards it.
     */
    async handleOnline() {
        if (this.debug) {
            console.log('[NotifuseAnalytics] Back online, flushing queue');
        }
        await this.flushQueue();
    }
    /**
     * Send session payload via fetch.
     *
     * Retry eligibility comes from the outcome, never from navigator.onLine: that
     * is a link-layer signal and stays true behind a captive portal, a dead
     * upstream, a CSP block or an ad-blocker, which is most real-world failure.
     */
    async sendSession(payload) {
        const id = this.persist(payload);
        if (this.isOffline()) {
            return { success: false, error: 'offline', queued: true };
        }
        if (this.debug) {
            console.log('[NotifuseAnalytics] Sending session payload:', payload);
        }
        const result = await this.attempt(payload);
        if (result.outcome !== 'retryable') {
            this.dequeue(id);
        }
        if (result.outcome === 'ok') {
            if (this.debug) {
                console.log('[NotifuseAnalytics] Session response:', result.data);
            }
            return { success: true };
        }
        if (this.debug) {
            console.error('[NotifuseAnalytics] Send failed:', result.error);
        }
        return {
            success: false,
            error: result.error,
            queued: result.outcome === 'retryable',
        };
    }
    /**
     * Send session payload via sendBeacon (for unload)
     * IMPORTANT: sent_at is set fresh at each send attempt, not cached.
     */
    sendSessionBeacon(payload) {
        // The terminal beat is the one whose loss is unrecoverable, and none of the
        // transports below can confirm delivery. So it is persisted first and left
        // queued: the next page load replays it once, and the server's beat_seq
        // guard turns that replay into a no-op if it did arrive.
        this.persist(payload);
        if (this.isOffline()) {
            return false;
        }
        const url = `${this.endpoint}/track`;
        if (this.debug) {
            console.log('[NotifuseAnalytics] Sending session beacon:', payload);
        }
        // 1. Try fetchLater first (Chrome 121+, guaranteed delivery)
        if (typeof fetchLater === 'function') {
            try {
                fetchLater(url, {
                    method: 'POST',
                    headers: { 'Content-Type': 'text/plain;charset=UTF-8' },
                    body: this.stringifyWithSentAt(payload), // Fresh sent_at
                    activateAfter: 0,
                });
                if (this.debug) {
                    console.log('[NotifuseAnalytics] Session queued via fetchLater');
                }
                return true;
            }
            catch {
                // Fall through to sendBeacon
            }
        }
        // Safari beacon limit is 64KB, but older versions had 16KB
        // Use 15KB threshold for safety
        const MAX_BEACON_SIZE = 15 * 1024;
        const bodyForBeacon = this.stringifyWithSentAt(payload); // Fresh sent_at
        const useBeacon = bodyForBeacon.length <= MAX_BEACON_SIZE;
        // 2. Try sendBeacon (if payload is small enough)
        if (useBeacon && navigator.sendBeacon) {
            try {
                const blob = new Blob([bodyForBeacon], { type: 'text/plain;charset=UTF-8' });
                const success = navigator.sendBeacon(url, blob);
                if (success) {
                    if (this.debug) {
                        console.log('[NotifuseAnalytics] Session sent via beacon');
                    }
                    return true;
                }
            }
            catch {
                // Fall through to fetch fallback
            }
        }
        // 3. Fallback to fetch with keepalive (also used for large payloads).
        // A keepalive body over the origin's 64KiB budget, a CSP violation or a
        // blocker all reject the promise rather than throwing synchronously, so the
        // catch below cannot see them — hence the explicit .catch(), which also
        // stops an unhandled rejection leaking into the customer's error tracking.
        try {
            const pending = fetch(url, {
                method: 'POST',
                headers: {
                    'Content-Type': 'text/plain;charset=UTF-8',
                },
                body: this.stringifyWithSentAt(payload), // Fresh sent_at
                keepalive: true,
            });
            if (pending && typeof pending.catch === 'function') {
                pending.catch(() => {
                    if (this.debug) {
                        console.warn('[NotifuseAnalytics] keepalive fetch rejected; beat stays queued');
                    }
                });
            }
            if (this.debug) {
                console.log('[NotifuseAnalytics] Session sent via fetch keepalive');
            }
            return true;
        }
        catch {
            return false;
        }
    }
}

/**
 * Device detection using ua-parser-js with Client Hints support
 */
class DeviceDetector {
    constructor() {
        this.parser = new UAParser.UAParser();
    }
    /**
     * Detect device info with Client Hints (Chrome 90+)
     * Client Hints provide accurate OS versions (Win10 vs 11, macOS versions)
     * This is a SILENT API - no user prompts or permissions required
     */
    async detectWithClientHints() {
        try {
            // withClientHints() uses navigator.userAgentData.getHighEntropyValues()
            // This is completely silent - no browser prompts
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            const result = await this.parser.withClientHints();
            return this.mapResult(result);
        }
        catch {
            // Fallback if Client Hints unavailable or blocked
            return this.detect();
        }
    }
    /**
     * Synchronous detection (fallback for non-Client Hints browsers)
     */
    detect() {
        const result = this.parser.getResult();
        return this.mapResult(result);
    }
    /**
     * Map ua-parser-js result to DeviceInfo
     */
    mapResult(result) {
        return {
            screen_width: window.screen.width,
            screen_height: window.screen.height,
            viewport_width: window.innerWidth,
            viewport_height: window.innerHeight,
            device: this.normalizeDeviceType(result.device.type),
            browser: result.browser.name || 'Unknown',
            browser_type: this.getBrowserType(result),
            os: this.normalizeOS(result.os.name, result.device.type),
            user_agent: navigator.userAgent,
            connection_type: this.getConnectionType(),
            timezone: this.getTimezone(),
            language: navigator.language || 'en',
        };
    }
    /**
     * Normalize device type
     */
    normalizeDeviceType(type) {
        switch (type) {
            case 'mobile':
                return 'mobile';
            case 'tablet':
                return 'tablet';
            default:
                // ua-parser-js returns undefined for desktop
                return 'desktop';
        }
    }
    /**
     * Normalize OS name
     */
    normalizeOS(osName, deviceType) {
        if (!osName)
            return 'Unknown';
        // Handle iPad specifically (iPadOS vs iOS)
        if (osName === 'iOS' && deviceType === 'tablet') {
            return 'iPadOS';
        }
        // Normalize common OS names
        const osMap = {
            'Mac OS': 'macOS',
            Windows: 'Windows',
            iOS: 'iOS',
            Android: 'Android',
            Linux: 'Linux',
            'Chrome OS': 'Chrome OS',
            Ubuntu: 'Linux',
            Fedora: 'Linux',
            Debian: 'Linux',
        };
        return osMap[osName] || osName;
    }
    /**
     * Detect special browser types
     */
    getBrowserType(_result) {
        const ua = navigator.userAgent.toLowerCase();
        // Crawler/bot detection
        if (/bot|crawler|spider|scraper/i.test(ua)) {
            return 'crawler';
        }
        // In-app browsers
        if (/fbav|fban|instagram|twitter|linkedin|pinterest/i.test(ua)) {
            return 'inapp';
        }
        // Email clients
        if (/thunderbird|outlook/i.test(ua)) {
            return 'email';
        }
        // Headless/fetchers
        if (/headless|phantom|puppeteer|selenium/i.test(ua)) {
            return 'fetcher';
        }
        // CLI tools
        if (/curl|wget|httpie/i.test(ua)) {
            return 'cli';
        }
        return null;
    }
    /**
     * Get connection type via Network Information API
     * Only Chromium-based browsers support this (Chrome 61+, Edge 79+, Opera 48+)
     * Firefox/Safari return empty string (graceful degradation)
     */
    getConnectionType() {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const connection = navigator.connection;
        return connection?.effectiveType || '';
    }
    /**
     * Get timezone
     */
    getTimezone() {
        try {
            return Intl.DateTimeFormat().resolvedOptions().timeZone;
        }
        catch {
            return '';
        }
    }
}

/**
 * Throttle utility for performance
 */
function throttle(fn, delay) {
    let lastCall = 0;
    let timeoutId = null;
    return ((...args) => {
        const now = Date.now();
        const timeSinceLastCall = now - lastCall;
        if (timeSinceLastCall >= delay) {
            lastCall = now;
            fn(...args);
        }
        else {
            // Schedule trailing call
            if (timeoutId) {
                clearTimeout(timeoutId);
            }
            timeoutId = setTimeout(() => {
                lastCall = Date.now();
                fn(...args);
                timeoutId = null;
            }, delay - timeSinceLastCall);
        }
    });
}

/**
 * Scroll tracking
 */
class ScrollTracker {
    constructor() {
        this.maxScrollPercent = 0;
        this.onMilestone = null;
        this.lastMilestone = 0;
        this.boundHandler = null;
        this.domReadyHandler = null;
        this.boundHandler = throttle(() => this.handleScroll(), 100);
    }
    /**
     * Start tracking scroll
     */
    start() {
        if (this.boundHandler) {
            window.addEventListener('scroll', this.boundHandler, { passive: true });
        }
        // The first measurement needs a parsed document. A snippet that drops the
        // `async` attribute runs the SDK from <head> while document.body is still
        // null, and the page height is not final until parsing ends anyway, so
        // measuring now would either throw or report a short page as fully seen.
        if (document.readyState === 'loading') {
            this.domReadyHandler = () => {
                this.domReadyHandler = null;
                this.handleScroll();
            };
            document.addEventListener('DOMContentLoaded', this.domReadyHandler, { once: true });
            return;
        }
        this.handleScroll();
    }
    /**
     * Stop tracking scroll
     */
    stop() {
        if (this.boundHandler) {
            window.removeEventListener('scroll', this.boundHandler);
        }
        if (this.domReadyHandler) {
            document.removeEventListener('DOMContentLoaded', this.domReadyHandler);
            this.domReadyHandler = null;
        }
    }
    /**
     * Set milestone callback (25%, 50%, 75%, 100%)
     */
    setMilestoneCallback(callback) {
        this.onMilestone = callback;
    }
    /**
     * Get max scroll percentage
     */
    getMaxScrollPercent() {
        return this.maxScrollPercent;
    }
    /**
     * Handle scroll event
     */
    handleScroll() {
        // A scroll event can fire before the body exists; a half-parsed document
        // has no meaningful height to compare against.
        if (!document.body) {
            return;
        }
        const scrollHeight = Math.max(document.documentElement.scrollHeight, document.body.scrollHeight);
        const clientHeight = window.innerHeight;
        const scrollTop = window.pageYOffset || document.documentElement.scrollTop;
        // Guard against division by zero
        const scrollableHeight = scrollHeight - clientHeight;
        if (scrollableHeight <= 0) {
            this.maxScrollPercent = 100; // No scrolling needed
            return;
        }
        const scrollPercent = Math.round((scrollTop / scrollableHeight) * 100);
        const clampedPercent = Math.min(100, Math.max(0, scrollPercent));
        if (clampedPercent > this.maxScrollPercent) {
            this.maxScrollPercent = clampedPercent;
            // Check milestones
            this.checkMilestones(clampedPercent);
        }
    }
    /**
     * Check and trigger milestone callbacks
     */
    checkMilestones(percent) {
        if (!this.onMilestone)
            return;
        const milestones = [25, 50, 75, 100];
        for (const milestone of milestones) {
            if (percent >= milestone && this.lastMilestone < milestone) {
                this.lastMilestone = milestone;
                this.onMilestone(milestone);
            }
        }
    }
    /**
     * Reset scroll tracking
     */
    reset() {
        this.maxScrollPercent = 0;
        this.lastMilestone = 0;
    }
}

/**
 * SPA Navigation tracking
 * Detects pushState, replaceState, popstate, and hashchange
 */
class NavigationTracker {
    constructor() {
        this.onNavigate = null;
        this.originalPushState = null;
        this.originalReplaceState = null;
        /**
         * Handle navigation event
         */
        this.handleNavigation = () => {
            const newUrl = window.location.href;
            if (newUrl !== this.currentUrl) {
                this.currentUrl = newUrl;
                if (this.onNavigate) {
                    this.onNavigate(newUrl);
                }
            }
        };
        this.currentUrl = window.location.href;
    }
    /**
     * Start tracking navigation
     */
    start() {
        this.patchHistory();
        window.addEventListener('popstate', this.handleNavigation);
        window.addEventListener('hashchange', this.handleNavigation);
    }
    /**
     * Stop tracking navigation
     */
    stop() {
        this.restoreHistory();
        window.removeEventListener('popstate', this.handleNavigation);
        window.removeEventListener('hashchange', this.handleNavigation);
    }
    /**
     * Set navigation callback
     */
    setNavigationCallback(callback) {
        this.onNavigate = callback;
    }
    /**
     * Patch History API
     */
    patchHistory() {
        this.originalPushState = history.pushState;
        this.originalReplaceState = history.replaceState;
        history.pushState = (...args) => {
            this.originalPushState?.apply(history, args);
            this.handleNavigation();
        };
        history.replaceState = (...args) => {
            this.originalReplaceState?.apply(history, args);
            this.handleNavigation();
        };
    }
    /**
     * Restore original History API
     */
    restoreHistory() {
        if (this.originalPushState) {
            history.pushState = this.originalPushState;
        }
        if (this.originalReplaceState) {
            history.replaceState = this.originalReplaceState;
        }
    }
    /**
     * Get current URL
     */
    getCurrentUrl() {
        return this.currentUrl;
    }
}

/**
 * Bot and crawler detection
 */
const BOT_PATTERNS = [
    /bot/i,
    /crawler/i,
    /spider/i,
    /scraper/i,
    /googlebot/i,
    /bingbot/i,
    /yandex/i,
    /baidu/i,
    /duckduck/i,
    /slurp/i,
    /msnbot/i,
    /ia_archiver/i,
    /facebook/i,
    /twitter/i,
    /linkedin/i,
    /pinterest/i,
    /headless/i,
    /phantom/i,
    /selenium/i,
    /puppeteer/i,
    /lighthouse/i,
    /pagespeed/i,
    /gtmetrix/i,
];
/**
 * Check if the current user is a bot/crawler
 */
function isBot() {
    // Layer 1: User-agent patterns
    const ua = navigator.userAgent.toLowerCase();
    if (BOT_PATTERNS.some((p) => p.test(ua))) {
        return true;
    }
    // Layer 2: WebDriver detection (Selenium, Puppeteer, etc.)
    if (navigator.webdriver) {
        return true;
    }
    // Layer 3: Feature fingerprinting
    const suspiciousFeatures = [
        !('plugins' in navigator) || navigator.plugins.length === 0,
        !('languages' in navigator) || navigator.languages.length === 0,
        // Fake Chrome detection
        !window.chrome && /chrome/i.test(ua),
        // Zero screen dimensions
        screen.width === 0 || screen.height === 0,
        // Fake mobile (no touch but mobile UA)
        !('ontouchstart' in window) && /mobile/i.test(ua),
    ];
    const suspiciousCount = suspiciousFeatures.filter(Boolean).length;
    if (suspiciousCount >= 3) {
        return true;
    }
    return false;
}

/**
 * Cross-Domain Session Linker
 * Handles URL decoration for cross-domain session continuity
 * Privacy-first: No cookies, URL parameters only
 */
/**
 * Default cross-domain URL parameter. Configurable via `crossDomainParam`:
 * it lands in the customer's own URLs, server logs and analytics reports.
 */
const DEFAULT_CROSS_DOMAIN_PARAM = '_nf';
const UUID_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const CLOCK_SKEW_TOLERANCE = 60; // seconds
/**
 * Encode payload to Base64URL string
 */
function encode(payload) {
    return btoa(JSON.stringify(payload))
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/, '');
}
/**
 * Decode Base64URL string to payload
 * Returns null if invalid
 */
function decode(encoded) {
    try {
        // Convert Base64URL to standard Base64
        const base64 = encoded.replace(/-/g, '+').replace(/_/g, '/');
        const json = atob(base64);
        const payload = JSON.parse(json);
        // Validate required fields exist
        if (typeof payload.s !== 'string' ||
            typeof payload.t !== 'number') {
            return null;
        }
        // Validate UUID format
        if (!UUID_REGEX.test(payload.s)) {
            return null;
        }
        return payload;
    }
    catch {
        return null;
    }
}
class CrossDomainLinker {
    constructor(config) {
        this.getSessionId = () => '';
        this.clickHandler = null;
        this.submitHandler = null;
        this.config = config;
    }
    /**
     * Set ID getter function
     */
    setIdGetters(getSessionId) {
        this.getSessionId = getSessionId;
    }
    /**
     * Start listening for link clicks and form submissions
     */
    start() {
        this.clickHandler = (e) => this.handleClick(e);
        this.submitHandler = (e) => this.handleSubmit(e);
        document.addEventListener('click', this.clickHandler, true);
        document.addEventListener('submit', this.submitHandler, true);
        if (this.config.debug) {
            console.log('[NotifuseAnalytics] CrossDomainLinker started', {
                domains: this.config.domains,
            });
        }
    }
    /**
     * Stop listening for events
     */
    stop() {
        if (this.clickHandler) {
            document.removeEventListener('click', this.clickHandler, true);
            this.clickHandler = null;
        }
        if (this.submitHandler) {
            document.removeEventListener('submit', this.submitHandler, true);
            this.submitHandler = null;
        }
        if (this.config.debug) {
            console.log('[NotifuseAnalytics] CrossDomainLinker stopped');
        }
    }
    /**
     * Handle click events - decorate links to configured domains
     */
    handleClick(e) {
        const target = e.target?.closest('a');
        if (!target || !target.href)
            return;
        if (!this.shouldDecorate(target.href))
            return;
        const decorated = this.decorateUrl(target.href);
        if (decorated !== target.href) {
            target.href = decorated;
            if (this.config.debug) {
                console.log('[NotifuseAnalytics] Decorated link:', decorated);
            }
        }
    }
    /**
     * Handle form submissions - add hidden input for GET forms
     */
    handleSubmit(e) {
        const form = e.target;
        if (!form || !form.action)
            return;
        // Only handle GET forms
        if (form.method.toLowerCase() !== 'get')
            return;
        if (!this.shouldDecorate(form.action))
            return;
        const sessionId = this.getSessionId();
        if (!sessionId)
            return;
        const payload = {
            s: sessionId,
            t: Math.floor(Date.now() / 1000),
        };
        // Remove existing hidden input if present
        const existing = form.querySelector(`input[name="${this.config.paramName}"]`);
        if (existing)
            existing.remove();
        // Add hidden input
        const input = document.createElement('input');
        input.type = 'hidden';
        input.name = this.config.paramName;
        input.value = encode(payload);
        form.appendChild(input);
        if (this.config.debug) {
            console.log('[NotifuseAnalytics] Decorated form:', form.action);
        }
    }
    /**
     * Decorate a URL with cross-domain parameters
     */
    decorateUrl(url) {
        const sessionId = this.getSessionId();
        if (!sessionId) {
            return url;
        }
        if (!this.shouldDecorate(url)) {
            return url;
        }
        try {
            const parsed = new URL(url, window.location.origin);
            const payload = {
                s: sessionId,
                t: Math.floor(Date.now() / 1000),
            };
            parsed.searchParams.set(this.config.paramName, encode(payload));
            return parsed.toString();
        }
        catch {
            return url;
        }
    }
    /**
     * Check if URL should be decorated
     */
    shouldDecorate(url) {
        try {
            const parsed = new URL(url, window.location.origin);
            // Don't decorate same-origin
            if (parsed.hostname === window.location.hostname) {
                return false;
            }
            // Check if target is in configured domains
            const normalizedTarget = this.normalizeHostname(parsed.hostname);
            return this.config.domains.some((domain) => {
                const normalizedDomain = this.normalizeHostname(domain);
                // Exact match or subdomain match
                return (normalizedTarget === normalizedDomain ||
                    normalizedTarget.endsWith('.' + normalizedDomain));
            });
        }
        catch {
            return false;
        }
    }
    /**
     * Normalize hostname (remove www. prefix)
     */
    normalizeHostname(hostname) {
        return hostname.toLowerCase().replace(/^www\./, '');
    }
    /**
     * Read cross-domain parameter from URL (static)
     * Returns null if not found or invalid
     */
    static readParam(expiry, paramName = DEFAULT_CROSS_DOMAIN_PARAM) {
        try {
            const params = new URLSearchParams(window.location.search);
            const encoded = params.get(paramName);
            if (!encoded) {
                return null;
            }
            const payload = decode(encoded);
            if (!payload) {
                return null;
            }
            // Validate timestamp
            const now = Math.floor(Date.now() / 1000);
            const age = now - payload.t;
            // Check if expired
            if (age > expiry) {
                return null;
            }
            // Check if too far in future (clock skew)
            if (payload.t > now + CLOCK_SKEW_TOLERANCE) {
                return null;
            }
            return payload;
        }
        catch {
            return null;
        }
    }
    /**
     * Strip cross-domain parameter from URL (static)
     */
    static stripParam(paramName = DEFAULT_CROSS_DOMAIN_PARAM) {
        try {
            const url = new URL(window.location.href);
            if (!url.searchParams.has(paramName)) {
                return;
            }
            url.searchParams.delete(paramName);
            // Build clean URL
            const cleanPath = window.location.pathname +
                (url.searchParams.toString() ? '?' + url.searchParams.toString() : '') +
                window.location.hash;
            window.history.replaceState(window.history.state, '', cleanPath);
        }
        catch {
            // Ignore errors (some browsers may restrict replaceState)
        }
    }
}

/**
 * Custom dimension URL parameter parsing
 * Parses custom_1 through custom_10 from URL
 */
const MIN_INDEX = 1;
const MAX_INDEX = 10;
const MAX_LENGTH = 256;
/**
 * Parse custom_1 through custom_10 parameters from URL
 * Returns only valid dimensions (string values, max 256 chars)
 */
function parseCustomDimensions(url) {
    const dimensions = {};
    try {
        const params = new URL(url).searchParams;
        for (let i = MIN_INDEX; i <= MAX_INDEX; i++) {
            const value = params.get(`custom_${i}`);
            if (value !== null && value.length <= MAX_LENGTH) {
                dimensions[i] = value;
            }
        }
    }
    catch {
        // Invalid URL, return empty dimensions
    }
    return dimensions;
}

/**
 * Monotonic time source for measuring elapsed intervals.
 *
 * Date.now() is a wall clock and steps backwards: an NTP correction after
 * suspend, a VM restore, a phone re-syncing after airplane mode. Any interval
 * computed as a Date.now() delta can therefore come out negative — and because
 * actions[] is cumulative and completed pages are never recomputed, a single
 * negative duration is re-sent on every later beat and rejected by the server
 * every time, turning one clock glitch into permanent loss for that session.
 *
 * performance.now() is monotonic by specification. Use this for every elapsed
 * measurement, and keep Date.now() for wall-clock stamps (entered_at,
 * exited_at, goal timestamps) that genuinely mean "what time was it".
 *
 * Never mix the two in one subtraction: their epochs are unrelated.
 */
function monotonicNow() {
    if (typeof performance !== 'undefined' && typeof performance.now === 'function') {
        return performance.now();
    }
    return Date.now();
}

/**
 * Notifuse Analytics SDK v6.0
 * Ultra-reliable web analytics for tracking TimeScore metrics
 * V3 Session Payload Architecture
 */
// Heartbeat constants
const MIN_HEARTBEAT_INTERVAL = 5000; // 5 seconds minimum
const MIN_HEARTBEAT_MAX_DURATION = 60 * 1000; // 1 minute minimum
const SEND_DEBOUNCE_MS = 100;
/** URL parameter a tracked email link uses to hand over a verified identity. */
const IDENTIFY_PARAM = 'nf_id';
// Default heartbeat tiers
const DEFAULT_HEARTBEAT_TIERS = [
    // 0-3 min: High frequency (initial engagement is critical)
    { after: 0, desktopInterval: 10000, mobileInterval: 7000 },
    // 3-5 min: Medium frequency (user is engaged, reduce load)
    { after: 3 * 60 * 1000, desktopInterval: 20000, mobileInterval: 14000 },
    // 5-10 min: Low frequency (long-form content, minimal pings)
    { after: 5 * 60 * 1000, desktopInterval: 30000, mobileInterval: 21000 },
];
// Default configuration
const DEFAULT_CONFIG = {
    debug: false,
    sessionTimeout: 30 * 60 * 1000, // 30 minutes
    heartbeatInterval: 10000, // 10 seconds (legacy, used as fallback)
    adClickIds: DEFAULT_AD_CLICK_IDS,
    trackSPA: true,
    trackScroll: true,
    trackClicks: false,
    heartbeatTiers: DEFAULT_HEARTBEAT_TIERS,
    heartbeatMaxDuration: 10 * 60 * 1000, // 10 minutes
    resetHeartbeatOnNavigation: false,
    // Cross-domain tracking
    crossDomains: [],
    crossDomainExpiry: 120, // 2 minutes
    crossDomainStripParams: true,
    crossDomainParam: DEFAULT_CROSS_DOMAIN_PARAM,
};
class NotifuseAnalyticsSDK {
    constructor() {
        this.config = null;
        this.storage = null;
        this.tabStorage = null;
        this.sessionManager = null;
        this.sessionState = null;
        this.sender = null;
        this.deviceDetector = null;
        this.scrollTracker = null;
        this.navigationTracker = null;
        this.crossDomainLinker = null;
        this.deviceInfo = null;
        this.heartbeatTimeout = null;
        this.sendDebounceTimeout = null;
        this.heartbeatState = {
            activeStartTime: 0,
            accumulatedActiveMs: 0,
            isActive: false,
            maxDurationReached: false,
            lastPingTime: 0,
            currentTierIndex: 0,
            pageActiveMs: 0,
            pageStartTime: 0,
        };
        this.isMobileDevice = false;
        this.isTracking = false;
        this.isPaused = false;
        this.isInitialized = false;
        this.initPromise = null;
        this.flushed = false;
        /**
         * Visibility change handler
         */
        this.onVisibilityChange = () => {
            if (document.visibilityState === 'hidden') {
                this.stopHeartbeat(true); // Accumulate active time
                this.flushOnce();
            }
            else if (document.visibilityState === 'visible') {
                this.resumeTracking();
            }
        };
        /**
         * Window focus handler
         */
        this.onFocus = () => {
            this.resumeTracking();
        };
        /**
         * Window blur handler
         */
        this.onBlur = () => {
            this.stopHeartbeat(true); // Accumulate active time
            this.flushOnce();
        };
        /**
         * Page freeze handler (mobile)
         */
        this.onFreeze = () => {
            this.stopHeartbeat(true); // Accumulate active time
            this.flushOnce();
        };
        /**
         * Page resume handler (mobile)
         */
        this.onResume = () => {
            this.resumeTracking();
        };
        /**
         * Page unload handler
         */
        this.onUnload = () => {
            this.flushOnce();
        };
        /**
         * Page show handler (bfcache)
         */
        this.onPageShow = (event) => {
            if (event.persisted) {
                // Restore SessionState first: it rewrites currentPageIndex from the
                // snapshot, which would undo the reopen below.
                this.sessionState?.restore();
                this.resumeTracking();
                if (this.config?.debug) {
                    console.log('[NotifuseAnalytics] Restored from bfcache');
                }
            }
        };
    }
    /**
     * Initialize the SDK (called by index.ts from global config or manual init)
     * Returns the init promise so callers can await if needed
     */
    init(userConfig) {
        // If already initialized or initializing, return existing promise
        if (this.isInitialized) {
            return Promise.resolve();
        }
        if (this.initPromise) {
            return this.initPromise;
        }
        // Store the promise for ensureInitialized to await
        this.initPromise = this.initializeAsync(userConfig);
        return this.initPromise;
    }
    /**
     * Async initialization logic
     */
    async initializeAsync(userConfig) {
        // Validate required fields
        if (!userConfig.workspace_id) {
            throw new Error('workspace_id is required');
        }
        if (!userConfig.endpoint) {
            throw new Error('endpoint is required');
        }
        // Check for bots
        if (isBot()) {
            console.log('[NotifuseAnalytics] Bot detected, tracking disabled');
            return;
        }
        // Merge config
        this.config = {
            ...DEFAULT_CONFIG,
            ...userConfig,
        };
        // Fold extraAdClickIds into the effective list, so everything downstream
        // keeps reading a single config.adClickIds. Applied AFTER the merge, so it
        // extends whichever list won — the defaults, or a caller's replacement.
        const extra = this.config.extraAdClickIds;
        if (extra && extra.length > 0) {
            const effective = [...this.config.adClickIds];
            for (const id of extra) {
                if (!effective.some((known) => known.toLowerCase() === id.toLowerCase())) {
                    effective.push(id);
                }
            }
            this.config.adClickIds = effective;
        }
        // Validate and normalize heartbeat tiers
        this.config.heartbeatTiers = this.validateTiers(this.config.heartbeatTiers);
        // Validate heartbeat max duration
        if (this.config.heartbeatMaxDuration !== 0 &&
            this.config.heartbeatMaxDuration < MIN_HEARTBEAT_MAX_DURATION) {
            this.config.heartbeatMaxDuration = MIN_HEARTBEAT_MAX_DURATION;
        }
        // Initialize storage
        this.storage = new Storage();
        this.tabStorage = new TabStorage();
        // Initialize device detector
        this.deviceDetector = new DeviceDetector();
        this.deviceInfo = await this.deviceDetector.detectWithClientHints();
        // Set mobile device flag for heartbeat intervals
        this.isMobileDevice = this.deviceInfo?.device !== 'desktop';
        // Read cross-domain param BEFORE session creation
        const crossDomainPayload = CrossDomainLinker.readParam(this.config.crossDomainExpiry, this.config.crossDomainParam);
        // Initialize session manager
        this.sessionManager = new SessionManager(this.storage, this.tabStorage, this.config);
        // Inject cross-domain payload if present
        if (crossDomainPayload) {
            this.sessionManager.setCrossDomainInput({
                sessionId: crossDomainPayload.s,
                timestamp: crossDomainPayload.t,
                expiry: this.config.crossDomainExpiry,
            });
        }
        // Read and strip nf_id BEFORE the session is created.
        //
        // getOrCreateSession snapshots window.location.href as landing_page and
        // persists it, and every later beat re-sends it — so stripping afterwards
        // would leave the workspace-signed credential sitting in the customer's own
        // landing-page reports, which is the exact leak the stripping exists to
        // prevent. The token is applied below, once there is a session to attach it
        // to; only the URL rewrite has to happen first.
        const identifyToken = new URLSearchParams(window.location.search).get(IDENTIFY_PARAM);
        if (identifyToken) {
            CrossDomainLinker.stripParam(IDENTIFY_PARAM);
        }
        // Get or create session (uses cross-domain payload if valid)
        const session = this.sessionManager.getOrCreateSession();
        // Apply URL dimensions (existing values take priority)
        const urlDimensions = parseCustomDimensions(window.location.href);
        if (Object.keys(urlDimensions).length > 0) {
            this.sessionManager.applyUrlDimensions(urlDimensions);
        }
        // Strip _nf param from URL after processing
        if (crossDomainPayload && this.config.crossDomainStripParams) {
            CrossDomainLinker.stripParam(this.config.crossDomainParam);
        }
        // Adopt an identity handed over by a tracked email link.
        //
        // Stripped immediately: the parameter is a workspace-signed credential, and
        // leaving it in the address bar puts it into the customer's own analytics,
        // their server logs, and the Referer of every third-party asset the page
        // loads. Stripping before the first beat also stops it being re-adopted on
        // an SPA navigation.
        //
        // Deliberately NOT propagated through the cross-domain _nf handoff: that
        // would spray the credential across every configured domain. A visitor who
        // crosses domains is simply anonymous there until that origin identifies
        // them itself.
        if (identifyToken) {
            this.sessionManager.setIdentity({ token: identifyToken });
        }
        // Initialize sender
        this.sender = new Sender(this.config.endpoint, this.storage, this.config.debug, this.sessionManager.getTabId());
        // Drain anything left over from a previous visit. The `online` event is the
        // only other trigger, and a fresh page load can never observe it — so
        // without this the commonest offline pattern (browse offline, close the tab,
        // reconnect with no page open, come back later) leaves those beats sitting
        // in storage until the TTL silently discards them. Fire-and-forget: a replay
        // must never delay this page's own first beat.
        this.sender.flushQueue().catch(() => { });
        // Initialize scroll tracker
        if (this.config.trackScroll) {
            this.scrollTracker = new ScrollTracker();
            // No milestone callback needed - we just track max scroll
            this.scrollTracker.start();
        }
        // Initialize navigation tracker
        if (this.config.trackSPA) {
            this.navigationTracker = new NavigationTracker();
            this.navigationTracker.setNavigationCallback((url) => this.onNavigation(url));
            this.navigationTracker.start();
        }
        // Initialize cross-domain linker if configured
        if (this.config.crossDomains.length > 0) {
            this.crossDomainLinker = new CrossDomainLinker({
                domains: this.config.crossDomains,
                expiry: this.config.crossDomainExpiry,
                debug: this.config.debug,
                paramName: this.config.crossDomainParam,
            });
            this.crossDomainLinker.setIdGetters(() => this.sessionManager?.getSessionId() || '');
            this.crossDomainLinker.start();
        }
        // Initialize SessionState (V3)
        const sessionStateConfig = {
            workspace_id: this.config.workspace_id,
            session_id: session.id,
            created_at: session.created_at,
            tab_id: this.sessionManager.getTabId(),
        };
        this.sessionState = new SessionState(sessionStateConfig);
        this.sessionState.restore(); // Restore from sessionStorage if available
        // Wire up focus time getter for accurate page duration tracking
        this.sessionState.setFocusTimeGetter(() => this.getPageActiveMs());
        // Add initial pageview
        this.sessionState.addPageview(window.location.pathname);
        // Bind events
        this.bindEvents();
        // Start tracking
        this.isTracking = true;
        this.isInitialized = true;
        // Initialize heartbeat state
        const now = monotonicNow();
        this.heartbeatState.pageStartTime = now;
        this.heartbeatState.activeStartTime = now;
        // Start heartbeat
        this.startHeartbeat();
        // Send initial payload (immediate, with attributes)
        await this.sendPayload();
        if (this.config.debug) {
            console.log('[NotifuseAnalytics] Initialized', {
                session_id: session.id,
                device: this.deviceInfo,
            });
        }
    }
    /**
     * Bind browser events
     */
    bindEvents() {
        // Visibility change (tab switch, minimize)
        document.addEventListener('visibilitychange', this.onVisibilityChange);
        // Window focus/blur (alt-tab, window switch)
        window.addEventListener('focus', this.onFocus);
        window.addEventListener('blur', this.onBlur);
        // Page lifecycle (mobile freeze/resume)
        document.addEventListener('freeze', this.onFreeze);
        document.addEventListener('resume', this.onResume);
        // Page unload
        window.addEventListener('pagehide', this.onUnload);
        window.addEventListener('beforeunload', this.onUnload);
        // Back-forward cache
        window.addEventListener('pageshow', this.onPageShow);
    }
    /**
     * Come back from a hidden, blurred or frozen page.
     *
     * Each of those paths ran flushOnce(), which finalized the current page to
     * send a complete beacon. Reopening it is what makes the visitor's time
     * after they return count towards the page they returned to.
     */
    resumeTracking() {
        this.flushed = false;
        this.sessionState?.reopenCurrentPage();
        if (!this.isPaused && !this.heartbeatState.maxDurationReached) {
            // Resume heartbeat with fresh timing
            this.resumeHeartbeat();
        }
    }
    /**
     * Flush once (deduplicate unload events)
     */
    flushOnce() {
        if (this.flushed)
            return;
        this.flushed = true;
        if (!this.sessionState || !this.sender)
            return;
        // Update scroll before finalizing
        if (this.scrollTracker) {
            this.sessionState.updateScroll(this.scrollTracker.getMaxScrollPercent());
        }
        // Finalize current page
        this.sessionState.finalizeForUnload();
        // Build and send via beacon
        const attributes = this.buildAttributes();
        const payload = this.sessionState.buildPayload(attributes, {
            identity: this.sessionManager?.getIdentity() ?? null,
            dimensions: this.sessionManager?.getDimensionsPayload() ?? {},
        });
        this.sender.sendSessionBeacon(payload);
        // Persist final state
        this.sessionState.persist();
    }
    /**
     * Navigation callback
     */
    onNavigation(url) {
        if (!this.sessionState)
            return;
        if (this.config?.debug) {
            console.log('[NotifuseAnalytics] Navigation:', url);
        }
        // Update scroll before finalizing page
        if (this.scrollTracker) {
            this.sessionState.updateScroll(this.scrollTracker.getMaxScrollPercent());
        }
        // Add new pageview (this finalizes the previous page)
        this.sessionState.addPageview(window.location.pathname);
        // Reset scroll tracking for new page
        this.scrollTracker?.reset();
        // Reset page timer
        this.resetPageActiveTime();
        // Optionally reset session heartbeat timer
        if (this.config?.resetHeartbeatOnNavigation) {
            this.resetHeartbeatState();
            this.startHeartbeat();
        }
        // Debounced send (navigation can be rapid in SPAs)
        this.scheduleDebouncedSend();
        // Persist state
        this.sessionState.persist();
    }
    /**
     * Start heartbeat with tiered intervals
     */
    startHeartbeat() {
        if (!this.config)
            return;
        // Don't restart if max duration reached
        if (this.heartbeatState.maxDurationReached) {
            if (this.config.debug) {
                console.log('[NotifuseAnalytics] Heartbeat not started: max duration reached');
            }
            return;
        }
        // Clear existing heartbeat
        this.stopHeartbeat(false); // Don't accumulate time (we're starting fresh)
        // Record when we became active
        const now = monotonicNow();
        this.heartbeatState.activeStartTime = now;
        this.heartbeatState.isActive = true;
        this.heartbeatState.lastPingTime = now;
        // Start the heartbeat loop
        this.scheduleNextHeartbeat();
    }
    /**
     * Resume heartbeat after visibility/focus change
     */
    resumeHeartbeat() {
        if (!this.config)
            return;
        // Don't restart if max duration reached
        if (this.heartbeatState.maxDurationReached) {
            return;
        }
        // Resume with fresh timing
        const now = monotonicNow();
        this.heartbeatState.activeStartTime = now;
        this.heartbeatState.pageStartTime = now;
        this.heartbeatState.isActive = true;
        this.heartbeatState.lastPingTime = now;
        this.scheduleNextHeartbeat();
    }
    /**
     * Schedule next heartbeat based on current tier
     */
    scheduleNextHeartbeat() {
        if (!this.config || !this.heartbeatState.isActive)
            return;
        // Check max duration BEFORE scheduling
        if (this.checkAndUpdateMaxDuration()) {
            this.stopHeartbeat(true);
            return;
        }
        // Get current interval based on active time
        const interval = this.getCurrentInterval();
        // Null interval means stop (tier config says to stop)
        if (interval === null) {
            this.heartbeatState.maxDurationReached = true;
            this.stopHeartbeat(true);
            if (this.config.debug) {
                console.log('[NotifuseAnalytics] Heartbeat stopped by tier configuration');
            }
            return;
        }
        // Calculate target time with drift compensation
        const targetTime = this.heartbeatState.lastPingTime + interval;
        const now = monotonicNow();
        const delay = Math.max(0, targetTime - now);
        // Schedule next ping
        this.heartbeatTimeout = setTimeout(() => {
            // CRITICAL: Check visibility and state before sending
            if (this.shouldSendPing()) {
                const actualTime = monotonicNow();
                const drift = actualTime - targetTime;
                // Log excessive drift in debug mode
                if (drift > 1000 && this.config?.debug) {
                    console.warn(`[NotifuseAnalytics] Heartbeat drift: ${drift}ms`);
                }
                // Update tier index for metadata
                const tierResult = this.getCurrentTier();
                if (tierResult) {
                    this.heartbeatState.currentTierIndex = tierResult.index;
                }
                // Send ping with SessionState payload
                this.sendPingEvent();
                // Update last ping time for next calculation
                this.heartbeatState.lastPingTime = actualTime;
                // Schedule next ping
                this.scheduleNextHeartbeat();
            }
        }, delay);
    }
    /**
     * Check if we should send a ping right now.
     * Guards against race conditions with visibility changes.
     */
    shouldSendPing() {
        return (!this.isPaused &&
            this.isTracking &&
            this.heartbeatState.isActive &&
            !document.hidden &&
            document.visibilityState === 'visible');
    }
    /**
     * Send ping event with SessionState payload
     */
    sendPingEvent() {
        if (!this.sessionState)
            return;
        // Update scroll from ScrollTracker
        if (this.scrollTracker) {
            this.sessionState.updateScroll(this.scrollTracker.getMaxScrollPercent());
        }
        // Send periodic payload (non-blocking). ensureFreshSession both records the
        // activity this beat represents and rotates a session that has aged out; it
        // sends on its own when it rotates, so only the un-rotated path sends here.
        this.ensureFreshSession()
            .then((same) => (same ? this.sendPayload() : undefined))
            .catch(() => { });
    }
    /**
     * Stop heartbeat with optional time accumulation
     */
    stopHeartbeat(accumulateTime = true) {
        // Accumulate active time before stopping
        if (accumulateTime && this.heartbeatState.isActive) {
            const now = monotonicNow();
            const activeTime = now - this.heartbeatState.activeStartTime;
            this.heartbeatState.accumulatedActiveMs += activeTime;
            // Also accumulate page active time
            const pageTime = now - this.heartbeatState.pageStartTime;
            this.heartbeatState.pageActiveMs += pageTime;
        }
        this.heartbeatState.isActive = false;
        if (this.heartbeatTimeout) {
            clearTimeout(this.heartbeatTimeout);
            this.heartbeatTimeout = null;
        }
    }
    /**
     * Get total active time in milliseconds
     */
    getTotalActiveMs() {
        let total = this.heartbeatState.accumulatedActiveMs;
        if (this.heartbeatState.isActive) {
            total += monotonicNow() - this.heartbeatState.activeStartTime;
        }
        return total;
    }
    /**
     * Check and update max duration flag
     */
    checkAndUpdateMaxDuration() {
        if (!this.config || this.config.heartbeatMaxDuration === 0) {
            return false; // Unlimited
        }
        const totalActiveMs = this.getTotalActiveMs();
        if (totalActiveMs >= this.config.heartbeatMaxDuration) {
            this.heartbeatState.maxDurationReached = true;
            if (this.config.debug) {
                const tierResult = this.getCurrentTier();
                console.log(`[NotifuseAnalytics] Heartbeat max duration reached ` +
                    `(${Math.round(totalActiveMs / 1000)}s active, tier ${tierResult?.index ?? 0})`);
            }
            return true;
        }
        return false;
    }
    /**
     * Get current tier based on active time
     */
    getCurrentTier() {
        if (!this.config)
            return null;
        const totalActiveMs = this.getTotalActiveMs();
        const tiers = this.config.heartbeatTiers;
        // Find the highest tier that applies (tiers sorted by 'after' ascending)
        let currentTier = tiers[0];
        let currentIndex = 0;
        for (let i = 0; i < tiers.length; i++) {
            if (totalActiveMs >= tiers[i].after) {
                currentTier = tiers[i];
                currentIndex = i;
            }
            else {
                break;
            }
        }
        return { tier: currentTier, index: currentIndex };
    }
    /**
     * Get current interval based on tier and device type
     */
    getCurrentInterval() {
        const result = this.getCurrentTier();
        if (!result)
            return null;
        const { tier } = result;
        return this.isMobileDevice ? tier.mobileInterval : tier.desktopInterval;
    }
    /**
     * Reset heartbeat state completely
     */
    resetHeartbeatState() {
        this.stopHeartbeat(false);
        this.heartbeatState = {
            activeStartTime: 0,
            accumulatedActiveMs: 0,
            isActive: false,
            maxDurationReached: false,
            lastPingTime: 0,
            currentTierIndex: 0,
            pageActiveMs: 0,
            pageStartTime: monotonicNow(),
        };
    }
    /**
     * Reset page active time only (for SPA navigation)
     */
    resetPageActiveTime() {
        // Keep session time, reset page time
        this.heartbeatState.pageActiveMs = 0;
        this.heartbeatState.pageStartTime = monotonicNow();
    }
    /**
     * Get current page's accumulated focus time in milliseconds.
     * This only counts time when the tab is visible/focused.
     * Used by SessionState to set accurate page duration.
     */
    getPageActiveMs() {
        let total = this.heartbeatState.pageActiveMs;
        if (this.heartbeatState.isActive) {
            // Add time since last focus start
            total += monotonicNow() - this.heartbeatState.pageStartTime;
        }
        return total;
    }
    /**
     * Validate and normalize heartbeat tiers
     */
    validateTiers(tiers) {
        if (!tiers || tiers.length === 0) {
            return DEFAULT_HEARTBEAT_TIERS;
        }
        // Sort by 'after' ascending
        const sorted = [...tiers].sort((a, b) => a.after - b.after);
        // Ensure first tier starts at 0
        if (sorted[0].after !== 0) {
            sorted.unshift({
                after: 0,
                desktopInterval: 10000,
                mobileInterval: 7000,
            });
        }
        // Enforce minimum intervals
        return sorted.map((tier) => ({
            ...tier,
            desktopInterval: tier.desktopInterval === null
                ? null
                : Math.max(tier.desktopInterval, MIN_HEARTBEAT_INTERVAL),
            mobileInterval: tier.mobileInterval === null
                ? null
                : Math.max(tier.mobileInterval, MIN_HEARTBEAT_INTERVAL),
        }));
    }
    /**
     * Schedule a debounced send (for rapid navigations)
     */
    scheduleDebouncedSend() {
        if (this.sendDebounceTimeout) {
            clearTimeout(this.sendDebounceTimeout);
        }
        this.sendDebounceTimeout = setTimeout(async () => {
            this.sendDebounceTimeout = null;
            await this.sendPayload();
        }, SEND_DEBOUNCE_MS);
    }
    /**
     * Send session payload to server
     * V3: Always sends all actions, always includes attributes.
     * Server uses ReplacingMergeTree to deduplicate events.
     */
    async sendPayload() {
        if (!this.sessionState || !this.sender)
            return;
        const attributes = this.buildAttributes();
        const payload = this.sessionState.buildPayload(attributes, {
            identity: this.sessionManager?.getIdentity() ?? null,
            dimensions: this.sessionManager?.getDimensionsPayload() ?? {},
        });
        const result = await this.sender.sendSession(payload);
        if (result.success) {
            // Persist state after successful send
            this.sessionState.persist();
        }
    }
    /**
     * Build session attributes from current state
     */
    buildAttributes() {
        const session = this.sessionManager?.getSession();
        const device = this.deviceInfo;
        return {
            landing_page: session?.landing_page || window.location.href,
            referrer: session?.referrer || undefined,
            utm_source: session?.utm?.source || undefined,
            utm_medium: session?.utm?.medium || undefined,
            utm_campaign: session?.utm?.campaign || undefined,
            utm_term: session?.utm?.term || undefined,
            utm_content: session?.utm?.content || undefined,
            utm_id: session?.utm?.id || undefined,
            utm_id_from: session?.utm?.id_from || undefined,
            screen_width: device?.screen_width,
            screen_height: device?.screen_height,
            viewport_width: device?.viewport_width,
            viewport_height: device?.viewport_height,
            device: device?.device,
            browser: device?.browser,
            browser_type: device?.browser_type || undefined,
            os: device?.os,
            user_agent: device?.user_agent,
            connection_type: device?.connection_type,
            language: device?.language,
            timezone: device?.timezone,
        };
    }
    // Public API
    /**
     * Get session ID
     */
    async getSessionId() {
        await this.ensureInitialized();
        return this.sessionManager?.getSessionId() || '';
    }
    /**
     * Get focus duration in milliseconds
     * In V3, this is calculated from pageview durations in actions[].
     * Current page uses live focus time from heartbeatState.
     */
    async getFocusDuration() {
        await this.ensureInitialized();
        if (!this.sessionState)
            return 0;
        // Sum all pageview durations from actions
        const actions = this.sessionState.getActions();
        let total = 0;
        const currentPage = this.sessionState.getCurrentPage();
        for (let i = 0; i < actions.length; i++) {
            const action = actions[i];
            if (action.type !== 'pageview')
                continue;
            // Check if this is the current page (by page_number match)
            if (currentPage && action.page_number === currentPage.page_number) {
                // Current page: use live focus time from heartbeatState
                total += this.getPageActiveMs();
            }
            else {
                // Completed page: use stored duration
                total += action.duration;
            }
        }
        return total;
    }
    /**
     * Get total duration in milliseconds
     */
    async getTotalDuration() {
        await this.ensureInitialized();
        const session = this.sessionManager?.getSession();
        if (!session)
            return 0;
        // created_at is a wall-clock stamp, so this subtraction stays on the wall
        // clock — but clamp it, because a backward step would otherwise report a
        // negative session age.
        return Math.max(0, Date.now() - session.created_at);
    }
    /**
     * Track page view
     */
    async trackPageView(url) {
        await this.ensureInitialized();
        if (!this.sessionState)
            return;
        // Update scroll before navigation
        if (this.scrollTracker) {
            this.sessionState.updateScroll(this.scrollTracker.getMaxScrollPercent());
        }
        // A lapsed window rotates here rather than silently waiting for the next
        // full page load; the rotation opens the pageview on the new session itself.
        if (!(await this.ensureFreshSession(url)))
            return;
        const path = url || window.location.pathname;
        this.sessionState.addPageview(path);
        // Reset scroll tracking
        this.scrollTracker?.reset();
        // Reset page timer
        this.resetPageActiveTime();
        // Debounced send
        this.scheduleDebouncedSend();
        // Persist state
        this.sessionState.persist();
    }
    /**
     * Track goal (immediate send)
     */
    async trackGoal(data) {
        await this.ensureInitialized();
        // Checked here, not before ensureInitialized: a call made before init should
        // still say the SDK is not configured, which is the more useful complaint.
        if (!data || !VALID_GOAL_TYPES.includes(data.type)) {
            throw new Error(`trackGoal requires a type, one of: ${VALID_GOAL_TYPES.join(', ')}`);
        }
        if (!this.sessionState)
            return;
        // Rotate first if the window lapsed, so the goal lands on the live session
        // rather than one the server will reject.
        await this.ensureFreshSession();
        // Add goal to SessionState
        this.sessionState.addGoal(data.action, data.type, data.value, data.properties);
        // Cancel any pending debounced send
        if (this.sendDebounceTimeout) {
            clearTimeout(this.sendDebounceTimeout);
            this.sendDebounceTimeout = null;
        }
        // Immediate send for goals (critical for conversion timing)
        await this.sendPayload();
        // Persist state
        this.sessionState.persist();
    }
    /**
     * Set custom dimension
     */
    async setDimension(index, value) {
        await this.ensureInitialized();
        this.sessionManager?.setDimension(index, value);
    }
    /**
     * Set multiple dimensions
     */
    async setDimensions(dimensions) {
        await this.ensureInitialized();
        this.sessionManager?.setDimensions(dimensions);
    }
    /**
     * Get dimension value
     */
    async getDimension(index) {
        await this.ensureInitialized();
        return this.sessionManager?.getDimension(index) || null;
    }
    /**
     * Clear all dimensions
     */
    async clearDimensions() {
        await this.ensureInitialized();
        this.sessionManager?.clearDimensions();
    }
    /**
     * Attach a verified contact identity.
     *
     * The hmac must be minted server-side by the customer over the raw address
     * with their workspace secret. /track is public and unauthenticated, so an
     * unsigned address is discarded — passing one would look like identification
     * while silently doing nothing.
     */
    async identify(email, hmac) {
        await this.ensureInitialized();
        if (typeof email !== 'string' || typeof hmac !== 'string' || !email || !hmac) {
            throw new Error('identify(email, hmac) requires both arguments');
        }
        this.sessionManager?.setIdentity({ email, hmac });
    }
    /**
     * Current identity, or null when anonymous.
     */
    async getIdentity() {
        await this.ensureInitialized();
        return this.sessionManager?.getIdentity() ?? null;
    }
    /**
     * Stop future beats carrying the identity.
     *
     * Does NOT anonymize what has already been recorded — see
     * SessionManager.clearIdentity.
     */
    async clearIdentity() {
        await this.ensureInitialized();
        this.sessionManager?.clearIdentity();
    }
    /**
     * Pause tracking
     */
    async pause() {
        await this.ensureInitialized();
        this.isPaused = true;
        this.stopHeartbeat(true); // Accumulate time
    }
    /**
     * Resume tracking
     */
    async resume() {
        await this.ensureInitialized();
        this.isPaused = false;
        // Fold any in-flight time into the page total before restarting.
        this.stopHeartbeat(true);
        // Clearing the cap is what lets resume() restart tracking after
        // heartbeatMaxDuration. Only the cap counter is cleared: accumulatedActiveMs
        // drives when to stop pinging, while pageActiveMs is the page's measured
        // engagement and must survive a pause/resume.
        this.heartbeatState.accumulatedActiveMs = 0;
        this.heartbeatState.currentTierIndex = 0;
        this.heartbeatState.maxDurationReached = false;
        this.sessionState?.reopenCurrentPage();
        this.resumeHeartbeat();
    }
    /**
     * Reset session
     */
    /**
     * Roll onto a fresh session id while keeping identity and custom dimensions.
     *
     * Distinct from reset(), which deliberately forgets the visitor. Rotation
     * happens when the inactivity window lapses or the id reaches its absolute
     * age. Whatever the old session accumulated since its last successful beat is
     * sent FIRST, under the old id — without that, every rotation silently drops
     * its own tail, trading a 48h cliff for a small guaranteed loss each time.
     */
    async rotateSession(path) {
        if (!this.sessionManager || !this.config || !this.sessionState)
            return;
        this.sessionState.finalizeForUnload();
        await this.sendPayload();
        const session = this.sessionManager.getOrCreateSession();
        this.sessionState = new SessionState({
            workspace_id: this.config.workspace_id,
            session_id: session.id,
            created_at: session.created_at,
            tab_id: this.sessionManager.getTabId(),
        });
        this.sessionState.setFocusTimeGetter(() => this.getPageActiveMs());
        this.sessionState.addPageview(path || window.location.pathname);
        this.scrollTracker?.reset();
        this.resetHeartbeatState();
        this.startHeartbeat();
    }
    /**
     * Record activity, rotating the session if its window has lapsed.
     *
     * Returns false when a rotation happened, so a caller that was about to open a
     * pageview knows the rotation already did it. Calling this on every beat,
     * navigation and goal is what makes the session window measure activity rather
     * than time since page load.
     */
    async ensureFreshSession(path) {
        if (!this.sessionManager)
            return true;
        if (this.sessionManager.touch())
            return true;
        await this.rotateSession(path);
        return false;
    }
    async reset() {
        await this.ensureInitialized();
        if (!this.sessionManager || !this.config)
            return;
        // Create new session
        this.sessionManager.reset();
        const session = this.sessionManager.getOrCreateSession();
        // Reinitialize SessionState with new session
        const sessionStateConfig = {
            workspace_id: this.config.workspace_id,
            session_id: session.id,
            created_at: session.created_at,
            tab_id: this.sessionManager.getTabId(),
        };
        this.sessionState = new SessionState(sessionStateConfig);
        // Wire up focus time getter for accurate page duration tracking
        this.sessionState.setFocusTimeGetter(() => this.getPageActiveMs());
        this.sessionState.addPageview(window.location.pathname);
        // Reset scroll and heartbeat
        this.scrollTracker?.reset();
        this.resetHeartbeatState();
        this.startHeartbeat();
        // Send initial payload for new session
        await this.sendPayload();
    }
    /**
     * Get current configuration (defensive copy)
     */
    getConfig() {
        if (!this.config)
            return null;
        return { ...this.config };
    }
    /**
     * Get debug info
     */
    debug() {
        return {
            session: this.sessionManager?.getSession() || null,
            config: this.config,
            isTracking: this.isTracking,
            actionsCount: this.sessionState?.getActions().length || 0,
            currentPage: this.sessionState?.getCurrentPage()?.path || null,
            pageActiveMs: this.getPageActiveMs(),
        };
    }
    /**
     * Decorate URL with cross-domain session params
     * Use this for programmatic navigation (window.location.href, window.open)
     */
    async decorateUrl(url) {
        await this.ensureInitialized();
        if (!this.crossDomainLinker) {
            return url; // Return unchanged if cross-domain not configured
        }
        return this.crossDomainLinker.decorateUrl(url);
    }
    /**
     * Ensure SDK is initialized (awaits init promise if needed)
     */
    async ensureInitialized() {
        if (this.isInitialized)
            return;
        if (!this.initPromise) {
            throw new Error('Notifuse Analytics not configured. Set window.NotifuseAnalyticsConfig before loading the SDK.');
        }
        await this.initPromise;
    }
}

/**
 * Notifuse Analytics SDK v5.0
 * Ultra-reliable web analytics for tracking TimeScore metrics
 *
 * @example
 * ```html
 * <script>
 * window.NotifuseAnalyticsConfig = {
 *   workspace_id: 'ws_abc123',
 *   endpoint: 'https://your-api.com',
 * };
 * </script>
 * <script async src="notifuse-analytics.min.js"></script>
 * ```
 *
 * Then use the SDK (all methods are async):
 * ```typescript
 * // Track custom dimension programmatically
 * await NotifuseAnalytics.setDimension(1, 'premium-user');
 *
 * // Track goal
 * await NotifuseAnalytics.trackGoal({
 *   action: 'purchase',
 *   type: 'purchase',
 *   value: 99.99,
 *   properties: { currency: 'USD', order_id: 'A-1234' },
 * });
 * ```
 *
 * Custom dimensions can also be set via URL parameters:
 * ```
 * https://example.com/page?custom_1=campaign_a&custom_2=variant_b
 * ```
 *
 * URL parameters custom_1 through custom_10 are automatically captured on init.
 * Existing dimension values take priority over URL parameters.
 */
/*
 * Kept out of the JSDoc above on purpose: rollup emits that block into
 * dist/*.d.ts and the unminified bundles, which ship to npm consumers. This
 * note is for maintainers only.
 *
 * Renaming or re-shaping anything in the public API: `npm run type-check`
 * passing does NOT mean the change is complete. These mirror the public surface
 * and none of them is typechecked —
 *   tests/e2e/fixtures/test-page.html   (inline JS)
 *   tests/e2e/*.spec.ts                 (tsconfig excludes tests/)
 *   README.md                           (examples)
 *   the @example block above
 *   ../../docs/web-analytics/*.mdx      (separate repository)
 * Budget a manual grep sweep for the old name.
 */
// Create singleton instance
const sdk = new NotifuseAnalyticsSDK();
// Public API wrapper with both auto-init and manual init support
const NotifuseAnalytics = {
    init: (config) => sdk.init(config),
    getSessionId: () => sdk.getSessionId(),
    getConfig: () => sdk.getConfig(),
    getFocusDuration: () => sdk.getFocusDuration(),
    getTotalDuration: () => sdk.getTotalDuration(),
    trackPageView: (url) => sdk.trackPageView(url),
    trackGoal: (data) => sdk.trackGoal(data),
    setDimension: (index, value) => sdk.setDimension(index, value),
    setDimensions: (dimensions) => sdk.setDimensions(dimensions),
    getDimension: (index) => sdk.getDimension(index),
    clearDimensions: () => sdk.clearDimensions(),
    identify: (email, hmac) => sdk.identify(email, hmac),
    getIdentity: () => sdk.getIdentity(),
    clearIdentity: () => sdk.clearIdentity(),
    pause: () => sdk.pause(),
    resume: () => sdk.resume(),
    reset: () => sdk.reset(),
    debug: () => sdk.debug(),
    decorateUrl: (url) => sdk.decorateUrl(url),
};
/**
 * Make installing the bundle idempotent.
 *
 * The same bundle is served at /na.js and /na.<hash>.js, so a site mid-migration
 * (legacy hardcoded tag plus a tag-manager tag) evaluates it twice. The
 * re-entrancy guard inside the SDK is per-instance, so nothing would stop the
 * second copy initialising — and both copies would then share one session id,
 * one persisted snapshot key and one tab_id, clobbering each other's actions and
 * colliding on seq. tab_id cannot separate them: sessionStorage is per tab, not
 * per instance, so the fix has to be here, at the install.
 *
 * Returning the FIRST wrapper matters as much as skipping the second init: the
 * UMD wrapper assigns window.NotifuseAnalytics from this default export, so a
 * fresh object would leave the global pointing at a dead instance and every
 * later trackGoal() would land there. First install wins — a customer may have
 * an older cached hash alongside the new one, and tearing down a running
 * instance to promote a newer bundle is more dangerous than running the older
 * one until the duplicate tag is removed.
 *
 * A plain flag is enough: script evaluation is atomic on a single thread, so
 * there is no race even with async tags.
 */
const INSTALL_KEY = '__notifuseAnalytics';
const host = (typeof window !== 'undefined' ? window : undefined);
const alreadyInstalled = host?.[INSTALL_KEY];
if (!alreadyInstalled) {
    if (host) {
        host[INSTALL_KEY] = NotifuseAnalytics;
    }
    // Auto-initialize from global config
    if (typeof window !== 'undefined' && window.NotifuseAnalyticsConfig) {
        sdk.init(window.NotifuseAnalyticsConfig);
    }
}
else {
    console.warn('[NotifuseAnalytics] SDK already loaded on this page; ignoring the duplicate ' +
        'install. Remove the extra script tag — two copies would corrupt each ' +
        "other's session data.");
}
// Default export for UMD/ESM/CJS
var index = alreadyInstalled ?? NotifuseAnalytics;

export { index as default };
//# sourceMappingURL=notifuse-analytics.esm.js.map
