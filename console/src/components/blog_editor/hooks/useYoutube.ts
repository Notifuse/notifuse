import type { Editor } from '@tiptap/react'

/**
 * Checks if YouTube embed can be inserted in the current editor state
 *
 * Validates:
 * - Editor is ready and editable
 * - YouTube node type exists in schema
 *
 * @param editor - The Tiptap editor instance
 * @returns true if YouTube embed can be inserted, false otherwise
 */
export function canInsertYoutube(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  // Check if youtube node exists in schema
  return !!editor.schema.nodes.youtube
}

/**
 * Checks if the current selection is within a YouTube node
 *
 * @param editor - The Tiptap editor instance
 * @returns true if current selection is in a YouTube node
 */
export function isYoutubeActive(editor: Editor | null): boolean {
  if (!editor) return false
  return editor.isActive('youtube')
}

/**
 * Inserts a YouTube embed at the current position
 *
 * @param editor - The Tiptap editor instance
 * @param url - Optional YouTube URL. If empty, shows input overlay
 * @returns true if insertion succeeded, false otherwise
 */
export function insertYoutube(editor: Editor | null, url?: string): boolean {
  if (!editor || !editor.isEditable) {
    return false
  }

  try {
    // Insert YouTube node directly using insertContent
    const result = editor
      .chain()
      .focus()
      .insertContent({
        type: 'youtube',
        attrs: {
          src: url || '',
          width: 560,
          height: 315
        }
      })
      .run()

    return result
  } catch (error) {
    console.error('YouTube insert error:', error)
    return false
  }
}
