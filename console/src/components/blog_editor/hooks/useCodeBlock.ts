import type { Editor } from '@tiptap/react'

/**
 * Checks if code block transformation is available
 * 
 * Validates:
 * - Editor is ready and editable
 * - Code block node type exists in schema
 * - Current selection can be converted to code block
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if code block toggle is available, false otherwise
 */
export function canToggle(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false
  
  // Check if codeBlock node exists in schema
  if (!editor.schema.nodes.codeBlock) return false

  // Check if we can toggle code block
  return editor.can().toggleCodeBlock()
}

/**
 * Toggles the current block to/from a code block
 * 
 * If already a code block, converts to paragraph
 * Otherwise, converts to a code block
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if toggle succeeded, false otherwise
 */
export function toggleCodeBlock(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    return editor.chain().focus().toggleCodeBlock().run()
  } catch {
    return false
  }
}

