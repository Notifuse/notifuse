import { Extension } from '@tiptap/core'
import type { Editor } from '@tiptap/react'
import type { Node } from '@tiptap/pm/model'
import type { Transaction } from '@tiptap/pm/state'

/**
 * Configuration options for the Alignment extension
 */
export interface AlignmentConfig {
  /**
   * Node types that support alignment
   * @default ['paragraph', 'heading', 'blockquote']
   */
  supportedNodes: string[]
  /**
   * Attribute name for text alignment
   * @default 'textAlignAttr'
   */
  textAlignAttribute: string
  /**
   * Attribute name for vertical alignment
   * @default 'verticalAlignAttr'
   */
  verticalAlignAttribute: string
  /**
   * Valid text alignment values
   * @default ['left', 'center', 'right', 'justify']
   */
  textAlignOptions: string[]
  /**
   * Valid vertical alignment values
   * @default ['top', 'middle', 'bottom']
   */
  verticalAlignOptions: string[]
}

/**
 * TypeScript declarations for commands
 */
declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    alignment: {
      /**
       * Sets text alignment on selected nodes
       */
      setTextAlign: (align: string) => ReturnType
      /**
       * Removes text alignment from selected nodes
       */
      clearTextAlign: () => ReturnType
      /**
       * Toggles text alignment on selected nodes
       */
      toggleTextAlign: (align: string) => ReturnType
      /**
       * Sets vertical alignment on selected nodes
       */
      setVerticalAlign: (align: string) => ReturnType
      /**
       * Removes vertical alignment from selected nodes
       */
      clearVerticalAlign: () => ReturnType
      /**
       * Toggles vertical alignment on selected nodes
       */
      toggleVerticalAlign: (align: string) => ReturnType
      /**
       * Sets both text and vertical alignment at once
       */
      setBothAlignments: (textAlign?: string, verticalAlign?: string) => ReturnType
      /**
       * Removes all alignments from selected nodes
       */
      clearAllAlignments: () => ReturnType
    }
  }
}

/**
 * Finds nodes in the current selection that match allowed types
 */
