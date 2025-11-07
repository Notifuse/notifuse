import { Form, Input, Select } from 'antd'

interface SEOSettingsFormProps {
  namePrefix?: (string | number)[] // For nested forms like ['web_publication_settings']
  showCanonical?: boolean // Only for posts
  titlePlaceholder?: string
  descriptionPlaceholder?: string
}

export function SEOSettingsForm({
  namePrefix = ['web_publication_settings'],
  showCanonical = false,
  titlePlaceholder = 'SEO title for search engines',
  descriptionPlaceholder = 'Brief description for search results'
}: SEOSettingsFormProps) {
  return (
    <>
      <Form.Item
        name={[...namePrefix, 'meta_title']}
        label="Meta Title"
        help="Recommended: 50-60 characters"
      >
        <Input placeholder={titlePlaceholder} maxLength={60} showCount />
      </Form.Item>

      <Form.Item
        name={[...namePrefix, 'meta_description']}
        label="Meta Description"
        help="Recommended: 150-160 characters"
      >
        <Input.TextArea placeholder={descriptionPlaceholder} maxLength={160} rows={3} showCount />
      </Form.Item>

      <Form.Item name={[...namePrefix, 'keywords']} label="Keywords">
        <Select mode="tags" placeholder="Add keywords..." />
      </Form.Item>

      <Form.Item
        name={[...namePrefix, 'og_title']}
        label="Open Graph Title"
        help="Title when shared on social media (optional)"
      >
        <Input maxLength={60} showCount placeholder="Defaults to meta title" />
      </Form.Item>

      <Form.Item name={[...namePrefix, 'og_description']} label="Open Graph Description">
        <Input.TextArea
          maxLength={160}
          rows={2}
          showCount
          placeholder="Defaults to meta description"
        />
      </Form.Item>

      <Form.Item name={[...namePrefix, 'og_image']} label="Open Graph Image URL">
        <Input placeholder="https://example.com/image.jpg" />
      </Form.Item>

      {showCanonical && (
        <Form.Item
          name={[...namePrefix, 'canonical_url']}
          label="Canonical URL"
          help="Preferred URL for this content (advanced)"
        >
          <Input placeholder="https://example.com/original-post" />
        </Form.Item>
      )}
    </>
  )
}
