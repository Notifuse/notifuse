import { CodeBlockLowlight } from '@tiptap/extension-code-block-lowlight'
import { mergeAttributes } from '@tiptap/core'
import { TextSelection } from '@tiptap/pm/state'
import { ReactNodeViewRenderer } from '@tiptap/react'
import { CodeBlockNodeView } from './CodeBlockNodeView'

/**
 * Extended CodeBlockLowlight with max-height support and scoped select all
 */
export const CodeBlockLowlightExtension = CodeBlockLowlight.extend({
  addNodeView() {
    return ReactNodeViewRenderer(CodeBlockNodeView)
  },
  addAttributes() {
    return {
      ...this.parent?.(),
      maxHeight: {
        default: 300,
        parseHTML: (element) => {
          const height = element.getAttribute('data-max-height')
          return height ? parseInt(height, 10) : 300
        },
        renderHTML: (attributes) => {
          if (!attributes.maxHeight) {
            return {}
          }
          return {
            'data-max-height': attributes.maxHeight,
            style: `max-height: ${attributes.maxHeight}px; overflow-y: auto;`
          }
        }
      },
      showCaption: {
        default: false,
        parseHTML: (element) => element.getAttribute('data-show-caption') === 'true',
        renderHTML: (attributes) => {
          if (!attributes.showCaption) {
            return {}
          }
          return {
            'data-show-caption': 'true'
          }
        }
      },
      caption: {
        default: '',
        parseHTML: (element) => element.getAttribute('data-caption') || '',
        renderHTML: (attributes) => {
          if (!attributes.caption) {
            return {}
          }
          return {
            'data-caption': attributes.caption
          }
        }
      }
    }
  },

  addKeyboardShortcuts() {
    return {
      ...this.parent?.(),
      // Cmd+A / Ctrl+A - Select all content within code block only
      'Mod-a': () => {
        const { state, view } = this.editor
        const { selection } = state
        const { $from, $to } = selection

        // Check if we're inside a code block
        let codeBlockDepth = -1
        for (let depth = $from.depth; depth > 0; depth--) {
          if ($from.node(depth).type.name === 'codeBlock') {
            codeBlockDepth = depth
            break
          }
        }

        // If not in a code block, use default behavior
        if (codeBlockDepth === -1) {
          return false
        }

        // Get the code block node and its position
        const codeBlock = $from.node(codeBlockDepth)
        const codeBlockStart = $from.start(codeBlockDepth)
        const codeBlockEnd = $from.end(codeBlockDepth)

        // Select all content within the code block
        const tr = state.tr.setSelection(
          TextSelection.create(state.doc, codeBlockStart, codeBlockEnd)
        )
        view.dispatch(tr)

        return true
      }
    }
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'pre',
      mergeAttributes(this.options.HTMLAttributes, HTMLAttributes),
      ['code', {}, 0]
    ]
  }
})

