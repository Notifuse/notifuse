import StarterKit from '@tiptap/starter-kit'
import Typography from '@tiptap/extension-typography'
import Underline from '@tiptap/extension-underline'
import Subscript from '@tiptap/extension-subscript'
import Superscript from '@tiptap/extension-superscript'
import { Node, Mark, InputRule, mergeAttributes } from '@tiptap/core'
import type { AnyExtension } from '@tiptap/core'
import type { Mark as ProseMirrorMark } from '@tiptap/pm/model'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import type { RawCommands } from '@tiptap/core'
import { TextStyleMark } from '../TiptapSchema'

// Custom Link extension that supports style attributes for email-friendly HTML
export const CustomLink = Mark.create({
  name: 'link',
  priority: 1010, // Higher priority to ensure it takes precedence
  keepOnSplit: false,
  exitable: true,

  addOptions() {
    return {
      HTMLAttributes: {},
      openOnClick: true,
      linkOnPaste: true,
      defaultProtocol: 'https',
      protocols: [],
      autolink: true,
      validate: undefined
    }
  },

  addAttributes() {
    return {
      href: {
        default: null,
        parseHTML: (element) => element.getAttribute('href'),
        renderHTML: (attributes) => {
          if (!attributes.href) return {}
          return { href: attributes.href }
        }
      },
      target: {
        default: null,
        parseHTML: (element) => element.getAttribute('target'),
        renderHTML: (attributes) => {
          if (!attributes.target) return {}
          return { target: attributes.target }
        }
      },
      rel: {
        default: null,
        parseHTML: (element) => element.getAttribute('rel'),
        renderHTML: (attributes) => {
          if (!attributes.rel) return {}
          return { rel: attributes.rel }
        }
      },
      class: {
        default: null,
        parseHTML: (element) => element.getAttribute('class'),
        renderHTML: (attributes) => {
          if (!attributes.class) return {}
          return { class: attributes.class }
        }
      },
      // Add style attributes for email compatibility
      color: {
        default: null,
        parseHTML: (element) => {
          try {
            const style = element.getAttribute('style')
            if (!style) return null
            const colorMatch = style.match(/color:\s*([^;]+)/i)
            return colorMatch ? colorMatch[1].trim() : null
          } catch {
            return null
          }
        }
      },
      backgroundColor: {
        default: null,
        parseHTML: (element) => {
          try {
            const style = element.getAttribute('style')
            if (!style) return null
            const bgMatch = style.match(/background-color:\s*([^;]+)/i)
            return bgMatch ? bgMatch[1].trim() : null
          } catch {
            return null
          }
        }
      },
      style: {
        default: null,
        parseHTML: (element) => element.getAttribute('style'),
        renderHTML: (attributes) => {
          try {
            const styles = []
            if (attributes.color) styles.push(`color: ${attributes.color}`)
            if (attributes.backgroundColor)
              styles.push(`background-color: ${attributes.backgroundColor}`)

            // Add any other styles that weren't parsed as individual attributes
            if (attributes.style) {
              const otherStyles = attributes.style
                .split(';')
                .map((s: string) => s.trim())
                .filter(
                  (s: string) => s && !s.startsWith('color:') && !s.startsWith('background-color:')
                )

              styles.push(...otherStyles)
            }

            if (styles.length === 0) return {}
            return { style: styles.join('; ') }
          } catch {
            // Fallback to original style if processing fails
            if (attributes.style) return { style: attributes.style }
            return {}
          }
        }
      }
    }
  },

  parseHTML() {
    return [
      {
        tag: 'a[href]',
        // Simplified and robust parsing
        getAttrs: (element) => {
          try {
            const el = element as HTMLElement
            const href = el.getAttribute('href')

            // Basic requirement: must have href
            if (!href) return false

            // Start with just the href - this ensures basic link recognition
            const attrs: Record<string, unknown> = { href }

            // Safely parse other attributes - if any fail, we still have the basic link
            try {
              const target = el.getAttribute('target')
              if (target) attrs.target = target
            } catch {
              /* ignore */
            }

            try {
              const rel = el.getAttribute('rel')
              if (rel) attrs.rel = rel
            } catch {
              /* ignore */
            }

            try {
              const className = el.getAttribute('class')
              if (className) attrs.class = className
            } catch {
              /* ignore */
            }

            // Parse style attributes safely - if this fails, we still have the basic link
            try {
              const style = el.getAttribute('style')
              if (style) {
                // Extract individual color properties
                const colorMatch = style.match(/color:\s*([^;]+)/i)
                const bgMatch = style.match(/background-color:\s*([^;]+)/i)

                if (colorMatch) {
                  attrs.color = colorMatch[1].trim()
                }
                if (bgMatch) {
                  attrs.backgroundColor = bgMatch[1].trim()
                }

                // Always store the complete style attribute
                attrs.style = style
              }
            } catch (error) {
              // Style parsing failed, but we still have the basic link
              console.warn('Style parsing failed for link, but link will still work:', error)
            }

            return attrs
          } catch (error) {
            // If everything fails, log error and return false
            console.error('Link parsing completely failed:', error)
            return false
          }
        }
      }
    ]
  },

  renderHTML({ HTMLAttributes }) {
    return ['a', mergeAttributes(this.options.HTMLAttributes, HTMLAttributes), 0]
  },

  addCommands(): Partial<RawCommands> {
    return {
      setLink:
        (attributes?: Record<string, unknown>) =>
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ({ chain, tr, state }: any): any => {
          if (!attributes) return false
          const { selection } = state
          const { from, to } = selection

          // First, collect any existing textStyle attributes in the selection
          const existingTextStyleAttrs: Record<string, unknown> = {}

          tr.doc.nodesBetween(from, to, (node: { isText?: boolean; marks?: { type: { name: string }; attrs: Record<string, unknown> }[] }) => {
            if (node.isText) {
              const textStyleMark = node.marks?.find((mark) => mark.type.name === 'textStyle')
              if (textStyleMark) {
                // Merge textStyle attributes
                Object.assign(existingTextStyleAttrs, textStyleMark.attrs)
              }
            }
          })

          // Merge existing textStyle attributes with new link attributes
          const mergedAttributes = { ...existingTextStyleAttrs, ...attributes }

          return chain()
            .setMark(this.name, mergedAttributes)
            .unsetMark('textStyle') // Remove textStyle marks to avoid nesting
            .setMeta('preventAutolink', true)
            .run()
        },

      toggleLink:
        (attributes?: Record<string, unknown>) =>
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ({ chain, tr, state }: any): any => {
          if (!attributes) return false
          const { selection } = state
          const { from, to } = selection

          // Check if we're toggling off an existing link
          const linkMark = this.editor.schema.marks[this.name]
          let hasLinkMark = false

          tr.doc.nodesBetween(from, to, (node: { marks?: readonly ProseMirrorMark[] }) => {
            if (node.marks && linkMark.isInSet(node.marks)) {
              hasLinkMark = true
              return false
            }
          })

          if (hasLinkMark) {
            // If removing link, preserve colors as textStyle
            const existingLinkAttrs: Record<string, unknown> = {}

            tr.doc.nodesBetween(from, to, (node: { marks?: readonly ProseMirrorMark[] }) => {
              const currentLinkMark = node.marks ? linkMark.isInSet(node.marks) : null
              if (currentLinkMark && typeof currentLinkMark === 'object' && 'attrs' in currentLinkMark) {
                // Extract color attributes from link
                const attrs = currentLinkMark.attrs as Record<string, unknown>
                if (attrs.color)
                  existingLinkAttrs.color = attrs.color
                if (attrs.backgroundColor)
                  existingLinkAttrs.backgroundColor = attrs.backgroundColor
              }
            })

            // Remove link and apply textStyle if there were colors
            if (Object.keys(existingLinkAttrs).length > 0) {
              return chain()
                .unsetMark(this.name)
                .setMark('textStyle', existingLinkAttrs)
                .setMeta('preventAutolink', true)
                .run()
            } else {
              return chain().unsetMark(this.name).setMeta('preventAutolink', true).run()
            }
          } else {
            // Adding link - collect existing textStyle attributes
            const existingTextStyleAttrs: Record<string, unknown> = {}

            tr.doc.nodesBetween(from, to, (node: { isText?: boolean; marks?: { type: { name: string }; attrs: Record<string, unknown> }[] }) => {
              if (node.isText) {
                const textStyleMark = node.marks?.find((mark) => mark.type.name === 'textStyle')
                if (textStyleMark) {
                  Object.assign(existingTextStyleAttrs, textStyleMark.attrs)
                }
              }
            })

            // Merge existing textStyle attributes with new link attributes
            const mergedAttributes = { ...existingTextStyleAttrs, ...attributes }

            return chain()
              .setMark(this.name, mergedAttributes)
              .unsetMark('textStyle') // Remove textStyle marks to avoid nesting
              .setMeta('preventAutolink', true)
              .run()
          }
        },

      unsetLink:
        () =>
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ({ chain, tr, state }: any): any => {
          const { selection } = state
          const { from, to } = selection
          const linkMark = this.editor.schema.marks[this.name]

          // Before removing link, preserve color attributes as textStyle
          const preservedAttrs: Record<string, unknown> = {}

          tr.doc.nodesBetween(from, to, (node: { marks?: readonly ProseMirrorMark[] }) => {
            const currentLinkMark = node.marks ? linkMark.isInSet(node.marks) : null
            if (currentLinkMark && typeof currentLinkMark === 'object' && 'attrs' in currentLinkMark) {
              const attrs = currentLinkMark.attrs as Record<string, unknown>
              if (attrs.color) preservedAttrs.color = attrs.color
              if (attrs.backgroundColor)
                preservedAttrs.backgroundColor = attrs.backgroundColor
            }
          })

          // Remove link and optionally preserve styles
          if (Object.keys(preservedAttrs).length > 0) {
            return chain()
              .unsetMark(this.name, { extendEmptyMarkRange: true })
              .setMark('textStyle', preservedAttrs)
              .setMeta('preventAutolink', true)
              .run()
          } else {
            return chain()
              .unsetMark(this.name, { extendEmptyMarkRange: true })
              .setMeta('preventAutolink', true)
              .run()
          }
        },

      // Custom command to update link styles
      updateLinkStyle:
        (attributes?: Record<string, unknown>) =>
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ({ tr, state }: any): any => {
          if (!attributes) return false
          const { selection } = state
          const markType = this.editor.schema.marks[this.name]

          if (!markType) return false

          const { from, to } = selection
          let linkFound = false

          tr.doc.nodesBetween(from, to, (node: { nodeSize?: number; marks?: readonly ProseMirrorMark[] }, pos: number) => {
            const linkMark = node.marks ? markType.isInSet(node.marks) : null
            if (linkMark && typeof linkMark === 'object' && 'attrs' in linkMark) {
              linkFound = true
              const start = pos
              const end = pos + (node.nodeSize || 0)

              // Update the link mark with new attributes
              const linkAttrs = linkMark.attrs as Record<string, unknown>
              const newAttrs = { ...linkAttrs, ...attributes }
              tr.removeMark(start, end, markType)
              tr.addMark(start, end, markType.create(newAttrs))
            }
            return !linkFound
          })

          return linkFound
        }
    }
  },

  addProseMirrorPlugins() {
    const plugins: Plugin[] = []

    // Plugin to prevent link clicks from navigating when openOnClick is false
    if (!this.options.openOnClick) {
      plugins.push(
        new Plugin({
          key: new PluginKey('handleClickLink'),
          props: {
            handleClick: (view, _pos, event) => {
              // Only handle if editor is editable
              if (!view.editable) {
                return false
              }

              // Check if the click target is an anchor element
              const target = event.target as HTMLElement
              const link = target.closest('a')

              if (link) {
                // Prevent the default link navigation
                event.preventDefault()
                return true
              }

              return false
            }
          }
        })
      )
    }

    return plugins
  }
})

