import type { Editor } from '@tiptap/react'
import type { Node } from '@tiptap/pm/model'
import {
  getAnchorNodeAndPos,
  getEditorExtension
  // isValidPosition
} from '../utils/editor-utils'

/**
 * Shortcut key constant for copy anchor link action
 */
export const COPY_ANCHOR_LINK_SHORTCUT_KEY = 'mod+ctrl+l'

/**
 * Checks if the editor is ready for operations
 * Validates that editor exists and is editable
 */
function isEditorReady(editor: Editor | null): boolean {
  return !!(editor && editor.isEditable)
}

/**
 * Gets the attribute name for unique IDs from the uniqueID extension
 * Falls back to 'data-id' if extension is not configured
 *
 * @param editor - The Tiptap editor instance
 * @returns The attribute name for node IDs
 */
function getAttributeName(editor: Editor): string {
  const ext = getEditorExtension(editor, 'uniqueID')
  return ext?.options?.attributeName || 'data-id'
}

/**
 * Extracts the data-id attribute from a node
 *
 * @param node - The ProseMirror node
 * @param attributeName - The attribute name to extract
 * @returns The node ID or null if not found
 */
export function extractNodeId(node: Node | null, attributeName: string): string | null {
  if (!node?.attrs?.[attributeName]) return null

  try {
    return node.attrs[attributeName]
  } catch {
    return null
  }
}

/**
 * Retrieves node info including the node ID
 * Returns null if editor is not ready or no node found
 */
function getNodeWithId(editor: Editor | null): {
  node: Node
  nodeId: string | null
  hasNodeId: boolean
} | null {
  if (!isEditorReady(editor)) return null

  const nodeInfo = getAnchorNodeAndPos(editor!)
  if (!nodeInfo) return null

  const attributeName = getAttributeName(editor!)
  const nodeId = extractNodeId(nodeInfo.node, attributeName)

  return {
    node: nodeInfo.node,
    nodeId,
    hasNodeId: nodeId !== null
  }
}

/**
 * Checks if a node has a data-id that can be copied as an anchor link
 *
 * Validates:
 * - Editor is ready
 * - Node exists at current selection
 * - Node has a valid ID attribute
 *
 * @param editor - The Tiptap editor instance
 * @returns true if anchor link can be copied, false otherwise
 */
export function canCopyAnchorLink(editor: Editor | null): boolean {
  const nodeWithId = getNodeWithId(editor)
  return nodeWithId?.hasNodeId ?? false
}

/**
 * Copies the node ID to clipboard as a full URL with hash
 *
 * Creates a URL like: https://example.com/page?source=copy_link#node-id
 * The hash allows for direct linking to the specific block
 *
 * @param editor - The Tiptap editor instance
 * @returns Promise resolving to true if copy succeeded, false otherwise
 */
export async function copyNodeId(editor: Editor | null): Promise<boolean> {
  const nodeWithId = getNodeWithId(editor)

  if (!nodeWithId) return false

  const { nodeId, hasNodeId } = nodeWithId

  // Can't copy if no valid ID exists
  if (!hasNodeId || !nodeId) return false

  try {
    // Build the full URL with hash for direct linking
    const currentUrl = new URL(window.location.href)

    // Add source parameter to track how the link was created
    currentUrl.searchParams.set('source', 'copy_link')

    // Set the hash to the node ID for direct navigation
    currentUrl.hash = nodeId

    // Write to clipboard
    await navigator.clipboard.writeText(currentUrl.toString())
    return true
  } catch (err) {
    console.error('Failed to copy node ID to clipboard:', err)
    return false
  }
}
