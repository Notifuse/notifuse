import StarterKit from '@tiptap/starter-kit'
import Typography from '@tiptap/extension-typography'
import Underline from '@tiptap/extension-underline'
import Subscript from '@tiptap/extension-subscript'
import Superscript from '@tiptap/extension-superscript'
import { Extension, Node, Mark, InputRule, PasteRule, getMarkRange } from '@tiptap/core'
import type { AnyExtension, RawCommands } from '@tiptap/core'
import type { MarkType } from '@tiptap/pm/model'
import type { Editor as ReactEditor } from '@tiptap/react'
import { toggleFormatting, wrapLooseInlineContent, type Formatting } from './utils'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import {
  LINK_STYLE_PROPERTIES,
  PARAGRAPH_LIKE_SELECTOR,
  keepsTextStyle,
  TEXT_PROPERTIES,
  TextStyleMark,
  holdsBlocks,
  paragraphStyleOfElement
} from '../TiptapSchema'
import { parseHtmlInert, sanitizeEditorHtml } from '../../../../../utils/safeHtml'
import {
  dataAttributesOf,
  mergeStyles,
  omitStyleProperties,
  pickStyleProperties,
  readStyleAttribute,
  styleAttributes
} from './inlineStyle'

// setLink and unsetLink are typed by @tiptap/extension-link, which StarterKit brings
// in; this mark implements them for the editor's own link.

// The class older versions of the editor put on every link they saved.
const EDITOR_LINK_CLASS = 'editor-link'

const linkClassOf = (element: HTMLElement): string | null => {
  const classes = (element.getAttribute('class') ?? '')
    .split(/\s+/)
    .filter((name) => name && name !== EDITOR_LINK_CLASS)
  return classes.length > 0 ? classes.join(' ') : null
}

const optionalAttribute = (name: string, keepOnSplit = true) => ({
  default: null,
  keepOnSplit,
  parseHTML: (element: HTMLElement) => element.getAttribute(name),
  // An empty value is kept: alt="" says an image is decorative
  renderHTML: (attributes: Record<string, unknown>) =>
    attributes[name] != null ? { [name]: attributes[name] } : {}
})

