import { Mark, getMarkRange } from '@tiptap/core'
import type { RawCommands } from '@tiptap/core'
import type { Mark as ProseMirrorMark, MarkType } from '@tiptap/pm/model'
import type { Transaction } from '@tiptap/pm/state'
import {
  getStyleProperty,
  mergeStyles,
  omitStyleProperties,
  pickStyleProperties,
  readStyleAttribute,
  dataAttributesOf,
  liquidBlocksOf,
  setStyleProperty,
  styleAttributes,
  stylePropertyNames
} from './shared/inlineStyle'

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    textStyle: {
      /**
       * Sets one CSS property on the selected text, or removes it when the value is
       * empty, leaving the text's other properties alone. Colours of linked text go
       * on the link itself, where every mail client reads them; with a cursor inside
       * a link they apply to the whole link.
       */
      setInlineStyle: (property: string, value: string | null) => ReturnType
    }
  }
}

// Properties a block element passes to its text. They are what a styled <div> or
// <td> keeps when its text is loaded as paragraphs.
export const TEXT_PROPERTIES = [
  'color',
  'background-color',
  'font',
  'font-family',
  'font-size',
  'font-style',
  'font-variant',
  'font-weight',
  'letter-spacing',
  'line-height',
  'text-decoration',
  'text-decoration-color',
  'text-decoration-line',
  'text-decoration-style',
  'text-shadow',
  'text-transform',
  'word-spacing'
] as const

// Properties that colour a link, which go on the <a> itself.
export const LINK_STYLE_PROPERTIES = ['color', 'background-color'] as const

// Elements whose style belongs to the node or mark that loads them: headings and
// paragraphs keep theirs on the tag, as written.
const ELEMENTS_WITH_THEIR_OWN_STYLE = new Set([
  'A',
  'BLOCKQUOTE',
  'BR',
  'H1',
  'H2',
  'H3',
  'H4',
  'H5',
  'H6',
  'IMG',
  'LI',
  'OL',
  'P',
  'UL'
])

// <font size="1"> to <font size="7">, as browsers draw them.
const FONT_SIZES = ['10px', '13px', '16px', '18px', '24px', '32px', '48px']

const fontElementStyle = (element: HTMLElement): string | null => {
  let style: string | null = null
  const color = element.getAttribute('color')?.trim()
  const face = element.getAttribute('face')?.trim()
  const sizeAttribute = (element.getAttribute('size') ?? '').trim()
  const parsedSize = Number.parseInt(sizeAttribute, 10)
  // "+1" and "-1" count from the default size, 3
  const size = /^[+-]/.test(sizeAttribute) ? 3 + parsedSize : parsedSize
  if (color) style = setStyleProperty(style, 'color', color)
  if (face) style = setStyleProperty(style, 'font-family', face)
  if (size >= 1) style = setStyleProperty(style, 'font-size', FONT_SIZES[Math.min(size, 7) - 1])
  return mergeStyles(style, readStyleAttribute(element))
}

/**
 * The text style an element gives the text it holds, or null. A span keeps its
 * whole style as written; another element passes only its text properties, its
 * box (margins, borders, widths) staying where the editor cannot hold it.
 */
export const textStyleOfElement = (element: HTMLElement): string | null => {
  const tag = element.tagName.toUpperCase()
  if (ELEMENTS_WITH_THEIR_OWN_STYLE.has(tag)) return null
  const style = readStyleAttribute(element)
  if (tag === 'SPAN') return style
  if (tag === 'FONT') return fontElementStyle(element)
  // A Liquid block may set text properties: it goes on the text, unless the element
  // becomes a paragraph, which keeps it
  const becomesParagraph = element.matches(PARAGRAPH_LIKE_SELECTOR) && !holdsBlocks(element)
  const textStyle = mergeStyles(
    pickStyleProperties(style, TEXT_PROPERTIES),
    becomesParagraph ? null : liquidBlocksOf(style)
  )
  return textStyle
}

/** The text properties an element declares in its own style. */
const declaredTextProperties = (element: HTMLElement): string[] =>
  stylePropertyNames(pickStyleProperties(readStyleAttribute(element), TEXT_PROPERTIES))

const hasItsOwnStyle = (element: HTMLElement) =>
  ELEMENTS_WITH_THEIR_OWN_STYLE.has(element.tagName.toUpperCase())

/**
 * The text style the text of an element gets, as the CSS cascade gives it: its
 * ancestors' text styles, outermost first, each overridden by the inner ones. A
 * paragraph or link that declares a property keeps it on itself, so the property
 * is left out of what its text inherits from further out.
 */
export const inheritedTextStyleOfElement = (element: HTMLElement): string | null => {
  const chain: HTMLElement[] = []
  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    chain.unshift(current)
  }
  return chain.reduce<string | null>(
    (merged, current) =>
      hasItsOwnStyle(current)
        ? omitStyleProperties(merged, declaredTextProperties(current))
        : mergeStyles(merged, textStyleOfElement(current)),
    null
  )
}

