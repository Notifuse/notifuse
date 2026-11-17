// Main editor components
export { TiptapRichEditor } from './TiptapRichEditor'
export { TiptapInlineEditor } from './TiptapInlineEditor'

// Toolbar components (can be used separately if needed)
export {
  TiptapToolbar,
  ToolbarButton,
  ColorButton,
  ToolbarSeparator,
  EmojiButton,
  LinkButton
} from './components/TiptapToolbar'

// Shared types
export type {
  BaseTiptapProps,
  TiptapRichEditorProps,
  TiptapInlineEditorProps,
  TiptapToolbarProps,
  ToolbarButtonProps,
  ColorButtonProps,
  EmojiButtonProps,
  LinkButtonProps,
  ButtonType,
  LinkType
} from './shared/types'

// Shared utilities (for advanced usage)
export {
  convertBlockToInline,
  processInlineContent,
  prepareInlineContent,
  getInitialInlineContent
} from './shared/utils'

// Shared extensions (for custom implementations)
export { createRichExtensions, createInlineExtensions, InlineDocument } from './shared/extensions'

// Styles utilities
export {
  injectTiptapStyles,
  defaultToolbarStyle,
  defaultToolbarClasses,
  getToolbarButtonClasses,
  toolbarSeparatorClasses
} from './shared/styles'
