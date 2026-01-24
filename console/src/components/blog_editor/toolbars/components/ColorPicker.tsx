import { useContext, useEffect, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { EditorContext } from '@tiptap/react'
import { Button, Popover, Tooltip } from 'antd'
import { ChevronDown } from 'lucide-react'
import {
  canSetTextColor,
  canSetHighlightColor
} from '../../core/registry/action-specs/link-color-actions'
import { useRecentColors } from './useRecentColors'
import { ColorPatch } from '../../components/colors/ColorPatch'
import {
  TEXT_COLORS,
  BACKGROUND_COLORS
} from '../../components/colors/ColorConstants'

export interface ColorPickerProps {
  /**
   * Whether to hide the button when color is not available
   * @default false
   */
  hideWhenUnavailable?: boolean
}

/**
 * ColorPicker - Button with popover for changing text and highlight colors
 */
export function ColorPicker({ hideWhenUnavailable = false }: ColorPickerProps) {
  const { t } = useLingui()
  const { editor } = useContext(EditorContext)!
  const [open, setOpen] = useState(false)
  const [isVisible, setIsVisible] = useState(true)
  const { recentColors, addRecentColor, isInitialized } = useRecentColors()

  const canSetText = canSetTextColor(editor)
  const canSetHighlight = canSetHighlightColor(editor)
  const canSetAny = canSetText || canSetHighlight

  const isActive = editor?.isActive('textStyle') || editor?.isActive('highlight') || false

  // Get current colors to display in the button
  const currentTextColor = editor?.getAttributes('textStyle').color
  const currentHighlightColor = editor?.getAttributes('highlight').color

  // Update visibility when editor state changes
  useEffect(() => {
    if (!editor) return

    const handleSelectionUpdate = () => {
      setIsVisible(canSetTextColor(editor) || canSetHighlightColor(editor))
    }

    handleSelectionUpdate()
    editor.on('selectionUpdate', handleSelectionUpdate)

    return () => {
      editor.off('selectionUpdate', handleSelectionUpdate)
    }
  }, [editor])

  if (!isVisible && hideWhenUnavailable) {
    return null
  }

  const handleTextColor = (colorValue: string | null, colorLabel?: string) => {
    if (!editor || !canSetText) return

    if (colorValue === null) {
      // Remove color to use default
      editor.chain().focus().unsetColor().run()
    } else {
      editor.chain().focus().setColor(colorValue).run()
      // Add to recent colors if label is provided
      if (colorLabel) {
        addRecentColor({ type: 'text', value: colorValue, label: colorLabel })
      }
    }

    setOpen(false)
    editor.commands.focus()
  }

  const handleHighlightColor = (colorValue: string | null, colorLabel?: string) => {
    if (!editor || !canSetHighlight) return

    if (colorValue === null) {
      // Remove highlight to use default
      editor.chain().focus().unsetHighlight().run()
    } else {
      editor.chain().focus().toggleHighlight({ color: colorValue }).run()
      // Add to recent colors if label is provided
      if (colorLabel) {
        addRecentColor({ type: 'background', value: colorValue, label: colorLabel })
      }
    }

    setOpen(false)
    editor.commands.focus()
  }

  // Split colors into two rows of 5
  const textColorsRow1 = TEXT_COLORS.slice(0, 5)
  const textColorsRow2 = TEXT_COLORS.slice(5, 10)
  const backgroundColorsRow1 = BACKGROUND_COLORS.slice(0, 5)
  const backgroundColorsRow2 = BACKGROUND_COLORS.slice(5, 10)

  const popoverContent = (
    <div style={{ width: '180px' }}>
      {/* Recently Used Section */}
      {isInitialized && recentColors.length > 0 && (
        <div style={{ marginBottom: '12px' }}>
          <div
            style={{
              fontSize: '12px',
              fontWeight: '600',
              color: '#8c8c8c',
              padding: '8px 12px 4px',
              textTransform: 'uppercase'
            }}
          >
            {t`Recently Used`}
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', padding: '0 12px' }}>
            {recentColors.map((recentColor, index) => (
              <button
                key={`${recentColor.type}-${recentColor.value}-${index}`}
                onClick={() => {
                  if (recentColor.type === 'text') {
                    handleTextColor(recentColor.value, recentColor.label)
                  } else {
                    handleHighlightColor(recentColor.value, recentColor.label)
                  }
                }}
                title={`${recentColor.label} ${
                  recentColor.type === 'text' ? t`text` : t`background`
                }`}
                style={{
                  width: '24px',
                  height: '24px',
                  borderRadius: '4px',
                  backgroundColor:
                    recentColor.type === 'background' ? recentColor.value : 'transparent',
                  border: recentColor.type === 'text' ? `1px solid ${recentColor.value}` : 'none',
                  boxShadow:
                    recentColor.type === 'background'
                      ? 'inset 0 0 0 1px rgba(0, 0, 0, 0.15)'
                      : 'none',
                  cursor: 'pointer',
                  padding: 0,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: '14px',
                  fontWeight: '500',
                  color: recentColor.type === 'text' ? recentColor.value : 'transparent',
                  transition: 'transform 0.1s'
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.transform = 'scale(1.1)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.transform = 'scale(1)'
                }}
              >
                {recentColor.type === 'text' && 'A'}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Text Colors Section */}
      {canSetText && (
        <div style={{ marginBottom: '12px' }}>
          <div
            style={{
              fontSize: '12px',
              fontWeight: '600',
              color: '#8c8c8c',
              padding: '8px 12px 4px',
              textTransform: 'uppercase'
            }}
          >
            {t`Text Color`}
          </div>
          <div style={{ padding: '0 12px' }}>
            <div style={{ display: 'flex', gap: '8px', marginBottom: '8px' }}>
              {textColorsRow1.map((color) => (
                <ColorPatch
                  key={color.value}
                  color={{ label: color.label, value: color.value }}
                  type="text"
                  onClick={() => handleTextColor(color.value, color.label)}
                />
              ))}
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              {textColorsRow2.map((color) => (
                <ColorPatch
                  key={color.value}
                  color={{ label: color.label, value: color.value }}
                  type="text"
                  onClick={() => handleTextColor(color.value, color.label)}
                />
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Background Colors Section */}
      {canSetHighlight && (
        <div>
          <div
            style={{
              fontSize: '12px',
              fontWeight: '600',
              color: '#8c8c8c',
              padding: '8px 12px 4px',
              textTransform: 'uppercase'
            }}
          >
            {t`Background Color`}
          </div>
          <div style={{ padding: '0 12px' }}>
            <div style={{ display: 'flex', gap: '8px', marginBottom: '8px' }}>
              {backgroundColorsRow1.map((color) => (
                <ColorPatch
                  key={color.value}
                  color={{ label: color.label, value: color.value }}
                  type="background"
                  onClick={() => handleHighlightColor(color.value, color.label)}
                />
              ))}
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              {backgroundColorsRow2.map((color) => (
                <ColorPatch
                  key={color.value}
                  color={{ label: color.label, value: color.value }}
                  type="background"
                  onClick={() => handleHighlightColor(color.value, color.label)}
                />
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )

  return (
    <Popover
      content={popoverContent}
      title={undefined}
      trigger="click"
      open={open}
      onOpenChange={setOpen}
      placement="bottom"
    >
      <Tooltip title={t`Text Color`} placement="top">
        <Button
          type="text"
          size="small"
          disabled={!canSetAny}
          className={`notifuse-editor-toolbar-button ${
            isActive ? 'notifuse-editor-toolbar-button-active' : ''
          }`}
          style={{ display: 'flex', alignItems: 'center', gap: '1px' }}
        >
          <span
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: '20px',
              height: '20px',
              border: '1px solid #d9d9d9',
              borderRadius: '3px',
              fontSize: '12px',
              fontWeight: '600',
              marginRight: '0', // Ensures no spacing between span and chevron
              color: currentTextColor || 'inherit',
              backgroundColor: currentHighlightColor || 'transparent'
            }}
          >
            A
          </span>
          <ChevronDown size={12} opacity={0.7} />
        </Button>
      </Tooltip>
    </Popover>
  )
}