/**
 * The text style a paragraph or link gives its text when it declares text
 * properties under a styled ancestor: what the ancestors give, without what it
 * declares, so that its own declarations show. When nothing else is inherited, its
 * own text properties, which still override the ancestors' mark on its text.
 */
const restoredTextStyle = (element: HTMLElement): string | null => {
  const declared = declaredTextProperties(element)
  const parent = element.parentElement
  if (declared.length === 0 || !parent) return null
  const inherited = inheritedTextStyleOfElement(parent)
  if (!inherited) return null
  return (
    omitStyleProperties(inherited, declared) ??
    pickStyleProperties(readStyleAttribute(element), TEXT_PROPERTIES)
  )
}

/** The textStyle mark's style for an element, null when it gives none. */
const textStyleAttributeOf = (element: HTMLElement): string | null =>
  hasItsOwnStyle(element) ? restoredTextStyle(element) : inheritedTextStyleOfElement(element)

// Formatting elements whose own style cancels what they mean, such as the
// <strong style="font-weight: normal"> that editors and word processors write.
const CANCELLING_STYLES: Record<string, (style: string | null) => boolean> = {
  EM: (style) => getStyleProperty(style, 'font-style') === 'normal',
  I: (style) => getStyleProperty(style, 'font-style') === 'normal',
  STRONG: (style) => isLightWeight(getStyleProperty(style, 'font-weight')),
  B: (style) => isLightWeight(getStyleProperty(style, 'font-weight')),
  U: (style) => hasNoDecoration(style),
  S: (style) => hasNoDecoration(style),
  STRIKE: (style) => hasNoDecoration(style),
  DEL: (style) => hasNoDecoration(style)
}

function isLightWeight(weight: string): boolean {
  return weight === 'normal' || weight === 'lighter' || (/^\d+$/.test(weight) && Number(weight) < 600)
}

function hasNoDecoration(style: string | null): boolean {
  return (
    getStyleProperty(style, 'text-decoration') === 'none' ||
    getStyleProperty(style, 'text-decoration-line') === 'none'
  )
}

const clearing = (...names: string[]) => (mark: ProseMirrorMark) => names.includes(mark.type.name)

const CANCELLING_STYLE_RULES = [
  ...['normal', 'lighter', '100', '200', '300', '400', '500'].map((weight) => ({
    style: `font-weight=${weight}`,
    clearMark: clearing('bold')
  })),
  { style: 'font-style=normal', clearMark: clearing('italic') },
  { style: 'text-decoration=none', clearMark: clearing('underline', 'strike') },
  { style: 'text-decoration-line=none', clearMark: clearing('underline', 'strike') }
]

const cancelsItsFormatting = (element: HTMLElement): boolean =>
  CANCELLING_STYLES[element.tagName.toUpperCase()]?.(readStyleAttribute(element)) ?? false

// Elements loaded as paragraphs when they hold no block of their own
export const PARAGRAPH_LIKE_SELECTOR = 'div, center'

// Elements that are blocks of their own in a Text block's content
export const BLOCK_TAGS = new Set([
  'ADDRESS', 'ARTICLE', 'ASIDE', 'BLOCKQUOTE', 'CENTER', 'DIV', 'DL', 'FIELDSET', 'FIGURE',
  'FOOTER', 'FORM', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'HEADER', 'HR', 'LI', 'MAIN', 'NAV',
  'OL', 'P', 'PRE', 'SECTION', 'TABLE', 'UL'
])

const NESTED_BLOCK_SELECTOR = Array.from(BLOCK_TAGS, (tag) => tag.toLowerCase()).join(', ')

/** True when an element holds a block, so its content is more than one paragraph. */
export const holdsBlocks = (element: HTMLElement): boolean =>
  element.querySelector(NESTED_BLOCK_SELECTOR) !== null

/**
 * The style a paragraph keeps from the element it is loaded from. A <p> keeps its
 * whole style; a <div>, <center> or heading keeps its box and alignment, its text
 * properties going to the text (textStyleOfElement).
 */
export const paragraphStyleOfElement = (element: HTMLElement): string | null => {
  const style = readStyleAttribute(element)
  const own = element.tagName.toUpperCase() === 'P' ? style : omitStyleProperties(style, TEXT_PROPERTIES)
  if (getStyleProperty(own, 'text-align')) return own
  const alignment = alignmentOf(element) ?? inheritedAlignment(element)
  return alignment ? setStyleProperty(own, 'text-align', alignment) : own
}

