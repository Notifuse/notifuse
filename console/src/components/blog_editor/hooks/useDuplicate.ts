import type { Editor } from '@tiptap/react'
import { NodeSelection } from '@tiptap/pm/state'

/**
 * Shortcut key constant for duplicate action
 */
export const DUPLICATE_SHORTCUT_KEY = 'mod+d'

/**
 * Checks if a node can be duplicated in the current editor state
 *
 * Validates:
 * - Editor exists and is editable
 * - Has a valid node to duplicate (either selected node or parent block)
 *
 * @param editor - The Tiptap editor instance
 * @returns true if a node can be duplicated, false otherwise
 */
export function canDuplicateNode(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    const { state } = editor
    const { selection } = state

    // Direct node selection - can duplicate the selected node
    if (selection instanceof NodeSelection) {
      return !!selection.node
    }

    // For text selection, check if there's a parent block at depth 1
    // (the first level below the document)
    const $anchor = selection.$anchor.node(1)

    return !!$anchor
  } catch {
    return false
  }
}

/**
 * Duplicates a node in the editor
 *
 * Handles multiple selection types:
 * - NodeSelection: Duplicates the selected node and inserts after it
 * - TextSelection: Finds the parent block and duplicates it
 *
 * The duplicated node is inserted immediately after the original,
 * maintaining document structure and position
 *
 * @param editor - The Tiptap editor instance
 * @returns true if duplication succeeded, false otherwise
 */
export function duplicateNode(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    const { state } = editor
    const { selection } = state
    const chain = editor.chain().focus()

    // Handle direct node selection
    if (selection instanceof NodeSelection) {
      const selectedNode = selection.node
      const insertPos = selection.to

      // Convert node to JSON and insert at the calculated position
      chain.insertContentAt(insertPos, selectedNode.toJSON()).run()
      return true
    }

    // Handle text selection - find the parent node to duplicate
    const $anchor = selection.$anchor

    // Start from depth 1 (first level below document) and go deeper
    for (let depth = 1; depth <= $anchor.depth; depth++) {
      const node = $anchor.node(depth)

      // Skip document and nodes without a group (not insertable)
      if (node.type.name === 'doc' || !node.type.spec.group) {
        continue
      }

      // Calculate insertion position (right after the current node)
      const nodeStart = $anchor.start(depth)
      const insertPos = Math.min(nodeStart + node.nodeSize, state.doc.content.size)

      // Insert the duplicated node
      chain.insertContentAt(insertPos, node.toJSON()).run()
      return true
    }

    return false
  } catch {
    // Fail gracefully if any error occurs
    return false
  }
}
