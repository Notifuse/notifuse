import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faFont } from '@fortawesome/free-solid-svg-icons'
import { Col, Row } from 'antd'
import type { MJMLComponentType, EmailBlock, MJTextAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import PanelLayout from '../panels/PanelLayout'
import InputLayout from '../ui/InputLayout'
import ColorPickerWithPresets from '../ui/ColorPickerWithPresets'
import PaddingInput from '../ui/PaddingInput'
import FontStyleInput from '../ui/FontStyleInput'
import HeightInput from '../ui/HeightInput'
import StringPopoverInput from '../ui/StringPopoverInput'
import { TiptapRichEditor } from '../ui/tiptap'

// Functional component for settings panel with i18n support
interface MjTextSettingsPanelProps {
  currentAttributes: MJTextAttributes
  blockDefaults: MergedBlockAttributes
  importedFonts: Array<{ name: string; href: string }>
  onUpdate: OnUpdateAttributesFunction
}

const MjTextSettingsPanel: React.FC<MjTextSettingsPanelProps> = ({
  currentAttributes,
  blockDefaults,
  importedFonts,
  onUpdate
}) => {
  const { t } = useLingui()

  return (
    <PanelLayout title={t`Text Attributes`}>
      <InputLayout label={t`Colors`} layout="vertical">
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Text`}</span>
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
              <span className="text-xs text-gray-500">{t`Container`}</span>
              <div style={{ marginTop: '4px' }}>
                <ColorPickerWithPresets
                  value={currentAttributes.containerBackgroundColor || undefined}
                  onChange={(color) => onUpdate({ containerBackgroundColor: color || undefined })}
                  placeholder={t`None`}
                />
              </div>
            </div>
          </Col>
        </Row>
      </InputLayout>

      <InputLayout label={t`Font Styling`} layout="vertical">
        <FontStyleInput
          value={{
            fontFamily: currentAttributes.fontFamily,
            fontSize: currentAttributes.fontSize,
            fontWeight: currentAttributes.fontWeight,
            fontStyle: currentAttributes.fontStyle,
            textTransform: currentAttributes.textTransform,
            textDecoration: currentAttributes.textDecoration,
            lineHeight: currentAttributes.lineHeight,
            letterSpacing: currentAttributes.letterSpacing,
            textAlign: currentAttributes.align
          }}
          defaultValue={{
            fontFamily: blockDefaults.fontFamily,
            fontSize: blockDefaults.fontSize,
            fontWeight: blockDefaults.fontWeight,
            fontStyle: blockDefaults.fontStyle,
            textTransform: blockDefaults.textTransform || 'none',
            textDecoration: blockDefaults.textDecoration,
            lineHeight: blockDefaults.lineHeight,
            letterSpacing: blockDefaults.letterSpacing,
            textAlign: blockDefaults.align
          }}
          onChange={(values) => {
            onUpdate({
              fontFamily: values.fontFamily,
              fontSize: values.fontSize,
              fontWeight: values.fontWeight,
              fontStyle: values.fontStyle,
              textTransform: values.textTransform,
              textDecoration: values.textDecoration,
              lineHeight: values.lineHeight,
              letterSpacing: values.letterSpacing,
              align: values.textAlign
            })
          }}
          importedFonts={importedFonts}
        />
      </InputLayout>

      <InputLayout label={t`Padding`} layout="vertical">
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

      <InputLayout label={t`Height`}>
        <HeightInput
          value={currentAttributes.height}
          onChange={(value) => onUpdate({ height: value })}
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
    const currentAttributes = this.block.attributes as MJTextAttributes

    // Find all imported fonts in the email tree
    const importedFonts: Array<{ name: string; href: string }> = []

    if (emailTree) {
      const fontBlocks = EmailBlockClass.findAllBlocksByType(emailTree, 'mj-font')
      fontBlocks.forEach((fontBlock) => {
        const attrs = fontBlock.attributes as { name?: string; href?: string }
        if (attrs?.name && attrs?.href) {
          importedFonts.push({
            name: attrs.name,
            href: attrs.href
          })
        }
      })
    }

    return (
      <MjTextSettingsPanel
        currentAttributes={currentAttributes}
        blockDefaults={blockDefaults}
        importedFonts={importedFonts}
        onUpdate={onUpdate}
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
  attributeDefaults
}) => {
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

  return (
    <div
      key={key}
      className={blockClasses}
      onClick={handleClick}
      style={{ position: 'relative' }}
      data-block-id={block.id}
    >
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
          />
        </div>
      ) : (
        <div
          style={textStyle}
          dangerouslySetInnerHTML={{ __html: localContent }}
          onClick={(e) => {
            // Check if the clicked element is an anchor tag
            const target = e.target as HTMLElement
            if (target.tagName === 'A' || target.closest('a')) {
              e.preventDefault()
              e.stopPropagation()
              // Select the block for editing when clicking on a link
              if (onSelectBlock) {
                onSelectBlock(block.id)
              }
            }
          }}
        />
      )}
    </div>
  )
}
