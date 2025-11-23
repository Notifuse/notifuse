import type { Editor } from '@tiptap/react'

/**
 * Supported mark types for text formatting
 */
export type Mark =
  | 'bold'
  | 'italic'
  | 'strike'
  | 'code'
  | 'underline'
  | 'superscript'
  | 'subscript'

/**
 * Checks if a mark exists in the editor schema
 * Validates that the mark type is registered in the editor
 * 
 * @param markName - The name of the mark to check
 * @param editor - The Tiptap editor instance
 * @returns true if mark exists in schema, false otherwise
 */
function isMarkInSchema(markName: string, editor: Editor | null): boolean {
  if (!editor?.schema) return false
  return editor.schema.spec.marks.get(markName) !== undefined
}

/**
 * Checks if a node type is currently selected
 * Used to prevent applying marks to non-text nodes like images
 * 
 * @param editor - The Tiptap editor instance
 * @param nodeTypeNames - Array of node type names to check
 * @returns true if any of the node types are selected
 */
function isNodeTypeSelected(
  editor: Editor | null,
  nodeTypeNames: string[] = []
): boolean {
  if (!editor || !editor.state.selection) return false

  const { selection } = editor.state
  if (selection.empty) return false

  // Check if selection contains any of the specified node types
  const { $from, $to } = selection
  let hasNodeType = false

  editor.state.doc.nodesBetween($from.pos, $to.pos, (node) => {
    if (nodeTypeNames.includes(node.type.name)) {
      hasNodeType = true
      return false // Stop iteration
    }
  })

  return hasNodeType
}

/**
 * Checks if a mark can be toggled in the current editor state
 * 
 * Validates:
 * - Editor is ready and editable
 * - Mark exists in the schema
 * - Current selection is not a non-text node (like image)
 * - Editor's state allows toggling the mark
 * 
 * @param editor - The Tiptap editor instance
 * @param type - The mark type to check
 * @returns true if mark can be toggled, false otherwise
 */
export function canToggleMark(editor: Editor | null, type: Mark): boolean {
  if (!editor || !editor.isEditable) return false
  
  // Check if mark exists in schema
  if (!isMarkInSchema(type, editor)) return false
  
  // Don't allow marks on images
  if (isNodeTypeSelected(editor, ['image'])) return false

  // Use Tiptap's built-in check
  return editor.can().toggleMark(type)
}

/**
 * Checks if a mark is currently active in the selection
 * 
 * @param editor - The Tiptap editor instance
 * @param type - The mark type to check
 * @returns true if mark is active, false otherwise
 */
export function isMarkActive(editor: Editor | null, type: Mark): boolean {
  if (!editor) return false
  return editor.isActive(type)
}

/**
 * Toggles a mark in the current selection
 * 
 * If the mark is active, it will be removed
 * If the mark is inactive, it will be applied
 * 
 * @param editor - The Tiptap editor instance
 * @param type - The mark type to toggle
 * @returns true if toggle succeeded, false otherwise
 */
export function toggleMark(editor: Editor | null, type: Mark): boolean {
  if (!editor || !editor.isEditable) return false

  try {
    return editor.chain().focus().toggleMark(type).run()
  } catch {
    return false
  }
}

