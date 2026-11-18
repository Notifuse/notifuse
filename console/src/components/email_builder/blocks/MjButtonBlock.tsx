import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Select } from 'antd'
import type { MJMLComponentType, EmailBlock, MJButtonAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { containedContentStyle, sanitizeHtml } from '../../../utils/safeHtml'
import { EmailBlockClass } from '../EmailBlockClass'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { DraftTextField } from '../ui/fields/DraftTextField'
import { ColorField } from '../ui/fields/ColorField'
import { FontFields } from '../ui/fields/FontFields'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { SegmentedField } from '../ui/fields/SegmentedField'
import { useAlignOptions } from '../ui/fields/segmentedOptions'
import { CornersField } from '../ui/fields/CornersField'
import BorderField from '../ui/fields/BorderField'
import { SpacingField } from '../ui/fields/SpacingField'
import { toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { useImportedFonts, inheritedSidesOf, panelFields, sidesOf, useEmailColors, useLinkTargetOptions } from './panelFields'
import { TiptapInlineEditor } from '../ui/tiptap'

// Liquid examples go into messages as variables: braces in a message are placeholders
const LIQUID_URL = '{{ url }}'

// Local state management for button content to prevent unnecessary re-renders
const useButtonContentState = (
  initialContent: string,
  blockId: string,
  onUpdateBlock?: (blockId: string, updatedBlock: EmailBlock) => void,
  block?: EmailBlock
) => {
  const [localContent, setLocalContent] = React.useState(initialContent)
  const [isEditing, setIsEditing] = React.useState(false)
  const updateTimeoutRef = React.useRef<number | undefined>(undefined)
  const lastSavedContentRef = React.useRef(initialContent)

  // Capture block data and update function when editing starts to avoid race conditions
  const editingContextRef = React.useRef<{
    blockData: EmailBlock
    onUpdateBlock: (blockId: string, updatedBlock: EmailBlock) => void
  } | null>(null)

  // Update local content when the prop content changes (from external sources)
  React.useEffect(() => {
    if (!isEditing && initialContent !== lastSavedContentRef.current) {
      setLocalContent(initialContent)
      lastSavedContentRef.current = initialContent
    }
  }, [initialContent, isEditing])

  // Capture editing context when editing starts
  React.useEffect(() => {
    if (isEditing && !editingContextRef.current && block && onUpdateBlock) {
      // Create a deep copy of the block to avoid mutations
      editingContextRef.current = {
        blockData: JSON.parse(JSON.stringify(block)),
        onUpdateBlock
      }
    } else if (!isEditing) {
      editingContextRef.current = null
    }
  }, [isEditing, block, onUpdateBlock])

  // Debounced update function
  const debouncedUpdate = React.useCallback(
    (content: string) => {
      if (updateTimeoutRef.current) {
        clearTimeout(updateTimeoutRef.current)
      }

      updateTimeoutRef.current = window.setTimeout(() => {
        const context = editingContextRef.current
        if (context && content !== lastSavedContentRef.current) {
          // Use the captured block data to avoid race conditions
          const updatedBlock = {
            ...context.blockData,
            content: content
          } as EmailBlock

          context.onUpdateBlock(blockId, updatedBlock)
          lastSavedContentRef.current = content
        }
        setIsEditing(false)
      }, 500) // 500ms debounce
    },
    [blockId]
  )

  // Handle content change from editor
  const handleContentChange = React.useCallback(
    (newContent: string) => {
      setLocalContent(newContent)
      setIsEditing(true)
      debouncedUpdate(newContent)
    },
    [debouncedUpdate]
  )

  // Immediate save function for when editor loses focus
  const handleSave = React.useCallback(() => {
    if (updateTimeoutRef.current) {
      clearTimeout(updateTimeoutRef.current)
      updateTimeoutRef.current = undefined
    }

    const context = editingContextRef.current
    if (context && localContent !== lastSavedContentRef.current) {
      const updatedBlock = {
        ...context.blockData,
        content: localContent
      } as EmailBlock

      context.onUpdateBlock(blockId, updatedBlock)
      lastSavedContentRef.current = localContent
    }
    setIsEditing(false)
  }, [blockId, localContent])

  // Cleanup timeout on unmount
  React.useEffect(() => {
    return () => {
      if (updateTimeoutRef.current) {
        clearTimeout(updateTimeoutRef.current)
        updateTimeoutRef.current = undefined
      }
    }
  }, [])

  return {
    localContent,
    handleContentChange,
    handleSave,
    isEditing
  }
}

// Functional component for settings panel with i18n support
interface MjButtonSettingsPanelProps {
  blockId: string
  currentAttributes: MJButtonAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

const BUTTON_FONT_FIELDS = [
  'fontFamily',
  'fontSize',
  'fontWeight',
  'fontStyle',
  'textTransform',
  'textDecoration',
  'lineHeight',
  'letterSpacing',
  'textAlign'
] as const
const BUTTON_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const
const BUTTON_BORDERS = ['borderTop', 'borderRight', 'borderBottom', 'borderLeft'] as const

const MjButtonSettingsPanel: React.FC<MjButtonSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  blockDefaults,
  onUpdate,
  emailTree
}) => {
  const { t } = useLingui()
  const f = panelFields(blockId, currentAttributes as Record<string, unknown>, blockDefaults as Record<string, unknown>, onUpdate)
  const colors = useEmailColors(emailTree)
  const importedFonts = useImportedFonts(emailTree)
  const alignOptions = useAlignOptions()
  const targetOptions = useLinkTargetOptions()
  const fontValues = Object.fromEntries(BUTTON_FONT_FIELDS.map((key) => [key, f.value(key)]))
  const fontInherited = Object.fromEntries(BUTTON_FONT_FIELDS.map((key) => [key, f.inherited(key)]))
  const fontSummary = [f.value('fontFamily')?.split(',')[0], f.value('fontSize'), f.value('fontWeight')]
    .filter(Boolean)
    .join(' · ')

  return (
    <>
      <SettingsSection
        sectionKey="mj-button:link"
        title={t`Link`}
        first
        setCount={f.setCount('href', 'target', 'rel', 'title')}
        summary={f.value('href')}
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
          <DraftTextField aria-label={t`Rel`} value={f.value('rel')} onChange={f.write('rel')} placeholder="noopener" mono />
        </SettingsRow>
        <SettingsRow label={t`Tooltip`} help={t`Shown when the pointer rests on the button (title attribute).`} isSet={f.isSet('title')} onReset={f.reset('title')}>
          <DraftTextField aria-label={t`Tooltip`} value={f.value('title')} onChange={f.write('title')} placeholder={t`Text on hover`} />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-button:colors"
        title={t`Colors`}
        setCount={f.setCount('backgroundColor', 'color', 'containerBackgroundColor')}
      >
        <SettingsRow label={t`Button`} isSet={f.isSet('backgroundColor')} onReset={f.reset('backgroundColor')}>
          <ColorField
            aria-label={t`Button color`}
            value={f.value('backgroundColor')}
            inherited={f.inherited('backgroundColor')}
            emailColors={colors}
            onChange={f.write('backgroundColor')}
          />
        </SettingsRow>
        <SettingsRow label={t`Text`} isSet={f.isSet('color')} onReset={f.reset('color')}>
          <ColorField
            aria-label={t`Text color`}
            value={f.value('color')}
            inherited={f.inherited('color')}
            emailColors={colors}
            onChange={f.write('color')}
          />
        </SettingsRow>
        <SettingsRow label={t`Container background`} isSet={f.isSet('containerBackgroundColor')} onReset={f.reset('containerBackgroundColor')}>
          <ColorField
            aria-label={t`Container background`}
            value={f.value('containerBackgroundColor')}
            inherited={f.inherited('containerBackgroundColor')}
            emailColors={colors}
            onChange={f.write('containerBackgroundColor')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-button:font"
        title={t`Font`}
        setCount={f.setCount(...BUTTON_FONT_FIELDS)}
        summary={fontSummary}
      >
        <FontFields
          value={fontValues}
          inherited={fontInherited}
          importedFonts={importedFonts}
          fields={BUTTON_FONT_FIELDS}
          onChange={(updates, options) => f.writeMany(updates, options)}
        />
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-button:size"
        title={t`Size and border`}
        setCount={f.setCount('width', 'height', 'align', 'borderRadius', ...BUTTON_BORDERS)}
        summary={[f.value('align'), f.value('borderRadius')].filter(Boolean).join(' · ')}
      >
        <SettingsRow label={t`Width`} isSet={f.isSet('width')} onReset={f.reset('width')}>
          <NumberUnitField
            aria-label={t`Button width`}
            value={f.value('width')}
            inherited={f.inherited('width')}
            placeholder={t`Auto`}
            units={['px']}
            max={1000}
            coalesceBase={f.base('width')}
            onChange={f.write('width')}
          />
        </SettingsRow>
        <SettingsRow label={t`Height`} isSet={f.isSet('height')} onReset={f.reset('height')}>
          <NumberUnitField
            aria-label={t`Button height`}
            value={f.value('height')}
            inherited={f.inherited('height')}
            placeholder={t`Auto`}
            units={['px']}
            max={1000}
            coalesceBase={f.base('height')}
            onChange={f.write('height')}
          />
        </SettingsRow>
        <SettingsRow label={t`Alignment`} isSet={f.isSet('align')} onReset={f.reset('align')}>
          <SegmentedField
            aria-label={t`Button alignment`}
            value={f.value('align')}
            inherited={f.inherited('align')}
            options={alignOptions}
            onChange={f.write('align')}
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
        <SettingsRow label={t`Border`} vertical isSet={f.isSet(...BUTTON_BORDERS)} onReset={f.reset(...BUTTON_BORDERS)}>
          <BorderField
            emailColors={colors}
            value={Object.fromEntries(BUTTON_BORDERS.map((key) => [key, f.value(key)]))}
            onChange={(borders, options) => f.writeMany({ ...borders }, options)}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-button:spacing"
        title={t`Spacing`}
        setCount={f.setCount(...BUTTON_PADDING, 'innerPadding')}
      >
        <SpacingField
          label={t`Around the button`}
          value={sidesOf(f, 'padding')}
          inherited={inheritedSidesOf(f, 'padding')}
          onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
            f.writeMany(toSideAttributes(sides, 'padding'), options)
          }
          inner={{
            label: t`Inside the button`,
            centerLabel: t`Text`,
            value: f.value('innerPadding'),
            inherited: f.inherited('innerPadding') ?? '10px 25px',
            onChange: (value: string | undefined, options?: FieldWriteOptions) =>
              f.writeMany({ innerPadding: value }, options)
          }}
        />
      </SettingsSection>

      <AdvancedSection type="mj-button" fields={f} />
    </>
  )
}

/**
 * Implementation for mj-button blocks
 */
export class MjButtonBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return (
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="svg-inline--fa"
      >
        <path d="M12.034 12.681a.498.498 0 0 1 .647-.647l9 3.5a.5.5 0 0 1-.033.943l-3.444 1.068a1 1 0 0 0-.66.66l-1.067 3.443a.5.5 0 0 1-.943.033z" />
        <path d="M21 11V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h6" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Button'
  }

  getDescription(): React.ReactNode {
    return 'Interactive call-to-action buttons with customizable styling'
  }

  getCategory(): 'content' | 'layout' {
    return 'content'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-button'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }



  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    return (
      <MjButtonSettingsPanel
        blockId={this.block.id}
        currentAttributes={this.block.attributes as MJButtonAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
        emailTree={emailTree}
      />
    )
  }


  getEdit(props: PreviewProps): React.ReactNode {
    // Return a functional wrapper component that can use hooks
    return <MjButtonBlockWrapper block={this.block} {...props} />
  }
}

// Functional wrapper component to handle hooks
 
const MjButtonBlockWrapper: React.FC<PreviewProps & { block: EmailBlock }> = ({
  block,
  selectedBlockId,
  onSelectBlock,
  onUpdateBlock,
  attributeDefaults
}) => {
  const { t } = useLingui()
  const key = block.id
  const isSelected = selectedBlockId === block.id
  const blockClasses = `email-block-hover ${isSelected ? 'selected' : ''}`.trim()

  const selectionStyle: React.CSSProperties = isSelected ? { position: 'relative', zIndex: 10 } : {}

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (onSelectBlock) {
      onSelectBlock(block.id)
    }
  }

  const attrs = EmailBlockClass.mergeWithAllDefaults(
    'mj-button',
    block.attributes as Record<string, unknown>,
    attributeDefaults
  )

  // Container style (equivalent to the td wrapper in MJML)
  const buttonContainerStyle: React.CSSProperties = {
    padding: `${attrs.paddingTop || '10px'} ${attrs.paddingRight || '25px'} ${
      attrs.paddingBottom || '10px'
    } ${attrs.paddingLeft || '25px'}`,
    textAlign: (attrs.align as React.CSSProperties['textAlign']) || 'center',
    backgroundColor: attrs.containerBackgroundColor,
    fontSize: '0px', // MJML sets font-size to 0 on container
    verticalAlign: 'middle', // MJML uses vertical-align: middle on container
    wordBreak: 'break-word',
    ...selectionStyle
  }

  // Button wrapper style (equivalent to the table structure in MJML)
  const buttonWrapperStyle: React.CSSProperties = {
    display: 'inline-block',
    verticalAlign: 'top',
    borderCollapse: 'separate',
    lineHeight: '100%'
  }

  // Button style (equivalent to the p element in MJML)
  const buttonStyle: React.CSSProperties = {
    display: 'inline-block',
    margin: '0',
    padding: attrs.innerPadding || '10px 25px',
    backgroundColor: attrs.backgroundColor,
    cursor: 'pointer',

    // Font styling attributes from FontStyleInput
    fontFamily: attrs.fontFamily,
    fontSize: attrs.fontSize,
    fontWeight: attrs.fontWeight,
    fontStyle: attrs.fontStyle,
    lineHeight: attrs.lineHeight || '120%', // MJML default is 120%
    letterSpacing: attrs.letterSpacing,
    textTransform: attrs.textTransform as React.CSSProperties['textTransform'],
    textDecoration: attrs.textDecoration || 'none',

    // Layout and positioning
    textAlign: attrs.textAlign as React.CSSProperties['textAlign'],
    verticalAlign: attrs.verticalAlign as React.CSSProperties['verticalAlign'],
    width: attrs.width,
    height: attrs.height,

    // Border styling
    border: attrs.border || 'none',
    borderTop: attrs.borderTop,
    borderRight: attrs.borderRight,
    borderBottom: attrs.borderBottom,
    borderLeft: attrs.borderLeft,
    borderRadius: attrs.borderRadius
  }

  const content = 'content' in block ? block.content : undefined

  // Ensure content is properly typed as string for Tiptap
  const htmlContent: string = typeof content === 'string' ? content : 'Button'

  // Use the custom hook for local state management
  const { localContent, handleContentChange, handleSave } = useButtonContentState(
    htmlContent,
    block.id,
    onUpdateBlock,
    block
  )

  // Track previous selection state to save when deselected
  const prevSelectedRef = React.useRef(isSelected)

  React.useEffect(() => {
    if (prevSelectedRef.current && !isSelected) {
      handleSave()
    }
    prevSelectedRef.current = isSelected
  }, [isSelected, handleSave])

  // Only shown while the block is not open in its editor, which has its own copy
  const canvasHtml = React.useMemo(
    () => (isSelected ? '' : sanitizeHtml(localContent)),
    [isSelected, localContent]
  )

  // Text content style for TiptapComponent
  const textStyle: React.CSSProperties = {
    color: attrs.color,
    fontFamily: attrs.fontFamily,
    fontSize: attrs.fontSize,
    fontWeight: attrs.fontWeight,
    fontStyle: attrs.fontStyle,
    lineHeight: attrs.lineHeight || '120%',
    letterSpacing: attrs.letterSpacing,
    textTransform: attrs.textTransform as React.CSSProperties['textTransform'],
    textDecoration: attrs.textDecoration || 'none',
    textAlign: attrs.textAlign as React.CSSProperties['textAlign'],
    backgroundColor: 'transparent',
    border: 'none',
    outline: 'none',
    width: '100%',
    minHeight: '1em'
  }

  return (
    <div
      key={key}
      style={{ ...buttonContainerStyle, position: 'relative' }}
      className={blockClasses}
      onClick={handleClick}
      data-block-id={block.id}
        data-display-condition={block.display_condition ? "true" : undefined}
    >
      <div style={buttonWrapperStyle}>
        <div style={buttonStyle}>
          {isSelected ? (
            <span
              style={{
                border: 'none',
                ...textStyle,
                outline: 'none',
                cursor: 'text'
              }}
            >
              <TiptapInlineEditor
                content={localContent}
                onChange={handleContentChange}
                readOnly={!isSelected}
                autoFocus={isSelected}
                placeholder={t`Enter button text...`}
                buttons={[
                  'undo',
                  'redo',
                  'bold',
                  'italic',
                  'underline',
                  'strikethrough',
                  'textColor',
                  'backgroundColor',
                  'emoji',
                  'superscript',
                  'subscript'
                ]}
              />
            </span>
          ) : (
            <span
              // Keeps what the button text draws to the button. On the text's
              // own box, made a block of its own for containment to apply, so a
              // fixed button size cuts off none of the text. Such a box takes no
              // text decoration from the button, so it carries the button's own.
              style={{
                color: attrs.color,
                display: 'inline-block',
                textDecoration: 'inherit',
                ...containedContentStyle
              }}
              dangerouslySetInnerHTML={{ __html: canvasHtml }}
              // Nothing in the button text acts on a click, a link or a label
              // included: the click goes on to select the block
              onClick={(e) => e.preventDefault()}
              onAuxClick={(e) => e.preventDefault()}
            />
          )}
        </div>
      </div>
    </div>
  )
}
