import { useLingui } from '@lingui/react/macro'
import type { SeoCheck } from './SeoChecks'
import type { SeoGap } from './seoGaps'
import type { EffectiveSeo } from './SeoPreview'

/**
 * The check of each empty SEO tag: what is missing, what the page uses instead, and the field
 * that settles it. `imageFallback` names where a fallback share image comes from.
 */
export function useSeoGapChecks(effective: EffectiveSeo, imageFallback: string): Record<SeoGap, Omit<SeoCheck, 'key' | 'ok'>> {
  const { t } = useLingui()
  // Named, so translators read « Uses “{title}” » rather than « Uses {0} »; the quote marks are
  // in the message, for each language to write its own.
  const title = effective.title
  const description = effective.description
  const shareTitle = effective.ogTitle
  const shareDescription = effective.ogDescription
  return {
    meta_title: { label: t`No meta title`, why: t`Uses “${title}”`, field: ['seo', 'meta_title'] },
    meta_description: {
      label: t`No meta description`,
      why: effective.description ? t`Uses “${description}”` : t`Search engines pick a sentence from the page`,
      field: ['seo', 'meta_description']
    },
    og_title: { label: t`No share title`, why: t`Uses “${shareTitle}”`, field: ['seo', 'og_title'] },
    og_description: {
      label: t`No share description`,
      why: effective.ogDescription ? t`Uses “${shareDescription}”` : t`Links show without a description`,
      field: ['seo', 'og_description']
    },
    og_image: {
      label: t`No share image`,
      why: effective.ogImage ? imageFallback : t`Links show without a picture`,
      field: ['seo', 'og_image']
    }
  }
}
