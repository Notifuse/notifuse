/**
 * Normalizers for the `validateSearch` of the routes. The router's default parser runs JSON.parse
 * on each value, so a file named "2024" arrives as a number and a folder named "true" as a boolean:
 * each normalizer turns those back into the text of the URL, and drops what cannot be one.
 */

import {
  isTemplateCategory,
  type TemplateCategory
} from '../components/templates/templateCategories'

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

/** Search of `/templates`: category tab, search box and the template whose preview is open. */
export interface TemplatesSearch {
  category?: TemplateCategory
  q?: string
  preview?: string
}

export function parseTemplatesSearch(search: Record<string, unknown>): TemplatesSearch {
  const category = searchString(search.category)
  const q = searchString(search.q)
  const preview = searchString(search.preview)
  return {
    ...(isTemplateCategory(category) ? { category } : {}),
    ...(q === undefined ? {} : { q }),
    ...(preview === undefined ? {} : { preview })
  }
}
