import { CodeBlockLowlight } from '@tiptap/extension-code-block-lowlight'
import { mergeAttributes } from '@tiptap/core'
import { TextSelection } from '@tiptap/pm/state'
import { ReactNodeViewRenderer } from '@tiptap/react'
import { CodeBlockNodeView } from './CodeBlockNodeView'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'

/**
 * Escape HTML special characters
 */
function escapeHtml(text: string): string {
  const map: { [key: string]: string } = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#039;'
  }
  return String(text).replace(/[&<>"']/g, (m) => map[m])
}

interface HastNode {
  type: string
  value?: string
  tagName?: string
  properties?: Record<string, unknown>
  children?: HastNode[]
}

/**
 * Convert lowlight hast tree to HTML string
 * Recursively processes the hast tree nodes and converts them to HTML
 * Based on how @tiptap/extension-code-block-lowlight handles the hast tree
 */
function hastToHtml(node: HastNode): string {
  // Handle text nodes
  if (node.type === 'text') {
    return escapeHtml(node.value || '')
  }

  // Handle element nodes
  if (node.type === 'element') {
    const tag = node.tagName || 'span'
    const attrs: string[] = []

    // Build attributes string
    if (node.properties) {
      if (node.properties.className) {
        const classes = Array.isArray(node.properties.className)
          ? node.properties.className.join(' ')
          : String(node.properties.className)
        attrs.push(`class="${escapeHtml(classes)}"`)
      }
      // Add other properties if needed
      Object.keys(node.properties).forEach((key) => {
        if (key !== 'className' && node.properties && node.properties[key]) {
          attrs.push(`${key}="${escapeHtml(String(node.properties[key]))}"`)
        }
      })
    }

    const attrsStr = attrs.length > 0 ? ' ' + attrs.join(' ') : ''

    // Process children
    const children = node.children || []
    const childrenHtml = children.map((child: HastNode) => hastToHtml(child)).join('')

    return `<${tag}${attrsStr}>${childrenHtml}</${tag}>`
  }

  // Handle root nodes or unknown types - process children
  if (node.children) {
    return node.children.map((child: HastNode) => hastToHtml(child)).join('')
  }

  return ''
}

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
        const { $from } = selection

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

        // Get the code block position
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

  renderHTML({ HTMLAttributes, node }) {
    // Merge pre attributes (including custom attributes like maxHeight, caption)
    const preAttrs = mergeAttributes(this.options.HTMLAttributes, HTMLAttributes)

    // Build code element attributes with language class
    const language = node.attrs.language || 'plaintext'
    const codeAttrs: Record<string, string> = {}
    if (language) {
      codeAttrs.class = `language-${language} hljs`
    }

    // Return structure - syntax highlighting will be added via post-processing in getHTML()
    // This ensures the language class is preserved so post-processing can find and highlight it
    return ['pre', preAttrs, ['code', codeAttrs, 0]]
  },

  // Override toDOM to insert raw HTML with syntax highlighting
  // toDOM is used for both editor rendering AND HTML serialization (getHTML())
  toDOM(node: ProseMirrorNode) {
    const code = node.textContent || ''
    const language = node.attrs.language || 'plaintext'
    const lowlight = this.options.lowlight

    // Build pre element
    const pre = document.createElement('pre')

    // Add pre attributes from node attrs
    if (node.attrs.maxHeight) {
      pre.setAttribute('data-max-height', String(node.attrs.maxHeight))
      pre.setAttribute('style', `max-height: ${node.attrs.maxHeight}px; overflow-y: auto;`)
    }
    if (node.attrs.showCaption) {
      pre.setAttribute('data-show-caption', 'true')
    }
    if (node.attrs.caption) {
      pre.setAttribute('data-caption', node.attrs.caption)
    }

    // Build code element
    const codeEl = document.createElement('code')
    if (language) {
      codeEl.className = `language-${language} hljs`
    }

    // Generate highlighted HTML if lowlight is available
    if (lowlight && language && language !== 'plaintext') {
      try {
        const result = lowlight.highlight(language, code)
        // Convert hast tree to HTML
        // lowlight v3 uses .children, v1 used .value
        const highlightedHtml = result.children ? hastToHtml(result) : result.value
        // Insert raw HTML with syntax highlighting spans
        // This preserves the <span> elements with .hljs-* classes
        codeEl.innerHTML = highlightedHtml
      } catch (error) {
        // If highlighting fails, fall back to plain text
        console.warn(`Failed to highlight code block with language ${language}:`, error)
        codeEl.textContent = code
      }
    } else {
      // Plain text for unsupported languages
      codeEl.textContent = code
    }

    pre.appendChild(codeEl)
    return pre
  }
})
