/**
 * Notifuse Editor - Public API
 *
 * Main entry point for the blog editor with dynamic styling support
 */

// Main component
export { NotifuseEditor, DEFAULT_INITIAL_CONTENT } from './NotifuseEditor'
export type { NotifuseEditorProps, NotifuseEditorRef, TOCAnchor } from './NotifuseEditor'

// Types
export type {
  EditorStyleConfig,
  CSSValue,
  DefaultStyles,
  ParagraphStyles,
  HeadingStyles,
  HeadingLevelStyles,
  CaptionStyles,
  SeparatorStyles,
  CodeBlockStyles,
  BlockquoteStyles,
  InlineCodeStyles,
  ListStyles,
  LinkStyles
} from './types/EditorStyleConfig'

// Default configuration
export { defaultEditorStyles } from './config/defaultEditorStyles'

// Style presets
export {
  academicPaperPreset
} from './presets'

// Utility functions
export { generateBlogPostCSS, clearCSSCache } from './utils/styleUtils'
export { validateStyleConfig, StyleConfigValidationError } from './utils/validateStyleConfig'
