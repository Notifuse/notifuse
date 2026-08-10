/**
 * UUID generation utilities
 * Uses crypto APIs for secure random generation (available in all ES2017+ browsers)
 */

/**
 * Generate a UUIDv4
 * Uses native crypto.randomUUID() when available (2-3x faster),
 * falls back to crypto.getRandomValues() for older browsers
 */
export function generateUUIDv4(): string {
  // Native implementation when available (Chrome 92+, Firefox 95+, Safari 15.4+)
  if (crypto.randomUUID) {
    return crypto.randomUUID();
  }

  // Secure fallback using getRandomValues (all ES2017+ browsers)
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  bytes[6] = (bytes[6] & 0x0f) | 0x40; // Version 4
  bytes[8] = (bytes[8] & 0x3f) | 0x80; // Variant 10

  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

/**
 * Generate a UUIDv7 (time-sortable)
 * Format: timestamp (48 bits) + version (4 bits) + random (12 bits) + variant (2 bits) + random (62 bits)
 */
export function generateUUIDv7(): string {
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
