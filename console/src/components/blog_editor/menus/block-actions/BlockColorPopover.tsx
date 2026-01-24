import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Popover } from 'antd'
import { Palette, ChevronRight } from 'lucide-react'
import type { MenuProps } from 'antd'
import { useNotifuseEditor } from '../../hooks/useEditor'
import { useRecentColors } from '../../toolbars/components/useRecentColors'
import { ColorGrid } from '../../components/colors/ColorGrid'

/**
 * BlockColorPopover - Color picker popover for block actions menu
 * Opens a popover with color patches for text and background colors
 */
export function useBlockColorPopover(
  onCloseMenu: () => void
): NonNullable<MenuProps['items']>[number] | null {
  const { t } = useLingui()
  const { editor } = useNotifuseEditor()
  const [open, setOpen] = useState(false)
  const { recentColors, addRecentColor, isInitialized } = useRecentColors()

  if (!editor || !editor.isEditable) {
    return null
  }

  const handleTextColor = (value: string | null, label: string) => {
    if (!editor) return

    // Close popover first
    setOpen(false)

    // Execute color change
    if (value === null) {
      editor.chain().focus().unsetColor().run()
    } else {
      editor.chain().focus().setColor(value).run()
      addRecentColor({ type: 'text', value, label })
    }

    // Then close main menu with a small delay
    setTimeout(() => {
      onCloseMenu()
    }, 0)
  }

  const handleBackgroundColor = (value: string | null, label: string) => {
    if (!editor) return

    // Close popover first
    setOpen(false)

    // Execute color change
    if (value === null) {
      editor.chain().focus().clearBackground().run()
    } else {
      editor.chain().focus().setBackground(value).run()
      addRecentColor({ type: 'background', value, label })
    }

    // Then close main menu with a small delay
    setTimeout(() => {
      onCloseMenu()
    }, 0)
  }

  const popoverContent = (
    <div onClick={(e) => e.stopPropagation()}>
      <ColorGrid
        onTextColorChange={handleTextColor}
        onBackgroundColorChange={handleBackgroundColor}
        showTextColors={true}
        showBackgroundColors={true}
        recentColors={recentColors}
        isInitialized={isInitialized}
      />
    </div>
  )

  return {
    key: 'color-popover',
    label: (
      <Popover
        content={popoverContent}
        trigger="hover"
        open={open}
        onOpenChange={setOpen}
        placement="rightTop"
      >
        <div
          style={{ display: 'flex', alignItems: 'center', gap: '8px', width: '100%' }}
          onClick={(e) => {
            e.stopPropagation()
            setOpen(!open)
          }}
        >
          <Palette size={16} />
          <span style={{ flex: 1 }}>{t`Color`}</span>
          <ChevronRight size={16} style={{ opacity: 0.45 }} />
        </div>
      </Popover>
    ),
    onClick: (e) => {
      e?.domEvent?.stopPropagation()
      setOpen(!open)
    }
  }
}
