import type { SEOSettings } from '../../services/api/blog'

/** The SEO fields a page should fill: the ones the sidebar's warning and the category checks list. */
export const SEO_GAP_FIELDS = ['meta_title', 'meta_description', 'og_title', 'og_description', 'og_image'] as const

export type SeoGap = (typeof SEO_GAP_FIELDS)[number]

/**
 * The fields of `SEO_GAP_FIELDS` left empty (or blank), in that order. A post's featured image is
 * its share image when it has none of its own (the theme's og:image falls back to it), so it fills
 * the og_image gap.
 */
export function seoGaps(seo?: SEOSettings | null, options: { featuredImage?: string } = {}): SeoGap[] {
  return SEO_GAP_FIELDS.filter(
    (field) => !seo?.[field]?.trim() && !(field === 'og_image' && options.featuredImage?.trim())
  )
}
