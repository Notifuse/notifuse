import type { Editor } from '@tiptap/react'

/**
 * Checks if blockquote transformation is available
 * 
 * Validates:
 * - Editor is ready and editable
 * - Blockquote node type exists in schema
 * - Current selection can be converted to blockquote
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if blockquote toggle is available, false otherwise
 */
export function canToggleBlockquote(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false
  
  // Check if blockquote node exists in schema
  if (!editor.schema.nodes.blockquote) return false

  // Check if we can toggle blockquote
  return editor.can().toggleBlockquote()
}

/**
 * Toggles the current block to/from a blockquote
 * 
 * If already a blockquote, converts to normal blocks
 * Otherwise, wraps the selection in a blockquote
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if toggle succeeded, false otherwise
 */
export function toggleBlockquote(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    return editor.chain().focus().toggleBlockquote().run()
  } catch {
    return false
  }
}

