import type { EmojiItem } from '@tiptap/extension-emoji'

/**
 * Maximum number of emoji suggestions to show
 * Limits results to avoid performance issues with large lists
 */
const MAX_SUGGESTIONS = 100

/**
 * Sorting function for emoji items
 * Sorts alphabetically by emoji name for consistent display
 */
const SORT_EMOJIS = <T extends EmojiItem>(a: T, b: T) => a.name.localeCompare(b.name)

/**
 * Searches for a query string within an emoji's metadata
 * Checks the emoji name, shortcodes, and tags
 *
 * @param query - The search query string
 * @param emojiData - The emoji item to search
 * @returns true if the query matches any of the emoji's metadata
 *
 * @example
 * searchEmojiData("smile", { name: "smile", shortcodes: [":smile:"], tags: ["happy"] })
 * // Returns true
 */
export const searchEmojiData = <T extends EmojiItem>(query: string, emojiData: T): boolean => {
  const lowercaseQuery = query.toLowerCase().trim()

  return (
    // Check if emoji name contains the query
    emojiData.name.toLowerCase().includes(lowercaseQuery) ||
    // Check if any shortcode contains the query
    emojiData.shortcodes.some((shortName) => shortName.toLowerCase().includes(lowercaseQuery)) ||
    // Check if any tag contains the query
    emojiData.tags.some((tag) => tag.toLowerCase().includes(lowercaseQuery))
  )
}

/**
 * Filters and sorts a list of emojis based on a search query
 * Returns up to MAX_SUGGESTIONS emojis that match the query
 *
 * @param props - Object containing query and emojis array
 * @param props.query - The search query string
 * @param props.emojis - Array of emoji items to filter
 * @returns Filtered and sorted array of emojis
 *
 * @example
 * getFilteredEmojis({ query: "smile", emojis: allEmojis })
 * // Returns emojis matching "smile", sorted alphabetically
 */
export const getFilteredEmojis = <T extends EmojiItem>(props: { query: string; emojis: T[] }) => {
  const { query, emojis } = props
  const trimmedQuery = query.trim()

  // If no query, return first MAX_SUGGESTIONS emojis
  // Otherwise, filter by query and limit results
  const filteredEmojis = !trimmedQuery
    ? emojis.slice(0, MAX_SUGGESTIONS)
    : emojis.filter((emoji) => searchEmojiData(trimmedQuery, emoji)).slice(0, MAX_SUGGESTIONS)

  // Sort results alphabetically by name
  return filteredEmojis.sort(SORT_EMOJIS)
}
