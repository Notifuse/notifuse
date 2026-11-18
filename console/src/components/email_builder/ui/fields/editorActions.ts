import { createContext, useContext } from 'react'

// What a settings panel can ask of the editor around it. Empty when the panel is rendered alone
// (a test) or when the editor's view is forced (the tour).
export interface EditorActions {
  // Switches the editor to its preview, which compiles the email (the breakpoint shows only there)
  showPreview?: () => void
}

export const EditorActionsContext = createContext<EditorActions>({})

export const useEditorActions = (): EditorActions => useContext(EditorActionsContext)
