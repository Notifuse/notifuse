import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faImage } from '@fortawesome/free-solid-svg-icons'
import { Select, Switch } from 'antd'
import type { MJMLComponentType, EmailBlock, MJImageAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { isSafeHref } from '../../../utils/safeHtml'
import { EmailBlockClass } from '../EmailBlockClass'
import { hasLiquidOutput } from '../../../utils/liquid'
import FileSrc from '../ui/FileSrc'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { DraftTextField } from '../ui/fields/DraftTextField'
import { ColorField } from '../ui/fields/ColorField'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { SegmentedField } from '../ui/fields/SegmentedField'
import { useAlignOptions } from '../ui/fields/segmentedOptions'
import { CornersField } from '../ui/fields/CornersField'
import BorderField from '../ui/fields/BorderField'
import { SpacingField } from '../ui/fields/SpacingField'
import { toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { inheritedSidesOf, panelFields, sidesOf, useEmailColors, useLinkTargetOptions } from './panelFields'

// Liquid examples go into messages as variables: braces in a message are placeholders
const LIQUID_URL = '{{ url }}'

// Functional component for settings panel with i18n support
interface MjImageSettingsPanelProps {
  blockId: string
  currentAttributes: MJImageAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

const IMAGE_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const
const IMAGE_BORDERS = ['borderTop', 'borderRight', 'borderBottom', 'borderLeft'] as const

// The image the URL points to, above the field; a Liquid URL or another scheme draws none.
const canPreview = (src: string | undefined): src is string =>
  Boolean(src) && /^https?:\/\//i.test(src ?? '') && !hasLiquidOutput(src ?? '')

const MjImageSettingsPanel: React.FC<MjImageSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  blockDefaults,
  onUpdate,
  emailTree
}) => {
  const { t } = useLingui()
  const f = panelFields(blockId, currentAttributes as Record<string, unknown>, blockDefaults as Record<string, unknown>, onUpdate)
  const colors = useEmailColors(emailTree)
  const alignOptions = useAlignOptions()
  const targetOptions = useLinkTargetOptions()
  const src = f.value('src')
  const fluidOnMobile = (f.value('fluidOnMobile') ?? f.inherited('fluidOnMobile')) === 'true'

  return (
    <>
      <SettingsSection
        sectionKey="mj-image:image"
        title={t`Image`}
        first
        setCount={f.setCount('src', 'alt', 'title')}
        summary={src}
      >
        {canPreview(src) ? (
          <div className="mb-2 flex h-24 items-center justify-center overflow-hidden rounded-md border border-solid border-line-soft bg-fill">
            <img src={src} alt={f.value('alt') ?? ''} className="max-h-full max-w-full object-contain" />
          </div>
        ) : null}
        <SettingsRow label={t`URL`} isSet={f.isSet('src')} onReset={f.reset('src')}>
          <FileSrc
            size="small"
            aria-label={t`Image URL`}
            value={src ?? ''}
            onChange={(value) => f.write('src')(value || undefined)}
            placeholder={t`Enter image URL`}
            acceptFileType="image/*"
            acceptItem={(item) => !item.is_folder && (item.file_info?.content_type?.startsWith('image/') ?? false)}
            buttonText={t`Browse images`}
            targetLabel={t`the Image block`}
          />
        </SettingsRow>
        <SettingsRow
          label={t`Alt text`}
          help={t`Read by screen readers, and shown when the image does not load.`}
          isSet={f.isSet('alt')}
          onReset={f.reset('alt')}
        >
          <DraftTextField aria-label={t`Alt text`} value={f.value('alt')} onChange={f.write('alt')} placeholder={t`Alternative text`} />
        </SettingsRow>
        <SettingsRow
          label={t`Tooltip`}
          help={t`Shown when the pointer rests on the image (title attribute).`}
          isSet={f.isSet('title')}
          onReset={f.reset('title')}
        >
          <DraftTextField aria-label={t`Tooltip`} value={f.value('title')} onChange={f.write('title')} placeholder={t`Text on hover`} />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-image:link"
        title={t`Link`}
        setCount={f.setCount('href', 'target', 'rel')}
        summary={f.value('href') ?? t`No link`}
      >
        <SettingsRow label={t`URL`} isSet={f.isSet('href')} onReset={f.reset('href')}>
          <DraftTextField
            aria-label={t`Link URL`}
            value={f.value('href')}
            onChange={f.write('href')}
            placeholder={t`https://… or ${LIQUID_URL}`}
            validate="url"
            mono
          />
        </SettingsRow>
        <SettingsRow label={t`Open in`} isSet={f.isSet('target')} onReset={f.reset('target')}>
          <Select
            size="small"
            className="w-full"
            aria-label={t`Open in`}
            value={f.value('target')}
            placeholder={targetOptions.find((option) => option.value === (f.inherited('target') ?? '_blank'))?.label}
            options={targetOptions}
            onChange={(value) => f.write('target')(value)}
          />
        </SettingsRow>
        <SettingsRow label={t`Rel`} help={t`The link's rel attribute, e.g. noopener.`} isSet={f.isSet('rel')} onReset={f.reset('rel')}>
          <DraftTextField
            aria-label={t`Rel`}
            value={f.value('rel')}
            onChange={f.write('rel')}
            placeholder={f.inherited('rel') ?? 'noopener'}
            mono
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-image:size"
        title={t`Size`}
        setCount={f.setCount('width', 'height', 'fluidOnMobile', 'align')}
        summary={[f.value('width'), f.value('align')].filter(Boolean).join(' · ')}
      >
        <SettingsRow
          label={t`Width`}
          help={t`Leave empty for full container width`}
          isSet={f.isSet('width')}
          onReset={f.reset('width')}
        >
          <NumberUnitField
            aria-label={t`Image width`}
            value={f.value('width')}
            inherited={f.inherited('width')}
            placeholder={t`Auto`}
            units={['px']}
            max={2000}
            coalesceBase={f.base('width')}
            onChange={f.write('width')}
          />
        </SettingsRow>
        <SettingsRow label={t`Height`} isSet={f.isSet('height')} onReset={f.reset('height')}>
          <NumberUnitField
            aria-label={t`Image height`}
            mode="height"
            value={f.value('height')}
            inherited={f.inherited('height')}
            placeholder={t`Auto`}
            coalesceBase={f.base('height')}
            onChange={f.write('height')}
          />
        </SettingsRow>
        <SettingsRow
          label={t`Full width on mobile`}
          help={t`On mobile, the image takes the full width even when a width is set.`}
          isSet={f.isSet('fluidOnMobile')}
          onReset={f.reset('fluidOnMobile')}
        >
          <Switch
            size="small"
            aria-label={t`Full width on mobile`}
            checked={fluidOnMobile}
            onChange={(checked) => f.write('fluidOnMobile')(checked ? 'true' : 'false')}
          />
        </SettingsRow>
        <SettingsRow label={t`Alignment`} isSet={f.isSet('align')} onReset={f.reset('align')}>
          <SegmentedField
            aria-label={t`Image alignment`}
            value={f.value('align')}
            inherited={f.inherited('align')}
            options={alignOptions}
            onChange={f.write('align')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-image:border"
        title={t`Border and corners`}
        setCount={f.setCount('borderRadius', ...IMAGE_BORDERS)}
        summary={f.value('borderRadius')}
      >
        <SettingsRow label={t`Border`} vertical isSet={f.isSet(...IMAGE_BORDERS)} onReset={f.reset(...IMAGE_BORDERS)}>
          <BorderField
            emailColors={colors}
            value={Object.fromEntries(IMAGE_BORDERS.map((key) => [key, f.value(key)]))}
            onChange={(borders, options) => f.writeMany({ ...borders }, options)}
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
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-image:colors"
        title={t`Colors`}
        setCount={f.setCount('containerBackgroundColor')}
      >
        <SettingsRow
          label={t`Container background`}
          isSet={f.isSet('containerBackgroundColor')}
          onReset={f.reset('containerBackgroundColor')}
        >
          <ColorField
            aria-label={t`Container background`}
            value={f.value('containerBackgroundColor')}
            inherited={f.inherited('containerBackgroundColor')}
            emailColors={colors}
            onChange={f.write('containerBackgroundColor')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection sectionKey="mj-image:spacing" title={t`Spacing`} setCount={f.setCount(...IMAGE_PADDING)}>
        <SpacingField
          label={t`Around the image`}
          centerLabel={t`Image`}
          value={sidesOf(f, 'padding')}
          inherited={inheritedSidesOf(f, 'padding')}
          onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
            f.writeMany(toSideAttributes(sides, 'padding'), options)
          }
        />
      </SettingsSection>

      <AdvancedSection type="mj-image" fields={f} />
    </>
  )
}

/**
 * Implementation for mj-image blocks
 */
export class MjImageBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faImage} className="opacity-70" />
  }

  getLabel(): string {
    return 'Image'
  }

  getDescription(): React.ReactNode {
    return (
      <div>
        <div style={{ marginBottom: 8 }}>Display images, logos, and visual content</div>
        <div style={{ opacity: 0.7 }}>
          <div
            style={{
              width: 60,
              height: 30,
              border: '2px solid #13c2c2',
              borderRadius: 4,
              backgroundColor: '#e6fffb',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              position: 'relative'
            }}
          >
            <div
              style={{
                width: 40,
                height: 20,
                backgroundColor: '#87e8de',
                borderRadius: 2,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center'
              }}
            >
              <FontAwesomeIcon icon={faImage} style={{ fontSize: 10, color: '#13c2c2' }} />
            </div>
          </div>
        </div>
      </div>
    )
  }

  getCategory(): 'content' | 'layout' {
    return 'content'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-image'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const {
      selectedBlockId,
      onSelectBlock,
      attributeDefaults
    } = props

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
      'mj-image',
      this.block.attributes as Record<string, unknown> | undefined,
      attributeDefaults
    )

    // MJML wraps images in a table structure - outer container gets padding and alignment
    const containerStyle: React.CSSProperties = {
      fontSize: '0px',
      padding: `${attrs.paddingTop || '10px'} ${attrs.paddingRight || '25px'} ${
        attrs.paddingBottom || '10px'
      } ${attrs.paddingLeft || '25px'}`,
      wordBreak: 'break-word',
      backgroundColor: attrs.containerBackgroundColor,
      textAlign: attrs.align as React.CSSProperties['textAlign'], // This handles the image alignment
      ...selectionStyle
    }

    // MJML table style for image presentation
    // Per MJML spec: "if no width is provided, the image will use the parent column width"
    // When explicit width is set, use inline-block so alignment (text-align) works
    // When no width, use block with 100% width to fill the column
    const hasExplicitWidth = attrs.width && attrs.width !== '100%'
    // Without a width, a real table at 100 %: as a block, the table's rows sat in an anonymous
    // table sized to the image, which drew it at its natural size on the left, whatever the Align.
    const tableStyle: React.CSSProperties = {
      borderCollapse: 'collapse',
      borderSpacing: '0px',
      display: hasExplicitWidth ? 'inline-block' : 'table',
      width: hasExplicitWidth ? undefined : '100%'
    }

    // MJML inner cell style - contains width constraint
    const cellStyle: React.CSSProperties = {
      width: hasExplicitWidth ? attrs.width : '100%'
    }

    // MJML image style - matches actual generated styles
    const imageStyle: React.CSSProperties = {
      border: attrs.border || 'none',
      borderTop: attrs.borderTop,
      borderRight: attrs.borderRight,
      borderBottom: attrs.borderBottom,
      borderLeft: attrs.borderLeft,
      borderRadius: attrs.borderRadius,
      display: 'block',
      outline: 'none',
      textDecoration: 'none',
      height: attrs.height || 'auto',
      width: '100%', // MJML always sets width to 100% on the img element
      fontSize: '13px' // MJML sets this font-size on images
    }

    const imageElement = (
      <img
        src={attrs.src || 'https://via.placeholder.com/300x200'}
        alt={attrs.alt || ''}
        title={attrs.title}
        useMap={attrs.usemap}
        sizes={attrs.sizes}
        srcSet={attrs.srcset}
        style={imageStyle}
        width={attrs.width ? parseInt(attrs.width) : undefined}
        height={attrs.height && attrs.height !== 'auto' ? parseInt(attrs.height) : undefined}
      />
    )

    const contentElement = attrs.href ? (
      <a
        href={isSafeHref(attrs.href) ? attrs.href : undefined}
        target={attrs.target || '_blank'}
        rel={attrs.rel}
        style={{
          textDecoration: 'none',
          display: 'block',
          borderRadius: attrs.borderRadius,
          overflow: 'hidden'
        }}
        onClick={(e) => e.preventDefault()}
        onAuxClick={(e) => e.preventDefault()}
      >
        {imageElement}
      </a>
    ) : (
      imageElement
    )

    // Simulate MJML's exact structure: table > tbody > tr > td > img
    return (
      <div
        key={key}
        className={`${attrs.cssClass || ''} ${blockClasses}`.trim()}
        onClick={handleClick}
        style={{ ...containerStyle, position: 'relative' }}
        data-block-id={this.block.id}
        data-display-condition={this.block.display_condition ? "true" : undefined}
      >
        <table border={0} cellPadding="0" cellSpacing="0" role="presentation" style={tableStyle}>
          <tbody>
            <tr>
              <td style={cellStyle}>{contentElement}</td>
            </tr>
          </tbody>
        </table>
      </div>
    )
  }

  /**
   * Render the settings panel for the image block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    return (
      <MjImageSettingsPanel
        blockId={this.block.id}
        currentAttributes={this.block.attributes as MJImageAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
        emailTree={emailTree}
      />
    )
  }
}
