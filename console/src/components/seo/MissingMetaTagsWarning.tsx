import { useLingui } from '@lingui/react/macro'
import { Popover } from 'antd'
import { WarningOutlined } from '@ant-design/icons'
import type { SEOSettings } from '../../services/api/blog'
import { seoGaps, type SeoGap } from './seoGaps'

interface MissingMetaTagsWarningProps {
  seo?: SEOSettings
  /** A post's featured image, which the theme shares when the post has no share image. */
  featuredImage?: string
  className?: string
}

interface MissingTag {
  name: string
  description: string
  category: 'meta' | 'og'
}

export function MissingMetaTagsWarning({ seo, featuredImage, className }: MissingMetaTagsWarningProps) {
  const { t } = useLingui()

  const tags: Record<SeoGap, MissingTag> = {
    meta_title: {
      name: t`Meta Title`,
      description: t`SEO title for search engines (recommended: 50-60 characters)`,
      category: 'meta'
    },
    meta_description: {
      name: t`Meta Description`,
      description: t`SEO description for search results (recommended: 150-160 characters)`,
      category: 'meta'
    },
    og_title: {
      name: t`Open Graph Title`,
      description: t`Title when shared on social media (og:title)`,
      category: 'og'
    },
    og_description: {
      name: t`Open Graph Description`,
      description: t`Description when shared on social media (og:description)`,
      category: 'og'
    },
    og_image: {
      name: t`Open Graph Image`,
      description: t`Image URL when shared on social media (og:image)`,
      category: 'og'
    }
  }

  const missingTags = seoGaps(seo, { featuredImage }).map((gap) => tags[gap])

  if (missingTags.length === 0) {
    return null
  }

  // A plain list: antd's List is deprecated.
  const content = (
    <ul data-unreset className="m-0 flex max-w-[300px] list-none flex-col gap-2 p-0">
      {missingTags.map((item) => (
        <li key={item.name}>
          <div className="text-xs font-medium text-ink">{item.name}</div>
          <div className="mt-0.5 text-2xs text-ink-3">{item.description}</div>
        </li>
      ))}
    </ul>
  )

  return (
    <Popover content={content} title={t`Missing Tags`} trigger="hover">
      <WarningOutlined className={`cursor-pointer text-base text-warning-fg ${className ?? ''}`} />
    </Popover>
  )
}