// The alignment an element sets for the text it holds
function alignmentOf(element: HTMLElement): string | null {
  if (element.tagName.toUpperCase() === 'CENTER') return 'center'
  return (
    getStyleProperty(readStyleAttribute(element), 'text-align') ||
    element.getAttribute('align')?.trim().toLowerCase() ||
    null
  )
}

/**
 * The alignment a paragraph inherits from wrappers that load as nothing of their
 * own (a <div> or <center> holding paragraphs, a table cell): CSS passes
 * text-align down, so the paragraph keeps it. A list or quote keeps its own.
 */
function inheritedAlignment(element: HTMLElement): string | null {
  for (let current = element.parentElement; current; current = current.parentElement) {
    if (hasItsOwnStyle(current)) return null
    const alignment = alignmentOf(current)
    if (alignment) return alignment
  }
  return null
}

const isSpan = (element: HTMLElement) => element.tagName.toUpperCase() === 'SPAN'

// The spans an element's text sits in, outermost first, the element included
const spansAround = (element: HTMLElement): HTMLElement[] => {
  const spans: HTMLElement[] = []
  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    if (isSpan(current)) spans.unshift(current)
  }
  return spans
}

// Text in nested spans gets one mark, the innermost's: it carries the classes and
// data-* of every span around it. An id stays on its own span only.
const inheritedClass = (element: HTMLElement): string | null => {
  const names = spansAround(element).flatMap((span) => (span.getAttribute('class') ?? '').split(/\s+/))
  const unique = Array.from(new Set(names.filter(Boolean)))
  return unique.length > 0 ? unique.join(' ') : null
}

const inheritedDataAttributes = (element: HTMLElement): Record<string, string> | null => {
  const merged = Object.assign({}, ...spansAround(element).map((span) => dataAttributesOf(span) ?? {}))
  return Object.keys(merged).length > 0 ? merged : null
}

const spanAttribute = (name: string, read: (element: HTMLElement) => string | null) => ({
  default: null,
  keepOnSplit: name !== 'id',
  parseHTML: read,
  renderHTML: (attributes: Record<string, unknown>) => (attributes[name] ? { [name]: attributes[name] } : {})
})

// A text style still worth a mark: a style, or a span's own attributes
export const keepsTextStyle = (attrs: Record<string, unknown>) =>
  !!(attrs.style || attrs.class || attrs.id || attrs.dataAttributes)

/** Sets or removes one property on the mark of `type` over [from, to) of `node`. */
const restyleMark = (
  tr: Transaction,
  type: MarkType,
  marks: readonly ProseMirrorMark[],
  from: number,
  to: number,
  property: string,
  value: string | null
) => {
  const current = type.isInSet(marks)
  const style = setStyleProperty(current?.attrs.style as string | null, property, value)
  if (current) tr.removeMark(from, to, type)
  const attrs = { ...(current?.attrs ?? {}), style }
  if ((current && type.name !== 'textStyle') || keepsTextStyle(attrs)) {
    tr.addMark(from, to, type.create(attrs))
  }
}

