import type { Editor } from '@tiptap/react'

/**
 * Supported list types
 */
export type ListType = 'bulletList' | 'orderedList'

/**
 * Checks if list transformation is available for a specific list type
 *
 * Validates:
 * - Editor is ready and editable
 * - List node type exists in schema
 * - Current selection can be converted to the specified list type
 *
 * @param editor - The Tiptap editor instance
 * @param listType - The list type ('bulletList' or 'orderedList')
 * @returns true if list toggle is available, false otherwise
 */
export function canToggleList(editor: Editor | null, listType: ListType): boolean {
  if (!editor || !editor.isEditable) return false

  // Check if list node exists in schema
  if (!editor.schema.nodes[listType]) return false

  // Check if we can toggle this list type
  switch (listType) {
    case 'bulletList':
      return editor.can().toggleBulletList()
    case 'orderedList':
      return editor.can().toggleOrderedList()
    default:
      return false
  }
}

/**
 * Checks if the current block is a list of the specified type
 *
 * @param editor - The Tiptap editor instance
 * @param listType - The list type to check
 * @returns true if current block is a list of the specified type
 */
export function isListActive(editor: Editor | null, listType: ListType): boolean {
  if (!editor) return false
  return editor.isActive(listType)
}

/**
 * Toggles the current block to/from a list of the specified type
 *
 * If already this list type, converts to paragraph
 * Otherwise, converts to the specified list type
 *
 * @param editor - The Tiptap editor instance
 * @param listType - The list type ('bulletList' or 'orderedList')
 * @returns true if toggle succeeded, false otherwise
 */
export function toggleList(editor: Editor | null, listType: ListType): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    const chain = editor.chain().focus()

    // Toggle the appropriate list type
    switch (listType) {
      case 'bulletList':
        return chain.toggleBulletList().run()
      case 'orderedList':
        return chain.toggleOrderedList().run()
      default:
        return false
    }
  } catch {
    return false
  }
}