// Custom inline document node for inline-only mode
export const InlineDocument = Node.create({
  name: 'inlineDoc',
  topNode: true,
  content: 'inline*',

  // Allow this node to contain any inline content including text
  group: 'block',

  // Make sure the node can be empty
  defining: false,

  parseHTML() {
    return [
      {
        tag: 'span[data-inline-doc]',
        // Preserve all attributes when parsing
        getAttrs: (element) => {
          if (element instanceof HTMLElement) {
            const attrs: Record<string, unknown> = {}
            // Copy all attributes except data-inline-doc
            Array.from(element.attributes).forEach((attr) => {
              if (attr.name !== 'data-inline-doc') {
                attrs[attr.name] = attr.value
              }
            })
            return attrs
          }
          return {}
        }
      },
      // Also parse divs with data-inline-doc for backward compatibility
      {
        tag: 'div[data-inline-doc]',
        getAttrs: (element) => {
          if (element instanceof HTMLElement) {
            const attrs: Record<string, unknown> = {}
            Array.from(element.attributes).forEach((attr) => {
              if (attr.name !== 'data-inline-doc') {
                attrs[attr.name] = attr.value
              }
            })
            return attrs
          }
          return {}
        }
      }
    ]
  },

  renderHTML({ HTMLAttributes }) {
    return ['span', { 'data-inline-doc': '', ...HTMLAttributes }, 0]
  },

  // Add attributes support
  addAttributes() {
    return {
      // Allow any HTML attributes to be preserved
      class: {
        default: null,
        parseHTML: (element) => element.getAttribute('class'),
        renderHTML: (attributes) => {
          if (!attributes.class) return {}
          return { class: attributes.class }
        }
      },
      style: {
        default: null,
        parseHTML: (element) => element.getAttribute('style'),
        renderHTML: (attributes) => {
          if (!attributes.style) return {}
          return { style: attributes.style }
        }
      },
      id: {
        default: null,
        parseHTML: (element) => element.getAttribute('id'),
        renderHTML: (attributes) => {
          if (!attributes.id) return {}
          return { id: attributes.id }
        }
      }
    }
  },

  // Prevent line breaks in inline mode
  addKeyboardShortcuts() {
    return {
      Enter: () => true, // Prevent Enter key from creating new lines
      'Shift-Enter': () => true // Prevent Shift+Enter as well
    }
  },

  // Handle parsing of content more robustly
  addInputRules() {
    return []
  },

  // Better paste handling for inline content
  addPasteRules() {
    return []
  }
})