// Text style mark: the inline CSS of a run of text, kept verbatim in one attribute.
export const TextStyleMark = Mark.create({
  name: 'textStyle',
  priority: 1001,

  addOptions() {
    return {
      HTMLAttributes: {}
    }
  },

  addAttributes() {
    return {
      style: {
        default: null,
        parseHTML: (element) => textStyleAttributeOf(element),
        renderHTML: (attributes) => styleAttributes(attributes.style)
      },
      // A span's own attributes, kept as written; other elements pass only a style
      class: spanAttribute('class', inheritedClass),
      id: spanAttribute('id', (element) => (isSpan(element) ? element.getAttribute('id') || null : null)),
      dataAttributes: {
        default: null,
        parseHTML: (element: HTMLElement) => inheritedDataAttributes(element),
        renderHTML: (attributes: Record<string, unknown>) =>
          (attributes.dataAttributes as Record<string, string> | null) ?? {}
      }
    }
  },

  parseHTML() {
    return [
      // A style that cancels bold, italic, underline or strike-through removes that
      // formatting from the text inside, as the browser shows it.
      ...CANCELLING_STYLE_RULES,
      // A span means nothing but its style and its own attributes.
      {
        tag: 'span',
        getAttrs: (element) => {
          const el = element as HTMLElement
          return textStyleOfElement(el) || el.id || el.className || dataAttributesOf(el) ? null : false
        }
      },
      // A formatting element whose style cancels its meaning is only that style.
      {
        tag: Object.keys(CANCELLING_STYLES).join(', ').toLowerCase(),
        getAttrs: (element: HTMLElement) => (cancelsItsFormatting(element) ? null : false)
      },
      // Any other element passes its text style on and keeps its own meaning: a
      // styled <strong> stays bold, a styled <div> still starts a paragraph. A
      // paragraph or link that declares a text property replaces what its text
      // inherits. It runs before the link and node rules, which consume elements.
      {
        tag: '*',
        consuming: false,
        priority: 60,
        getAttrs: (element) => {
          const el = element as HTMLElement
          if (el.tagName.toUpperCase() === 'SPAN') return false
          return textStyleAttributeOf(el) ? null : false
        }
      }
    ]
  },

  renderHTML({ HTMLAttributes }) {
    return ['span', HTMLAttributes, 0]
  },

  addCommands(): Partial<RawCommands> {
    return {
      setInlineStyle:
        (property: string, value: string | null) =>
        ({ state, tr, dispatch }) => {
          const name = property.toLowerCase()
          const textStyleType = this.type
          const linkType = state.schema.marks.link as MarkType | undefined
          const goesOnLink = (LINK_STYLE_PROPERTIES as readonly string[]).includes(name)
          const { selection } = state
          let ranges = selection.ranges.map((range) => ({ from: range.$from.pos, to: range.$to.pos }))

          if (selection.empty) {
            const $pos = selection.$from
            const link = linkType?.isInSet($pos.marks())
            const linkRange = link && goesOnLink ? getMarkRange($pos, link.type, link.attrs) : undefined
            if (linkRange) {
              ranges = [linkRange]
            } else {
              // Applies to the text typed next
              if (!dispatch) return true
              const marks = state.storedMarks ?? $pos.marks()
              const current = textStyleType.isInSet(marks)
              const style = setStyleProperty(current?.attrs.style as string | null, name, value)
              const attrs = { ...(current?.attrs ?? {}), style }
              tr.removeStoredMark(textStyleType)
              if (keepsTextStyle(attrs)) tr.addStoredMark(textStyleType.create(attrs))
              return true
            }
          }

          if (!dispatch) return true
          ranges.forEach(({ from, to }) => {
            state.doc.nodesBetween(from, to, (node, pos) => {
              // Line breaks and images carry the marks of the text around them
              if (!node.isInline) return
              const start = Math.max(pos, from)
              const end = Math.min(pos + node.nodeSize, to)
              if (start >= end) return
              const link = linkType?.isInSet(node.marks)
              // Part of a link takes the colour on its text: styling the link
              // there would split it into several links
              const linkRange = link && linkType ? getMarkRange(state.doc.resolve(start), linkType, link.attrs) : undefined
              const coversLink = !!linkRange && from <= linkRange.from && to >= linkRange.to
              if (link && linkType && (goesOnLink || !value) && coversLink) {
                restyleMark(tr, linkType, node.marks, start, end, name, value)
                // A colour left on the text inside would hide the link's
                restyleMark(tr, textStyleType, node.marks, start, end, name, null)
              } else {
                restyleMark(tr, textStyleType, node.marks, start, end, name, value)
              }
            })
          })
          // Clearing also clears the property where a paragraph declares it
          if (!value) {
            ranges.forEach(({ from, to }) => {
              state.doc.nodesBetween(from, to, (node, pos) => {
                if (!node.isTextblock) return
                const style = node.attrs.style as string | null | undefined
                const declared = getStyleProperty(style, name)
                if (!declared) return
                tr.setNodeMarkup(pos, undefined, { ...node.attrs, style: setStyleProperty(style, name, null) })
                // The paragraph's text outside the selection keeps the property
                const contentStart = pos + 1
                const contentEnd = pos + node.nodeSize - 1
                const kept = [
                  [contentStart, Math.min(from, contentEnd)],
                  [Math.max(to, contentStart), contentEnd]
                ]
                kept.forEach(([keepFrom, keepTo]) => {
                  if (keepFrom >= keepTo) return
                  tr.doc.nodesBetween(keepFrom, keepTo, (child, childPos) => {
                    if (!child.isInline) return
                    const start = Math.max(childPos, keepFrom)
                    const end = Math.min(childPos + child.nodeSize, keepTo)
                    const own = textStyleType.isInSet(child.marks)?.attrs.style as string | null
                    if (start >= end || getStyleProperty(own, name)) return
                    restyleMark(tr, textStyleType, child.marks, start, end, name, declared)
                  })
                })
              })
            })
          }
          return true
        }
    }
  }
})

/**
 * The value of `property` where the selection starts, as the email shows it: from
 * the text's own style (inside a link), then the link's, then the paragraph's.
 */
export const inlineStyleAt = (
  marks: readonly ProseMirrorMark[],
  property: string,
  blockStyle?: string | null
): string => {
  const name = property.toLowerCase()
  const textStyle = marks.find((mark) => mark.type.name === 'textStyle')
  const fromText = getStyleProperty(textStyle?.attrs.style as string | null, name)
  if (fromText) return fromText
  const link = marks.find((mark) => mark.type.name === 'link')
  const fromLink = getStyleProperty(link?.attrs.style as string | null, name)
  if (fromLink) return fromLink
  return getStyleProperty(blockStyle, name)
}
