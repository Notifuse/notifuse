import type { Editor } from '@tiptap/react'
import { Bold, Italic, Underline, Strikethrough, Code2, Superscript, Subscript } from 'lucide-react'

import {
  canToggleMark,
  isMarkActive,
  toggleMark,
  type Mark
} from '../../../hooks/useMark'

import type { ActionDefinition } from '../ActionRegistry'

/**
 * Text mark action definitions
 * These actions apply inline formatting to text
 */

export const boldAction: ActionDefinition = {
  id: 'bold',
  type: 'mark',
  label: 'Bold',
  icon: Bold,
  shortcut: 'mod+b',
  group: 'Text Formatting',
  checkAvailability: (editor: Editor | null) => canToggleMark(editor, 'bold' as Mark),
  checkActive: (editor: Editor | null) => isMarkActive(editor, 'bold' as Mark),
  execute: (editor: Editor | null) => toggleMark(editor, 'bold' as Mark)
}

export const italicAction: ActionDefinition = {
  id: 'italic',
  type: 'mark',
  label: 'Italic',
  icon: Italic,
  shortcut: 'mod+i',
  group: 'Text Formatting',
  checkAvailability: (editor: Editor | null) => canToggleMark(editor, 'italic' as Mark),
  checkActive: (editor: Editor | null) => isMarkActive(editor, 'italic' as Mark),
  execute: (editor: Editor | null) => toggleMark(editor, 'italic' as Mark)
}

export const underlineAction: ActionDefinition = {
  id: 'underline',
  type: 'mark',
  label: 'Underline',
  icon: Underline,
  shortcut: 'mod+u',
  group: 'Text Formatting',
  checkAvailability: (editor: Editor | null) => canToggleMark(editor, 'underline' as Mark),
  checkActive: (editor: Editor | null) => isMarkActive(editor, 'underline' as Mark),
  execute: (editor: Editor | null) => toggleMark(editor, 'underline' as Mark)
}

export const strikeAction: ActionDefinition = {
  id: 'strike',
  type: 'mark',
  label: 'Strikethrough',
  icon: Strikethrough,
  shortcut: 'mod+shift+s',
  group: 'Text Formatting',
  checkAvailability: (editor: Editor | null) => canToggleMark(editor, 'strike' as Mark),
  checkActive: (editor: Editor | null) => isMarkActive(editor, 'strike' as Mark),
  execute: (editor: Editor | null) => toggleMark(editor, 'strike' as Mark)
}

export const codeAction: ActionDefinition = {
  id: 'code',
  type: 'mark',
  label: 'Code',
  icon: Code2,
  shortcut: 'mod+e',
  group: 'Text Formatting',
  checkAvailability: (editor: Editor | null) => canToggleMark(editor, 'code' as Mark),
  checkActive: (editor: Editor | null) => isMarkActive(editor, 'code' as Mark),
  execute: (editor: Editor | null) => toggleMark(editor, 'code' as Mark)
}

export const superscriptAction: ActionDefinition = {
  id: 'superscript',
  type: 'mark',
  label: 'Superscript',
  icon: Superscript,
  shortcut: 'mod+.',
  group: 'Text Formatting',
  checkAvailability: (editor: Editor | null) => canToggleMark(editor, 'superscript' as Mark),
  checkActive: (editor: Editor | null) => isMarkActive(editor, 'superscript' as Mark),
  execute: (editor: Editor | null) => toggleMark(editor, 'superscript' as Mark)
}

export const subscriptAction: ActionDefinition = {
  id: 'subscript',
  type: 'mark',
  label: 'Subscript',
  icon: Subscript,
  shortcut: 'mod+,',
  group: 'Text Formatting',
  checkAvailability: (editor: Editor | null) => canToggleMark(editor, 'subscript' as Mark),
  checkActive: (editor: Editor | null) => isMarkActive(editor, 'subscript' as Mark),
  execute: (editor: Editor | null) => toggleMark(editor, 'subscript' as Mark)
}

/**
 * Export all text mark action specs
 */
export const textMarkSpecs: ActionDefinition[] = [
  boldAction,
  italicAction,
  underlineAction,
  strikeAction,
  codeAction,
  superscriptAction,
  subscriptAction
]
