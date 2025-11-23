import type { Editor } from '@tiptap/react'

/**
 * Checks if text/paragraph node transformation is available
 * 
 * Validates:
 * - Editor is ready and editable
 * - Paragraph node type exists in schema
 * - Current selection can be converted to paragraph
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if paragraph toggle is available, false otherwise
 */
export function canToggleText(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false
  
  // Check if paragraph node exists in schema
  if (!editor.schema.nodes.paragraph) return false

  // Check if we can set the current block to paragraph
  return editor.can().setParagraph()
}

/**
 * Checks if the current block is a paragraph
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if current block is a paragraph, false otherwise
 */
export function isParagraphActive(editor: Editor | null): boolean {
  if (!editor) return false
  return editor.isActive('paragraph')
}

/**
 * Toggles the current block to/from a paragraph
 * 
 * Converts the current block node to a paragraph (plain text block)
 * This is typically used to "reset" a heading or other block to plain text
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if toggle succeeded, false otherwise
 */
export function toggleParagraph(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    return editor.chain().focus().setParagraph().run()
  } catch {
    return false
  }
}

