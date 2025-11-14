import type { Editor } from '@tiptap/react'

/**
 * Supported heading levels
 */
export type Level = 1 | 2 | 3 | 4 | 5 | 6

/**
 * Checks if heading transformation is available for a specific level
 * 
 * Validates:
 * - Editor is ready and editable
 * - Heading node type exists in schema
 * - Current selection can be converted to the specified heading level
 * 
 * @param editor - The Tiptap editor instance
 * @param level - The heading level (1-6)
 * @returns true if heading toggle is available, false otherwise
 */
export function canToggle(editor: Editor | null, level: Level): boolean {
  if (!editor || !editor.isEditable) return false
  
  // Check if heading node exists in schema
  if (!editor.schema.nodes.heading) return false

  // Check if we can set heading with this level
  return editor.can().setHeading({ level })
}

/**
 * Checks if the current block is a heading of the specified level
 * 
 * @param editor - The Tiptap editor instance
 * @param level - The heading level to check (1-6)
 * @returns true if current block is a heading of the specified level
 */
export function isHeadingActive(editor: Editor | null, level: Level): boolean {
  if (!editor) return false
  return editor.isActive('heading', { level })
}

/**
 * Toggles the current block to/from a heading at the specified level
 * 
 * If already a heading of this level, converts to paragraph
 * Otherwise, converts to the specified heading level
 * 
 * @param editor - The Tiptap editor instance
 * @param level - The heading level (1-6)
 * @returns true if toggle succeeded, false otherwise
 */
export function toggleHeading(editor: Editor | null, level: Level): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    // If already this heading level, convert to paragraph
    if (editor.isActive('heading', { level })) {
      return editor.chain().focus().setParagraph().run()
    }
    
    // Otherwise, set to the heading level
    return editor.chain().focus().setHeading({ level }).run()
  } catch {
    return false
  }
}

