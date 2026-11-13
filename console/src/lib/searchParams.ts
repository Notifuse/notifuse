/**
 * Normalizers for the `validateSearch` of the routes. The router's default parser runs JSON.parse
 * on each value, so a file named "2024" arrives as a number and a folder named "true" as a boolean:
 * each normalizer turns those back into the text of the URL, and drops what cannot be one.
 */

import type { BlogPostStatus } from '../services/api/blog'
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

/**
 * A value read straight from `location.search`, as the router writes it: the router quotes a text
 * that would otherwise parse as JSON (an id "12345" is written `%2212345%22`), so the quotes are
 * taken off again. For the screens that still read the location themselves.
 */
export function rawSearchValue(params: URLSearchParams, key: string): string | null {
  const value = params.get(key)
  if (value === null || value.length < 2 || !value.startsWith('"') || !value.endsWith('"'))
    return value
  try {
    const parsed: unknown = JSON.parse(value)
    return typeof parsed === 'string' ? parsed : value
  } catch {
    return value
  }
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

/** Search of `/blog`: status tab, category and title search. */
export interface BlogSearch {
  status?: BlogPostStatus
  category_id?: string
  q?: string
}

// Kept here rather than imported from the API module, which the router would then load early.
const BLOG_POST_STATUSES: readonly BlogPostStatus[] = ['all', 'draft', 'published', 'scheduled']

const isBlogPostStatus = (value: string | undefined): value is BlogPostStatus =>
  value !== undefined && (BLOG_POST_STATUSES as readonly string[]).includes(value)

export function parseBlogSearch(search: Record<string, unknown>): BlogSearch {
  const status = searchString(search.status)
  const categoryId = searchString(search.category_id)
  const q = searchString(search.q)
  return {
    // An unknown status would reach the server, which answers 400.
    ...(isBlogPostStatus(status) ? { status } : {}),
    ...(categoryId === undefined ? {} : { category_id: categoryId }),
    ...(q === undefined ? {} : { q })
  }
}

/** Search of `/file-manager`: the open folder and the previewed file. */
export interface FileManagerSearch {
  path?: string
  file?: string
}

export function parseFileManagerSearch(search: Record<string, unknown>): FileManagerSearch {
  const rawPath = searchString(search.path)
  // A folder is a prefix ending with "/" and never starting with one: "docs" opens "docs/".
  const trimmed = rawPath?.replace(/^\/+/, '')
  const path = trimmed ? (trimmed.endsWith('/') ? trimmed : trimmed + '/') : undefined
  const file = searchString(search.file)
  return {
    ...(path === undefined ? {} : { path }),
    ...(file === undefined ? {} : { file })
  }
}
