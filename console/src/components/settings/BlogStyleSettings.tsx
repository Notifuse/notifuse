import { Form, Input, InputNumber, Select, ColorPicker, Row, Col, Collapse, Divider, Switch } from 'antd'
import { DEFAULT_BLOG_STYLES } from '../../utils/defaultBlogStyles'

const { Panel } = Collapse

interface CSSValueInputProps {
  value?: { value: number; unit: string }
  onChange?: (value: { value: number; unit: string }) => void
}

function CSSValueInput({ value = { value: 16, unit: 'px' }, onChange }: CSSValueInputProps) {
  return (
    <Input.Group compact>
      <InputNumber
        size="small"
        style={{ width: '70%' }}
        value={value.value}
        onChange={(num) => onChange?.({ ...value, value: num || 0 })}
        min={0}
      />
      <Select
        size="small"
        style={{ width: '30%' }}
        value={value.unit}
        onChange={(unit) => onChange?.({ ...value, unit })}
      >
        <Select.Option value="px">px</Select.Option>
        <Select.Option value="rem">rem</Select.Option>
        <Select.Option value="em">em</Select.Option>
      </Select>
    </Input.Group>
  )
}

export function BlogStyleSettings() {
  return (
    <div className="mb-4" style={{ fontSize: '12px' }}>
      <style>
        {`
          .blog-style-settings .ant-form-item { margin-bottom: 12px; }
          .blog-style-settings .ant-form-item-label > label { font-size: 12px; height: auto; }
          .blog-style-settings .ant-collapse-header { font-size: 13px; padding: 8px 0; }
        `}
      </style>
      <Collapse defaultActiveKey={['default']} ghost className="blog-style-settings">
        {/* Default Styles */}
        <Panel header="Default Styles" key="default">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'default', 'fontFamily']}
                label="Font Family"
                tooltip="Base font family for all text"
                initialValue={DEFAULT_BLOG_STYLES.default.fontFamily}
              >
                <Input size="small" placeholder="system-ui, sans-serif" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'default', 'fontSize']}
                label="Font Size"
                initialValue={DEFAULT_BLOG_STYLES.default.fontSize}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'default', 'color']}
                label="Text Color"
                initialValue={DEFAULT_BLOG_STYLES.default.color}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'default', 'backgroundColor']}
                label="Background Color"
                initialValue={DEFAULT_BLOG_STYLES.default.backgroundColor}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'default', 'lineHeight']}
                label="Line Height"
                initialValue={DEFAULT_BLOG_STYLES.default.lineHeight}
              >
                <InputNumber size="small" min={1} max={3} step={0.1} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
        </Panel>

        {/* Paragraph Styles */}
        <Panel header="Paragraph" key="paragraph">
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'paragraph', 'marginTop']}
                label="Margin Top"
                initialValue={DEFAULT_BLOG_STYLES.paragraph.marginTop}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'paragraph', 'marginBottom']}
                label="Margin Bottom"
                initialValue={DEFAULT_BLOG_STYLES.paragraph.marginBottom}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'paragraph', 'lineHeight']}
                label="Line Height"
                initialValue={DEFAULT_BLOG_STYLES.paragraph.lineHeight}
              >
                <InputNumber size="small" min={1} max={3} step={0.1} style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
        </Panel>

        {/* Headings */}
        <Panel header="Headings" key="headings">
          <Form.Item
            name={['blog_settings', 'styling', 'headings', 'fontFamily']}
            label="Headings Font Family"
            tooltip="Font family for all headings (use 'inherit' to use default font)"
            initialValue={DEFAULT_BLOG_STYLES.headings.fontFamily}
          >
            <Input placeholder="inherit" />
          </Form.Item>

          <Divider orientation="left" plain>
            H1
          </Divider>
          <Row gutter={16}>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h1', 'fontSize']}
                label="Font Size"
                initialValue={DEFAULT_BLOG_STYLES.h1.fontSize}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h1', 'color']}
                label="Color"
                initialValue={DEFAULT_BLOG_STYLES.h1.color}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h1', 'marginTop']}
                label="Margin Top"
                initialValue={DEFAULT_BLOG_STYLES.h1.marginTop}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h1', 'marginBottom']}
                label="Margin Bottom"
                initialValue={DEFAULT_BLOG_STYLES.h1.marginBottom}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
          </Row>

          <Divider orientation="left" plain>
            H2
          </Divider>
          <Row gutter={16}>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h2', 'fontSize']}
                label="Font Size"
                initialValue={DEFAULT_BLOG_STYLES.h2.fontSize}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h2', 'color']}
                label="Color"
                initialValue={DEFAULT_BLOG_STYLES.h2.color}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h2', 'marginTop']}
                label="Margin Top"
                initialValue={DEFAULT_BLOG_STYLES.h2.marginTop}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h2', 'marginBottom']}
                label="Margin Bottom"
                initialValue={DEFAULT_BLOG_STYLES.h2.marginBottom}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
          </Row>

          <Divider orientation="left" plain>
            H3
          </Divider>
          <Row gutter={16}>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h3', 'fontSize']}
                label="Font Size"
                initialValue={DEFAULT_BLOG_STYLES.h3.fontSize}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h3', 'color']}
                label="Color"
                initialValue={DEFAULT_BLOG_STYLES.h3.color}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h3', 'marginTop']}
                label="Margin Top"
                initialValue={DEFAULT_BLOG_STYLES.h3.marginTop}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'h3', 'marginBottom']}
                label="Margin Bottom"
                initialValue={DEFAULT_BLOG_STYLES.h3.marginBottom}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
          </Row>
        </Panel>

        {/* Blockquote */}
        <Panel header="Blockquote" key="blockquote">
          <Row gutter={16}>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'blockquote', 'fontSize']}
                label="Font Size"
                initialValue={DEFAULT_BLOG_STYLES.blockquote.fontSize}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'blockquote', 'color']}
                label="Color"
                initialValue={DEFAULT_BLOG_STYLES.blockquote.color}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'blockquote', 'marginTop']}
                label="Margin Top"
                initialValue={DEFAULT_BLOG_STYLES.blockquote.marginTop}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item
                name={['blog_settings', 'styling', 'blockquote', 'marginBottom']}
                label="Margin Bottom"
                initialValue={DEFAULT_BLOG_STYLES.blockquote.marginBottom}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item
            name={['blog_settings', 'styling', 'blockquote', 'lineHeight']}
            label="Line Height"
            initialValue={DEFAULT_BLOG_STYLES.blockquote.lineHeight}
          >
            <InputNumber size="small" min={1} max={3} step={0.1} style={{ width: 200 }} />
          </Form.Item>
        </Panel>

        {/* Code */}
        <Panel header="Code" key="code">
          <div style={{ marginBottom: 16 }}>
            <strong>Inline Code</strong>
          </div>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'inlineCode', 'fontFamily']}
                label="Font Family"
                initialValue={DEFAULT_BLOG_STYLES.inlineCode.fontFamily}
              >
                <Input placeholder="monospace" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'inlineCode', 'fontSize']}
                label="Font Size"
                initialValue={DEFAULT_BLOG_STYLES.inlineCode.fontSize}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'inlineCode', 'color']}
                label="Text Color"
                initialValue={DEFAULT_BLOG_STYLES.inlineCode.color}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'inlineCode', 'backgroundColor']}
                label="Background Color"
                initialValue={DEFAULT_BLOG_STYLES.inlineCode.backgroundColor}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
          </Row>

          <Divider />

          <div style={{ marginBottom: 16 }}>
            <strong>Code Block</strong>
          </div>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'codeBlock', 'marginTop']}
                label="Margin Top"
                initialValue={DEFAULT_BLOG_STYLES.codeBlock.marginTop}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'codeBlock', 'marginBottom']}
                label="Margin Bottom"
                initialValue={DEFAULT_BLOG_STYLES.codeBlock.marginBottom}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
          </Row>
        </Panel>

        {/* Lists */}
        <Panel header="Lists" key="list">
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'list', 'marginTop']}
                label="Margin Top"
                initialValue={DEFAULT_BLOG_STYLES.list.marginTop}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'list', 'marginBottom']}
                label="Margin Bottom"
                initialValue={DEFAULT_BLOG_STYLES.list.marginBottom}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'list', 'paddingLeft']}
                label="Padding Left"
                initialValue={DEFAULT_BLOG_STYLES.list.paddingLeft}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
          </Row>
        </Panel>

        {/* Links */}
        <Panel header="Links" key="link">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'link', 'color']}
                label="Link Color"
                initialValue={DEFAULT_BLOG_STYLES.link.color}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'link', 'hoverColor']}
                label="Hover Color"
                initialValue={DEFAULT_BLOG_STYLES.link.hoverColor}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
          </Row>
        </Panel>

        {/* Separator */}
        <Panel header="Separator (Horizontal Rule)" key="separator">
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'separator', 'color']}
                label="Color"
                initialValue={DEFAULT_BLOG_STYLES.separator.color}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'separator', 'marginTop']}
                label="Margin Top"
                initialValue={DEFAULT_BLOG_STYLES.separator.marginTop}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item
                name={['blog_settings', 'styling', 'separator', 'marginBottom']}
                label="Margin Bottom"
                initialValue={DEFAULT_BLOG_STYLES.separator.marginBottom}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
          </Row>
        </Panel>

        {/* Caption */}
        <Panel header="Caption (Images & Code Blocks)" key="caption">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'caption', 'fontSize']}
                label="Font Size"
                initialValue={DEFAULT_BLOG_STYLES.caption.fontSize}
              >
                <CSSValueInput />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'caption', 'color']}
                label="Color"
                initialValue={DEFAULT_BLOG_STYLES.caption.color}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
          </Row>
        </Panel>

        {/* Newsletter Settings */}
        <Panel header="Newsletter" key="newsletter">
          <Row gutter={16}>
            <Col span={24}>
              <Form.Item
                name={['blog_settings', 'styling', 'newsletter', 'enabled']}
                label="Enable Newsletter"
                tooltip="Show newsletter subscription form in footer"
                initialValue={DEFAULT_BLOG_STYLES.newsletter.enabled}
                valuePropName="checked"
              >
                <Switch size="small" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'newsletter', 'buttonColor']}
                label="Button Color"
                initialValue={DEFAULT_BLOG_STYLES.newsletter.buttonColor}
              >
                <ColorPicker size="small" showText format="hex" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name={['blog_settings', 'styling', 'newsletter', 'buttonText']}
                label="Button Text"
                initialValue={DEFAULT_BLOG_STYLES.newsletter.buttonText}
              >
                <Input size="small" placeholder="Subscribe" />
              </Form.Item>
            </Col>
          </Row>
        </Panel>
      </Collapse>
    </div>
  )
}