/**
 * True when text ending at the cursor is inside an unclosed Liquid tag. Follows how
 * Liquid's tokenizer delimits markup in the common cases, whatever quotes it holds: a
 * `{%` tag runs to the next `%}`, a `{{` variable to the next `}`.
 */
export const isInsideLiquidMarkup = (textBefore: string): boolean => {
  let from = 0
  for (;;) {
    const variable = textBefore.indexOf('{{', from)
    const tag = textBefore.indexOf('{%', from)
    if (variable === -1 && tag === -1) return false
    const isTag = tag !== -1 && (variable === -1 || tag < variable)
    const closer = isTag ? '%}' : '}'
    const end = textBefore.indexOf(closer, (isTag ? tag : variable) + 2)
    if (end === -1) return true
    from = end + closer.length
  }
}

/**
 * Input rules that never rewrite Liquid markup. Smart punctuation and Markdown
 * shortcuts are wanted in prose but corrupt Liquid: a typed ' becomes ‘, != becomes
 * ≠ and ` _x_ ` becomes <em>x</em>, none of which Liquid parses (`default: ‘Guest’`
 * renders nothing). A rule is skipped when either end of the text it would rewrite
 * lies inside a tag: the cursor, or the start of a shortcut opened in one
 * (`{% assign _a = 1 %} b_`). A shortcut around a whole tag, like **{{ name }}**,
 * still applies. The backend normalizes the typographic forms too, for templates
 * saved before this and for other API clients.
 */
