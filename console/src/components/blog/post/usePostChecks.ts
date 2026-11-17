import { useLingui } from '@lingui/react/macro'
import type { SeoCheck } from '../../seo/SeoChecks'
import { useSeoGapChecks } from '../../seo/useSeoGapChecks'
import { seoGaps } from '../../seo/seoGaps'
import type { EffectiveSeo } from '../../seo/SeoPreview'
import type { PostDraft } from './postSeo'

/** The checks a post needs: what it cannot be saved without, then the SEO tags it would fall back for. */
export function usePostChecks(draft: PostDraft, effective: EffectiveSeo): SeoCheck[] {
  const { t } = useLingui()
  const gapCheck = useSeoGapChecks(effective, t`Uses the category's or the blog's share image`)
  const missing = !draft.title?.trim() ? ['title'] : !draft.category_id ? ['category_id'] : !draft.slug?.trim() ? ['slug'] : undefined
  return [
    missing
      ? { key: 'basics', ok: false, label: t`Title, slug and category required`, field: missing }
      : { key: 'basics', ok: true, label: t`Title, slug and category` },
    ...seoGaps(draft.seo, { featuredImage: draft.featured_image_url }).map(
      (gap): SeoCheck => ({ key: gap, ok: false, ...gapCheck[gap] })
    ),
    // The theme shares the featured image when the post has no share image of its own.
    ...(!draft.seo?.og_image?.trim() && draft.featured_image_url?.trim()
      ? [{ key: 'og_image', ok: true, label: t`Share image`, why: t`The featured image` }]
      : [])
  ]
}
