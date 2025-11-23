import type { Editor, Range } from '@tiptap/react'
import type React from 'react'

/**
 * Generic item structure for suggestion menus
 */
export interface SuggestionItem<TContext = any> {
  /** Unique identifier for the item */
  id: string

  /** Primary display text */
  label: string

  /** Secondary descriptive text (e.g., shortcut, description) */
  subtext?: string

  /** Icon component from lucide-react or string (emoji) */
  icon?: React.ComponentType<{ className?: string; style?: React.CSSProperties }> | string

  /** Group identifier for organizing items */
  group?: string

  /** Additional keywords for search/filtering */
  keywords?: string[]

  /** Custom context data passed to onSelect */
  context?: TContext
}

/**
 * Configuration for a suggestion menu type
 */
export interface SuggestionConfig<TContext = any> {
  /** Trigger character (e.g., '/', ':', '@') */
  char: string

  /** Unique plugin key identifier */
  pluginKey: string

  /** Fetch or generate items based on query (filtering should be done here) */
  getItems: (
    query: string,
    editor: Editor | null
  ) => Promise<SuggestionItem<TContext>[]> | SuggestionItem<TContext>[]

  /** Optional group extractor for organizing items */
  groupBy?: (item: SuggestionItem<TContext>) => string | undefined

  /** Handler when item is selected */
  onSelect: (item: SuggestionItem<TContext>, editor: Editor | null, range: Range) => void

  /** Optional custom item renderer */
  renderItem?: (item: SuggestionItem<TContext>, isSelected: boolean) => React.ReactNode

  /** CSS class for the decoration node */
  decorationClass?: string

  /** Content to show in the decoration node */
  decorationContent?: string

  /** Maximum height of the menu in pixels */
  maxHeight?: number
}

/**
 * Props for rendering a suggestion menu item
 */
export interface SuggestionMenuItemProps {
  /** The item to render */
  item: SuggestionItem

  /** Whether this item is currently selected */
  isSelected: boolean

  /** Click handler */
  onClick: () => void
}
