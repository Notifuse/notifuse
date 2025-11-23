import type { Editor } from '@tiptap/react'
import { AlignLeft, AlignCenter, AlignRight, AlignJustify } from 'lucide-react'

import {
  canSetTextAlign,
  setTextAlign,
  isTextAlignActive,
  type TextAlign
} from '../../../hooks/useTextAlign'

import type { ActionDefinition } from '../ActionRegistry'

/**
 * Text alignment action definitions
 * These actions change the alignment of text blocks
 */

export const alignLeftAction: ActionDefinition = {
  id: 'align-left',
  type: 'alignment',
  label: 'Align left',
  icon: AlignLeft,
  shortcut: 'mod+shift+l',
  group: 'Text Alignment',
  checkAvailability: (editor: Editor | null) => canSetTextAlign(editor, 'left'),
  checkActive: (editor: Editor | null) => isTextAlignActive(editor, 'left'),
  execute: (editor: Editor | null) => setTextAlign(editor, 'left' as TextAlign)
}

export const alignCenterAction: ActionDefinition = {
  id: 'align-center',
  type: 'alignment',
  label: 'Align center',
  icon: AlignCenter,
  shortcut: 'mod+shift+e',
  group: 'Text Alignment',
  checkAvailability: (editor: Editor | null) => canSetTextAlign(editor, 'center'),
  checkActive: (editor: Editor | null) => isTextAlignActive(editor, 'center'),
  execute: (editor: Editor | null) => setTextAlign(editor, 'center' as TextAlign)
}

export const alignRightAction: ActionDefinition = {
  id: 'align-right',
  type: 'alignment',
  label: 'Align right',
  icon: AlignRight,
  shortcut: 'mod+shift+r',
  group: 'Text Alignment',
  checkAvailability: (editor: Editor | null) => canSetTextAlign(editor, 'right'),
  checkActive: (editor: Editor | null) => isTextAlignActive(editor, 'right'),
  execute: (editor: Editor | null) => setTextAlign(editor, 'right' as TextAlign)
}

export const alignJustifyAction: ActionDefinition = {
  id: 'align-justify',
  type: 'alignment',
  label: 'Align justify',
  icon: AlignJustify,
  shortcut: 'mod+shift+j',
  group: 'Text Alignment',
  checkAvailability: (editor: Editor | null) => canSetTextAlign(editor, 'justify'),
  checkActive: (editor: Editor | null) => isTextAlignActive(editor, 'justify'),
  execute: (editor: Editor | null) => setTextAlign(editor, 'justify' as TextAlign)
}

/**
 * Export all text alignment action specs
 */
export const textAlignmentSpecs: ActionDefinition[] = [
  alignLeftAction,
  alignCenterAction,
  alignRightAction,
  alignJustifyAction
]
