/**
 * Default toolbar configuration
 * Defines the layout and actions for the floating selection toolbar
 */

/**
 * Action IDs for the left section (block transformations)
 */
export const LEFT_SECTION_ACTIONS = ['turn-into'] // Special component

/**
 * Action IDs for the center section (text formatting marks)
 */
export const CENTER_SECTION_ACTIONS = ['bold', 'italic', 'underline', 'strike', 'code']

/**
 * Action IDs for the right section (link, color, and more options)
 */
export const RIGHT_SECTION_ACTIONS = ['link', 'color', 'more'] // Special components

/**
 * Default toolbar layout configuration
 *
 * Layout:
 * [TurnIntoDropdown] | [B] [I] [U] [S] [</>] | [Link] [Color] [More]
 */
export const DEFAULT_TOOLBAR_CONFIG = {
  leftActions: LEFT_SECTION_ACTIONS,
  centerActions: CENTER_SECTION_ACTIONS,
  rightActions: RIGHT_SECTION_ACTIONS,
  hideWhenUnavailable: true
}
