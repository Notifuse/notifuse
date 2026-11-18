import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faFont } from '@fortawesome/free-solid-svg-icons'
import type { MJMLComponentType, EmailBlock, MJTextAttributes, MergedBlockAttributes } from '../types'
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
import { ColorField } from '../ui/fields/ColorField'
import { FontFields } from '../ui/fields/FontFields'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { SpacingField } from '../ui/fields/SpacingField'
import { toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { useImportedFonts, inheritedSidesOf, panelFields, sidesOf, useEmailColors } from './panelFields'
import { TiptapRichEditor } from '../ui/tiptap'
import { findSkips, headingLevelBeforeBlock, headingOutline } from '../utils/headingOutline'

// Functional component for settings panel with i18n support
interface MjTextSettingsPanelProps {
  blockId: string
  currentAttributes: MJTextAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

// The font keys FontFields edits; mj-text names its alignment `align`, FontFields `textAlign`.
const TEXT_FONT_FIELDS = [
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
const TEXT_FONT_KEYS = TEXT_FONT_FIELDS.map((key) => (key === 'textAlign' ? 'align' : key))
const TEXT_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const

const attributeOfFontKey = (key: string) => (key === 'textAlign' ? 'align' : key)

const MjTextSettingsPanel: React.FC<MjTextSettingsPanelProps> = ({
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
  const fontValues = Object.fromEntries(TEXT_FONT_FIELDS.map((key) => [key, f.value(attributeOfFontKey(key))]))
  const fontInherited = Object.fromEntries(TEXT_FONT_FIELDS.map((key) => [key, f.inherited(attributeOfFontKey(key))]))
  const fontSummary = [f.value('fontFamily')?.split(',')[0], f.value('fontSize'), f.value('lineHeight')]
    .filter(Boolean)
    .join(' · ')

  return (
    <>
      <SettingsSection
        sectionKey="mj-text:colors"
        title={t`Colors`}
        first
        setCount={f.setCount('color', 'containerBackgroundColor')}
      >
        <SettingsRow label={t`Text`} isSet={f.isSet('color')} onReset={f.reset('color')}>
          <ColorField
            aria-label={t`Text color`}
            value={f.value('color')}
            inherited={f.inherited('color')}
            emailColors={colors}
            onChange={f.write('color')}
          />
        </SettingsRow>
        <SettingsRow
          label={t`Container background`}
          isSet={f.isSet('containerBackgroundColor')}
          onReset={f.reset('containerBackgroundColor')}
        >
          <ColorField
            aria-label={t`Container background`}
            value={f.value('containerBackgroundColor')}
            inherited={f.inherited('containerBackgroundColor')}
            placeholder={t`None`}
            emailColors={colors}
            onChange={f.write('containerBackgroundColor')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-text:font"
        title={t`Font`}
        setCount={f.setCount(...TEXT_FONT_KEYS)}
        summary={fontSummary}
      >
        <FontFields
          value={fontValues}
          inherited={fontInherited}
          importedFonts={importedFonts}
          fields={TEXT_FONT_FIELDS}
          onChange={(updates, options) =>
            f.writeMany(
              Object.fromEntries(Object.entries(updates).map(([key, value]) => [attributeOfFontKey(key), value])),
              options
            )
          }
        />
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-text:size"
        title={t`Size`}
        setCount={f.setCount('height')}
        summary={f.value('height')}
      >
        <SettingsRow label={t`Height`} isSet={f.isSet('height')} onReset={f.reset('height')}>
          <NumberUnitField
            aria-label={t`Text height`}
            mode="height"
            value={f.value('height')}
            inherited={f.inherited('height')}
            placeholder={t`Auto`}
            coalesceBase={f.base('height')}
            onChange={f.write('height')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection sectionKey="mj-text:spacing" title={t`Spacing`} setCount={f.setCount(...TEXT_PADDING)}>
        <SpacingField
          label={t`Around the text`}
          centerLabel={t`Text`}
          value={sidesOf(f, 'padding')}
          inherited={inheritedSidesOf(f, 'padding')}
          onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
            f.writeMany(toSideAttributes(sides, 'padding'), options)
          }
        />
      </SettingsSection>

      <AdvancedSection type="mj-text" fields={f} />
    </>
  )
}

// Local state management for text content to prevent unnecessary re-renders
const useTextContentState = (
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

/**
 * Implementation for mj-text blocks
 */
export class MjTextBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faFont} className="opacity-70" />
  }

  getLabel(): string {
    return 'Text'
  }

  getDescription(): React.ReactNode {
    return (
      <div>
        <div style={{ marginBottom: 8 }}>Add paragraphs, headings, and formatted text content</div>
        <div style={{ opacity: 0.7 }}>
          <div
            style={{
              width: 60,
              height: 30,
              border: '2px solid #722ed1',
              borderRadius: 4,
              backgroundColor: '#f9f0ff',
              padding: 4
            }}
          >
            <div
              style={{ height: 3, backgroundColor: '#722ed1', marginBottom: 2, borderRadius: 1 }}
            />
            <div
              style={{
                height: 2,
                backgroundColor: '#d3adf7',
                marginBottom: 1,
                borderRadius: 1,
                width: '80%'
              }}
            />
            <div style={{ height: 2, backgroundColor: '#d3adf7', borderRadius: 1, width: '60%' }} />
            <FontAwesomeIcon
              icon={faFont}
              style={{
                position: 'absolute',
                marginTop: -25,
                marginLeft: 40,
                fontSize: 10,
                color: '#722ed1'
              }}
            />
          </div>
        </div>
      </div>
    )
  }

  getCategory(): 'content' | 'layout' {
    return 'content'
  }

  getDefaults(): Record<string, unknown> {
    const defaults = MJML_COMPONENT_DEFAULTS['mj-text'] || {}

    // Add default HTML content for Tiptap
    const defaultContent = '<p>Start typing your content...</p>'

    return {
      ...defaults,
      content: defaultContent
    }
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  /**
   * Render the settings panel for the text block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    return (
      <MjTextSettingsPanel
        blockId={this.block.id}
        currentAttributes={this.block.attributes as MJTextAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
        emailTree={emailTree}
      />
    )
  }

  getEdit(props: PreviewProps): React.ReactNode {
    // Return a functional wrapper component that can use hooks
    return <MjTextBlockWrapper block={this.block} {...props} />
  }
}

// Functional wrapper component to handle hooks
 
const MjTextBlockWrapper: React.FC<PreviewProps & { block: EmailBlock }> = ({
  block,
  selectedBlockId,
  onSelectBlock,
  onUpdateBlock,
  attributeDefaults,
  emailTree
}) => {
  const { t } = useLingui()
  const key = block.id
  const isSelected = selectedBlockId === block.id
  const blockClasses = `email-block-hover email-text-preview ${isSelected ? 'selected' : ''}`.trim()

  const selectionStyle: React.CSSProperties = isSelected ? { position: 'relative', zIndex: 10 } : {}

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (onSelectBlock) {
      onSelectBlock(block.id)
    }
  }

  const attrs = EmailBlockClass.mergeWithAllDefaults('mj-text', block.attributes as Record<string, unknown>, attributeDefaults)

  const textStyle: React.CSSProperties = {
    padding: `${attrs.paddingTop} ${attrs.paddingRight} ${attrs.paddingBottom} ${attrs.paddingLeft}`,
    color: attrs.color,
    fontSize: attrs.fontSize,
    fontFamily: attrs.fontFamily,
    fontWeight: attrs.fontWeight,
    fontStyle: attrs.fontStyle,
    textAlign: attrs.align as React.CSSProperties['textAlign'],
    lineHeight: attrs.lineHeight,
    textDecoration: attrs.textDecoration,
    textTransform: attrs.textTransform as React.CSSProperties['textTransform'],
    letterSpacing: attrs.letterSpacing,
    backgroundColor: attrs.containerBackgroundColor,
    height: attrs.height,
    width: attrs.width,
    verticalAlign: attrs.verticalAlign as React.CSSProperties['verticalAlign'],
    ...selectionStyle
  }

  const content = 'content' in block ? block.content : undefined

  // Ensure content is properly typed as string for Tiptap
  const htmlContent: string =
    typeof content === 'string' ? content : '<p>Start typing your content...</p>'

  // Use the custom hook for local state management
  const { localContent, handleContentChange, handleSave } = useTextContentState(
    htmlContent,
    block.id,
    onUpdateBlock,
    block
  )

  // Track previous selection state to save when deselected
  const prevSelectedRef = React.useRef(isSelected)

  React.useEffect(() => {
    if (prevSelectedRef.current && !isSelected) {
      // console.log('MjTextBlock: Block deselected, saving content for', block.id)
      handleSave()
    }
    prevSelectedRef.current = isSelected
  }, [isSelected, handleSave])

  // Headings of the whole email: the level above this block, and a skip in it
  const { levelBefore, skip } = React.useMemo(() => {
    if (!emailTree?.children) return { levelBefore: null, skip: undefined }
    return {
      levelBefore: headingLevelBeforeBlock(emailTree, block.id),
      skip: findSkips(headingOutline(emailTree)).find((found) => found.heading.blockId === block.id)
    }
  }, [emailTree, block.id])

  // Only shown while the block is not open in its editor, which has its own copy
  const canvasHtml = React.useMemo(
    () => (isSelected ? '' : sanitizeHtml(localContent)),
    [isSelected, localContent]
  )

  return (
    <div
      key={key}
      className={blockClasses}
      onClick={handleClick}
      style={{ position: 'relative' }}
      data-block-id={block.id}
        data-display-condition={block.display_condition ? "true" : undefined}
    >
      {skip && (
        <span className="email-heading-skip-badge">
          ⚠ {t`H${skip.heading.level} after H${skip.previousLevel}`}
        </span>
      )}
      {isSelected ? (
        <div
          style={{
            position: 'relative',
            border: 'none',
            ...textStyle,
            outline: 'none',
            cursor: 'text'
          }}
        >
          <TiptapRichEditor
            content={localContent}
            onChange={handleContentChange}
            readOnly={!isSelected}
            autoFocus={isSelected}
            headingLevelBeforeBlock={levelBefore}
          />
        </div>
      ) : (
        <div style={textStyle}>
          <div
            // Keeps what the content draws to the block. On a box of its own,
            // sized by the content, so a fixed block height cuts off no text the
            // email would show. Content wider than the block, an image or a
            // table the email would let overflow, is cut 25px past it.
            style={containedContentStyle}
            dangerouslySetInnerHTML={{ __html: canvasHtml }}
            // Nothing in the content acts on a click here, a link or a label
            // included: the click goes on to select the block
            onClick={(e) => e.preventDefault()}
            onAuxClick={(e) => e.preventDefault()}
          />
        </div>
      )}
    </div>
  )
}
