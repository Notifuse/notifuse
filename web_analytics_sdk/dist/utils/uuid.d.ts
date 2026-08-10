/**
 * UUID generation utilities
 * Uses crypto APIs for secure random generation (available in all ES2017+ browsers)
 */
/**
 * Generate a UUIDv4
 * Uses native crypto.randomUUID() when available (2-3x faster),
 * falls back to crypto.getRandomValues() for older browsers
 */
export declare function generateUUIDv4(): string;
/**
 * Generate a UUIDv7 (time-sortable)
 * Format: timestamp (48 bits) + version (4 bits) + random (12 bits) + variant (2 bits) + random (62 bits)
 */
export declare function generateUUIDv7(): string;
/**
 * Random identifier for one browser tab, as a JS-safe integer.
 *
 * Lands in a BIGINT column that forms part of the web_pages and web_goals
 * primary keys, so it only has to be unique among one session's tabs — a UUID
 * would add 16 bytes to the highest-volume partitioned table and widen its PK
 * index for uniqueness nobody needs. 53 bits keeps the value an exact float64
 * integer, so it survives JSON round-tripping without precision loss.
 */
export declare function generateTabId(): number;
