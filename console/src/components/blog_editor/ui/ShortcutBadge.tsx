import { Badge } from 'antd'
import { parseShortcutKeys } from '../utils/shortcuts'

/**
 * Props for the ShortcutBadge component
 */
export interface ShortcutBadgeProps {
  /**
   * The keyboard shortcut string to display (e.g., "mod+d", "ctrl+shift+k")
   * Will be parsed and formatted for the current platform
   */
  shortcutKeys?: string
}

/**
 * ShortcutBadge - Displays keyboard shortcuts in a badge format
 * 
 * Automatically formats shortcuts for the user's platform:
 * - On Mac: "mod+d" becomes "⌘ D"
 * - On Windows: "mod+d" becomes "Ctrl D"
 * 
 * Uses Antd Badge component for consistent styling
 * 
 * @example
 * <ShortcutBadge shortcutKeys="mod+d" />
 * // Displays: ⌘ D (on Mac) or Ctrl D (on Windows)
 */
export function ShortcutBadge({ shortcutKeys }: ShortcutBadgeProps) {
  // Parse the shortcut keys into formatted symbols
  const formattedKeys = parseShortcutKeys({ shortcutKeys })
  
  // If no keys to display, don't render anything
  if (formattedKeys.length === 0) {
    return null
  }
  
  // Join keys with space for display (e.g., ["⌘", "D"] -> "⌘ D")
  const displayText = formattedKeys.join(' ')
  
  return <Badge count={displayText} style={{ backgroundColor: '#f0f0f0', color: '#666' }} />
}

