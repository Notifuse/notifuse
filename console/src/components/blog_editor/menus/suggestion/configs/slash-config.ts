import type { Editor, Range } from '@tiptap/react'
import { Smile } from 'lucide-react'
import { notifuseActionRegistry } from '../../../core/registry/ActionRegistry'
import type { ActionDefinition } from '../../../core/registry/ActionRegistry'
import type { SuggestionConfig, SuggestionItem } from '../types'

/**
 * Slash command suggestion configuration
 * Triggered by '/' character
 */

/**
 * Convert ActionDefinition to SuggestionItem
 */
const actionToSuggestionItem = (
  action: ActionDefinition,
  editor: Editor | null
): SuggestionItem<ActionDefinition> | null => {
  // Skip actions that are unavailable
  if (!action.checkAvailability(editor)) {
    return null
  }

  return {
    id: action.id,
    label: action.label,
    icon: action.icon,
    group: action.group || 'Actions',
    keywords: [action.label.toLowerCase(), action.id],
    context: action
  }
}

/**
 * Filter actions by query string
 */
const filterActions = (items: SuggestionItem[], query: string): SuggestionItem[] => {
  if (!query.trim()) {
    return items
  }

  const lowerQuery = query.toLowerCase()

  return items.filter((item) => {
    // Match on label
    if (item.label.toLowerCase().includes(lowerQuery)) {
      return true
    }

    // Match on keywords
    if (item.keywords?.some((kw) => kw.toLowerCase().includes(lowerQuery))) {
      return true
    }

    // Match on group
    if (item.group?.toLowerCase().includes(lowerQuery)) {
      return true
    }

    return false
  })
}

/**
 * Slash command configuration for suggestion menu
 */
export const slashConfig: SuggestionConfig<ActionDefinition> = {
  char: '/',
  pluginKey: 'slash-command',

  // Get available actions from registry and filter by query
  getItems: async (query: string, editor: Editor | null) => {
    // Get only transform actions (Style section)
    const transformActions = notifuseActionRegistry.getByType('transform')

    // Convert to SuggestionItems, filtering out unavailable ones
    const actionItems = transformActions
      .map((action) => actionToSuggestionItem(action, editor))
      .filter((item): item is SuggestionItem<ActionDefinition> => item !== null)

    // Add emoji picker trigger
    const emojiItem: SuggestionItem<{ type: 'emoji-picker' }> = {
      id: 'emoji-picker',
      label: 'Emoji',
      icon: Smile,
      group: 'Inline',
      keywords: ['emoji', 'emoticon', 'insert'],
      context: { type: 'emoji-picker' }
    }

    const allItems = [...actionItems, emojiItem]

    // Filter by query
    return filterActions(allItems, query)
  },

  // Group by action group
  groupBy: (item: SuggestionItem<ActionDefinition>) => {
    return item.group || 'Actions'
  },

  // Handle action selection
  onSelect: (item: SuggestionItem<any>, editor: Editor | null, range: Range) => {
    if (!editor || !item.context) return

    // Delete the slash command text
    editor.chain().focus().deleteRange(range).run()

    // Check if it's the emoji picker trigger
    if (item.context.type === 'emoji-picker') {
      // Insert ':' to trigger emoji suggestion menu
      editor.chain().focus().insertContent(':').run()
    } else if ('execute' in item.context) {
      // Execute action
      item.context.execute(editor)
    }
  },

  maxHeight: 384
}
