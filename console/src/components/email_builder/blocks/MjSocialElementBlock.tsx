import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Select, Switch } from 'antd'
import type { EmailBlock, MJMLComponentType, MJSocialElementAttributes, MergedBlockAttributes } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction, type PreviewProps } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { isSafeHref } from '../../../utils/safeHtml'
import { EmailBlockClass } from '../EmailBlockClass'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { DraftTextField } from '../ui/fields/DraftTextField'
import { ColorField } from '../ui/fields/ColorField'
import { FontFields } from '../ui/fields/FontFields'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { SegmentedField } from '../ui/fields/SegmentedField'
import { useAlignOptions, useVerticalAlignOptions } from '../ui/fields/segmentedOptions'
import { CornersField } from '../ui/fields/CornersField'
import { SingleBorderField } from '../ui/fields/BorderField'
import { SpacingField } from '../ui/fields/SpacingField'
import { parsePaddingShorthand, toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { useImportedFonts, inheritedSidesOf, panelFields, sidesOf, useEmailColors, useLinkTargetOptions } from './panelFields'
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

// Liquid examples go into messages as variables: braces in a message are placeholders
const LIQUID_URL = '{{ url }}'

// Networks that have share URL templates in safe-mjml.
// When name is plain (e.g. "facebook"), safe-mjml wraps href in share URL,
// unless href already points to that network (a profile or share URL).
// When name has "-noshare" suffix (e.g. "facebook-noshare"), href passes through unchanged.
const NETWORKS_WITH_SHARE_URL = new Set([
  'facebook',
  'twitter',
  'x',
  'google',
  'pinterest',
  'linkedin',
  'tumblr',
  'xing'
])

const getBaseNetworkName = (name?: string): string => {
  if (!name) return ''
  return name.replace(/-noshare$/, '')
}

const isShareMode = (name?: string): boolean => {
  if (!name) return false
  const baseName = getBaseNetworkName(name)
  return NETWORKS_WITH_SHARE_URL.has(baseName) && !name.endsWith('-noshare')
}

// A px length as a number ("20px", "20p" or "20"), for the placeholders
const pixels = (value?: string): number | undefined => {
  const match = value?.match(/^(\d+(?:\.\d+)?)px?$/)
  return match ? parseFloat(match[1]) : undefined
}

// The networks offered for a new choice, with their names. « Twitter » is no longer offered (X
// replaces it); an element saved as twitter still shows it (see networkOptions).
const NETWORKS: ReadonlyArray<{ value: string; label: string }> = [
  { value: 'facebook', label: 'Facebook' },
  { value: 'x', label: 'X' },
  { value: 'instagram', label: 'Instagram' },
  { value: 'linkedin', label: 'LinkedIn' },
  { value: 'github', label: 'GitHub' },
  { value: 'youtube', label: 'YouTube' },
  { value: 'pinterest', label: 'Pinterest' },
  { value: 'google', label: 'Google' },
  { value: 'snapchat', label: 'Snapchat' },
  { value: 'dribbble', label: 'Dribbble' },
  { value: 'medium', label: 'Medium' },
  { value: 'tumblr', label: 'Tumblr' },
  { value: 'vimeo', label: 'Vimeo' },
  { value: 'soundcloud', label: 'SoundCloud' },
  { value: 'xing', label: 'Xing' }
]

// The settings panel of an mj-social-element block
interface MjSocialElementSettingsPanelProps {
  blockId: string
  currentAttributes: MJSocialElementAttributes
  /** The element's label, shown beside its icon */
  content?: string
  /** What the element inherits: its defaults, what its mj-social passes, the template's default attributes */
  blockDefaults: Record<string, unknown>
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

// The font group of an element: its size is a row of its own (8 to 48 px), beside its colour
const ELEMENT_FONT_FIELDS = ['fontFamily', 'fontWeight', 'fontStyle', 'textDecoration', 'lineHeight'] as const
const ELEMENT_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const

const MjSocialElementSettingsPanel: React.FC<MjSocialElementSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  content,
  blockDefaults,
  onUpdate,
  emailTree
}) => {
  const { t } = useLingui()
  const f = panelFields(blockId, currentAttributes as Record<string, unknown>, blockDefaults, onUpdate)
  const colors = useEmailColors(emailTree)
  const importedFonts = useImportedFonts(emailTree)
  const alignOptions = useAlignOptions()
  const verticalAlignOptions = useVerticalAlignOptions()
  const targetOptions = useLinkTargetOptions()

  const baseName = getBaseNetworkName(currentAttributes.name)
  const networkOptions = [
    { value: 'custom', label: t`Custom` },
    ...NETWORKS.slice(0, 1),
    // Kept for an element saved as twitter, which still shows and edits as such
    ...(baseName === 'twitter' ? [{ value: 'twitter', label: 'Twitter' }] : []),
    ...NETWORKS.slice(1),
    { value: 'web', label: t`Website` }
  ]
  const networkLabel = networkOptions.find((option) => option.value === baseName)?.label

  const fontValues = Object.fromEntries(
    [...ELEMENT_FONT_FIELDS, 'fontSize'].map((key) => [key, f.value(key)])
  )
  const fontInherited = Object.fromEntries(
    [...ELEMENT_FONT_FIELDS, 'fontSize'].map((key) => [key, f.inherited(key)])
  )

  // The element's padding sides inherit the padding its mj-social passes (its inner-padding), else 4px
  const passedPadding = parsePaddingShorthand(f.inherited('padding'))
  const inheritedSides = inheritedSidesOf(f, 'padding')
  const elementPaddingInherited: SpacingSides = {
    top: inheritedSides.top ?? passedPadding.top,
    right: inheritedSides.right ?? passedPadding.right,
    bottom: inheritedSides.bottom ?? passedPadding.bottom,
    left: inheritedSides.left ?? passedPadding.left
  }

  // Without a height, the icon is as high as its size
  const iconHeightPlaceholder = String(
    pixels(f.value('iconSize')) ?? pixels(f.inherited('iconSize')) ?? 20
  )
  const contentValue = content === '' ? undefined : content

  return (
    <>
      <SettingsSection
        sectionKey="mj-social-element:network"
        title={t`Network`}
        first
        setCount={f.setCount('name', 'href', 'src', 'alt') + (contentValue ? 1 : 0)}
        summary={networkLabel}
      >
        <SettingsRow label={t`Network`} isSet={f.isSet('name')} onReset={f.reset('name')}>
          <Select
            size="small"
            className="w-full"
            aria-label={t`Social network`}
            value={baseName || 'custom'}
            options={networkOptions}
            onChange={(value: string) => {
              if (value === 'custom') {
                f.write('name')(undefined)
              } else if (NETWORKS_WITH_SHARE_URL.has(value)) {
                f.write('name')(value + '-noshare')
              } else {
                f.write('name')(value)
              }
            }}
          />
        </SettingsRow>
        <SettingsRow label={t`Link`} isSet={f.isSet('href')} onReset={f.reset('href')}>
          <DraftTextField
            aria-label={t`Link URL`}
            value={f.value('href')}
            onChange={f.write('href')}
            placeholder={t`https://example.com or ${LIQUID_URL}`}
            validate="url"
            mono
          />
        </SettingsRow>
        {NETWORKS_WITH_SHARE_URL.has(baseName) && (
          <SettingsRow
            label={t`Share link`}
            help={t`When enabled, the link will prompt visitors to share the URL on this social network. When disabled, the link opens directly.`}
          >
            <Switch
              size="small"
              aria-label={t`Share link`}
              checked={isShareMode(currentAttributes.name)}
              onChange={(checked) => f.write('name')(checked ? baseName : baseName + '-noshare')}
            />
          </SettingsRow>
        )}
        <SettingsRow
          label={t`Icon`}
          help={t`Empty: the network's icon. A URL replaces it, on a network too.`}
          isSet={f.isSet('src')}
          onReset={f.reset('src')}
        >
          <DraftTextField
            aria-label={t`Custom icon URL`}
            value={f.value('src')}
            onChange={f.write('src')}
            placeholder={networkLabel && baseName !== 'web' ? t`${networkLabel} icon · URL to replace it` : 'https://example.com/icon.png'}
            mono
          />
        </SettingsRow>
        <SettingsRow label={t`Alt text`} isSet={f.isSet('alt')} onReset={f.reset('alt')}>
          <DraftTextField
            aria-label={t`Alt text`}
            value={f.value('alt')}
            onChange={f.write('alt')}
            placeholder={t`Social media icon`}
          />
        </SettingsRow>
        <SettingsRow
          label={t`Label`}
          help={t`The text shown beside the icon.`}
          isSet={contentValue !== undefined}
          onReset={() => onUpdate({ content: undefined })}
        >
          <DraftTextField
            aria-label={t`Label`}
            value={contentValue}
            onChange={(value) => onUpdate({ content: value })}
            placeholder={t`Text beside the icon`}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-social-element:icon"
        title={t`Icon`}
        setCount={f.setCount('iconSize', 'iconHeight', 'backgroundColor', 'borderRadius', 'border', 'verticalAlign', 'align')}
      >
        <SettingsRow
          label={t`Size, height`}
          help={t`10 to 100 px. Without a height, the icon is as high as its size.`}
          isSet={f.isSet('iconSize', 'iconHeight')}
          onReset={f.reset('iconSize', 'iconHeight')}
        >
          <div className="grid grid-cols-2 gap-1.5">
            <NumberUnitField
              aria-label={t`Icon size`}
              value={f.value('iconSize')}
              inherited={f.inherited('iconSize')}
              placeholder="20"
              units={['px']}
              min={10}
              max={100}
              coalesceBase={f.base('iconSize')}
              onChange={f.write('iconSize')}
            />
            <NumberUnitField
              aria-label={t`Icon height`}
              value={f.value('iconHeight')}
              inherited={f.inherited('iconHeight')}
              placeholder={iconHeightPlaceholder}
              units={['px']}
              min={10}
              max={100}
              coalesceBase={f.base('iconHeight')}
              onChange={f.write('iconHeight')}
            />
          </div>
        </SettingsRow>
        <SettingsRow label={t`Background`} isSet={f.isSet('backgroundColor')} onReset={f.reset('backgroundColor')}>
          <ColorField
            aria-label={t`Icon background`}
            value={f.value('backgroundColor')}
            inherited={f.inherited('backgroundColor')}
            emailColors={colors}
            onChange={f.write('backgroundColor')}
          />
        </SettingsRow>
        <SettingsRow label={t`Corners`} isSet={f.isSet('borderRadius')} onReset={f.reset('borderRadius')}>
          <CornersField
            aria-label={t`Corners`}
            value={f.value('borderRadius')}
            inherited={f.inherited('borderRadius')}
            coalesceBase={f.base('borderRadius')}
            onChange={f.write('borderRadius')}
          />
        </SettingsRow>
        <SettingsRow
          label={t`Border`}
          help={t`Around the icon. None by default, else the border of its social block.`}
          vertical
          isSet={f.isSet('border')}
          onReset={f.reset('border')}
        >
          <SingleBorderField value={f.value('border')} onChange={f.write('border')} noneValue="none" emailColors={colors} />
        </SettingsRow>
        <SettingsRow label={t`Vertical align`} isSet={f.isSet('verticalAlign')} onReset={f.reset('verticalAlign')}>
          <SegmentedField
            aria-label={t`Vertical align`}
            value={f.value('verticalAlign')}
            inherited={f.inherited('verticalAlign')}
            options={verticalAlignOptions}
            onChange={f.write('verticalAlign')}
          />
        </SettingsRow>
        <SettingsRow label={t`Alignment`} isSet={f.isSet('align')} onReset={f.reset('align')}>
          <SegmentedField
            aria-label={t`Alignment`}
            value={f.value('align')}
            inherited={f.inherited('align')}
            options={alignOptions}
            onChange={f.write('align')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-social-element:text"
        title={t`Text`}
        setCount={f.setCount('color', 'fontSize', ...ELEMENT_FONT_FIELDS)}
        summary={[f.value('fontFamily')?.split(',')[0], f.value('fontSize')].filter(Boolean).join(' · ')}
      >
        <SettingsRow
          label={t`Color, size`}
          help={t`Font size: 8 to 48 px.`}
          isSet={f.isSet('color', 'fontSize')}
          onReset={f.reset('color', 'fontSize')}
        >
          <div className="grid grid-cols-2 gap-1.5">
            <ColorField
              aria-label={t`Text color`}
              value={f.value('color')}
              inherited={f.inherited('color')}
              emailColors={colors}
              onChange={f.write('color')}
            />
            <NumberUnitField
              aria-label={t`Font size`}
              value={f.value('fontSize')}
              inherited={f.inherited('fontSize')}
              placeholder="13"
              units={['px']}
              min={8}
              max={48}
              coalesceBase={f.base('fontSize')}
              onChange={f.write('fontSize')}
            />
          </div>
        </SettingsRow>
        <FontFields
          value={fontValues}
          inherited={fontInherited}
          importedFonts={importedFonts}
          fields={ELEMENT_FONT_FIELDS}
          onChange={(updates, options) => f.writeMany(updates, options)}
        />
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-social-element:link"
        title={t`Link`}
        setCount={f.setCount('target', 'rel', 'title')}
        summary={targetOptions.find((option) => option.value === f.value('target'))?.label}
      >
        <SettingsRow label={t`Open in`} isSet={f.isSet('target')} onReset={f.reset('target')}>
          <Select
            size="small"
            className="w-full"
            aria-label={t`Open in`}
            value={f.value('target')}
            placeholder={targetOptions.find((option) => option.value === (f.inherited('target') ?? '_blank'))?.label}
            options={targetOptions}
            onChange={(value: string) => f.write('target')(value)}
          />
        </SettingsRow>
        <SettingsRow label={t`Rel`} help={t`The link's rel attribute, e.g. noopener.`} isSet={f.isSet('rel')} onReset={f.reset('rel')}>
          <DraftTextField aria-label={t`Rel`} value={f.value('rel')} onChange={f.write('rel')} placeholder="noopener" mono />
        </SettingsRow>
        <SettingsRow
          label={t`Tooltip`}
          help={t`Shown when the pointer rests on the icon (title attribute).`}
          isSet={f.isSet('title')}
          onReset={f.reset('title')}
        >
          <DraftTextField aria-label={t`Tooltip`} value={f.value('title')} onChange={f.write('title')} placeholder={t`Tooltip text`} />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-social-element:spacing"
        title={t`Spacing`}
        setCount={f.setCount('iconPadding', 'textPadding', ...ELEMENT_PADDING)}
      >
        <SettingsRow
          label={t`Icon padding`}
          help={t`Around the icon's image, as a CSS padding (e.g. 4px 8px).`}
          isSet={f.isSet('iconPadding')}
          onReset={f.reset('iconPadding')}
        >
          <DraftTextField
            aria-label={t`Icon padding`}
            value={f.value('iconPadding')}
            onChange={f.write('iconPadding')}
            placeholder={f.inherited('iconPadding') ?? '0px'}
            mono
          />
        </SettingsRow>
        <SettingsRow
          label={t`Text padding`}
          help={t`Around the text, as a CSS padding (e.g. 4px 4px 4px 0).`}
          isSet={f.isSet('textPadding')}
          onReset={f.reset('textPadding')}
        >
          <DraftTextField
            aria-label={t`Text padding`}
            value={f.value('textPadding')}
            onChange={f.write('textPadding')}
            placeholder={f.inherited('textPadding') ?? '4px 4px 4px 0'}
            mono
          />
        </SettingsRow>
        <div className="mt-2">
          <SpacingField
            label={t`Around the element`}
            centerLabel={t`Icon and text`}
            value={sidesOf(f, 'padding')}
            inherited={elementPaddingInherited}
            onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
              f.writeMany(toSideAttributes(sides, 'padding'), options)
            }
          />
        </div>
      </SettingsSection>

      <AdvancedSection type="mj-social-element" fields={f} />
    </>
  )
}

