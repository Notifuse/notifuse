/**
 * Editor utility functions
 */

import { NodeSelection } from '@tiptap/pm/state'
import { CellSelection } from '@tiptap/pm/tables'
import type { Editor } from '@tiptap/react'
import { isTextSelection, isNodeSelection } from '@tiptap/react'
import type { Node } from '@tiptap/pm/model'

/**
 * Node type display name mapping
 */
const NODE_TYPE_LABELS: Record<string, string> = {
  paragraph: 'Text',
  heading: 'Heading',
  blockquote: 'Blockquote',
  listItem: 'List Item',
  codeBlock: 'Code Block',
  table: 'Table'
}

/**
 * Overflow position type
 */
export type OverflowPosition = 'none' | 'top' | 'bottom' | 'both'

/**
 * Returns a display name for the current node in the editor
 * @param editor The Tiptap editor instance
 * @returns The display name of the current node
 */
export function getNodeDisplayName(editor: Editor | null): string {
  if (!editor) return 'Node'

  const { selection } = editor.state

  if (selection instanceof NodeSelection) {
    const nodeType = selection.node.type.name
    return NODE_TYPE_LABELS[nodeType] || nodeType.toLowerCase()
  }

  if (selection instanceof CellSelection) {
    return 'Table'
  }

  const { $anchor } = selection
  const nodeType = $anchor.parent.type.name
  return NODE_TYPE_LABELS[nodeType] || nodeType.toLowerCase()
}

/**
 * Determines how a target element overflows relative to a container element
 */
export function getElementOverflowPosition(
  targetElement: Element,
  containerElement: HTMLElement
): OverflowPosition {
  const targetBounds = targetElement.getBoundingClientRect()
  const containerBounds = containerElement.getBoundingClientRect()

  const isOverflowingTop = targetBounds.top < containerBounds.top
  const isOverflowingBottom = targetBounds.bottom > containerBounds.bottom

  if (isOverflowingTop && isOverflowingBottom) return 'both'
  if (isOverflowingTop) return 'top'
  if (isOverflowingBottom) return 'bottom'
  return 'none'
}

/**
 * Checks if the current text selection is valid for editing
 * - Not empty
 * - Not a code block
 * - Not a node selection
 */
export function isTextSelectionValid(editor: Editor | null): boolean {
  if (!editor) return false
  const { state } = editor
  const { selection } = state
  const isValid =
    isTextSelection(selection) &&
    !selection.empty &&
    !selection.$from.parent.type.spec.code &&
    !isNodeSelection(selection)

  return isValid
}

/**
 * Gets the anchor node and its position in the editor.
 * @param editor The Tiptap editor instance
 * @param allowEmptySelection If true, still returns the node at the cursor position even if selection is empty
 * @returns An object containing the anchor node and its position, or null if not found
 */
export function getAnchorNodeAndPos(
  editor: Editor | null,
  allowEmptySelection: boolean = true
): { node: Node; pos: number } | null {
  if (!editor) return null

  const { state } = editor
  const { selection } = state

  if (selection instanceof NodeSelection) {
    const node = selection.node
    const pos = selection.from

    if (node && isValidPosition(pos)) {
      return { node, pos }
    }
  }

  if (selection.empty && !allowEmptySelection) return null

  const $anchor = selection.$anchor
  const depth = 1 // explicitly use depth 1
  const node = $anchor.node(depth)
  const pos = $anchor.before(depth)

  return { node, pos }
}

/**
 * Retrieves a specific extension by name from the Tiptap editor.
 * @param editor - The Tiptap editor instance
 * @param extensionName - The name of the extension to retrieve
 * @returns The extension instance if found, otherwise null
 */
export function getEditorExtension(editor: Editor | null, extensionName: string) {
  if (!editor) return null

  const extension = editor.extensionManager.extensions.find((ext) => ext.name === extensionName)

  if (!extension) {
    console.warn(
      `Extension "${extensionName}" not found in the editor schema. Ensure it is included in the editor configuration.`
    )
    return null
  }

  return extension
}

/**
 * Checks if a value is a valid number (not null, undefined, or NaN)
 * @param value - The value to check
 * @returns boolean indicating if the value is a valid number
 */
export function isValidPosition(pos: number | null | undefined): pos is number {
  return typeof pos === 'number' && pos >= 0
}

/**
 * Finds the position and instance of a node in the document
 * Re-exported from useInsertBlock for convenience
 * @param props Object containing editor, node (optional), and nodePos (optional)
 * @param props.editor The Tiptap editor instance
 * @param props.node The node to find (optional if nodePos is provided)
 * @param props.nodePos The position of the node to find (optional if node is provided)
 * @returns An object with the position and node, or null if not found
 */
export function findNodePosition(params: {
  editor: Editor
  node?: Node
  nodePos?: number
}): { pos: number; node: Node } | null {
  const { editor, node, nodePos } = params

  if (isValidPosition(nodePos)) {
    try {
      // nodePos is the position of the node in the document
      // We need to find the actual node at this position
      const $pos = editor.state.doc.resolve(nodePos)

      // Get the node at this depth (not the parent doc node)
      // We want the block-level node
      for (let d = $pos.depth; d > 0; d--) {
        const node = $pos.node(d)
        const nodeStart = $pos.start(d) - 1
        if (nodeStart === nodePos) {
          return { pos: nodePos, node }
        }
      }

      // If we didn't find it by depth, try to get the node directly
      const nodeAtPos = editor.state.doc.nodeAt(nodePos)
      if (nodeAtPos) {
        return { pos: nodePos, node: nodeAtPos }
      }

      return null
    } catch (e) {
      console.error('Error resolving position:', e)
      return null
    }
  }

  if (node) {
    // Search for the node in the document
    let found: { pos: number; node: Node } | null = null
    editor.state.doc.descendants((docNode, pos) => {
      if (docNode === node) {
        found = { pos, node: docNode }
        return false
      }
      return true
    })
    return found
  }

  return null
}
