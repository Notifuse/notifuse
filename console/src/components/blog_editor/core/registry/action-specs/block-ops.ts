import type { Editor } from '@tiptap/react'
import { Copy, Trash2, Clipboard, Link } from 'lucide-react'

import { canDuplicateNode, duplicateNode } from '../../../hooks/useDuplicate'
import { canDeleteNode, deleteNode } from '../../../hooks/useDeleteNode'
import {
  canCopyToClipboard,
  copyToClipboard
} from '../../../hooks/useCopyToClipboard'
import { canCopyAnchorLink, copyNodeId } from '../../../hooks/useCopyAnchorLink'

import type { ActionDefinition } from '../ActionRegistry'

/**
 * Block operation action definitions
 * These actions operate on entire blocks/nodes
 */

export const duplicateAction: ActionDefinition = {
  id: 'duplicate',
  type: 'block-op',
  label: 'Duplicate',
  icon: Copy,
  shortcut: 'mod+d',
  group: 'Block Operations',
  checkAvailability: (editor: Editor | null) => canDuplicateNode(editor),
  execute: (editor: Editor | null) => duplicateNode(editor)
}

export const deleteAction: ActionDefinition = {
  id: 'delete',
  type: 'block-op',
  label: 'Delete',
  icon: Trash2,
  shortcut: 'mod+backspace',
  group: 'Block Operations',
  checkAvailability: (editor: Editor | null) => canDeleteNode(editor),
  execute: (editor: Editor | null) => deleteNode(editor)
}

export const copyToClipboardAction: ActionDefinition = {
  id: 'copy-to-clipboard',
  type: 'block-op',
  label: 'Copy',
  icon: Clipboard,
  shortcut: 'mod+c',
  group: 'Block Operations',
  checkAvailability: (editor: Editor | null) => canCopyToClipboard(editor),
  execute: async (editor: Editor | null) => {
    return await copyToClipboard(editor, true)
  }
}

export const copyAnchorLinkAction: ActionDefinition = {
  id: 'copy-anchor-link',
  type: 'block-op',
  label: 'Copy link to block',
  icon: Link,
  shortcut: 'mod+ctrl+l',
  group: 'Block Operations',
  checkAvailability: (editor: Editor | null) => canCopyAnchorLink(editor),
  execute: async (editor: Editor | null) => {
    return await copyNodeId(editor)
  },
  hideWhenUnavailable: true
}

/**
 * Export all block operation action specs
 */
export const blockOperationSpecs: ActionDefinition[] = [
  duplicateAction,
  deleteAction,
  copyToClipboardAction,
  copyAnchorLinkAction
]
