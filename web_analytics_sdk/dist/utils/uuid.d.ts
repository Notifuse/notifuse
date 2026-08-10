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
