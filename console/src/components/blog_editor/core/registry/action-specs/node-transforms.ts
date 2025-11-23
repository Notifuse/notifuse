import type { Editor } from '@tiptap/react'
import {
  Type,
  Heading1,
  Heading2,
  Heading3,
  List,
  ListOrdered,
  Code,
  TextQuote,
  Minus
} from 'lucide-react'

import {
  canToggleText,
  toggleParagraph,
  isParagraphActive
} from '../../../hooks/useText'
import {
  canToggle as canToggleHeading,
  toggleHeading,
  isHeadingActive,
  type Level
} from '../../../hooks/useHeading'
import {
  canToggleList,
  toggleList,
  isListActive,
  type ListType
} from '../../../hooks/useList'
import { canToggleBlockquote, toggleBlockquote } from '../../../hooks/useBlockquote'
import {
  canToggle as canToggleCodeBlock,
  toggleCodeBlock
} from '../../../hooks/useCodeBlock'

import type { ActionDefinition } from '../ActionRegistry'
import { toYoutubeAction } from './youtube-action'
import { toImageAction } from './image-action'

/**
 * Node transformation action definitions
 * These actions transform blocks from one type to another
 */

export const toParagraphAction: ActionDefinition = {
  id: 'to-paragraph',
  type: 'transform',
  label: 'Text',
  icon: Type,
  shortcut: 'mod+alt+0',
  group: 'Transform',
  checkAvailability: (editor: Editor | null) => canToggleText(editor),
  checkActive: (editor: Editor | null) => isParagraphActive(editor),
  execute: (editor: Editor | null) => toggleParagraph(editor)
}

export const toHeading1Action: ActionDefinition = {
  id: 'to-heading-1',
  type: 'transform',
  label: 'Heading 1',
  icon: Heading1,
  shortcut: 'ctrl+alt+1',
  group: 'Basics',
  checkAvailability: (editor: Editor | null) => {
    // Check if H1 is disabled via editor config
    if (editor?.storage?.notifuseEditorControls?.disableH1 === true) {
      return false
    }
    return canToggleHeading(editor, 1)
  },
  checkActive: (editor: Editor | null) => isHeadingActive(editor, 1),
  execute: (editor: Editor | null) => toggleHeading(editor, 1 as Level)
}

export const toHeading2Action: ActionDefinition = {
  id: 'to-heading-2',
  type: 'transform',
  label: 'Heading 2',
  icon: Heading2,
  shortcut: 'ctrl+alt+2',
  group: 'Basics',
  checkAvailability: (editor: Editor | null) => canToggleHeading(editor, 2),
  checkActive: (editor: Editor | null) => isHeadingActive(editor, 2),
  execute: (editor: Editor | null) => toggleHeading(editor, 2 as Level)
}

export const toHeading3Action: ActionDefinition = {
  id: 'to-heading-3',
  type: 'transform',
  label: 'Heading 3',
  icon: Heading3,
  shortcut: 'ctrl+alt+3',
  group: 'Basics',
  checkAvailability: (editor: Editor | null) => canToggleHeading(editor, 3),
  checkActive: (editor: Editor | null) => isHeadingActive(editor, 3),
  execute: (editor: Editor | null) => toggleHeading(editor, 3 as Level)
}

export const toBulletListAction: ActionDefinition = {
  id: 'to-bullet-list',
  type: 'transform',
  label: 'Bullet List',
  icon: List,
  shortcut: 'mod+shift+8',
  group: 'Basics',
  checkAvailability: (editor: Editor | null) => canToggleList(editor, 'bulletList'),
  checkActive: (editor: Editor | null) => isListActive(editor, 'bulletList'),
  execute: (editor: Editor | null) => toggleList(editor, 'bulletList' as ListType)
}

export const toNumberedListAction: ActionDefinition = {
  id: 'to-numbered-list',
  type: 'transform',
  label: 'Numbered List',
  icon: ListOrdered,
  shortcut: 'mod+shift+7',
  group: 'Basics',
  checkAvailability: (editor: Editor | null) => canToggleList(editor, 'orderedList'),
  checkActive: (editor: Editor | null) => isListActive(editor, 'orderedList'),
  execute: (editor: Editor | null) => toggleList(editor, 'orderedList' as ListType)
}

export const toQuoteAction: ActionDefinition = {
  id: 'to-quote',
  type: 'transform',
  label: 'Quote',
  icon: TextQuote,
  shortcut: 'mod+shift+b',
  group: 'Basics',
  checkAvailability: (editor: Editor | null) => canToggleBlockquote(editor),
  checkActive: (editor: Editor | null) => editor?.isActive('blockquote') ?? false,
  execute: (editor: Editor | null) => toggleBlockquote(editor)
}

export const toCodeBlockAction: ActionDefinition = {
  id: 'to-code-block',
  type: 'transform',
  label: 'Code Block',
  icon: Code,
  shortcut: 'mod+alt+c',
  group: 'Media',
  checkAvailability: (editor: Editor | null) => canToggleCodeBlock(editor),
  checkActive: (editor: Editor | null) => editor?.isActive('codeBlock') ?? false,
  execute: (editor: Editor | null) => toggleCodeBlock(editor)
}

export const toSeparatorAction: ActionDefinition = {
  id: 'to-separator',
  type: 'transform',
  label: 'Separator',
  icon: Minus,
  shortcut: '',
  group: 'Basics',
  checkAvailability: (editor: Editor | null) => {
    if (!editor || !editor.isEditable) return false
    return editor.can().setHorizontalRule()
  },
  checkActive: () => false,
  execute: (editor: Editor | null) => {
    if (!editor) return false
    return editor.chain().focus().setHorizontalRule().run()
  }
}

/**
 * Export all node transformation action specs
 */
export const nodeTransformSpecs: ActionDefinition[] = [
  toParagraphAction,
  toHeading1Action,
  toHeading2Action,
  toHeading3Action,
  toBulletListAction,
  toNumberedListAction,
  toQuoteAction,
  toSeparatorAction,
  toCodeBlockAction,
  toImageAction,
  toYoutubeAction
]
