import type { Editor } from '@tiptap/react'
import { NodeSelection } from '@tiptap/pm/state'

export const HIDE_FLOATING_META = 'hideFloatingToolbar'

/**
 * Programmatically select a node and hide floating toolbar for that selection
 * Used when clicking drag handle to prevent floating toolbar from appearing
 * @param editor - The Tiptap editor instance
 * @param pos - The position of the node to select
 */
export const selectNodeAndHideFloating = (editor: Editor, pos: number) => {
  if (!editor) return
  const { state, view } = editor
  view.dispatch(
    state.tr.setSelection(NodeSelection.create(state.doc, pos)).setMeta(HIDE_FLOATING_META, true)
  )
}
