import type { Editor } from '@tiptap/react'
import type { Transaction } from '@tiptap/pm/state'

/**
 * Removes all marks from the transaction except those specified in the skip array
 *
 * Iterates through all selection ranges and removes marks from inline nodes
 * while preserving marks that should be kept (e.g., 'inlineThread')
 *
 * @param tr - The Tiptap transaction to modify
 * @param skip - Array of mark names to preserve (not remove)
 * @returns The modified transaction with marks removed
 */
export function removeAllMarksExcept(tr: Transaction, skip: string[] = []): Transaction {
  const { selection } = tr
  const { empty, ranges } = selection

  // Can't remove marks from empty selection
  if (empty) return tr

  // Process each range in the selection
  ranges.forEach((range) => {
    const from = range.$from.pos
    const to = range.$to.pos

    // Iterate through nodes in the range
    tr.doc.nodesBetween(from, to, (node, pos) => {
      // Only process inline nodes (text and inline elements)
      if (!node.isInline) return true

      // Remove each mark that's not in the skip list
      node.marks.forEach((mark) => {
        if (!skip.includes(mark.type.name)) {
          tr.removeMark(pos, pos + node.nodeSize, mark.type)
        }
      })

      return true
    })
  })

  return tr
}

/**
 * Checks whether the current selection has marks that can be reset (removed)
 *
 * Validates that there are removable marks in the selection,
 * excluding marks that should be preserved
 *
 * @param tr - The Tiptap transaction to check
 * @param skip - Array of mark names to skip when checking
 * @returns true if there are marks that can be removed, false otherwise
 */
export function canResetMarks(tr: Transaction, skip: string[] = []): boolean {
  const { selection } = tr
  const { empty, ranges } = selection

  // Can't reset marks in empty selection
  if (empty) return false

  // Check each range for removable marks
  for (const range of ranges) {
    const from = range.$from.pos
    const to = range.$to.pos

    let hasRemovableMarks = false

    tr.doc.nodesBetween(from, to, (node) => {
      // Only check inline nodes
      if (!node.isInline) return true

      // Check if node has any marks that aren't in skip list
      for (const mark of node.marks) {
        if (!skip.includes(mark.type.name)) {
          hasRemovableMarks = true
          return false // Stop iteration - we found removable marks
        }
      }

      return true
    })

    if (hasRemovableMarks) {
      return true
    }
  }

  return false
}

/**
 * Checks if there's a background color in the current selection
 *
 * @param editor - The Tiptap editor instance
 * @returns true if any selected nodes have background colors
 */
function hasBackgroundColor(editor: Editor): boolean {
  const { selection, doc } = editor.state
  const { from, to } = selection

  let hasColor = false

  doc.nodesBetween(from, to, (node) => {
    if (node.attrs.bgColor) {
      hasColor = true
      return false // Stop iteration
    }
    return true
  })

  // If nothing found, check parent nodes at cursor
  if (!hasColor) {
    const $pos = selection.$from
    for (let depth = $pos.depth; depth > 0; depth--) {
      const node = $pos.node(depth)
      if (node.attrs.bgColor) {
        hasColor = true
        break
      }
    }
  }

  return hasColor
}

/**
 * Checks if formatting can be reset for the current selection
 *
 * Validates:
 * - Editor is ready and editable
 * - Selection has marks that can be removed OR has background colors
 *
 * @param editor - The Tiptap editor instance
 * @param preserveMarks - Array of mark names to preserve
 * @returns true if formatting can be reset, false otherwise
 */
export function canResetFormatting(editor: Editor | null, preserveMarks?: string[]): boolean {
  if (!editor || !editor.isEditable) return false

  const tr = editor.state.tr
  return canResetMarks(tr, preserveMarks) || hasBackgroundColor(editor)
}

/**
 * Resets formatting for the current selection
 *
 * Removes all marks from the selection except those specified
 * in preserveMarks (useful for keeping certain marks like comments)
 * Also clears background colors from selected nodes
 *
 * @param editor - The Tiptap editor instance
 * @param preserveMarks - Array of mark names to preserve
 * @returns true if formatting was reset successfully, false otherwise
 */
export function resetFormatting(editor: Editor | null, preserveMarks?: string[]): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    const { view, state } = editor
    const { tr } = state

    // Remove marks except those in preserveMarks
    const transaction = removeAllMarksExcept(tr, preserveMarks)

    // Apply the transaction
    view.dispatch(transaction)

    // Clear background colors using the extension command
    editor.commands.clearBackground()

    // Restore focus
    editor.commands.focus()

    return true
  } catch {
    return false
  }
}