function findAlignmentTargets(
  editor: Editor,
  allowedTypes: string[]
): Array<{ node: Node; pos: number }> {
  const results: Array<{ node: Node; pos: number }> = []
  const { selection, doc } = editor.state
  const { from, to } = selection
  const typeSet = new Set(allowedTypes)

  // Walk through selection range
  doc.nodesBetween(from, to, (node, pos) => {
    if (typeSet.has(node.type.name)) {
      results.push({ node, pos })
    }
  })

  // Fallback to parent nodes if nothing found
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
 * Applies alignment attribute to nodes
 */
function applyAlignmentToNodes(
  tr: Transaction,
  targets: Array<{ node: Node; pos: number }>,
  attrName: string,
  alignValue: string | null
): boolean {
  if (targets.length === 0) return false

  let hasChanges = false

  for (const { pos } of targets) {
    const currentNode = tr.doc.nodeAt(pos)
    if (!currentNode) continue

    const newAttrs = { ...currentNode.attrs }

    if (alignValue === null) {
      delete newAttrs[attrName]
    } else {
      newAttrs[attrName] = alignValue
    }

    if (currentNode.attrs[attrName] !== newAttrs[attrName]) {
      tr.setNodeMarkup(pos, null, newAttrs)
      hasChanges = true
    }
  }

  return hasChanges
}

/**
 * Alignment Extension
 *
 * Adds text and vertical alignment support to specific node types.
 * Alignments are stored as inline styles in HTML output.
 */
export const AlignmentExtension = Extension.create<AlignmentConfig>({
  name: 'alignment',

  addOptions() {
    return {
      supportedNodes: ['paragraph', 'heading', 'blockquote'],
      textAlignAttribute: 'textAlignAttr',
      verticalAlignAttribute: 'verticalAlignAttr',
      textAlignOptions: ['left', 'center', 'right', 'justify'],
      verticalAlignOptions: ['top', 'middle', 'bottom']
    }
  },

  addGlobalAttributes() {
    return [
      {
        types: this.options.supportedNodes,
        attributes: {
          // Text alignment attribute
          [this.options.textAlignAttribute]: {
            default: null,

            parseHTML: (el: HTMLElement) => {
              // Check inline style
              const styleAlign = el.style.getPropertyValue('text-align')
              if (styleAlign && this.options.textAlignOptions.includes(styleAlign)) {
                return styleAlign
              }

              // Check data attribute
              const dataAlign = el.getAttribute(`data-${this.options.textAlignAttribute}`)
              if (dataAlign && this.options.textAlignOptions.includes(dataAlign)) {
                return dataAlign
              }

              return null
            },

            renderHTML: (attrs) => {
              const alignValue = attrs[this.options.textAlignAttribute]
              if (!alignValue || !this.options.textAlignOptions.includes(alignValue)) {
                return {}
              }

              return {
                style: `text-align: ${alignValue};`
              }
            }
          },

          // Vertical alignment attribute
          [this.options.verticalAlignAttribute]: {
            default: null,

            parseHTML: (el: HTMLElement) => {
              // Check inline style
              const styleVAlign = el.style.getPropertyValue('vertical-align')
              if (styleVAlign && this.options.verticalAlignOptions.includes(styleVAlign)) {
                return styleVAlign
              }

              // Check data attribute
              const dataVAlign = el.getAttribute(`data-${this.options.verticalAlignAttribute}`)
              if (dataVAlign && this.options.verticalAlignOptions.includes(dataVAlign)) {
                return dataVAlign
              }

              return null
            },

            renderHTML: (attrs) => {
              const alignValue = attrs[this.options.verticalAlignAttribute]
              if (!alignValue || !this.options.verticalAlignOptions.includes(alignValue)) {
                return {}
              }

              return {
                style: `vertical-align: ${alignValue};`
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
       * Sets text alignment on selected nodes
       */
      setTextAlign:
        (align: string) =>
        ({ editor, tr }) => {
          if (!this.options.textAlignOptions.includes(align)) return false

          const targets = findAlignmentTargets(editor, this.options.supportedNodes)
          return applyAlignmentToNodes(tr, targets, this.options.textAlignAttribute, align)
        },

      /**
       * Removes text alignment from selected nodes
       */
      clearTextAlign:
        () =>
        ({ editor, tr }) => {
          const targets = findAlignmentTargets(editor, this.options.supportedNodes)
          return applyAlignmentToNodes(tr, targets, this.options.textAlignAttribute, null)
        },

      /**
       * Toggles text alignment on selected nodes
       */
      toggleTextAlign:
        (align: string) =>
        ({ editor, tr }) => {
          if (!this.options.textAlignOptions.includes(align)) return false

          const targets = findAlignmentTargets(editor, this.options.supportedNodes)
          if (targets.length === 0) return false

          // Check if all targets have this alignment
          const allHaveAlign = targets.every(
            ({ node }) => node.attrs[this.options.textAlignAttribute] === align
          )

          const newValue = allHaveAlign ? null : align
          return applyAlignmentToNodes(tr, targets, this.options.textAlignAttribute, newValue)
        },

      /**
       * Sets vertical alignment on selected nodes
       */
      setVerticalAlign:
        (align: string) =>
        ({ editor, tr }) => {
          if (!this.options.verticalAlignOptions.includes(align)) return false

          const targets = findAlignmentTargets(editor, this.options.supportedNodes)
          return applyAlignmentToNodes(tr, targets, this.options.verticalAlignAttribute, align)
        },

      /**
       * Removes vertical alignment from selected nodes
       */
      clearVerticalAlign:
        () =>
        ({ editor, tr }) => {
          const targets = findAlignmentTargets(editor, this.options.supportedNodes)
          return applyAlignmentToNodes(tr, targets, this.options.verticalAlignAttribute, null)
        },

      /**
       * Toggles vertical alignment on selected nodes
       */
      toggleVerticalAlign:
        (align: string) =>
        ({ editor, tr }) => {
          if (!this.options.verticalAlignOptions.includes(align)) return false

          const targets = findAlignmentTargets(editor, this.options.supportedNodes)
          if (targets.length === 0) return false

          // Check if all targets have this alignment
          const allHaveAlign = targets.every(
            ({ node }) => node.attrs[this.options.verticalAlignAttribute] === align
          )

          const newValue = allHaveAlign ? null : align
          return applyAlignmentToNodes(tr, targets, this.options.verticalAlignAttribute, newValue)
        },

      /**
       * Sets both text and vertical alignment at once
       */
      setBothAlignments:
        (textAlign?: string, verticalAlign?: string) =>
        ({ editor, tr }) => {
          const targets = findAlignmentTargets(editor, this.options.supportedNodes)
          if (targets.length === 0) return false

          let hasChanges = false

          for (const { pos } of targets) {
            const currentNode = tr.doc.nodeAt(pos)
            if (!currentNode) continue

            const newAttrs = { ...currentNode.attrs }

            if (textAlign && this.options.textAlignOptions.includes(textAlign)) {
              newAttrs[this.options.textAlignAttribute] = textAlign
              hasChanges = true
            }

            if (verticalAlign && this.options.verticalAlignOptions.includes(verticalAlign)) {
              newAttrs[this.options.verticalAlignAttribute] = verticalAlign
              hasChanges = true
            }

            if (hasChanges) {
              tr.setNodeMarkup(pos, null, newAttrs)
            }
          }

          return hasChanges
        },

      /**
       * Removes all alignments from selected nodes
       */
      clearAllAlignments:
        () =>
        ({ editor, tr }) => {
          const targets = findAlignmentTargets(editor, this.options.supportedNodes)
          if (targets.length === 0) return false

          let hasChanges = false

          for (const { pos } of targets) {
            const currentNode = tr.doc.nodeAt(pos)
            if (!currentNode) continue

            const hasTextAlign = currentNode.attrs[this.options.textAlignAttribute]
            const hasVerticalAlign = currentNode.attrs[this.options.verticalAlignAttribute]

            if (hasTextAlign || hasVerticalAlign) {
              const newAttrs = { ...currentNode.attrs }
              delete newAttrs[this.options.textAlignAttribute]
              delete newAttrs[this.options.verticalAlignAttribute]

              tr.setNodeMarkup(pos, null, newAttrs)
              hasChanges = true
            }
          }

          return hasChanges
        }
    }
  }
})
