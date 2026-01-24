import { useLingui } from '@lingui/react/macro'
import { Form, Input, Select, Row, Col, Tooltip } from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { ImageURLInput } from '../common/ImageURLInput'
import Subtitle from '../common/subtitle'

interface SEOSettingsFormProps {
  namePrefix?: (string | number)[] // For nested forms like ['web_publication_settings']
  titlePlaceholder?: string
  descriptionPlaceholder?: string
  twoColumns?: boolean // Layout OpenGraph fields in a second column
}

export function SEOSettingsForm({
  namePrefix = ['web_publication_settings'],
  titlePlaceholder,
  descriptionPlaceholder,
  twoColumns = false
}: SEOSettingsFormProps) {
  const { t } = useLingui()
  const resolvedTitlePlaceholder = titlePlaceholder ?? t`SEO title for search engines`
  const resolvedDescriptionPlaceholder =
    descriptionPlaceholder ?? t`Brief description for search results`
  if (twoColumns) {
    return (
      <>
        <Row gutter={32}>
          <Col span={12}>
            <Subtitle className="mb-6" borderBottom primary>
              {t`SEO`}
            </Subtitle>
            <Form.Item
              name={[...namePrefix, 'meta_title']}
              label={
                <span>
                  {t`Meta Title`}&nbsp;
                  <Tooltip title={t`Recommended: 50-60 characters`}>
                    <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
                  </Tooltip>
                </span>
              }
            >
              <Input placeholder={resolvedTitlePlaceholder} maxLength={60} showCount />
            </Form.Item>

            <Form.Item
              name={[...namePrefix, 'meta_description']}
              label={
                <span>
                  {t`Meta Description`}&nbsp;
                  <Tooltip title={t`Recommended: 150-160 characters`}>
                    <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
                  </Tooltip>
                </span>
              }
            >
              <Input.TextArea
                placeholder={resolvedDescriptionPlaceholder}
                maxLength={160}
                rows={1}
                showCount
              />
            </Form.Item>

            <Form.Item name={[...namePrefix, 'keywords']} label={t`Keywords`}>
              <Select mode="tags" placeholder={t`Add keywords...`} />
            </Form.Item>

            <Form.Item
              name={[...namePrefix, 'meta_robots']}
              label={
                <span>
                  {t`Search Engine Indexing`}&nbsp;
                  <Tooltip title={t`Control how search engines index and follow links on your blog`}>
                    <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
                  </Tooltip>
                </span>
              }
            >
              <Select placeholder={t`Select indexing option`} defaultValue="index,follow">
                <Select.Option value="index,follow">{t`Index and follow links`}</Select.Option>
                <Select.Option value="noindex,follow">{t`Don't index, but follow links`}</Select.Option>
                <Select.Option value="index,nofollow">{t`Index, but don't follow links`}</Select.Option>
                <Select.Option value="noindex,nofollow">
                  {t`Don't index and don't follow links`}
                </Select.Option>
              </Select>
            </Form.Item>

            <Form.Item
              name={[...namePrefix, 'canonical_url']}
              label={
                <span>
                  {t`Canonical URL`}&nbsp;
                  <Tooltip title={t`Preferred URL for this content (advanced)`}>
                    <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
                  </Tooltip>
                </span>
              }
            >
              <Input placeholder="https://example.com/original-post" />
            </Form.Item>
          </Col>

          <Col span={12}>
            <Subtitle className="mb-6" borderBottom primary>
              {t`Social Share`}
            </Subtitle>
            <Form.Item
              name={[...namePrefix, 'og_title']}
              label={
                <span>
                  {t`Social Share Title`}&nbsp;
                  <Tooltip title={t`Title when shared on social media (optional)`}>
                    <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
                  </Tooltip>
                </span>
              }
            >
              <Input maxLength={60} showCount placeholder={t`Defaults to meta title`} />
            </Form.Item>

            <Form.Item name={[...namePrefix, 'og_description']} label={t`Social Share Description`}>
              <Input.TextArea
                maxLength={160}
                rows={1}
                showCount
                placeholder={t`Defaults to meta description`}
              />
            </Form.Item>

            <Form.Item name={[...namePrefix, 'og_image']} label={t`Social Share Image URL`}>
              <ImageURLInput placeholder="https://example.com/image.jpg" />
            </Form.Item>
          </Col>
        </Row>
      </>
    )
  }

  return (
    <>
      <Subtitle className="mb-6" borderBottom primary>
        {t`SEO`}
      </Subtitle>
      <Form.Item
        name={[...namePrefix, 'meta_title']}
        label={
          <span>
            {t`Meta Title`}&nbsp;
            <Tooltip title={t`Recommended: 50-60 characters`}>
              <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
            </Tooltip>
          </span>
        }
      >
        <Input placeholder={resolvedTitlePlaceholder} maxLength={60} showCount />
      </Form.Item>

      <Form.Item
        name={[...namePrefix, 'meta_description']}
        label={
          <span>
            {t`Meta Description`}&nbsp;
            <Tooltip title={t`Recommended: 150-160 characters`}>
              <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
            </Tooltip>
          </span>
        }
      >
        <Input.TextArea
          placeholder={resolvedDescriptionPlaceholder}
          maxLength={160}
          rows={3}
          showCount
        />
      </Form.Item>

      <Form.Item name={[...namePrefix, 'keywords']} label={t`Keywords`}>
        <Select mode="tags" placeholder={t`Add keywords...`} />
      </Form.Item>

      <Form.Item
        name={[...namePrefix, 'meta_robots']}
        label={
          <span>
            {t`Search Engine Indexing`}&nbsp;
            <Tooltip title={t`Control how search engines index and follow links on your blog`}>
              <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
            </Tooltip>
          </span>
        }
      >
        <Select placeholder={t`Select indexing option`}>
          <Select.Option value="index,follow">{t`Index and follow links`}</Select.Option>
          <Select.Option value="noindex,follow">{t`Don't index, but follow links`}</Select.Option>
          <Select.Option value="index,nofollow">{t`Index, but don't follow links`}</Select.Option>
          <Select.Option value="noindex,nofollow">{t`Don't index and don't follow links`}</Select.Option>
        </Select>
      </Form.Item>

      <Form.Item
        name={[...namePrefix, 'canonical_url']}
        label={
          <span>
            {t`Canonical URL`}&nbsp;
            <Tooltip title={t`Preferred URL for this content (advanced)`}>
              <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
            </Tooltip>
          </span>
        }
      >
        <Input placeholder="https://example.com/original-post" />
      </Form.Item>

      <Subtitle className="mb-6" borderBottom primary>
        {t`Social Share`}
      </Subtitle>
      <Form.Item
        name={[...namePrefix, 'og_title']}
        label={
          <span>
            {t`Social Share Title`}&nbsp;
            <Tooltip title={t`Title when shared on social media (optional)`}>
              <InfoCircleOutlined style={{ cursor: 'pointer' }} className="pl-1" />
            </Tooltip>
          </span>
        }
      >
        <Input maxLength={60} showCount placeholder={t`Defaults to meta title`} />
      </Form.Item>

      <Form.Item name={[...namePrefix, 'og_description']} label={t`Social Share Description`}>
        <Input.TextArea
          maxLength={160}
          rows={2}
          showCount
          placeholder={t`Defaults to meta description`}
        />
      </Form.Item>

      <Form.Item name={[...namePrefix, 'og_image']} label={t`Social Share Image URL`}>
        <ImageURLInput placeholder="https://example.com/image.jpg" />
      </Form.Item>
    </>
  )
}