// Link mark. Its attributes are saved as written: the target may hold Liquid
// (`{{ url }}`), and its style is the one place every mail client reads a link
// colour from.
export const CustomLink = Mark.create({
  name: 'link',
  priority: 1010,
  keepOnSplit: false,
  exitable: true,

  addOptions() {
    return {
      openOnClick: true
    }
  },

  addAttributes() {
    return {
      href: optionalAttribute('href'),
      target: optionalAttribute('target'),
      rel: optionalAttribute('rel'),
      id: optionalAttribute('id'),
      title: optionalAttribute('title'),
      class: {
        default: null,
        parseHTML: (element: HTMLElement) => linkClassOf(element),
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.class ? { class: attributes.class } : {}
      },
      style: {
        default: null,
        parseHTML: (element: HTMLElement) => readStyleAttribute(element),
        renderHTML: (attributes: Record<string, unknown>) => styleAttributes(attributes.style)
      }
    }
  },

  parseHTML() {
    return [{ tag: 'a[href]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['a', HTMLAttributes, 0]
  },

  addCommands(): Partial<RawCommands> {
    return {
      setLink:
        (attributes: { href: string; target?: string | null; rel?: string | null }) =>
        ({ chain, state }) => {
          const linkType = this.type as MarkType
          const textStyleType = state.schema.marks.textStyle as MarkType | undefined
          const { from, to, empty } = state.selection

          // The colours of the text move onto the link, where every mail client
          // reads them, when the whole text has the same ones: otherwise they stay
          // on each run, and the link stays one link.
          // A text's colour is its own, or that of the link it is already in.
          // Images and line breaks have none and do not take part.
          const colours = new Set<string | null>()
          // An existing link's style carries over to the whole new range, so that
          // extending a styled link keeps one link
          const linkStyles = new Set<string>()
          if (!empty) {
            state.doc.nodesBetween(from, to, (node) => {
              if (!node.isInline) return
              const link = linkType.isInSet(node.marks)
              const linkStyle = link?.attrs.style as string | null | undefined
              if (linkStyle) linkStyles.add(linkStyle)
              if (!node.isText) return
              const style = textStyleType?.isInSet(node.marks)?.attrs.style as string | null | undefined
              colours.add(
                pickStyleProperties(style, LINK_STYLE_PROPERTIES) ??
                  pickStyleProperties(linkStyle, LINK_STYLE_PROPERTIES)
              )
            })
          }
          const [moved] = colours.size === 1 ? [...colours] : [null]
          const [carriedStyle] = linkStyles.size === 1 ? [...linkStyles] : [undefined]

          return chain()
            .setMark(this.name, carriedStyle ? { ...attributes, style: carriedStyle } : attributes)
            .command(({ tr }) => {
              if (!moved || !textStyleType) return true
              tr.doc.nodesBetween(from, to, (node, pos) => {
                if (!node.isInline) return
                const start = Math.max(pos, from)
                const end = Math.min(pos + node.nodeSize, to)
                const link = linkType.isInSet(node.marks)
                const textStyle = textStyleType.isInSet(node.marks)
                if (link) {
                  tr.removeMark(start, end, linkType)
                  tr.addMark(
                    start,
                    end,
                    linkType.create({ ...link.attrs, style: mergeStyles(link.attrs.style as string | null, moved) })
                  )
                }
                if (textStyle) {
                  const rest = omitStyleProperties(textStyle.attrs.style as string | null, LINK_STYLE_PROPERTIES)
                  const attrs = { ...textStyle.attrs, style: rest }
                  tr.removeMark(start, end, textStyleType)
                  if (keepsTextStyle(attrs)) tr.addMark(start, end, textStyleType.create(attrs))
                }
              })
              return true
            })
            .run()
        },

      unsetLink:
        () =>
        ({ state, tr, dispatch }) => {
          const linkType = this.type as MarkType
          const textStyleType = state.schema.marks.textStyle as MarkType | undefined
          const { selection } = state
          let ranges = selection.ranges.map((range) => ({ from: range.$from.pos, to: range.$to.pos }))
          if (selection.empty) {
            const $pos = selection.$from
            const link = linkType.isInSet($pos.marks())
            const range = link ? getMarkRange($pos, linkType, link.attrs) : undefined
            if (!range) return false
            ranges = [range]
          }
          if (!dispatch) return true

          ranges.forEach(({ from, to }) => {
            state.doc.nodesBetween(from, to, (node, pos) => {
              if (!node.isInline) return
              const link = linkType.isInSet(node.marks)
              if (!link) return
              const start = Math.max(pos, from)
              const end = Math.min(pos + node.nodeSize, to)
              tr.removeMark(start, end, linkType)
              // An image keeps no colour; text keeps the link's
              if (!node.isText) return
              // Its text keeps how it looked, but not the link's underline or its absence
              const colours = omitStyleProperties(
                pickStyleProperties(link.attrs.style as string | null, TEXT_PROPERTIES),
                ['text-decoration']
              )
              if (colours && textStyleType) {
                const current = textStyleType.isInSet(node.marks)
                tr.removeMark(start, end, textStyleType)
                tr.addMark(
                  start,
                  end,
                  textStyleType.create({
                    ...(current?.attrs ?? {}),
                    style: mergeStyles(colours, current?.attrs.style as string | null)
                  })
                )
              }
            })
          })
          return true
        }
    }
  },

  addProseMirrorPlugins() {
    if (this.options.openOnClick) return []
    // Clicking a link while editing places the cursor instead of opening it
    return [
      new Plugin({
        key: new PluginKey('handleClickLink'),
        props: {
          handleClick: (view, _pos, event) => {
            if (!view.editable) return false
            const link = (event.target as HTMLElement | null)?.closest('a')
            if (!link) return false
            event.preventDefault()
            return true
          }
        }
      })
    ]
  }
})

// Bold, italic, underline and strike-through shortcuts toggle the formatting the
// way the toolbar does: also the CSS that draws it (toggleFormatting)
export const FormattingShortcuts = Extension.create({
  name: 'formattingShortcuts',
  priority: 1100,

  addKeyboardShortcuts() {
    const toggle = (formatting: Formatting) => () => {
      toggleFormatting(this.editor as unknown as ReactEditor, formatting)
      return true
    }
    return {
      'Mod-b': toggle('bold'),
      'Mod-B': toggle('bold'),
      'Mod-i': toggle('italic'),
      'Mod-I': toggle('italic'),
      'Mod-u': toggle('underline'),
      'Mod-U': toggle('underline'),
      'Mod-Shift-s': toggle('strike'),
      'Mod-Shift-S': toggle('strike')
    }
  }
})

// The style of paragraphs, headings, lists and quotes, kept as written: alignment, margins
// and the rest of what an imported or hand-written email puts on them.
export const BlockStyle = Extension.create({
  name: 'blockStyle',

  // An id names one element. A paragraph split in its middle (ProseMirror copies its
  // attributes) or pasted twice would repeat it: the later blocks lose it, with the
  // data-* attributes that came along.
  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey('uniqueBlockIds'),
        appendTransaction: (transactions, _oldState, newState) => {
          if (!transactions.some((transaction) => transaction.docChanged)) return null
          const seen = new Set<string>()
          const tr = newState.tr
          newState.doc.descendants((node, pos) => {
            const id = node.attrs?.id as string | null | undefined
            if (!node.isBlock || !id) return
            if (seen.has(id)) tr.setNodeMarkup(pos, undefined, { ...node.attrs, id: null, dataAttributes: null })
            else seen.add(id)
          })
          return tr.docChanged ? tr : null
        }
      })
    ]
  },

  addGlobalAttributes() {
    return [
      {
        types: ['paragraph', 'bulletList', 'orderedList', 'listItem', 'blockquote'],
        attributes: {
          // A split paragraph keeps its class; its id and data-* stay on the first half
          id: optionalAttribute('id', false),
          class: optionalAttribute('class'),
          dataAttributes: {
            default: null,
            keepOnSplit: false,
            parseHTML: (element: HTMLElement) => dataAttributesOf(element),
            renderHTML: (attributes: Record<string, unknown>) =>
              (attributes.dataAttributes as Record<string, string> | null) ?? {}
          }
        }
      },
      {
        types: ['paragraph'],
        attributes: {
          style: {
            default: null,
            parseHTML: (element: HTMLElement) => paragraphStyleOfElement(element),
            renderHTML: (attributes: Record<string, unknown>) => styleAttributes(attributes.style)
          }
        }
      },
      {
        types: ['heading', 'bulletList', 'orderedList', 'listItem', 'blockquote'],
        attributes: {
          style: {
            default: null,
            parseHTML: (element: HTMLElement) => readStyleAttribute(element),
            renderHTML: (attributes: Record<string, unknown>) => styleAttributes(attributes.style)
          }
        }
      }
    ]
  }
})

