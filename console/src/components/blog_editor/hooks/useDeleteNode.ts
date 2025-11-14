import type { Editor } from '@tiptap/react'
import { NodeSelection } from '@tiptap/pm/state'

/**
 * Shortcut key constant for delete node action
 */
export const DELETE_NODE_SHORTCUT_KEY = 'backspace'

/**
 * Checks if a node can be deleted in the current editor state
 * 
 * Validates:
 * - Editor exists and is editable
 * - Has a valid selection (NodeSelection or text selection with parent blocks)
 * - Can perform delete operation without breaking document structure
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if a node can be deleted, false otherwise
 */
export function canDeleteNode(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  const { state } = editor
  const { selection } = state

  // Direct node selection is always deletable
  if (selection instanceof NodeSelection) {
    return true
  }

  // Check if we can delete a parent block at any depth
  const $pos = selection.$anchor

  for (let depth = $pos.depth; depth > 0; depth--) {
    const node = $pos.node(depth)
    const pos = $pos.before(depth)

    // Try to check if deletion would be valid
    // by simulating a delete operation
    const tr = state.tr.delete(pos, pos + node.nodeSize)
    if (tr.doc !== state.doc) {
      return true
    }
  }

  return false
}

/**
 * Deletes a node at a specific position using multiple strategies
 * 
 * First attempts deleteRange, then falls back to setNodeSelection + deleteSelection
 * This ensures deletion works in various edge cases
 * 
 * @param editor - The Tiptap editor instance
 * @param pos - The position of the node to delete
 * @param nodeSize - The size of the node
 * @returns true if deletion succeeded, false otherwise
 */
export function deleteNodeAtPosition(
  editor: Editor,
  pos: number,
  nodeSize: number
): boolean {
  const chain = editor.chain().focus()
  
  // Strategy 1: Try to delete the range directly
  const success = chain.deleteRange({ from: pos, to: pos + nodeSize }).run()
  if (success) return true

  // Strategy 2: Fallback to selecting the node then deleting
  return chain.setNodeSelection(pos).deleteSelection().run()
}

/**
 * Deletes the currently selected node or parent block
 * 
 * Handles multiple selection types:
 * - NodeSelection: Deletes the selected node directly
 * - TextSelection: Finds and deletes the closest parent block
 * 
 * Special handling:
 * - Skips table cells/rows/headers to avoid breaking table structure
 * - Uses fallback strategies for robust deletion
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if deletion succeeded, false otherwise
 */
export function deleteNode(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    const { state } = editor
    const { selection } = state

    // Handle direct node selection
    if (selection instanceof NodeSelection) {
      const pos = selection.from
      const selectedNode = selection.node

      if (!selectedNode) return false

      return deleteNodeAtPosition(editor, pos, selectedNode.nodeSize)
    }

    // Handle text selection - find parent block to delete
    const $pos = selection.$from

    // Traverse up the document tree to find a deletable block
    for (let depth = $pos.depth; depth > 0; depth--) {
      const node = selection.$from.node(depth)
      const pos = selection.$from.before(depth)

      // Only delete block nodes, skip table-related nodes to preserve structure
      if (
        node &&
        node.isBlock &&
        node.type.name !== 'tableRow' &&
        node.type.name !== 'tableHeader' &&
        node.type.name !== 'tableCell'
      ) {
        return deleteNodeAtPosition(editor, pos, node.nodeSize)
      }
    }

    return false
  } catch {
    // Fail gracefully if any error occurs during deletion
    return false
  }
}

