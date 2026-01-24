import { useLingui } from '@lingui/react/macro'
import { Popover, List } from 'antd'
import { WarningOutlined } from '@ant-design/icons'
import type { SEOSettings } from '../../services/api/blog'

interface MissingMetaTagsWarningProps {
  seo?: SEOSettings
  className?: string
}

interface MissingTag {
  name: string
  description: string
  category: 'meta' | 'og'
}

export function MissingMetaTagsWarning({ seo, className }: MissingMetaTagsWarningProps) {
  const { t } = useLingui()

  const getMissingTags = (): MissingTag[] => {
    const missing: MissingTag[] = []

    // Meta tags
    if (!seo?.meta_title || seo.meta_title.trim() === '') {
      missing.push({
        name: t`Meta Title`,
        description: t`SEO title for search engines (recommended: 50-60 characters)`,
        category: 'meta'
      })
    }

    if (!seo?.meta_description || seo.meta_description.trim() === '') {
      missing.push({
        name: t`Meta Description`,
        description: t`SEO description for search results (recommended: 150-160 characters)`,
        category: 'meta'
      })
    }

    // Open Graph tags
    if (!seo?.og_title || seo.og_title.trim() === '') {
      missing.push({
        name: t`Open Graph Title`,
        description: t`Title when shared on social media (og:title)`,
        category: 'og'
      })
    }

    if (!seo?.og_description || seo.og_description.trim() === '') {
      missing.push({
        name: t`Open Graph Description`,
        description: t`Description when shared on social media (og:description)`,
        category: 'og'
      })
    }

    if (!seo?.og_image || seo.og_image.trim() === '') {
      missing.push({
        name: t`Open Graph Image`,
        description: t`Image URL when shared on social media (og:image)`,
        category: 'og'
      })
    }

    return missing
  }

  const missingTags = getMissingTags()

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