// An anchor without target (<a name="top"></a>) that links point to. It holds no
// text, so it is a node of its own rather than a link.
export const InlineAnchor = Node.create({
  name: 'anchor',
  inline: true,
  group: 'inline',
  atom: true,

  addAttributes() {
    return {
      name: optionalAttribute('name'),
      id: optionalAttribute('id')
    }
  },

  parseHTML() {
    // An anchor holding text keeps its text, which an atom node would drop
    const empty = (element: HTMLElement) => ((element.textContent ?? '').trim() || element.children.length ? false : null)
    return [
      { tag: 'a[name]:not([href])', getAttrs: empty },
      { tag: 'a[id]:not([href])', getAttrs: empty }
    ]
  },

  renderHTML({ HTMLAttributes }) {
    return ['a', HTMLAttributes]
  }
})

/**
 * A table, kept whole as its HTML: the editor shows it and saves it as written but
 * does not edit it (tables are edited in a Raw HTML block or in code mode). Its HTML
 * is sanitized when loaded, as the rest of the content is.
 */
export const HtmlTable = Node.create({
  name: 'htmlTable',
  group: 'block',
  atom: true,
  selectable: true,

  addAttributes() {
    return {
      html: { default: '', rendered: false }
    }
  },

  parseHTML() {
    return [
      {
        tag: 'table',
        priority: 70,
        getAttrs: (element: HTMLElement) => {
          // ProseMirror marks the first element it copies; that mark is not the author's
          const table = element.cloneNode(true) as HTMLElement
          table.removeAttribute('data-pm-slice')
          return { html: sanitizeEditorHtml(table.outerHTML) }
        }
      }
    ]
  },

  renderHTML({ node }) {
    return tableElement(node.attrs.html as string).cloneNode(true) as HTMLElement
  }
})

