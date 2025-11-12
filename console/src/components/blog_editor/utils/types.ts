import type { Editor } from '@tiptap/react'

/**
 * Props for the BlogContentEditor component
 */
export interface BlogContentEditorProps {
  /** Initial content as Tiptap JSON or HTML string */
  content?: any
  /** Callback when content changes, receives Tiptap JSON */
  onChange?: (json: any) => void
  /** Callback when editor loses focus */
  onBlur?: () => void
  /** Whether the editor is read-only */
  readOnly?: boolean
  /** Placeholder text when editor is empty */
  placeholder?: string
  /** Auto-focus the editor on mount */
  autoFocus?: boolean
  /** Minimum height of the editor */
  minHeight?: string
  /** Custom CSS class name */
  className?: string
  /** Whether draft is currently being saved */
  isSaving?: boolean
  /** Last time draft was saved */
  lastSaved?: Date | null
}

/**
 * Props for the EditorToolbar component
 */
export interface EditorToolbarProps {
  /** Tiptap editor instance */
  editor: Editor | null
  /** Whether to show advanced formatting options */
  showAdvanced?: boolean
  /** Current mode: edit or preview */
  mode?: 'edit' | 'preview'
  /** Callback when mode changes */
  onModeChange?: (mode: 'edit' | 'preview') => void
  /** Whether draft is currently being saved */
  isSaving?: boolean
  /** Last time draft was saved */
  lastSaved?: Date | null
  /** Content statistics */
  stats?: {
    words: number
    characters: number
    readingTime: number
  }
}

/**
 * Heading level for table of contents
 */
export interface HeadingNode {
  id: string
  level: number
  text: string
  children?: HeadingNode[]
}

/**
 * Content statistics
 */
export interface ContentStats {
  characters: number
  words: number
  paragraphs: number
  readingTime: number // in minutes
}

