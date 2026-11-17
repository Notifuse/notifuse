import { useLingui } from '@lingui/react/macro'
import { Form, Input, Select } from 'antd'
import { ImageOff, Search, Share2 } from 'lucide-react'
import { Panel } from '../layout'
import { ImageURLInput } from '../common/ImageURLInput'

const { TextArea } = Input

export interface SearchEnginesCardProps {
  /** Beside the title: what the card sets (« How the post appears in search results »). */
  hint: string
  /** What an empty field falls back to, as the theme writes it. */
  titlePlaceholder: string
  descriptionPlaceholder: string
  /** The address the page is served at, the canonical one when the field is empty. */
  canonicalPlaceholder: string
}

/**
 * The tags search engines read: meta title and description, keywords, canonical URL, under
 * `seo`. No indexing field: the default theme writes the blog's robots tag, never a page's.
 */
export function SearchEnginesCard({ hint, titlePlaceholder, descriptionPlaceholder, canonicalPlaceholder }: SearchEnginesCardProps) {
  const { t } = useLingui()
  return (
    <Panel title={t`Search engines`} icon={<Search size={14} aria-hidden="true" />} extra={<span className="text-xs text-ink-3">{hint}</span>}>
      <Form.Item name={['seo', 'meta_title']} label={t`Meta title`}>
        <Input maxLength={60} showCount placeholder={titlePlaceholder} />
      </Form.Item>
      <Form.Item name={['seo', 'meta_description']} label={t`Meta description`}>
        <TextArea rows={2} maxLength={160} showCount placeholder={descriptionPlaceholder} />
      </Form.Item>
      <Form.Item name={['seo', 'keywords']} label={t`Keywords`}>
        <Select mode="tags" placeholder={t`Add keywords...`} />
      </Form.Item>
      <Form.Item name={['seo', 'canonical_url']} label={t`Canonical URL`} extra={t`Only when the same content lives at another address`}>
        <Input placeholder={canonicalPlaceholder} />
      </Form.Item>
    </Panel>
  )
}

export interface SocialShareCardProps {
  hint: string
  titlePlaceholder: string
  descriptionPlaceholder?: string
  imagePlaceholder: string
  /** The image the shared link will carry, its own or the one it falls back to. */
  image: string
}

/** The Open Graph tags a shared link shows: title, description and image, under `seo`. */
export function SocialShareCard({ hint, titlePlaceholder, descriptionPlaceholder, imagePlaceholder, image }: SocialShareCardProps) {
  const { t } = useLingui()
  return (
    <Panel title={t`Social share`} icon={<Share2 size={14} aria-hidden="true" />} extra={<span className="text-xs text-ink-3">{hint}</span>}>
      <div className="grid grid-cols-2 gap-x-3">
        <Form.Item name={['seo', 'og_title']} label={t`Share title`}>
          <Input maxLength={60} showCount placeholder={titlePlaceholder} />
        </Form.Item>
        <Form.Item name={['seo', 'og_description']} label={t`Share description`}>
          <TextArea autoSize={{ minRows: 2, maxRows: 4 }} maxLength={160} showCount placeholder={descriptionPlaceholder} />
        </Form.Item>
        <div className="col-span-2 flex items-start gap-3">
          <div className="min-w-0 flex-1">
            <Form.Item name={['seo', 'og_image']} label={t`Share image`} extra={t`1200 × 630 px recommended`}>
              <ImageURLInput placeholder={imagePlaceholder} />
            </Form.Item>
          </div>
          {/* Under the label, beside the field: the image the link will carry. */}
          <div className="mt-7 w-28 shrink-0">
            {image ? (
              <img src={image} alt={t`Share image`} className="aspect-[1.91/1] w-full rounded-sm border border-line object-cover" />
            ) : (
              <div className="flex aspect-[1.91/1] w-full items-center justify-center rounded-sm border border-dashed border-line bg-subtle text-ink-4">
                <ImageOff size={16} aria-label={t`No share image`} />
              </div>
            )}
          </div>
        </div>
      </div>
    </Panel>
  )
}
