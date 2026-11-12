/**
 * Normalizers for the `validateSearch` of the routes. The router's default parser runs JSON.parse
 * on each value, so a file named "2024" arrives as a number and a folder named "true" as a boolean:
 * each normalizer turns those back into the text of the URL, and drops what cannot be one.
 */

/** A search value as a non-empty, trimmed string, or undefined. */
export function searchString(value: unknown): string | undefined {
  const single = Array.isArray(value) ? value[0] : value
  if (typeof single === 'number' || typeof single === 'boolean') return String(single)
  if (typeof single !== 'string') return undefined
  const trimmed = single.trim()
  return trimmed === '' ? undefined : trimmed
}

/** Search of `/settings/$section`: the webhook whose detail panel is open. */
export interface SettingsSearch {
  webhook?: string
}

export function parseSettingsSearch(search: Record<string, unknown>): SettingsSearch {
  const webhook = searchString(search.webhook)
  // Optional output: a Link to a section needs no search, and an absent key stays absent.
  return webhook === undefined ? {} : { webhook }
}
