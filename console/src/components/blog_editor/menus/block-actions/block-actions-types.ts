import type { Node } from '@tiptap/pm/model'
import type { Editor } from '@tiptap/react'

/**
 * Configuration for Block Actions Menu
 */
export interface BlockActionsConfig {
  showSlashTrigger?: boolean
}

/**
 * Data about a block's position in the document
 */
export interface BlockPositionData {
  node: Node | null
  editor: Editor
  pos: number
}

/**
 * Configuration for a single action item in the menu
 */
export interface ActionItemConfig {
  icon: React.ComponentType<{ className?: string; style?: React.CSSProperties }>
  label: string
  action: () => void
  disabled?: boolean
  active?: boolean
  shortcut?: React.ReactNode
}
