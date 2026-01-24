import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Select, InputNumber, Row, Col, Input } from 'antd'
import type { MJMLComponentType, MJSocialElementAttributes } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction, type PreviewProps } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import InputLayout from '../ui/InputLayout'
import ColorPickerWithPresets from '../ui/ColorPickerWithPresets'
import AlignSelector from '../ui/AlignSelector'
import PaddingInput from '../ui/PaddingInput'
import StringPopoverInput from '../ui/StringPopoverInput'
import BorderRadiusInput from '../ui/BorderRadiusInput'
import PanelLayout from '../panels/PanelLayout'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faFacebook,
  faTwitter,
  faInstagram,
  faLinkedin,
  faGithub,
  faYoutube,
  faPinterest,
  faGoogle,
  faSnapchat,
  faDribbble,
  faMedium,
  faTumblr,
  faVimeo,
  faSoundcloud,
  faXing,
  faSquareXTwitter
} from '@fortawesome/free-brands-svg-icons'
import { faGlobe } from '@fortawesome/free-solid-svg-icons'

// Functional component for settings panel with i18n support
interface MjSocialElementSettingsPanelProps {
  currentAttributes: MJSocialElementAttributes
  blockDefaults: MJSocialElementAttributes
  onUpdate: OnUpdateAttributesFunction
  parsePixelValue: (value?: string) => number | undefined
}