const skipInsideLiquid = (rules: InputRule[]): InputRule[] =>
  rules.map(
    (rule) =>
      new InputRule({
        find: rule.find,
        undoable: rule.undoable,
        handler: (props) => {
          const { doc } = props.state
          const blockStart = doc.resolve(props.range.from).start()
          const beforeMatch = doc.textBetween(blockStart, props.range.from, undefined, '\ufffc')
          return isInsideLiquidMarkup(beforeMatch) || isInsideLiquidMarkup(beforeMatch + props.match[0])
            ? null
            : rule.handler(props)
        }
      })
  )

const withLiquidSafeInputRules = (extension: AnyExtension): AnyExtension =>
  extension.extend({
    addInputRules() {
      return skipInsideLiquid(this.parent?.() ?? [])
    }
  })

// StarterKit's Markdown shortcuts (bold, italic, strike, code) and Typography's smart
// punctuation, both kept out of Liquid markup.
const LiquidSafeStarterKit = StarterKit.extend({
  addExtensions() {
    return (this.parent?.() ?? []).map(withLiquidSafeInputRules)
  }
})

const LiquidSafeTypography = withLiquidSafeInputRules(Typography)

// Extensions for rich text editor (full-featured)
export const createRichExtensions = () => [
  // Use StarterKit for basic functionality and commands
  LiquidSafeStarterKit.configure({
    // Disable headings for email compatibility, but enable lists
    heading: false,
    // Disable StarterKit's bundled Link/Underline so the explicit CustomLink
    // (email-friendly HTML) and Underline added below are the only ones registered.
    // Otherwise tiptap warns "Duplicate extension names found: ['link', 'underline']"
    // and the resolution is ambiguous.
    link: false,
    underline: false,
    bulletList: {
      HTMLAttributes: {}
    },
    orderedList: {
      HTMLAttributes: {}
    },
    listItem: {
      HTMLAttributes: {}
    },
    // Configure the built-in extensions to accept more HTML
    bold: {
      HTMLAttributes: {}
    },
    italic: {
      HTMLAttributes: {}
    },
    strike: {
      HTMLAttributes: {}
    },
    code: {
      HTMLAttributes: {}
    },
    paragraph: {
      HTMLAttributes: {}
    }
  }),
  // Add our comprehensive TextStyle mark for CSS support (higher priority)
  TextStyleMark,
  // Add additional formatting
  Underline.configure({
    HTMLAttributes: {}
  }),
  Subscript,
  Superscript,
  // Add typography improvements (never inside Liquid markup)
  LiquidSafeTypography,
  // Add custom Link extension with style support
  CustomLink.configure({
    HTMLAttributes: {
      class: 'editor-link'
    },
    openOnClick: false
  })
]

