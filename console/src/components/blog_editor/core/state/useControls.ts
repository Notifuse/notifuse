import type { Editor } from '@tiptap/react'
import { useEditorState } from '@tiptap/react'
import { INITIAL_EDITOR_CONTROLS, type EditorControls } from './EditorControls'

/**
 * Hook to access editor controls state with automatic reactivity
 *
 * This hook uses Tiptap's useEditorState with a selector pattern to:
 * - Provide reactive access to editor controls state
 * - Prevent unnecessary re-renders by using a selector
 * - Handle null editor gracefully with fallback state
 * - Warn if extension is not properly configured
 *
 * @param editor - The Tiptap editor instance (can be null)
 * @returns EditorControls state object
 *
 * @example
 * ```tsx
 * const { isDragging, dragHandleLocked, activeMenuId } = useControls(editor)
 *
 * // Use in component
 * <div style={{ cursor: isDragging ? 'grabbing' : 'auto' }}>
 *   {content}
 * </div>
 * ```
 */
export function useControls(editor: Editor | null): EditorControls {
  return (
    useEditorState({
      editor,
      selector: ({ editor }) => {
        if (!editor) return INITIAL_EDITOR_CONTROLS

        const controls = editor.storage.notifuseEditorControls
        if (!controls) {
          console.warn(
            'ControlsExtension is not initialized. Ensure you have added ControlsExtension to your editor extensions.'
          )
          return INITIAL_EDITOR_CONTROLS
        }

        return { ...INITIAL_EDITOR_CONTROLS, ...controls }
      }
    }) ?? INITIAL_EDITOR_CONTROLS
  )
}

export default useControls
