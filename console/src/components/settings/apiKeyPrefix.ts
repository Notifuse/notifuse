/** Longest part before "@" the server accepts for an API key's address. */
export const API_KEY_PREFIX_MAX_LENGTH = 64

/**
 * Turns the name typed for an API key into the part of its address before "@", as
 * the server accepts it (^[a-z0-9_-]{1,64}$): lowercase, spaces as underscores,
 * hyphens kept, anything else dropped. It runs while the name is typed, so it does
 * not trim.
 */
export function toApiKeyPrefix(name: string): string {
  return name
    .toLowerCase()
    .replace(/\s+/g, '_')
    .replace(/[^a-z0-9_-]/g, '')
    .slice(0, API_KEY_PREFIX_MAX_LENGTH)
}
