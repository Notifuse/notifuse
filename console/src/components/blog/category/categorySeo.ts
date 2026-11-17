import type { SEOSettings } from '../../../services/api/blog'
import type { EffectiveSeo } from '../../seo/SeoPreview'
import { firstFilled } from '../../seo/firstFilled'

export interface CategoryDraft {
  name?: string
  slug?: string
  description?: string
  seo?: SEOSettings
}

export interface BlogDefaults {
  /** The workspace's name, which the default theme adds to a page title. */
  workspaceName: string
  /** The blog's own SEO settings (Settings › Blog), used when a category leaves a field empty. */
  seo?: SEOSettings
}

/** What a category page shows to search engines and on social networks. */
export type EffectiveCategorySeo = EffectiveSeo

const filled = firstFilled

/**
 * The tags of a category page as the default theme writes them (themePresets, the head partial):
 * the title falls back to « name - workspace », the share title to the name, both descriptions to
 * the category's description, then each to the blog's own settings. A theme of one's own may differ.
 */
export function effectiveCategorySeo(draft: CategoryDraft, defaults: BlogDefaults): EffectiveCategorySeo {
  const seo = draft.seo ?? {}
  const blog = defaults.seo ?? {}
  const name = draft.name?.trim() ?? ''
  return {
    title: filled(seo.meta_title, name ? `${name} - ${defaults.workspaceName}` : undefined, blog.meta_title, defaults.workspaceName),
    description: filled(seo.meta_description, draft.description, blog.meta_description),
    ogTitle: filled(seo.og_title, name, blog.og_title, defaults.workspaceName),
    ogDescription: filled(seo.og_description, draft.description, blog.og_description),
    ogImage: filled(seo.og_image, blog.og_image)
  }
}

/** The address of the blog's home: the workspace's custom domain, or the API's. */
export function blogBaseUrl(customEndpointUrl: string | undefined, apiEndpoint: string | undefined): string {
  return (customEndpointUrl || apiEndpoint || 'https://example.com').replace(/\/+$/, '')
}

const clean = (value: unknown): unknown => {
  if (typeof value === 'string') return value.trim() === '' ? undefined : value
  if (Array.isArray(value)) return value.length === 0 ? undefined : value
  if (value && typeof value === 'object') {
    const entries = Object.entries(value)
      .map(([key, v]) => [key, clean(v)] as const)
      .filter(([, v]) => v !== undefined)
      .sort(([a], [b]) => a.localeCompare(b))
    return entries.length === 0 ? undefined : Object.fromEntries(entries)
  }
  return value ?? undefined
}

/** Two drafts are the same when they differ only by empty fields. */
export function sameDraft(a: CategoryDraft | undefined, b: CategoryDraft | undefined): boolean {
  return JSON.stringify(clean(a ?? {}) ?? {}) === JSON.stringify(clean(b ?? {}) ?? {})
}
