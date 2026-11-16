import type { SEOSettings } from '../../../services/api/blog'
import type { EffectiveSeo } from '../../seo/SeoPreview'
import { firstFilled } from '../../seo/firstFilled'
import type { BlogDefaults } from '../category/categorySeo'

export interface PostDraft {
  title?: string
  slug?: string
  category_id?: string
  excerpt?: string
  featured_image_url?: string
  seo?: SEOSettings
}

/** The category a post sits in, as its page receives it. */
export interface PostCategory {
  name?: string
  description?: string
  seo?: SEOSettings
}

const filled = firstFilled

/**
 * The tags of a post page as the default theme writes them (themePresets, the head partial): the
 * title falls back to « title - workspace », both descriptions to the excerpt, the share title to
 * the title and the share image to the featured image; then to the category's, then the blog's.
 */
export function effectivePostSeo(draft: PostDraft, category: PostCategory | undefined, defaults: BlogDefaults): EffectiveSeo {
  const seo = draft.seo ?? {}
  const cat = category?.seo ?? {}
  const blog = defaults.seo ?? {}
  const title = draft.title?.trim() ?? ''
  const categoryName = category?.name?.trim() ?? ''
  const workspace = defaults.workspaceName
  return {
    title: filled(
      seo.meta_title,
      title ? `${title} - ${workspace}` : undefined,
      cat.meta_title,
      categoryName ? `${categoryName} - ${workspace}` : undefined,
      blog.meta_title,
      workspace
    ),
    description: filled(seo.meta_description, draft.excerpt, cat.meta_description, category?.description, blog.meta_description),
    ogTitle: filled(seo.og_title, title, cat.og_title, categoryName, blog.og_title, workspace),
    ogDescription: filled(seo.og_description, draft.excerpt, cat.og_description, category?.description, blog.og_description),
    ogImage: filled(seo.og_image, draft.featured_image_url, cat.og_image, blog.og_image)
  }
}

/** The words of a post's text, for the writing summary. */
export function countWords(text: string): number {
  return text.match(/\S+/g)?.length ?? 0
}