// Tables are rendered on every save: each one's HTML is parsed once
const parsedTables = new Map<string, Element>()

const tableElement = (html: string): Element => {
  let element = parsedTables.get(html)
  if (!element) {
    const holder = document.createElement('template')
    holder.innerHTML = html
    element = holder.content.firstElementChild ?? document.createElement('table')
    if (parsedTables.size > 200) parsedTables.clear()
    parsedTables.set(html, element)
  }
  return element
}

/**
 * Pasted HTML. What the editor copied itself keeps everything (ProseMirror marks it
 * with data-pm-slice). HTML from elsewhere (Word, Outlook, a web page, a chat) loses
 * its classes, ids and data-* attributes, which mean nothing in the email, and its
 * tables are unfolded into their cells' text, as editable paragraphs.
 */
export const preparePastedHtml = (html: string): string => {
  const root = parseHtmlInert(html)
  if (!root.querySelector('[data-pm-slice]')) {
    root.querySelectorAll('*').forEach((element) => {
      element.getAttributeNames().forEach((name) => {
        if (name === 'class' || name === 'id' || name.startsWith('data-')) element.removeAttribute(name)
      })
    })
    Array.from(root.querySelectorAll('table'))
      .reverse()
      .forEach((table) => {
        const cells = Array.from(table.querySelectorAll('td, th')).filter((cell) => cell.closest('table') === table)
        const unfolded = cells.map((cell) => {
          const block = root.ownerDocument.createElement('div')
          while (cell.firstChild) block.appendChild(cell.firstChild)
          return block
        })
        table.replaceWith(...unfolded)
      })
  }
  return wrapLooseInlineContent(root.innerHTML)
}

export const PasteCleanup = Extension.create({
  name: 'pasteCleanup',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: new PluginKey('pasteCleanup'),
        props: { transformPastedHTML: (html) => preparePastedHtml(html) }
      })
    ]
  }
})

