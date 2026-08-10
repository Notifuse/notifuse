/**
 * Cross-Domain Session Linker
 * Handles URL decoration for cross-domain session continuity
 * Privacy-first: No cookies, URL parameters only
 */
/**
 * Default cross-domain URL parameter. Configurable via `crossDomainParam`:
 * it lands in the customer's own URLs, server logs and analytics reports.
 */
export declare const DEFAULT_CROSS_DOMAIN_PARAM = "_nf";
export interface CrossDomainPayload {
    s: string;
    t: number;
}
export interface CrossDomainConfig {
    domains: string[];
    expiry: number;
    debug: boolean;
    paramName: string;
}
/**
 * Encode payload to Base64URL string
 */
export declare function encode(payload: CrossDomainPayload): string;
/**
 * Decode Base64URL string to payload
 * Returns null if invalid
 */
export declare function decode(encoded: string): CrossDomainPayload | null;
export declare class CrossDomainLinker {
    private config;
    private getSessionId;
    private clickHandler;
    private submitHandler;
    constructor(config: CrossDomainConfig);
    /**
     * Set ID getter function
     */
    setIdGetters(getSessionId: () => string): void;
    /**
     * Start listening for link clicks and form submissions
     */
    start(): void;
    /**
     * Stop listening for events
     */
    stop(): void;
    /**
     * Handle click events - decorate links to configured domains
     */
    private handleClick;
    /**
     * Handle form submissions - add hidden input for GET forms
     */
    private handleSubmit;
    /**
     * Decorate a URL with cross-domain parameters
     */
    decorateUrl(url: string): string;
    /**
     * Check if URL should be decorated
     */
    shouldDecorate(url: string): boolean;
    /**
     * Normalize hostname (remove www. prefix)
     */
    private normalizeHostname;
    /**
     * Read cross-domain parameter from URL (static)
     * Returns null if not found or invalid
     */
    static readParam(expiry: number, paramName?: string): CrossDomainPayload | null;
    /**
     * Strip cross-domain parameter from URL (static)
     */
    static stripParam(paramName?: string): void;
}
