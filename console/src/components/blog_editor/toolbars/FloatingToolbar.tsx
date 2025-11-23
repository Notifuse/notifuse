import type { ReactNode } from 'react'
import { useContext } from 'react'
import { EditorContext } from '@tiptap/react'
import { flip, offset, shift } from '@floating-ui/react'
import { useFloatingMenu } from '../hooks/useFloatingMenu'
import { useControls } from '../core/state/useControls'
import './floating-toolbar.css'

export interface FloatingToolbarProps {
  /**
   * Whether the toolbar should be visible
   */
  shouldShow: boolean
  /**
   * Function to get the bounding rect for positioning
   */
  getAnchorRect: () => DOMRect | null
  /**
   * Children to render in the toolbar (typically ToolbarSections)
   */
  children: ReactNode
  /**
   * Z-index for the toolbar
   * @default 1000
   */
  zIndex?: number
}

/**
 * FloatingToolbar - Main container for the floating selection toolbar
 * Positions itself above text selections using Floating UI
 */
export function FloatingToolbar({
  shouldShow,
  getAnchorRect,
  children,
  zIndex = 1000
}: FloatingToolbarProps) {
  const { editor } = useContext(EditorContext)!
  const { isDragging } = useControls(editor)

  const { isMounted, ref, style } = useFloatingMenu(
    shouldShow && !isDragging,
    getAnchorRect,
    zIndex,
    {
      placement: 'top',
      middleware: [
        offset(8),
        flip({
          fallbackPlacements: ['bottom', 'top'],
          padding: 8
        }),
        shift({ padding: 8 })
      ]
    }
  )

  if (!isMounted || !shouldShow || isDragging) {
    return null
  }

  return (
    <div
      ref={ref}
      style={style}
      className="notifuse-editor-floating-toolbar"
      onMouseDown={(e) => {
        // Prevent toolbar clicks from affecting editor selection
        // But allow clicks on interactive elements (inputs, buttons inside popovers)
        const target = e.target as HTMLElement
        const isInteractive =
          target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.tagName === 'BUTTON' ||
          target.closest('input, textarea, button, [role="button"]')

        if (!isInteractive) {
          e.preventDefault()
        }
      }}
    >
      <div className="notifuse-editor-floating-toolbar-content">{children}</div>
    </div>
  )
}
