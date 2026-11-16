import { useLingui } from '@lingui/react/macro'
import { Popover, List } from 'antd'
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

  const content = (
    <div style={{ maxWidth: 300 }}>
      <List
        size="small"
        dataSource={missingTags}
        renderItem={(item) => (
          <List.Item style={{ padding: '4px 0', border: 'none' }}>
            <div>
              <div style={{ fontWeight: 500, fontSize: 12 }}>{item.name}</div>
              <div style={{ fontSize: 11, color: '#666', marginTop: 2 }}>{item.description}</div>
            </div>
          </List.Item>
        )}
      />
    </div>
  )

  return (
    <Popover content={content} title={t`Missing Tags`} trigger="hover">
      <WarningOutlined
        style={{
          color: '#ff9800',
          fontSize: 16,
          cursor: 'pointer'
        }}
        className={className}
      />
    </Popover>
  )
}
