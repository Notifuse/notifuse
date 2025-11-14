/**
 * Keyboard shortcut utilities for displaying shortcuts in UI
 */

/**
 * Mac-specific keyboard symbol mappings
 * Used to display shortcuts in a Mac-friendly format
 */
export const MAC_SYMBOLS: Record<string, string> = {
  mod: '⌘',
  command: '⌘',
  meta: '⌘',
  ctrl: '⌃',
  control: '⌃',
  alt: '⌥',
  option: '⌥',
  shift: '⇧',
  backspace: 'Del',
  delete: '⌦',
  enter: '⏎',
  escape: '⎋',
  capslock: '⇪'
} as const

/**
 * Checks if the current platform is macOS
 * Used to determine which keyboard symbols to display
 */
export function isMac(): boolean {
  return typeof navigator !== 'undefined' && navigator.platform.toLowerCase().includes('mac')
}

/**
 * Formats a single keyboard shortcut key for display
 * Converts keys like 'ctrl', 'alt' to platform-specific symbols
 *
 * @param key - The key to format (e.g., "ctrl", "alt", "shift")
 * @param isMacPlatform - Whether the current platform is Mac
 * @param capitalize - Whether to capitalize non-symbol keys (default: true)
 * @returns Formatted key string (e.g., "⌘" on Mac, "Ctrl" on Windows)
 */
export function formatShortcutKey(
  key: string,
  isMacPlatform: boolean,
  capitalize: boolean = true
): string {
  if (isMacPlatform) {
    const lowerKey = key.toLowerCase()
    // Return Mac symbol if available, otherwise capitalize the key
    return MAC_SYMBOLS[lowerKey] || (capitalize ? key.toUpperCase() : key)
  }

  // On non-Mac platforms, just capitalize the first letter
  return capitalize ? key.charAt(0).toUpperCase() + key.slice(1) : key
}

/**
 * Parses a shortcut key string into an array of formatted symbols
 * Converts strings like "mod+shift+d" into ["⌘", "⇧", "D"] on Mac
 * or ["Ctrl", "Shift", "D"] on Windows
 *
 * @param shortcutKeys - The shortcut string (e.g., "mod+shift+d")
 * @param delimiter - The character separating keys (default: "+")
 * @param capitalize - Whether to capitalize non-symbol keys (default: true)
 * @returns Array of formatted key symbols
 *
 * @example
 * parseShortcutKeys({ shortcutKeys: "mod+d" })
 * // Returns ["⌘", "D"] on Mac or ["Ctrl", "D"] on Windows
 */
export function parseShortcutKeys(props: {
  shortcutKeys: string | undefined
  delimiter?: string
  capitalize?: boolean
}): string[] {
  const { shortcutKeys, delimiter = '+', capitalize = true } = props

  // Return empty array if no shortcut provided
  if (!shortcutKeys) return []

  const isMacPlatform = isMac()

  return shortcutKeys
    .split(delimiter)
    .map((key) => key.trim())
    .map((key) => formatShortcutKey(key, isMacPlatform, capitalize))
}