// Extensions for inline editor (inline-only)
export const createInlineExtensions = () => [
  // For inline mode: use only inline extensions, no paragraphs
  InlineDocument,
  // Add our comprehensive TextStyle mark for CSS support
  TextStyleMark,
  // Add basic formatting marks
  LiquidSafeStarterKit.configure({
    // Disable all block-level elements for inline mode
    document: false, // We'll use our custom InlineDocument instead
    paragraph: false,
    heading: false,
    bulletList: false,
    orderedList: false,
    listItem: false,
    blockquote: false,
    codeBlock: false,
    horizontalRule: false,
    // Use the explicit CustomLink + Underline added below instead of StarterKit's
    // bundled ones (prevents "Duplicate extension names" warnings and ambiguous resolution).
    link: false,
    underline: false,
    // Disable the trailing node. StarterKit enables it by default to keep a trailing
    // paragraph in block documents, but our inline schema has no paragraph. In that case
    // TrailingNode resolves its node to the inline schema's default type (hardBreak) and
    // appends a <br> after every transaction, so each typed character ends up on its own
    // line (see issue #352). Inline button text never needs a trailing node.
    trailingNode: false,
    // Keep only inline marks and commands
    bold: {
      HTMLAttributes: {}
    },
    italic: {
      HTMLAttributes: {}
    },
    strike: {
      HTMLAttributes: {}
    },
    code: {
      HTMLAttributes: {}
    }
  }),
  // Add additional formatting
  Underline.configure({
    HTMLAttributes: {}
  }),
  Subscript,
  Superscript,
  // Add typography improvements (never inside Liquid markup)
  LiquidSafeTypography,
  // Add custom Link extension with style support
  CustomLink.configure({
    HTMLAttributes: {
      class: 'editor-link'
    },
    openOnClick: false
  })
]
