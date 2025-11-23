import type { Editor } from '@tiptap/react'
import { Link2, Palette } from 'lucide-react'

import type { ActionDefinition } from '../ActionRegistry'

/**
 * Link and color action definitions
 * These actions handle link editing and text/highlight coloring
 */

/**
 * Check if link can be set/unset in current selection
 */
function canToggleLink(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false
  try {
    return editor.can().setLink({ href: '' }) || editor.isActive('link')
  } catch {
    return false
  }
}

/**
 * Check if link is currently active
 */
function isLinkActive(editor: Editor | null): boolean {
  if (!editor) return false
  return editor.isActive('link')
}

/**
 * Get current link href if active
 */
function getCurrentLinkHref(editor: Editor | null): string {
  if (!editor || !editor.isActive('link')) return ''
  return editor.getAttributes('link').href || ''
}

/**
 * Set or update link on current selection
 */
function setLink(editor: Editor | null, href: string): boolean {
  if (!editor || !canToggleLink(editor)) return false

  if (!href || href.trim() === '') {
    return editor.chain().focus().unsetLink().run()
  }

  return editor.chain().focus().setLink({ href }).run()
}

/**
 * Remove link from current selection
 */
function unsetLink(editor: Editor | null): boolean {
  if (!editor || !editor.isActive('link')) return false
  return editor.chain().focus().unsetLink().run()
}

/**
 * Check if text color can be applied
 */
function canSetTextColor(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false
  try {
    return editor.can().setMark('textStyle', { color: 'test' })
  } catch {
    return false
  }
}

/**
 * Check if highlight color can be applied
 */
function canSetHighlightColor(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false
  try {
    return editor.can().setMark('highlight', { color: 'test' })
  } catch {
    return false
  }
}

/**
 * Link action - opens link popover
 * Note: The actual link editing is handled by the LinkPopover component
 * This action just checks availability
 */
export const linkAction: ActionDefinition = {
  id: 'link',
  type: 'mark',
  label: 'Link',
  icon: Link2,
  shortcut: 'mod+k',
  group: 'Text Formatting',
  checkAvailability: (editor: Editor | null) => canToggleLink(editor),
  checkActive: (editor: Editor | null) => isLinkActive(editor),
  execute: () => {
    // This is a placeholder - actual execution handled by LinkPopover
    return true
  }
}

/**
 * Text/Highlight color action - opens color picker
 * Note: The actual color selection is handled by the ColorPicker component
 * This action just checks availability
 */
export const colorAction: ActionDefinition = {
  id: 'color',
  type: 'mark',
  label: 'Color',
  icon: Palette,
  shortcut: 'mod+shift+c',
  group: 'Text Formatting',
  checkAvailability: (editor: Editor | null) => {
    return canSetTextColor(editor) || canSetHighlightColor(editor)
  },
  checkActive: (editor: Editor | null) => {
    if (!editor) return false
    return editor.isActive('textStyle') || editor.isActive('highlight')
  },
  execute: () => {
    // This is a placeholder - actual execution handled by ColorPicker
    return true
  }
}

/**
 * Export all link and color action specs
 */
export const linkColorSpecs: ActionDefinition[] = [linkAction, colorAction]

// Export helper functions for use in components
export {
  canToggleLink,
  isLinkActive,
  getCurrentLinkHref,
  setLink,
  unsetLink,
  canSetTextColor,
  canSetHighlightColor
}
