import { useCallback, useEffect, useState } from 'react'
import type { Editor } from '@tiptap/react'
import type { Node } from '@tiptap/pm/model'

// --- Hooks ---
import { useNotifuseEditor } from '../hooks/useEditor'

/**
 * Configuration for the insert block functionality
 */
export interface UseInsertBlockConfig {
  /**
   * The Tiptap editor instance.
   */
  editor?: Editor | null
  /**
   * The node to apply trigger to
   */
  node?: Node | null
  /**
   * The position of the node in the document
   */
  nodePos?: number | null
  /**
   * The trigger text to insert
   * @default "/"
   */
  trigger?: string
  /**
   * Callback function called after a successful trigger insertion.
   */
  onTriggered?: (trigger: string) => void
}

/**
 * Checks if a position is valid
 */
function isValidPosition(pos: number | null | undefined): pos is number {
  return typeof pos === 'number' && pos >= 0
}

/**
 * Finds the position of a node in the document
 */
function findNodePosition(params: {
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

/**
 * Checks if a specific node type is selected
 */
function isNodeTypeSelected(editor: Editor, nodeTypes: string[]): boolean {
  const { selection } = editor.state
  const { $from } = selection

  for (let depth = $from.depth; depth > 0; depth--) {
    const node = $from.node(depth)
    if (nodeTypes.includes(node.type.name)) {
      return true
    }
  }

  return false
}

/**
 * Checks if a slash command can be inserted in the current editor state
 */
export function canInsertSlashCommand(
  editor: Editor | null,
  node?: Node | null,
  nodePos?: number | null
): boolean {
  if (!editor || !editor.isEditable) return false
  if (isNodeTypeSelected(editor, ['image'])) return false

  if (node || isValidPosition(nodePos)) {
    if (isValidPosition(nodePos) && nodePos! >= 0) return true

    if (node) {
      const foundPos = findNodePosition({ editor, node })
      return foundPos !== null
    }
  }

  return true
}

/**
 * Inserts a slash command at a specified node position or after the current selection
 */
export function insertSlashCommand(
  editor: Editor | null,
  trigger: string = '/',
  node?: Node | null,
  nodePos?: number | null
): boolean {
  if (!editor || !editor.isEditable) return false
  if (!canInsertSlashCommand(editor, node, nodePos)) return false

  try {
    if ((node !== undefined && node !== null) || isValidPosition(nodePos)) {
      const foundPos = findNodePosition({
        editor,
        node: node || undefined,
        nodePos: nodePos || undefined
      })

      if (!foundPos) {
        return false
      }

      const isEmpty = foundPos.node.type.name === 'paragraph' && foundPos.node.content.size === 0
      const insertPos = isEmpty ? foundPos.pos + 1 : foundPos.pos + foundPos.node.nodeSize

      // If the block is empty, just insert the trigger
      if (isEmpty) {
        return editor.chain().focus(insertPos).insertContent(trigger).run()
      }

      // If the block has content, create a new paragraph after it with the trigger
      const docSize = editor.state.doc.content.size
      const isAtEnd = insertPos >= docSize

      // For inserting at the end of document, use a different approach
      if (isAtEnd) {
        // First, create an empty paragraph at the end
        const success = editor
          .chain()
          .command(({ tr, state }) => {
            const para = state.schema.nodes.paragraph.create()
            tr.insert(tr.doc.content.size, para)
            return true
          })
          .focus('end')
          .run()
        
        if (!success) return false
        
        // Then insert the trigger character using insertContent to trigger the suggestion plugin
        return editor.chain().insertContent(trigger).run()
      } else {
        // For middle positions, insert empty paragraph then trigger character
        const success = editor
          .chain()
          .insertContentAt(insertPos, { type: 'paragraph' })
          .focus(insertPos + 1)
          .run()
        
        if (!success) return false
        
        // Insert the trigger character to trigger the suggestion plugin
        return editor.chain().insertContent(trigger).run()
      }
    }

    const { $from } = editor.state.selection
    const currentNode = $from.node()
    const isEmpty = currentNode.textContent.length === 0
    const isStartOfBlock = $from.parentOffset === 0

    // Check if we're at the document node level
    // This is important if we dont have focus on the editor
    // and we want to insert the slash at the end of the document
    const isTopLevel = $from.depth === 0

    if (!isEmpty || !isStartOfBlock) {
      const insertPosition = isTopLevel ? editor.state.doc.content.size : $from.after()

      return editor
        .chain()
        .insertContentAt(insertPosition, {
          type: 'paragraph',
          content: [{ type: 'text', text: trigger }]
        })
        .focus()
        .run()
    }

    return editor.chain().insertContent({ type: 'text', text: trigger }).focus().run()
  } catch {
    return false
  }
}

/**
 * Determines if the insert block button should be shown
 */
export function shouldShowButton(props: {
  editor: Editor | null
  node?: Node | null
  nodePos?: number | null
}): boolean {
  const { editor, node, nodePos } = props

  if (!editor || !editor.isEditable) return false

  return canInsertSlashCommand(editor, node, nodePos)
}

/**
 * Custom hook that provides insert block functionality for the notifuse editor
 *
 * @example
 * ```tsx
 * function MyInsertBlockButton({ node, nodePos }) {
 *   const { isVisible, handleInsertBlock, canInsert } = useInsertBlock({ node, nodePos })
 *
 *   if (!isVisible) return null
 *
 *   return <button onClick={handleInsertBlock} disabled={!canInsert}>Insert Block</button>
 * }
 * ```
 */
export function useInsertBlock(config?: UseInsertBlockConfig) {
  const { editor: providedEditor, node, nodePos, trigger = '/', onTriggered } = config || {}

  const { editor } = useNotifuseEditor(providedEditor)
  const [isVisible, setIsVisible] = useState<boolean>(true)
  const canInsert = canInsertSlashCommand(editor, node, nodePos)

  useEffect(() => {
    if (!editor) return

    const handleSelectionUpdate = () => {
      setIsVisible(shouldShowButton({ editor, node, nodePos }))
    }

    handleSelectionUpdate()

    editor.on('selectionUpdate', handleSelectionUpdate)

    return () => {
      editor.off('selectionUpdate', handleSelectionUpdate)
    }
  }, [editor, node, nodePos])

  const handleInsertBlock = useCallback(() => {
    if (!editor) return false

    const success = insertSlashCommand(editor, trigger, node, nodePos)
    if (success) {
      onTriggered?.(trigger)
    }
    return success
  }, [editor, trigger, node, nodePos, onTriggered])

  return {
    isVisible,
    handleInsertBlock,
    canInsert,
    label: 'Insert block'
  }
}
