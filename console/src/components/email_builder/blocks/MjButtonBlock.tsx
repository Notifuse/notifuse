import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { InputNumber, Row, Col } from 'antd'
import type { MJMLComponentType, EmailBlock, MJButtonAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import StringPopoverInput from '../ui/StringPopoverInput'
import PaddingInput from '../ui/PaddingInput'
import InputLayout from '../ui/InputLayout'
import ColorPickerWithPresets from '../ui/ColorPickerWithPresets'
import BorderInput from '../ui/BorderInput'
import FontStyleInput from '../ui/FontStyleInput'
import AlignSelector from '../ui/AlignSelector'
import PanelLayout from '../panels/PanelLayout'
import BorderRadiusInput from '../ui/BorderRadiusInput'
import { TiptapInlineEditor } from '../ui/tiptap'

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
  currentAttributes: MJButtonAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
  parseWidthNumber: (width?: string) => number | undefined
  parseHeightNumber: (height?: string) => number | undefined
}

const MjButtonSettingsPanel: React.FC<MjButtonSettingsPanelProps> = ({
  currentAttributes,
  blockDefaults,
  onUpdate,
  emailTree,
  parseWidthNumber,
  parseHeightNumber
}) => {
  const { t } = useLingui()

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
    <PanelLayout title={t`Button Attributes`}>
      <InputLayout label={t`Link URL`}>
        <StringPopoverInput
          value={currentAttributes.href || ''}
          onChange={(value) => onUpdate({ href: value || undefined })}
          placeholder={t`Enter button link URL or {{ url }}`}
          buttonText={t`Set link`}
          validateUri={true}
        />
      </InputLayout>

      <InputLayout label={t`Colors`} layout="vertical">
        <Row gutter={16}>
          <Col span={8}>
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
          <Col span={8}>
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
          <Col span={8}>
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

      <InputLayout label={t`Border radius`}>
        <BorderRadiusInput
          value={currentAttributes.borderRadius}
          onChange={(value) => onUpdate({ borderRadius: value })}
          defaultValue={blockDefaults.borderRadius}
        />
      </InputLayout>

      <InputLayout label={t`Button Align`}>
        <AlignSelector
          value={currentAttributes.align || 'center'}
          onChange={(value) => onUpdate({ align: value })}
        />
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
            textAlign: currentAttributes.textAlign
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
            textAlign: blockDefaults.textAlign
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
              textAlign: values.textAlign
            })
          }}
          importedFonts={importedFonts}
        />
      </InputLayout>

      <InputLayout label={t`Button Padding`} layout="vertical">
        <PaddingInput
          value={currentAttributes.innerPadding}
          defaultValue={blockDefaults.innerPadding}
          onChange={(value: string | undefined) => {
            onUpdate({
              innerPadding: value
            })
          }}
        />
      </InputLayout>

      <InputLayout label={t`Container Padding`} layout="vertical">
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

      <InputLayout label={t`Border`} layout="vertical">
        <BorderInput
          className="-mt-6"
          value={{
            borderTop: currentAttributes.borderTop,
            borderRight: currentAttributes.borderRight,
            borderBottom: currentAttributes.borderBottom,
            borderLeft: currentAttributes.borderLeft
          }}
          onChange={(borderValues) => {
            onUpdate({
              borderTop: borderValues.borderTop,
              borderRight: borderValues.borderRight,
              borderBottom: borderValues.borderBottom,
              borderLeft: borderValues.borderLeft
            })
          }}
        />
      </InputLayout>

      <InputLayout label={t`Button Size`} layout="vertical">
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Width`}</span>
              <div style={{ marginTop: '4px' }}>
                <InputNumber
                  size="small"
                  value={parseWidthNumber(currentAttributes.width)}
                  onChange={(value) => onUpdate({ width: value ? `${value}px` : undefined })}
                  placeholder={(parseWidthNumber(blockDefaults.width) || t`Auto`).toString()}
                  min={0}
                  max={1000}
                  step={1}
                  suffix="px"
                  style={{ width: '100%' }}
                />
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Height`}</span>
              <div style={{ marginTop: '4px' }}>
                <InputNumber
                  size="small"
                  value={parseHeightNumber(currentAttributes.height)}
                  onChange={(value) => onUpdate({ height: value ? `${value}px` : undefined })}
                  placeholder={(
                    parseHeightNumber(blockDefaults.height) || t`Auto`
                  ).toString()}
                  min={0}
                  max={1000}
                  step={1}
                  suffix="px"
                  style={{ width: '100%' }}
                />
              </div>
            </div>
          </Col>
        </Row>
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

  /**
   * Render the settings panel for the button block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJButtonAttributes

    return (
      <MjButtonSettingsPanel
        currentAttributes={currentAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
        emailTree={emailTree}
        parseWidthNumber={this.parseWidthNumber}
        parseHeightNumber={this.parseHeightNumber}
      />
    )
  }

  /**
   * Parse height to get numeric value
   */
  private parseHeightNumber(height?: string): number | undefined {
    if (!height) return undefined
    const match = height.match(/^(\d+(?:\.\d+)?)px?$/)
    return match ? parseFloat(match[1]) : undefined
  }

  /**
   * Parse width to get numeric value
   */
  private parseWidthNumber(width?: string): number | undefined {
    if (!width) return undefined
    const match = width.match(/^(\d+(?:\.\d+)?)px?$/)
    return match ? parseFloat(match[1]) : undefined
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
              style={{ color: attrs.color }}
              dangerouslySetInnerHTML={{ __html: localContent }}
            />
          )}
        </div>
      </div>
    </div>
  )
}