const MjSocialElementSettingsPanel: React.FC<MjSocialElementSettingsPanelProps> = ({
  currentAttributes,
  blockDefaults,
  onUpdate,
  parsePixelValue
}) => {
  const { t } = useLingui()

  return (
    <PanelLayout title={t`Social Element Attributes`}>
      <InputLayout label={t`Social Network`}>
        <Select
          size="small"
          value={currentAttributes.name || 'custom'}
          onChange={(value) => onUpdate({ name: value === 'custom' ? undefined : value })}
          style={{ width: '100%' }}
        >
          <Select.Option value="custom">{t`Custom`}</Select.Option>
          <Select.Option value="facebook">Facebook</Select.Option>
          <Select.Option value="twitter">Twitter</Select.Option>
          <Select.Option value="x">X (Twitter)</Select.Option>
          <Select.Option value="instagram">Instagram</Select.Option>
          <Select.Option value="linkedin">LinkedIn</Select.Option>
          <Select.Option value="github">GitHub</Select.Option>
          <Select.Option value="youtube">YouTube</Select.Option>
          <Select.Option value="pinterest">Pinterest</Select.Option>
          <Select.Option value="google">Google</Select.Option>
          <Select.Option value="snapchat">Snapchat</Select.Option>
          <Select.Option value="dribbble">Dribbble</Select.Option>
          <Select.Option value="medium">Medium</Select.Option>
          <Select.Option value="tumblr">Tumblr</Select.Option>
          <Select.Option value="vimeo">Vimeo</Select.Option>
          <Select.Option value="soundcloud">SoundCloud</Select.Option>
          <Select.Option value="xing">Xing</Select.Option>
          <Select.Option value="web">{t`Website`}</Select.Option>
        </Select>
      </InputLayout>

      <InputLayout label={t`Link URL`}>
        <StringPopoverInput
          value={currentAttributes.href || ''}
          onChange={(value) => onUpdate({ href: value || undefined })}
          placeholder={t`https://example.com or {{ url }}`}
          buttonText={t`Set link URL`}
          validateUri={true}
        />
      </InputLayout>

      <InputLayout label={t`Custom Icon URL`}>
        <Input
          size="small"
          value={currentAttributes.src || ''}
          onChange={(e) => onUpdate({ src: e.target.value || undefined })}
          placeholder="https://example.com/icon.png"
          style={{ width: '100%' }}
          disabled={!!(currentAttributes.name && currentAttributes.name !== 'custom')}
        />
      </InputLayout>

      <InputLayout label={t`Alt Text`}>
        <Input
          size="small"
          value={currentAttributes.alt || ''}
          onChange={(e) => onUpdate({ alt: e.target.value || undefined })}
          placeholder={t`Social media icon`}
          style={{ width: '100%' }}
        />
      </InputLayout>

      <InputLayout label={t`Alignment`}>
        <AlignSelector
          value={currentAttributes.align || 'center'}
          onChange={(value) => onUpdate({ align: value })}
        />
      </InputLayout>

      <InputLayout label={t`Icon Settings`} layout="vertical">
        <Row gutter={16}>
          <Col span={8}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Size`}</span>
              <div style={{ marginTop: '4px' }}>
                <InputNumber
                  size="small"
                  value={parsePixelValue(currentAttributes.iconSize)}
                  onChange={(value) => onUpdate({ iconSize: value ? `${value}px` : undefined })}
                  placeholder={(parsePixelValue(blockDefaults.iconSize) || 20).toString()}
                  min={10}
                  max={100}
                  step={1}
                  suffix="px"
                  style={{ width: '100%' }}
                />
              </div>
            </div>
          </Col>
          <Col span={8}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Height`}</span>
              <div style={{ marginTop: '4px' }}>
                <InputNumber
                  size="small"
                  value={parsePixelValue(currentAttributes.iconHeight)}
                  onChange={(value) => onUpdate({ iconHeight: value ? `${value}px` : undefined })}
                  placeholder={(parsePixelValue(blockDefaults.iconHeight) || 20).toString()}
                  min={10}
                  max={100}
                  step={1}
                  suffix="px"
                  style={{ width: '100%' }}
                />
              </div>
            </div>
          </Col>
        </Row>
      </InputLayout>

      <InputLayout label={t`Icon Styling`} layout="vertical">
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Background`}</span>
              <div style={{ marginTop: '4px' }}>
                <ColorPickerWithPresets
                  value={currentAttributes.backgroundColor || undefined}
                  onChange={(color) => onUpdate({ backgroundColor: color || undefined })}
                />
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Border Radius`}</span>
              <div style={{ marginTop: '4px' }}>
                <BorderRadiusInput
                  value={currentAttributes.borderRadius}
                  onChange={(value) => onUpdate({ borderRadius: value })}
                  defaultValue={blockDefaults.borderRadius}
                />
              </div>
            </div>
          </Col>
        </Row>
      </InputLayout>

      <InputLayout label={t`Text Settings`} layout="vertical">
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Color`}</span>
              <div style={{ marginTop: '4px' }}>
                <ColorPickerWithPresets
                  value={currentAttributes.color || undefined}
                  onChange={(color) => onUpdate({ color: color || undefined })}
                />
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Font Size`}</span>
              <div style={{ marginTop: '4px' }}>
                <InputNumber
                  size="small"
                  value={parsePixelValue(currentAttributes.fontSize)}
                  onChange={(value) => onUpdate({ fontSize: value ? `${value}px` : undefined })}
                  placeholder={(parsePixelValue(blockDefaults.fontSize) || 13).toString()}
                  min={8}
                  max={48}
                  step={1}
                  suffix="px"
                  style={{ width: '100%' }}
                />
              </div>
            </div>
          </Col>
        </Row>
      </InputLayout>

      <InputLayout label={t`Font Settings`} layout="vertical">
        <Row gutter={16}>
          <Col span={8}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Weight`}</span>
              <div style={{ marginTop: '4px' }}>
                <Select
                  size="small"
                  value={currentAttributes.fontWeight || 'normal'}
                  onChange={(value) => onUpdate({ fontWeight: value })}
                  style={{ width: '100%' }}
                >
                  <Select.Option value="normal">{t`Normal`}</Select.Option>
                  <Select.Option value="bold">{t`Bold`}</Select.Option>
                  <Select.Option value="lighter">{t`Lighter`}</Select.Option>
                  <Select.Option value="100">100</Select.Option>
                  <Select.Option value="200">200</Select.Option>
                  <Select.Option value="300">300</Select.Option>
                  <Select.Option value="400">400</Select.Option>
                  <Select.Option value="500">500</Select.Option>
                  <Select.Option value="600">600</Select.Option>
                  <Select.Option value="700">700</Select.Option>
                  <Select.Option value="800">800</Select.Option>
                  <Select.Option value="900">900</Select.Option>
                </Select>
              </div>
            </div>
          </Col>
          <Col span={8}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Style`}</span>
              <div style={{ marginTop: '4px' }}>
                <Select
                  size="small"
                  value={currentAttributes.fontStyle || 'normal'}
                  onChange={(value) => onUpdate({ fontStyle: value })}
                  style={{ width: '100%' }}
                >
                  <Select.Option value="normal">{t`Normal`}</Select.Option>
                  <Select.Option value="italic">{t`Italic`}</Select.Option>
                  <Select.Option value="oblique">{t`Oblique`}</Select.Option>
                </Select>
              </div>
            </div>
          </Col>
          <Col span={8}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Decoration`}</span>
              <div style={{ marginTop: '4px' }}>
                <Select
                  size="small"
                  value={currentAttributes.textDecoration || 'none'}
                  onChange={(value) => onUpdate({ textDecoration: value })}
                  style={{ width: '100%' }}
                >
                  <Select.Option value="none">{t`None`}</Select.Option>
                  <Select.Option value="underline">{t`Underline`}</Select.Option>
                  <Select.Option value="overline">{t`Overline`}</Select.Option>
                  <Select.Option value="line-through">{t`Line Through`}</Select.Option>
                </Select>
              </div>
            </div>
          </Col>
        </Row>
      </InputLayout>

      <InputLayout label={t`Font Family`}>
        <Input
          size="small"
          value={currentAttributes.fontFamily || ''}
          onChange={(e) => onUpdate({ fontFamily: e.target.value || undefined })}
          placeholder={blockDefaults.fontFamily || 'Ubuntu, Helvetica, Arial, sans-serif'}
          style={{ width: '100%' }}
        />
      </InputLayout>

      <InputLayout label={t`Line Height`}>
        <StringPopoverInput
          value={currentAttributes.lineHeight || ''}
          onChange={(value) => onUpdate({ lineHeight: value || undefined })}
          placeholder={blockDefaults.lineHeight || '22px'}
          buttonText={t`Set height`}
        />
      </InputLayout>

      <InputLayout label={t`Vertical Align`}>
        <Select
          size="small"
          value={currentAttributes.verticalAlign || 'middle'}
          onChange={(value) => onUpdate({ verticalAlign: value })}
          style={{ width: '100%' }}
        >
          <Select.Option value="top">{t`Top`}</Select.Option>
          <Select.Option value="middle">{t`Middle`}</Select.Option>
          <Select.Option value="bottom">{t`Bottom`}</Select.Option>
        </Select>
      </InputLayout>

      <InputLayout label={t`Link Settings`} layout="vertical">
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Target`}</span>
              <div style={{ marginTop: '4px' }}>
                <Select
                  size="small"
                  value={currentAttributes.target || '_blank'}
                  onChange={(value) => onUpdate({ target: value })}
                  style={{ width: '100%' }}
                >
                  <Select.Option value="_blank">{t`New Window`}</Select.Option>
                  <Select.Option value="_self">{t`Same Window`}</Select.Option>
                  <Select.Option value="_parent">{t`Parent Frame`}</Select.Option>
                  <Select.Option value="_top">{t`Top Frame`}</Select.Option>
                </Select>
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Rel`}</span>
              <div style={{ marginTop: '4px' }}>
                <Input
                  size="small"
                  value={currentAttributes.rel || ''}
                  onChange={(e) => onUpdate({ rel: e.target.value || undefined })}
                  placeholder="noopener"
                  style={{ width: '100%' }}
                />
              </div>
            </div>
          </Col>
        </Row>
      </InputLayout>

      <InputLayout label={t`Title`}>
        <Input
          size="small"
          value={currentAttributes.title || ''}
          onChange={(e) => onUpdate({ title: e.target.value || undefined })}
          placeholder={t`Tooltip text`}
          style={{ width: '100%' }}
        />
      </InputLayout>

      <InputLayout label={t`Padding Settings`} layout="vertical">
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Icon Padding`}</span>
              <div style={{ marginTop: '4px' }}>
                <StringPopoverInput
                  value={currentAttributes.iconPadding || ''}
                  onChange={(value) => onUpdate({ iconPadding: value || undefined })}
                  placeholder={blockDefaults.iconPadding || '0px'}
                  buttonText={t`Set padding`}
                />
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Text Padding`}</span>
              <div style={{ marginTop: '4px' }}>
                <StringPopoverInput
                  value={currentAttributes.textPadding || ''}
                  onChange={(value) => onUpdate({ textPadding: value || undefined })}
                  placeholder={blockDefaults.textPadding || '4px 4px 4px 0'}
                  buttonText={t`Set padding`}
                />
              </div>
            </div>
          </Col>
        </Row>
      </InputLayout>

      <InputLayout label={t`Element Padding`} layout="vertical">
        <PaddingInput
          value={{
            top: currentAttributes.paddingTop,
            right: currentAttributes.paddingRight,
            bottom: currentAttributes.paddingBottom,
            left: currentAttributes.paddingLeft
          }}
          defaultValue={{
            top: blockDefaults.paddingTop,
            right: blockDefaults.paddingRight,
            bottom: blockDefaults.paddingBottom,
            left: blockDefaults.paddingLeft
          }}
          onChange={(values: {
            top: string | undefined
            right: string | undefined
            bottom: string | undefined
            left: string | undefined
          }) => {
            onUpdate({
              paddingTop: values.top,
              paddingRight: values.right,
              paddingBottom: values.bottom,
              paddingLeft: values.left
            })
          }}
        />
      </InputLayout>

      <InputLayout label={t`CSS Class`}>
        <StringPopoverInput
          value={currentAttributes.cssClass || ''}
          onChange={(value) => onUpdate({ cssClass: value || undefined })}
          placeholder={t`Enter CSS class name`}
        />
      </InputLayout>
    </PanelLayout>
  )
}

/**
 * Implementation for mj-social-element blocks
 */
export class MjSocialElementBlock extends BaseEmailBlock {
  /**
   * Get FontAwesome icon based on social network name
   */
  private getSocialFontAwesomeIcon(name?: string): React.ReactNode {
    // Get the current attributes to check if a custom icon is used
    const currentAttributes = this.block.attributes as MJSocialElementAttributes

    // If custom src is provided, use generic web icon
    if (currentAttributes.src) {
      return <FontAwesomeIcon icon={faGlobe} />
    }

    switch (name?.toLowerCase()) {
      case 'facebook':
        return <FontAwesomeIcon icon={faFacebook} />
      case 'twitter':
        return <FontAwesomeIcon icon={faTwitter} />
      case 'x':
        return <FontAwesomeIcon icon={faSquareXTwitter} />
      case 'instagram':
        return <FontAwesomeIcon icon={faInstagram} />
      case 'linkedin':
        return <FontAwesomeIcon icon={faLinkedin} />
      case 'github':
        return <FontAwesomeIcon icon={faGithub} />
      case 'youtube':
        return <FontAwesomeIcon icon={faYoutube} />
      case 'pinterest':
        return <FontAwesomeIcon icon={faPinterest} />
      case 'google':
        return <FontAwesomeIcon icon={faGoogle} />
      case 'snapchat':
        return <FontAwesomeIcon icon={faSnapchat} />
      case 'dribbble':
        return <FontAwesomeIcon icon={faDribbble} />
      case 'medium':
        return <FontAwesomeIcon icon={faMedium} />
      case 'tumblr':
        return <FontAwesomeIcon icon={faTumblr} />
      case 'vimeo':
        return <FontAwesomeIcon icon={faVimeo} />
      case 'soundcloud':
        return <FontAwesomeIcon icon={faSoundcloud} />
      case 'xing':
        return <FontAwesomeIcon icon={faXing} />
      case 'web':
      default:
        return <FontAwesomeIcon icon={faGlobe} />
    }
  }

  getIcon(): React.ReactNode {
    const currentAttributes = this.block.attributes as MJSocialElementAttributes
    return this.getSocialFontAwesomeIcon(currentAttributes.name)
  }

  getLabel(): string {
    const currentAttributes = this.block.attributes as MJSocialElementAttributes
    return currentAttributes.name
      ? currentAttributes.name.charAt(0).toUpperCase() + currentAttributes.name.slice(1)
      : 'Social Element'
  }

  getDescription(): React.ReactNode {
    return 'Individual social media icon and link'
  }

  getCategory(): 'content' | 'layout' {
    return 'content'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-social-element'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  /**
   * Render the settings panel for the social element block
   */
  renderSettingsPanel(onUpdate: OnUpdateAttributesFunction): React.ReactNode {
    const currentAttributes = this.block.attributes as MJSocialElementAttributes
    const blockDefaults = this.getDefaults() as MJSocialElementAttributes

    return (
      <MjSocialElementSettingsPanel
        currentAttributes={currentAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
        parsePixelValue={this.parsePixelValue}
      />
    )
  }

  /**
   * Parse pixel value from string (e.g., "20px" -> 20)
   */
  private parsePixelValue(value?: string): number | undefined {
    if (!value) return undefined
    const match = value.match(/^(\d+(?:\.\d+)?)px?$/)
    return match ? parseFloat(match[1]) : undefined
  }

  /**
   * Get the default background color for a social network
   * Since we're using true color icons, we use transparent backgrounds to avoid hiding the icon
   */
  private getNetworkDefaultColor(name?: string): string {
    // For true color icons, we typically want transparent backgrounds
    // or light/dark contrasting backgrounds that complement the icon
    const networkColors: Record<string, string> = {
      facebook: 'transparent',
      'facebook-noshare': 'transparent',
      twitter: 'transparent',
      'twitter-noshare': 'transparent',
      x: 'transparent',
      'x-noshare': 'transparent',
      google: 'transparent',
      'google-noshare': 'transparent',
      pinterest: 'transparent',
      'pinterest-noshare': 'transparent',
      linkedin: 'transparent',
      'linkedin-noshare': 'transparent',
      tumblr: 'transparent',
      'tumblr-noshare': 'transparent',
      xing: 'transparent',
      'xing-noshare': 'transparent',
      github: 'transparent',
      instagram: 'transparent',
      youtube: 'transparent',
      vimeo: 'transparent',
      medium: 'transparent',
      soundcloud: 'transparent',
      dribbble: 'transparent',
      snapchat: 'transparent',
      web: 'transparent'
    }
    return networkColors[name || ''] || 'transparent'
  }

  /**
   * Get the icon URL for a social network from MageCDN
   */
  private getSocialIcon(name?: string): string {
    const iconUrls: Record<string, string> = {
      facebook: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/facebook.png',
      twitter: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/twitter.png',
      x: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/twitter-x.png',
      instagram: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/instagram.png',
      linkedin: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/linkedin.png',
      github: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/github.png',
      youtube: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/youtube.png',
      pinterest: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/pinterest.png',
      google: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/google-plus.png',
      snapchat: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/snapchat.png',
      dribbble: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/dribbble.png',
      medium: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/medium.png',
      tumblr: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/tumblr.png',
      vimeo: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/vimeo.png',
      soundcloud: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/soundcloud.png',
      xing: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/xing.png',
      web: 'https://www.mailjet.com/images/theme/v1/icons/ico-social/web.png'
    }
    return (
      iconUrls[name || ''] || 'https://www.mailjet.com/images/theme/v1/icons/ico-social/web.png'
    )
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const { selectedBlockId, onSelectBlock, attributeDefaults } = props

    const key = this.block.id
    const isSelected = selectedBlockId === this.block.id
    const blockClasses = `email-block-hover ${isSelected ? 'selected' : ''}`.trim()

    const selectionStyle: React.CSSProperties = isSelected
      ? { position: 'relative', zIndex: 10 }
      : {}

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onSelectBlock) {
        onSelectBlock(this.block.id)
      }
    }

    const attrs = EmailBlockClass.mergeWithAllDefaults(
      'mj-social-element',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    // Get content (text label) if available
    const blockWithContent = this.block as unknown as Record<string, unknown>
    const content = typeof blockWithContent.content === 'string' ? blockWithContent.content : ''

    // Get icon source URL (either custom src or MageCDN URL)
    const iconSrc = attrs.src || this.getSocialIcon(attrs.name)

    // Parse dimensions
    const iconWidth = this.parsePixelValue(attrs.iconSize) || 20
    const iconHeight = this.parsePixelValue(attrs.iconHeight) || 20

    // Table cell padding style (outer wrapper)
    const cellPaddingStyle: React.CSSProperties = {
      padding: `${attrs.paddingTop || '4px'} ${attrs.paddingRight || '4px'} ${
        attrs.paddingBottom || '4px'
      } ${attrs.paddingLeft || '4px'}`,
      verticalAlign: attrs.verticalAlign || 'middle'
    }

    // Icon table style (simulates the nested table structure)
    const iconTableStyle: React.CSSProperties = {
      backgroundColor: attrs.backgroundColor || this.getNetworkDefaultColor(attrs.name),
      borderRadius: attrs.borderRadius || '3px',
      width: `${iconWidth}px`,
      border: '0',
      borderCollapse: 'collapse' as const,
      borderSpacing: '0'
    }

    // Icon cell style
    const iconCellStyle: React.CSSProperties = {
      fontSize: '0',
      height: `${iconHeight}px`,
      verticalAlign: 'middle',
      width: `${iconWidth}px`,
      padding: attrs.iconPadding || '0px'
    }

    // Image style for all icons (both custom and CDN)
    const imageStyle: React.CSSProperties = {
      borderRadius: attrs.borderRadius || '3px',
      display: 'block',
      width: `${iconWidth}px`,
      height: `${iconHeight}px`,
      objectFit: 'cover'
    }

    // Text cell style (if content exists)
    const textCellStyle: React.CSSProperties = {
      verticalAlign: 'middle',
      padding: attrs.textPadding || '4px 4px 4px 0'
    }

    // Link style for text
    const textLinkStyle: React.CSSProperties = {
      color: attrs.color || '#333333',
      fontSize: attrs.fontSize || '15px',
      fontFamily: attrs.fontFamily || 'Ubuntu, Helvetica, Arial, sans-serif',
      lineHeight: attrs.lineHeight || '22px',
      textDecoration: attrs.textDecoration || 'none',
      fontWeight: attrs.fontWeight || 'normal',
      fontStyle: attrs.fontStyle || 'normal'
    }

    // Icon element (always an image now)
    const iconElement = (
      <img
        alt={attrs.alt || ''}
        height={iconHeight}
        src={iconSrc}
        style={imageStyle}
        width={iconWidth}
      />
    )

    // Link wrapper for icon
    const iconLink = (
      <a
        href={attrs.href || '#'}
        target={attrs.target || '_blank'}
        rel={attrs.rel}
        title={attrs.title}
        onClick={(e) => e.preventDefault()} // Prevent navigation in preview
        style={{ textDecoration: 'none' }}
      >
        {iconElement}
      </a>
    )

    // Text link (if content exists)
    const textLink = content ? (
      <a
        href={attrs.href || '#'}
        target={attrs.target || '_blank'}
        rel={attrs.rel}
        title={attrs.title}
        style={textLinkStyle}
        onClick={(e) => e.preventDefault()} // Prevent navigation in preview
      >
        {content}
      </a>
    ) : null

    // Main table structure that simulates MJML output
    return (
      <table
        key={key}
        align="center"
        border={0}
        cellPadding={0}
        cellSpacing={0}
        role="presentation"
        style={{
          float: 'none',
          display: 'inline-table',
          ...selectionStyle
        }}
        className={blockClasses}
        onClick={handleClick}
        data-block-id={this.block.id}
      >
        <tbody>
          <tr>
            {/* Icon cell */}
            <td style={cellPaddingStyle}>
              <table
                border={0}
                cellPadding={0}
                cellSpacing={0}
                role="presentation"
                style={iconTableStyle}
              >
                <tbody>
                  <tr>
                    <td style={iconCellStyle}>{iconLink}</td>
                  </tr>
                </tbody>
              </table>
            </td>

            {/* Text cell (only if content exists) */}
            {content && <td style={textCellStyle}>{textLink}</td>}
          </tr>
        </tbody>
      </table>
    )
  }
}
