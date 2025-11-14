import type { Editor } from '@tiptap/react'

/**
 * Checks if Image can be inserted in the current editor state
 *
 * Validates:
 * - Editor is ready and editable
 * - Image node type exists in schema
 *
 * @param editor - The Tiptap editor instance
 * @returns true if Image can be inserted, false otherwise
 */
export function canInsertImage(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  // Check if image node exists in schema
  return !!editor.schema.nodes.image
}

/**
 * Checks if the current selection is within an Image node
 *
 * @param editor - The Tiptap editor instance
 * @returns true if current selection is in an Image node
 */
export function isImageActive(editor: Editor | null): boolean {
  if (!editor) return false
  return editor.isActive('image')
}

/**
 * Inserts an Image at the current position
 *
 * @param editor - The Tiptap editor instance
 * @param url - Optional image URL. If empty, shows input overlay
 * @param alt - Optional alt text
 * @param title - Optional title
 * @returns true if insertion succeeded, false otherwise
 */
export function insertImage(
  editor: Editor | null,
  url?: string,
  alt?: string,
  title?: string
): boolean {
  if (!editor || !editor.isEditable) {
    return false
  }

  try {
    // Insert Image node directly using insertContent
    const result = editor
      .chain()
      .focus()
      .insertContent({
        type: 'image',
        attrs: {
          src: url || '',
          alt: alt || '',
          title: title || ''
        }
      })
      .run()

    return result
  } catch (error) {
    console.error('Image insert error:', error)
    return false
  }
}




