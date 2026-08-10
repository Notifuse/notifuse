/**
 * Cross-Domain Session Linker
 * Handles URL decoration for cross-domain session continuity
 * Privacy-first: No cookies, URL parameters only
 */

/**
 * Default cross-domain URL parameter. Configurable via `crossDomainParam`:
 * it lands in the customer's own URLs, server logs and analytics reports.
 */
export const DEFAULT_CROSS_DOMAIN_PARAM = '_nf';
const UUID_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const CLOCK_SKEW_TOLERANCE = 60; // seconds

export interface CrossDomainPayload {
  s: string; // session_id
  t: number; // timestamp (Unix epoch seconds)
}

export interface CrossDomainConfig {
  domains: string[];
  expiry: number; // seconds
  debug: boolean;
  paramName: string;
}

/**
 * Encode payload to Base64URL string
 */
export function encode(payload: CrossDomainPayload): string {
  return btoa(JSON.stringify(payload))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

/**
 * Decode Base64URL string to payload
 * Returns null if invalid
 */
export function decode(encoded: string): CrossDomainPayload | null {
  try {
    // Convert Base64URL to standard Base64
    const base64 = encoded.replace(/-/g, '+').replace(/_/g, '/');
    const json = atob(base64);
    const payload = JSON.parse(json);

    // Validate required fields exist
    if (
      typeof payload.s !== 'string' ||
      typeof payload.t !== 'number'
    ) {
      return null;
    }

    // Validate UUID format
    if (!UUID_REGEX.test(payload.s)) {
      return null;
    }

    return payload as CrossDomainPayload;
  } catch {
    return null;
  }
}

export class CrossDomainLinker {
  private config: CrossDomainConfig;
  private getSessionId: () => string = () => '';
  private clickHandler: ((e: MouseEvent) => void) | null = null;
  private submitHandler: ((e: SubmitEvent) => void) | null = null;

  constructor(config: CrossDomainConfig) {
    this.config = config;
  }

  /**
   * Set ID getter function
   */
  setIdGetters(getSessionId: () => string): void {
    this.getSessionId = getSessionId;
  }

  /**
   * Start listening for link clicks and form submissions
   */
  start(): void {
    this.clickHandler = (e: MouseEvent) => this.handleClick(e);
    this.submitHandler = (e: SubmitEvent) => this.handleSubmit(e);

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
  stop(): void {
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
  private handleClick(e: MouseEvent): void {
    const target = (e.target as Element)?.closest('a');
    if (!target || !target.href) return;

    if (!this.shouldDecorate(target.href)) return;

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
  private handleSubmit(e: SubmitEvent): void {
    const form = e.target as HTMLFormElement;
    if (!form || !form.action) return;

    // Only handle GET forms
    if (form.method.toLowerCase() !== 'get') return;

    if (!this.shouldDecorate(form.action)) return;

    const sessionId = this.getSessionId();

    if (!sessionId) return;

    const payload: CrossDomainPayload = {
      s: sessionId,
      t: Math.floor(Date.now() / 1000),
    };

    // Remove existing hidden input if present
    const existing = form.querySelector(`input[name="${this.config.paramName}"]`);
    if (existing) existing.remove();

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
  decorateUrl(url: string): string {
    const sessionId = this.getSessionId();

    if (!sessionId) {
      return url;
    }

    if (!this.shouldDecorate(url)) {
      return url;
    }

    try {
      const parsed = new URL(url, window.location.origin);
      const payload: CrossDomainPayload = {
        s: sessionId,
        t: Math.floor(Date.now() / 1000),
      };

      parsed.searchParams.set(this.config.paramName, encode(payload));
      return parsed.toString();
    } catch {
      return url;
    }
  }

  /**
   * Check if URL should be decorated
   */
  shouldDecorate(url: string): boolean {
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
        return (
          normalizedTarget === normalizedDomain ||
          normalizedTarget.endsWith('.' + normalizedDomain)
        );
      });
    } catch {
      return false;
    }
  }

  /**
   * Normalize hostname (remove www. prefix)
   */
  private normalizeHostname(hostname: string): string {
    return hostname.toLowerCase().replace(/^www\./, '');
  }

  /**
   * Read cross-domain parameter from URL (static)
   * Returns null if not found or invalid
   */
  static readParam(
    expiry: number,
    paramName: string = DEFAULT_CROSS_DOMAIN_PARAM
  ): CrossDomainPayload | null {
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
    } catch {
      return null;
    }
  }

  /**
   * Strip cross-domain parameter from URL (static)
   */
  static stripParam(paramName: string = DEFAULT_CROSS_DOMAIN_PARAM): void {
    try {
      const url = new URL(window.location.href);

      if (!url.searchParams.has(paramName)) {
        return;
      }

      url.searchParams.delete(paramName);

      // Build clean URL
      const cleanPath =
        window.location.pathname +
        (url.searchParams.toString() ? '?' + url.searchParams.toString() : '') +
        window.location.hash;

      window.history.replaceState(window.history.state, '', cleanPath);
    } catch {
      // Ignore errors (some browsers may restrict replaceState)
    }
  }
}
