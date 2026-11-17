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

/**
 * The prefix sent: the typed one without its leading and trailing `_` and `-`, which the field keeps
 * while typing (a space becomes `_` at once, so « website » and a space would be sent as `website_`).
 */
export function finalApiKeyPrefix(typed: string): string {
  return toApiKeyPrefix(typed).replace(/^[_-]+|[_-]+$/g, '')
}

/**
 * The domain of an API key's address, as the server mints it from the API endpoint: its host,
 * without scheme, path or port ("key@localhost:4000" is no email address).
 */
export function apiKeyDomain(apiEndpoint: string | undefined): string {
  const fallback = 'api.example.com'
  if (!apiEndpoint) return fallback
  const withScheme = /^https?:\/\//.test(apiEndpoint) ? apiEndpoint : `https://${apiEndpoint}`
  try {
    return new URL(withScheme).hostname.replace(/^\[|\]$/g, '') || fallback
  } catch {
    return fallback
  }
}