// A CSS padding shorthand ("4px", "4px 8px", "1px 2px 3px", "1px 2px 3px 4px") with the sides given replacing
// its own
export function paddingShorthand(
  shorthand: string,
  sides: { top?: string; right?: string; bottom?: string; left?: string }
): string {
  const parts = shorthand.trim().split(/\s+/)
  const top = parts[0]
  const right = parts[1] ?? top
  const bottom = parts[2] ?? top
  const left = parts[3] ?? right
  return [sides.top || top, sides.right || right, sides.bottom || bottom, sides.left || left].join(' ')
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

    const baseName = name?.toLowerCase().replace(/-noshare$/, '')
    switch (baseName) {
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
    const baseName = currentAttributes.name?.replace(/-noshare$/, '')
    return baseName
      ? baseName.charAt(0).toUpperCase() + baseName.slice(1)
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
   * Render the settings panel for the social element block. blockDefaults is what the element
   * inherits: its defaults, what its mj-social passes, then the template's default attributes.
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = (this.block.attributes ?? {}) as MJSocialElementAttributes
    const content = (this.block as { content?: unknown }).content

    return (
      <MjSocialElementSettingsPanel
        blockId={this.block.id}
        currentAttributes={currentAttributes}
        content={typeof content === 'string' ? content : undefined}
        blockDefaults={blockDefaults as Record<string, unknown>}
        onUpdate={onUpdate}
        emailTree={emailTree}
      />
    )
  }

  /**
   * Parse pixel value from string (e.g., "20px" -> 20)
   */
  private parsePixelValue(value?: string): number | undefined {
    return pixels(value)
  }

  /**
   * Get the default background color for a social network
   * Since we're using true color icons, we use transparent backgrounds to avoid hiding the icon
   */
  private getNetworkDefaultColor(name?: string): string {
    const baseName = (name || '').replace(/-noshare$/, '')
    const networkColors: Record<string, string> = {
      facebook: 'transparent',
      twitter: 'transparent',
      x: 'transparent',
      google: 'transparent',
      pinterest: 'transparent',
      linkedin: 'transparent',
      tumblr: 'transparent',
      xing: 'transparent',
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
    return networkColors[baseName] || 'transparent'
  }

  /**
   * Get the icon URL for a social network from MageCDN
   */
  private getSocialIcon(name?: string): string {
    const baseName = (name || '').replace(/-noshare$/, '')
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
      iconUrls[baseName] || 'https://www.mailjet.com/images/theme/v1/icons/ico-social/web.png'
    )
  }

  /**
   * @param inherited the attributes the parent mj-social passes to its elements (see MjSocialBlock). As in MJML,
   * they win over the element's defaults and lose to the template's default mj-social-element attributes, which
   * mjml-core writes into the element's attributes when it parses the template, and to the element's own
   */
  getEdit(props: PreviewProps, inherited: Record<string, unknown> = {}): React.ReactNode {
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
      { ...attributeDefaults, 'mj-social-element': { ...inherited, ...attributeDefaults['mj-social-element'] } }
    )

    // Get content (text label) if available
    const blockWithContent = this.block as unknown as Record<string, unknown>
    const content = typeof blockWithContent.content === 'string' ? blockWithContent.content : ''

    // Get icon source URL (either custom src or MageCDN URL)
    const iconSrc = attrs.src || this.getSocialIcon(attrs.name)

    // Parse dimensions
    const iconWidth = this.parsePixelValue(attrs.iconSize) || 20
    // Without a height of its own, an icon is as high as it is wide, as in MJML
    const iconHeight = this.parsePixelValue(attrs.iconHeight) || iconWidth

    // Table cell padding style (outer wrapper)
    // The padding shorthand (mj-social's inner-padding reaches the elements as it), each side set replacing its
    // part, written as one shorthand: React does not put a shorthand back when a longhand beside it goes away
    const cellPaddingStyle: React.CSSProperties = {
      padding: paddingShorthand(attrs.padding || '4px', {
        top: attrs.paddingTop,
        right: attrs.paddingRight,
        bottom: attrs.paddingBottom,
        left: attrs.paddingLeft
      }),
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
      // The border goes around the icon, as in the email (the console's CSS would put it inside its size)
      boxSizing: 'content-box',
      border: attrs.border || '0',
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

    const href = isSafeHref(attrs.href) ? attrs.href : '#'

    // Link wrapper for icon
    const iconLink = (
      <a
        href={href}
        target={attrs.target || '_blank'}
        rel={attrs.rel}
        title={attrs.title}
        onClick={(e) => e.preventDefault()} // Prevent navigation in preview
        onAuxClick={(e) => e.preventDefault()}
        style={{ textDecoration: 'none' }}
      >
        {iconElement}
      </a>
    )

    // Text link (if content exists)
    const textLink = content ? (
      <a
        href={href}
        target={attrs.target || '_blank'}
        rel={attrs.rel}
        title={attrs.title}
        style={textLinkStyle}
        onClick={(e) => e.preventDefault()} // Prevent navigation in preview
        onAuxClick={(e) => e.preventDefault()}
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
