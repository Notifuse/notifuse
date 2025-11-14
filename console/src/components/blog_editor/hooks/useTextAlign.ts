import type { Editor } from '@tiptap/react'

/**
 * Supported text alignment values
 */
export type TextAlign = 'left' | 'center' | 'right' | 'justify'

/**
 * Checks if text alignment can be set to a specific value
 *
 * Validates:
 * - Editor is ready and editable
 * - TextAlign extension is available
 * - Current selection supports text alignment
 *
 * @param editor - The Tiptap editor instance
 * @param align - The alignment value ('left', 'center', 'right', 'justify')
 * @returns true if alignment can be set, false otherwise
 */
export function canSetTextAlign(editor: Editor | null, align: TextAlign): boolean {
  if (!editor || !editor.isEditable) return false

  // Check if we can set text alignment
  return editor.can().setTextAlign(align)
}

/**
 * Checks if the current selection has a specific text alignment
 *
 * @param editor - The Tiptap editor instance
 * @param align - The alignment value to check
 * @returns true if current selection has the specified alignment
 */
export function isTextAlignActive(editor: Editor | null, align: TextAlign): boolean {
  if (!editor) return false
  return editor.isActive({ textAlign: align })
}

/**
 * Sets the text alignment for the current selection
 *
 * Applies the specified alignment to the selected blocks
 *
 * @param editor - The Tiptap editor instance
 * @param align - The alignment value ('left', 'center', 'right', 'justify')
 * @returns true if alignment was set successfully, false otherwise
 */
export function setTextAlign(editor: Editor | null, align: TextAlign): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    return editor.chain().focus().setTextAlign(align).run()
  } catch {
    return false
  }
}
