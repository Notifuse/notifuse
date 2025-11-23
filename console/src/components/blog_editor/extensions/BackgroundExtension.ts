import { Extension } from '@tiptap/core'
import type { Editor } from '@tiptap/react'
import type { Node } from '@tiptap/pm/model'
import type { Transaction } from '@tiptap/pm/state'

/**
 * Configuration options for the Background extension
 */
export interface BackgroundConfig {
  /**
   * Node types that support background colors
   * @default ['paragraph', 'heading', 'blockquote', 'bulletList', 'orderedList']
   */
  supportedNodes: string[]
  /**
   * Name of the attribute to store background color
   * @default 'bgColor'
   */
  attributeName: string
  /**
   * CSS property name for background color
   * @default 'background-color'
   */
  cssProperty: string
}

/**
 * TypeScript declarations for commands
 */
declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    background: {
      /**
       * Sets background color on selected nodes
       */
      setBackground: (color: string) => ReturnType
      /**
       * Removes background color from selected nodes
       */
      clearBackground: () => ReturnType
      /**
       * Toggles background color on selected nodes
       * If all nodes have this exact color, removes it
       * Otherwise, applies the color to all nodes
       */
      toggleBackground: (color: string) => ReturnType
    }
  }
}

/**
 * Finds nodes in the current selection that match allowed types
 * Uses nodesBetween with fallback to parent node search
 */
function findTargetNodes(
  editor: Editor,
  allowedTypes: string[]
): Array<{ node: Node; pos: number }> {
  const results: Array<{ node: Node; pos: number }> = []
  const { selection, doc } = editor.state
  const { from, to } = selection
  const typeSet = new Set(allowedTypes)

  // Strategy 1: Walk through selection range
  doc.nodesBetween(from, to, (node, pos) => {
    if (typeSet.has(node.type.name)) {
      results.push({ node, pos })
    }
  })

  // Strategy 2: If nothing found, check parent nodes at cursor
  if (results.length === 0) {
    const $pos = selection.$from
    for (let depth = $pos.depth; depth > 0; depth--) {
      const node = $pos.node(depth)
      if (typeSet.has(node.type.name)) {
        const nodePos = $pos.before(depth)
        results.push({ node, pos: nodePos })
        break
      }
    }
  }

  return results
}

/**
 * Applies a color attribute to a list of nodes
 * Returns true if any changes were made
 */
function applyColorToNodes(
  tr: Transaction,
  targets: Array<{ node: Node; pos: number }>,
  attrName: string,
  colorValue: string | null
): boolean {
  if (targets.length === 0) return false

  let hasChanges = false

  for (const { pos } of targets) {
    // Read current node from transaction (it may have changed)
    const currentNode = tr.doc.nodeAt(pos)
    if (!currentNode) continue

    // Build new attributes
    const newAttrs = { ...currentNode.attrs }

    if (colorValue === null) {
      // Remove: delete the attribute key
      delete newAttrs[attrName]
    } else {
      // Set: assign new value
      newAttrs[attrName] = colorValue
    }

    // Only update if actually different
    if (currentNode.attrs[attrName] !== newAttrs[attrName]) {
      tr.setNodeMarkup(pos, null, newAttrs)
      hasChanges = true
    }
  }

  return hasChanges
}

/**
 * Background Extension
 *
 * Adds background color support to specific node types.
 * Colors are stored as inline styles in HTML output.
 */
export const BackgroundExtension = Extension.create<BackgroundConfig>({
  name: 'background',

  addOptions() {
    return {
      supportedNodes: ['paragraph', 'heading', 'blockquote', 'bulletList', 'orderedList'],
      attributeName: 'bgColor',
      cssProperty: 'background-color'
    }
  },

  addGlobalAttributes() {
    return [
      {
        types: this.options.supportedNodes,
        attributes: {
          [this.options.attributeName]: {
            default: null,

            // Parse background color from HTML
            parseHTML: (el: HTMLElement) => {
              // Priority 1: Inline style
              const inlineColor = el.style.getPropertyValue(this.options.cssProperty)
              if (inlineColor) return inlineColor

              // Priority 2: Data attribute fallback
              const dataAttr = el.getAttribute(`data-${this.options.attributeName}`)
              return dataAttr || null
            },

            // Render background color to HTML
            renderHTML: (attrs) => {
              const colorValue = attrs[this.options.attributeName]
              if (!colorValue) return {}

              return {
                style: `${this.options.cssProperty}: ${colorValue};`
              }
            }
          }
        }
      }
    ]
  },

  addCommands() {
    return {
      /**
       * Sets background color on selected nodes
       */
      setBackground:
        (color: string) =>
        ({ editor, tr }) => {
          const targets = findTargetNodes(editor, this.options.supportedNodes)
          return applyColorToNodes(tr, targets, this.options.attributeName, color)
        },

      /**
       * Removes background color from selected nodes
       */
      clearBackground:
        () =>
        ({ editor, tr }) => {
          const targets = findTargetNodes(editor, this.options.supportedNodes)
          return applyColorToNodes(tr, targets, this.options.attributeName, null)
        },

      /**
       * Toggles background color on selected nodes
       * If all nodes have this exact color, removes it
       * Otherwise, applies the color to all nodes
       */
      toggleBackground:
        (color: string) =>
        ({ editor, tr }) => {
          const targets = findTargetNodes(editor, this.options.supportedNodes)

          if (targets.length === 0) return false

          // Check if ALL targets have this exact color
          const allHaveThisColor = targets.every(
            ({ node }) => node.attrs[this.options.attributeName] === color
          )

          // Toggle: remove if all have it, otherwise apply
          const newValue = allHaveThisColor ? null : color

          return applyColorToNodes(tr, targets, this.options.attributeName, newValue)
        }
    }
  }
})
