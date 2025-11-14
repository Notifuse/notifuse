import { useCallback, useEffect, useRef, useState } from 'react'
import type { Editor } from '@tiptap/react'
import { isNodeSelection } from '@tiptap/react'
import { NodeSelection, type Transaction } from '@tiptap/pm/state'

export const HIDE_FLOATING_META = 'hideFloatingToolbar'

/**
 * Check if the current selection is valid for showing the floating toolbar
 */
function isSelectionValid(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  const { state } = editor
  const { selection } = state
  const { empty, from, to } = selection

  // Don't show on empty selections
  if (empty) return false

  // Don't show if no actual text is selected
  const text = state.doc.textBetween(from, to, ' ')
  if (!text || text.trim().length === 0) return false

  // Don't show in code blocks
  if (selection.$from.parent.type.spec.code) return false

  // Don't show for node selections (like images)
  if (isNodeSelection(selection)) return false

  return true
}

/**
 * Get the bounding rect of the current selection
 */
function getSelectionRect(editor: Editor | null): DOMRect | null {
  if (!editor) return null

  const { state, view } = editor
  const { selection } = state
  const { from, to } = selection

  const start = view.coordsAtPos(from)
  const end = view.coordsAtPos(to)

  const top = Math.min(start.top, end.top)
  const bottom = Math.max(start.bottom, end.bottom)
  const left = Math.min(start.left, end.left)
  const right = Math.max(start.right, end.right)

  return new DOMRect(left, top, right - left, bottom - top)
}

export interface UseFloatingToolbarReturn {
  /**
   * Whether the toolbar should be shown
   */
  shouldShow: boolean
  /**
   * Function to get the anchor rect for positioning
   */
  getAnchorRect: () => DOMRect | null
}

/**
 * Hook for managing floating toolbar visibility and positioning
 *
 * Handles:
 * - Valid text selection detection
 * - HIDE_FLOATING_META flag from drag handle
 * - Mobile device detection
 * - Selection rect calculation
 */
export function useFloatingToolbar(
  editor: Editor | null,
  options?: {
    /**
     * Additional condition to hide the toolbar
     */
    extraHideWhen?: boolean
  }
): UseFloatingToolbarReturn {
  const { extraHideWhen = false } = options || {}
  const [shouldShow, setShouldShow] = useState(false)
  const hideRef = useRef(false)

  // Listen for transactions with HIDE_FLOATING_META
  useEffect(() => {
    if (!editor) return

    const onTx = ({ transaction }: { transaction: Transaction }) => {
      if (transaction.getMeta(HIDE_FLOATING_META)) {
        hideRef.current = true
      } else if (transaction.selectionSet) {
        // Clear hide flag when a new selection is made without the meta
        hideRef.current = false
      }
    }

    editor.on('transaction', onTx)

    return () => {
      editor.off('transaction', onTx)
    }
  }, [editor])

  // Handle re-click on the same selected node
  useEffect(() => {
    if (!editor) return
    const dom = editor.view.dom

    const onPointerDown = (e: PointerEvent) => {
      const sel = editor.state.selection
      if (!(sel instanceof NodeSelection)) return
      const nodeDom = editor.view.nodeDOM(sel.from) as HTMLElement | null
      if (!nodeDom) return
      if (nodeDom.contains(e.target as Node)) {
        hideRef.current = false
        const valid = isSelectionValid(editor)
        setShouldShow(valid && !extraHideWhen)
      }
    }

    dom.addEventListener('pointerdown', onPointerDown, { capture: true })
    return () =>
      dom.removeEventListener('pointerdown', onPointerDown, {
        capture: true
      })
  }, [editor, extraHideWhen])

  // Update visibility based on selection changes
  useEffect(() => {
    if (!editor) return

    const handleSelectionUpdate = () => {
      const { selection } = editor.state
      const valid = isSelectionValid(editor)

      if (extraHideWhen || (isNodeSelection(selection) && hideRef.current)) {
        setShouldShow(false)
        return
      }
      setShouldShow(valid)
    }

    handleSelectionUpdate()
    editor.on('selectionUpdate', handleSelectionUpdate)
    return () => {
      editor.off('selectionUpdate', handleSelectionUpdate)
    }
  }, [editor, extraHideWhen])

  // Handle outside clicks to dismiss the toolbar
  useEffect(() => {
    if (!editor || !shouldShow) return

    const handleOutsideClick = (e: PointerEvent) => {
      const target = e.target as Node

      // Check if click is inside the editor
      if (editor.view.dom.contains(target)) {
        return
      }

      // Check if click is inside the floating toolbar
      const toolbar = document.querySelector('.notifuse-editor-floating-toolbar')
      if (toolbar && toolbar.contains(target)) {
        return
      }

      // Check if click is inside an Ant Design popover/dropdown (ColorPicker, LinkPopover, etc.)
      const targetElement = e.target as HTMLElement
      if (
        targetElement.closest('.ant-popover') ||
        targetElement.closest('.ant-dropdown') ||
        targetElement.closest('.ant-tooltip')
      ) {
        return
      }

      // Click is outside both editor and toolbar - dismiss the toolbar
      setShouldShow(false)
    }

    // Use capture phase to catch clicks before they bubble
    document.addEventListener('pointerdown', handleOutsideClick, { capture: true })

    return () => {
      document.removeEventListener('pointerdown', handleOutsideClick, { capture: true })
    }
  }, [editor, shouldShow])

  // Memoize getAnchorRect function
  const getAnchorRect = useCallback(() => {
    return getSelectionRect(editor)
  }, [editor])

  return {
    shouldShow,
    getAnchorRect
  }
}

/**
 * Programmatically select a node and hide floating for that selection
 */
export function selectNodeAndHideFloating(editor: Editor, pos: number) {
  if (!editor) return
  const { state, view } = editor
  view.dispatch(
    state.tr.setSelection(NodeSelection.create(state.doc, pos)).setMeta(HIDE_FLOATING_META, true)
  )
}