// Images inside text, kept with their attributes. The toolbar inserts none: the
// node exists so that opening a text holding one does not delete it.
export const InlineImage = Node.create({
  name: 'image',
  inline: true,
  group: 'inline',
  atom: true,
  draggable: false,

  addAttributes() {
    return {
      src: optionalAttribute('src'),
      alt: optionalAttribute('alt'),
      title: optionalAttribute('title'),
      width: optionalAttribute('width'),
      height: optionalAttribute('height'),
      srcset: optionalAttribute('srcset'),
      id: optionalAttribute('id'),
      class: optionalAttribute('class'),
      border: optionalAttribute('border'),
      align: optionalAttribute('align'),
      style: {
        default: null,
        parseHTML: (element: HTMLElement) => readStyleAttribute(element),
        renderHTML: (attributes: Record<string, unknown>) => styleAttributes(attributes.style)
      }
    }
  },

  parseHTML() {
    return [{ tag: 'img[src]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['img', HTMLAttributes]
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

// The same for paste rules, which turn pasted `_x_` or `**x**` into formatting
const skipPasteInsideLiquid = (rules: PasteRule[]): PasteRule[] =>
  rules.map(
    (rule) =>
      new PasteRule({
        find: rule.find,
        handler: (props) => {
          const { doc } = props.state
          const blockStart = doc.resolve(props.range.from).start()
          const beforeMatch = doc.textBetween(blockStart, props.range.from, undefined, '\ufffc')
          // undefined skips this match only; null would cancel the whole paste's rule
          return isInsideLiquidMarkup(beforeMatch) || isInsideLiquidMarkup(beforeMatch + props.match[0])
            ? undefined
            : rule.handler(props)
        }
      })
  )

const withLiquidSafeInputRules = (extension: AnyExtension): AnyExtension =>
  extension.extend({
    addInputRules() {
      return skipInsideLiquid(this.parent?.() ?? [])
    },
    addPasteRules() {
      return skipPasteInsideLiquid(this.parent?.() ?? [])
    }
  })

// Formatting marks are made from their tags only. Their CSS rules (font-weight,
// font-style, text-decoration, vertical-align) would add a <strong> or <em> to
// text whose style already says it, which then could not be toggled off.
const FORMATTING_MARKS = ['bold', 'italic', 'strike', 'underline', 'subscript', 'superscript']

const withoutStyleRules = (extension: AnyExtension): AnyExtension =>
  !FORMATTING_MARKS.includes(extension.name)
    ? extension
    : extension.extend({
        parseHTML(this: { parent?: () => unknown }) {
          const rules = (this.parent?.() ?? []) as Array<Record<string, unknown>>
          return rules.filter((rule) => 'tag' in rule)
        }
      })

// StarterKit's Markdown shortcuts (bold, italic, strike, code) and Typography's smart
// punctuation, both kept out of Liquid markup.
// A <div> or <center> holding only text is loaded as a paragraph, so that
// its alignment and box are kept on it (paragraphStyleOfElement).
const withParagraphLikeElements = (extension: AnyExtension): AnyExtension =>
  extension.name === 'listItem'
    ? // A list item may open with a heading (<li><h3>Step 1</h3>…), as emails write them
      extension.extend({ content: '(paragraph | heading) block*' })
    : extension.name !== 'paragraph'
    ? extension
    : extension.extend({
        parseHTML() {
          return [
            { tag: 'p' },
            {
              tag: PARAGRAPH_LIKE_SELECTOR,
              getAttrs: (element: HTMLElement) => (holdsBlocks(element) ? false : null)
            }
          ]
        }
      })

const LiquidSafeStarterKit = StarterKit.extend({
  addExtensions() {
    return (this.parent?.() ?? [])
      .map(withLiquidSafeInputRules)
      .map(withParagraphLikeElements)
      .map(withoutStyleRules)
  }
})

const LiquidSafeTypography = withLiquidSafeInputRules(Typography)

const TagOnlyUnderline = withoutStyleRules(Underline)
const TagOnlySubscript = withoutStyleRules(Subscript)
const TagOnlySuperscript = withoutStyleRules(Superscript)

// Extensions for the rich text editor (Text blocks)
export const createRichExtensions = () => [
  LiquidSafeStarterKit.configure({
    heading: { levels: [1, 2, 3, 4, 5, 6] },
    // No empty paragraph after a heading that ends the text
    trailingNode: { notAfter: ['paragraph', 'heading'] },
    // The explicit CustomLink and Underline below are the only ones registered;
    // otherwise tiptap warns about duplicate extension names.
    link: false,
    underline: false
  }),
  TextStyleMark,
  FormattingShortcuts,
  BlockStyle,
  InlineImage,
  InlineAnchor,
  HtmlTable,
  PasteCleanup,
  TagOnlyUnderline,
  TagOnlySubscript,
  TagOnlySuperscript,
  // Typography improvements (never inside Liquid markup)
  LiquidSafeTypography,
  CustomLink.configure({ openOnClick: false })
]

// Extensions for the inline editor (button text)
export const createInlineExtensions = () => [
  InlineDocument,
  TextStyleMark,
  FormattingShortcuts,
  InlineImage,
  LiquidSafeStarterKit.configure({
    // Inline content only: our InlineDocument replaces the document node
    document: false,
    paragraph: false,
    heading: false,
    bulletList: false,
    orderedList: false,
    listItem: false,
    blockquote: false,
    codeBlock: false,
    horizontalRule: false,
    link: false,
    underline: false,
    // StarterKit's trailing node would resolve to the inline schema's default type
    // (hardBreak) and append a <br> after every transaction, putting each typed
    // character on its own line.
    trailingNode: false
  }),
  TagOnlyUnderline,
  TagOnlySubscript,
  TagOnlySuperscript,
  LiquidSafeTypography,
  CustomLink.configure({ openOnClick: false })
]
