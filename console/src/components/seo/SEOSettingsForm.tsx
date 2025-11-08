import { Form, Input, Select, Row, Col, Tooltip } from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { ImageURLInput } from '../common/ImageURLInput'

interface SEOSettingsFormProps {
  namePrefix?: (string | number)[] // For nested forms like ['web_publication_settings']
  titlePlaceholder?: string
  descriptionPlaceholder?: string
  twoColumns?: boolean // Layout OpenGraph fields in a second column
}

export function SEOSettingsForm({
  namePrefix = ['web_publication_settings'],
  titlePlaceholder = 'SEO title for search engines',
  descriptionPlaceholder = 'Brief description for search results',
  twoColumns = false
}: SEOSettingsFormProps) {
  if (twoColumns) {
    return (
      <>
        <Row gutter={32}>
          <Col span={12}>
            <Form.Item
              name={[...namePrefix, 'meta_title']}
              label={
                <span>
                  Meta Title&nbsp;
                  <Tooltip title="Recommended: 50-60 characters">
                    <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
                  </Tooltip>
                </span>
              }
            >
              <Input placeholder={titlePlaceholder} maxLength={60} showCount />
            </Form.Item>

            <Form.Item
              name={[...namePrefix, 'meta_description']}
              label={
                <span>
                  Meta Description&nbsp;
                  <Tooltip title="Recommended: 150-160 characters">
                    <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
                  </Tooltip>
                </span>
              }
            >
              <Input.TextArea
                placeholder={descriptionPlaceholder}
                maxLength={160}
                rows={2}
                showCount
              />
            </Form.Item>

            <Form.Item name={[...namePrefix, 'keywords']} label="Keywords">
              <Select mode="tags" placeholder="Add keywords..." />
            </Form.Item>

            <Form.Item
              name={[...namePrefix, 'canonical_url']}
              label={
                <span>
                  Canonical URL&nbsp;
                  <Tooltip title="Preferred URL for this content (advanced)">
                    <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
                  </Tooltip>
                </span>
              }
            >
              <Input placeholder="https://example.com/original-post" />
            </Form.Item>
          </Col>

          <Col span={12}>
            <Form.Item
              name={[...namePrefix, 'og_title']}
              label={
                <span>
                  Open Graph Title&nbsp;
                  <Tooltip title="Title when shared on social media (optional)">
                    <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
                  </Tooltip>
                </span>
              }
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
              <ImageURLInput placeholder="https://example.com/image.jpg" />
            </Form.Item>
          </Col>
        </Row>
      </>
    )
  }

  return (
    <>
      <Form.Item
        name={[...namePrefix, 'meta_title']}
        label={
          <span>
            Meta Title&nbsp;
            <Tooltip title="Recommended: 50-60 characters">
              <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
            </Tooltip>
          </span>
        }
      >
        <Input placeholder={titlePlaceholder} maxLength={60} showCount />
      </Form.Item>

      <Form.Item
        name={[...namePrefix, 'meta_description']}
        label={
          <span>
            Meta Description&nbsp;
            <Tooltip title="Recommended: 150-160 characters">
              <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
            </Tooltip>
          </span>
        }
      >
        <Input.TextArea placeholder={descriptionPlaceholder} maxLength={160} rows={3} showCount />
      </Form.Item>

      <Form.Item name={[...namePrefix, 'keywords']} label="Keywords">
        <Select mode="tags" placeholder="Add keywords..." />
      </Form.Item>

      <Form.Item
        name={[...namePrefix, 'canonical_url']}
        label={
          <span>
            Canonical URL&nbsp;
            <Tooltip title="Preferred URL for this content (advanced)">
              <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
            </Tooltip>
          </span>
        }
      >
        <Input placeholder="https://example.com/original-post" />
      </Form.Item>

      <Form.Item
        name={[...namePrefix, 'og_title']}
        label={
          <span>
            Open Graph Title&nbsp;
            <Tooltip title="Title when shared on social media (optional)">
              <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
            </Tooltip>
          </span>
        }
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
        <ImageURLInput placeholder="https://example.com/image.jpg" />
      </Form.Item>
    </>
  )
}
