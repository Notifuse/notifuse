import type { Editor, Range } from '@tiptap/react'
import { gitHubEmojis, type EmojiItem } from '@tiptap/extension-emoji'
import { getFilteredEmojis } from '../../../utils/emoji'
import type { SuggestionConfig, SuggestionItem } from '../types'

/**
 * Emoji suggestion configuration
 * Triggered by ':' character
 */

// Filter out regional indicator emojis
const availableEmojis = gitHubEmojis.filter((emoji) => !emoji.name.includes('regional'))

/**
 * Convert EmojiItem to SuggestionItem
 */
const emojiToSuggestionItem = (emoji: EmojiItem): SuggestionItem<EmojiItem> => ({
  id: emoji.name,
  label: emoji.name,
  subtext: emoji.shortcodes.join(', '),
  icon: emoji.emoji,
  keywords: [...emoji.shortcodes, ...emoji.tags],
  context: emoji
})

/**
 * Emoji configuration for suggestion menu
 */
export const emojiConfig: SuggestionConfig<EmojiItem> = {
  char: ':',
  pluginKey: 'emoji-suggestion',

  // Get and filter emoji items
  getItems: async (query: string, _editor: Editor | null) => {
    // Use the existing filtering utility
    const filtered = getFilteredEmojis({ query, emojis: availableEmojis })

    // Convert to our SuggestionItem format
    return filtered.map(emojiToSuggestionItem)
  },

  // Handle emoji selection
  onSelect: (item: SuggestionItem<EmojiItem>, editor: Editor | null, range: Range) => {
    if (!editor || !item.context) return

    const emoji = item.context
    if (!emoji.emoji) return

    editor.chain().focus().deleteRange(range).insertContent(emoji.emoji).run()
  },

  maxHeight: 384
}
