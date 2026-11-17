import { getMarkRange, type Editor } from '@tiptap/react'
import { DOMSerializer, type Mark, type Node } from '@tiptap/pm/model'
import { parseHtmlInert, sanitizeEditorHtml, textOnlyHtml } from '../../../../../utils/safeHtml'
import { BLOCK_TAGS, LINK_STYLE_PROPERTIES, inlineStyleAt, keepsTextStyle } from '../TiptapSchema'
import {
  RAW_STYLE_ATTRIBUTE,
  dataAttributesOf,
  getStyleProperty,
  omitStyleProperties,
  pickStyleProperties
} from './inlineStyle'
import type { LinkType } from './types'


/**
 * The editor's content as HTML, styles as written. Use it instead of getHTML(),
 * whose serializer rewrites every style through the CSSOM (see RAW_STYLE_ATTRIBUTE).
 */
// Documents are immutable: each one's HTML is computed once, however often the
// editor and its parent ask for it while typing.
const serializedDocuments = new WeakMap<Node, string>()
let serializationDocument: Document | undefined

export const getEditorHTML = (editor: Pick<Editor, 'schema' | 'state'>): string => {
  const { doc } = editor.state
  const cached = serializedDocuments.get(doc)
  if (cached !== undefined) return cached

  serializationDocument ??= document.implementation.createHTMLDocument()
  const container = serializationDocument.createElement('div')
  container.appendChild(
    DOMSerializer.fromSchema(editor.schema).serializeFragment(doc.content, { document: serializationDocument })
  )
  container.querySelectorAll(`[${RAW_STYLE_ATTRIBUTE}]`).forEach((element) => {
    element.setAttribute('style', element.getAttribute(RAW_STYLE_ATTRIBUTE) ?? '')
    element.removeAttribute(RAW_STYLE_ATTRIBUTE)
  })
  const html = container.innerHTML
  serializedDocuments.set(doc, html)
  return html
}

// Helper function to strip plain spans (spans without style attributes) while preserving inner content
export const stripPlainSpans = (htmlContent: string): string => {
  if (!htmlContent || htmlContent.trim() === '') {
    return htmlContent
  }

  try {
    // Use DOM parsing for accurate manipulation
    const tempDiv = parseHtmlInert(htmlContent)
    stripPlainSpansIn(tempDiv)
    return tempDiv.innerHTML
  } catch (error) {
    console.error('Error during plain span stripping:', error)
    return htmlContent
  }
}

// Unwraps, in place, the spans that carry nothing of their own
const stripPlainSpansIn = (tempDiv: Element) => {
    // Recursively process all span elements
    const processSpans = (element: Element) => {
      const spans = Array.from(element.querySelectorAll('span'))

      // Process in reverse order to handle nested spans correctly
      spans.reverse().forEach((span) => {
        // Check if this span has any meaningful attributes
        // We consider style, class with style-related content, or data attributes as meaningful
        const hasStyle = span.hasAttribute('style') && span.getAttribute('style')?.trim()
        const hasDataInlineDoc = span.hasAttribute('data-inline-doc')
        const hasClass = span.hasAttribute('class') && span.getAttribute('class')?.trim()
        // An id or data-* attributes the author wrote are kept too
        const hasOwnAttributes = !!span.id || dataAttributesOf(span as HTMLElement) !== null

        // Strip the span if it has no styling attributes
        if (!hasStyle && !hasDataInlineDoc && !hasClass && !hasOwnAttributes) {
          // Create a document fragment with the span's children
          const fragment = tempDiv.ownerDocument.createDocumentFragment()
          while (span.firstChild) {
            fragment.appendChild(span.firstChild)
          }

          // Replace the span with its contents
          span.parentNode?.replaceChild(fragment, span)
        }
      })
    }

    processSpans(tempDiv)
}

// Helper function to convert block-level HTML to inline spans
export const convertBlockToInline = (htmlContent: string): string => {
  if (!htmlContent) {
    return htmlContent
  }

  try {
    // Use DOM parsing for accurate conversion
    const tempDiv = parseHtmlInert(htmlContent)

    // List of block-level tags to convert to spans
    const blockTags = [
      'p',
      'div',
      'h1',
      'h2',
      'h3',
      'h4',
      'h5',
      'h6',
      'section',
      'article',
      'header',
      'footer',
      'main',
      'aside',
      'blockquote',
      'pre'
    ]

    // Blocks already turned into spans: a block followed by one still needs a space
    const convertedBlocks = new WeakSet<Element>()

    blockTags.forEach((tag) => {
      const elements = tempDiv.querySelectorAll(tag)

      // Convert to array and reverse to avoid issues with live NodeList
      Array.from(elements)
        .reverse()
        .forEach((element) => {
          // Create a new span element
          const span = tempDiv.ownerDocument.createElement('span')

          // Copy all attributes (including styles)
          Array.from(element.attributes).forEach((attr) => {
            span.setAttribute(attr.name, attr.value)
          })

          // Copy innerHTML
          span.innerHTML = element.innerHTML

          // If this element has siblings that will also be converted,
          // add a space after it to prevent text concatenation
          const nextSibling = element.nextElementSibling
          if (
            nextSibling &&
            (convertedBlocks.has(nextSibling) || blockTags.includes(nextSibling.tagName.toLowerCase()))
          ) {
            span.innerHTML += ' '
          }

          // Replace the original element
          element.parentNode?.replaceChild(span, element)
          convertedBlocks.add(span)
        })
    })

    // Clean up multiple consecutive spaces and normalize whitespace
    let converted = tempDiv.innerHTML
    converted = converted.replace(/\s+/g, ' ').trim()

    return converted
  } catch (error) {
    console.error('Error during block-to-inline conversion:', error)
    return htmlContent
  }
}

// Helper function to process content for inline mode
export const processInlineContent = (htmlContent: string): string => {
  // Handle empty content
  if (!htmlContent || htmlContent.trim() === '') {
    return ''
  }

  try {
    // Use DOM parsing for more accurate content extraction
    const tempDiv = parseHtmlInert(htmlContent)

    let processed = htmlContent

    // Find and extract content from inline document wrapper (both span and div)
    const inlineDocElement = tempDiv.querySelector('[data-inline-doc]')

    if (inlineDocElement) {
      // Get the innerHTML, which preserves all inner markup including marks
      processed = inlineDocElement.innerHTML
    } else {
      // If no wrapper found, check if we have block-level elements to convert
      const blockSelector =
        'p, div, h1, h2, h3, h4, h5, h6, section, article, header, footer, main, aside, blockquote, pre'
      const hasBlockElements = tempDiv.querySelector(blockSelector)

      if (hasBlockElements) {
        // Convert block elements to inline before processing
        processed = convertBlockToInline(htmlContent)
      } else {
        // Just remove paragraph tags if they exist
        processed = processed.replace(/<p[^>]*>/g, '').replace(/<\/p>/g, '')
      }
    }

    // Clean up any empty or whitespace-only content
    processed = processed.trim()

    // If content is still empty after processing, return empty string
    if (processed === '' || processed === '<br>' || processed === '<br/>') {
      return ''
    }

    return processed
  } catch (error) {
    console.error('Error during inline content processing:', error)
    return htmlContent.trim()
  }
}

// Helper function to prepare content for inline editor. The content is
// sanitized first: it is saved by members and shown in the console.
export const prepareInlineContent = (rawContent: string): string => {
  if (!rawContent) {
    return rawContent
  }
  const content = sanitizeEditorHtml(rawContent)

  try {
    // First, strip plain spans to avoid TipTap errors
    let processedContent = stripPlainSpans(content)

    // Then, convert block-level tags to inline spans
    processedContent = convertBlockToInline(processedContent)

    // For inline mode, wrap the content in our custom inline document
    // if it's not already wrapped
    const hasWrapper = processedContent.includes('data-inline-doc')

    if (!hasWrapper) {
      return `<span data-inline-doc="">${processedContent}</span>`
    }

    return processedContent
  } catch (error) {
    console.error('Error during content preparation:', error)
    return content
  }
}

const isLooseInline = (node: ChildNode) =>
  node.nodeType === 3
    ? /[^ \t\r\n\f]/.test(node.nodeValue ?? '')
    : node.nodeType === 1 && !BLOCK_TAGS.has((node as Element).tagName)

/**
 * Gives each run of loose inline content of a wrapper that also holds blocks a <p>
 * of its own. The parser would otherwise make that paragraph itself, without the
 * wrapper's alignment, which paragraphs read from their ancestors.
 */
export const wrapLooseInlineContent = (htmlContent: string): string => {
  if (!htmlContent) return htmlContent
  const root = parseHtmlInert(htmlContent)
  wrapLooseInlineIn(root)
  return root.innerHTML
}

const wrapLooseInlineIn = (root: Element) => {
  const visit = (element: Element) => {
    const children = Array.from(element.childNodes)
    const isBlock = (child: ChildNode) => child.nodeType === 1 && BLOCK_TAGS.has((child as Element).tagName)
    if (children.some(isBlock) && element !== root && !['P', 'LI', 'TABLE'].includes(element.tagName)) {
      let run: ChildNode[] = []
      const flush = () => {
        if (run.some(isLooseInline)) {
          const paragraph = element.ownerDocument.createElement('p')
          run[0].before(paragraph)
          run.forEach((node) => paragraph.appendChild(node))
        }
        run = []
      }
      children.forEach((child) => {
        if (isBlock(child)) flush()
        else run.push(child)
      })
      flush()
    }
    Array.from(element.children).forEach((child) => {
      if (child.tagName !== 'TABLE') visit(child)
    })
  }
  visit(root)
}

// Helper function to sanitize content for rich text editor (sanitizes it for the
// console, strips plain spans, preserves styled content)
export const sanitizeRichContent = (rawContent: string): string => {
  if (!rawContent || rawContent.trim() === '') {
    return rawContent
  }
  const htmlContent = sanitizeEditorHtml(rawContent)

  try {
    // One parse for both: plain spans unwrapped, loose text given its paragraphs
    const root = parseHtmlInert(htmlContent)
    stripPlainSpansIn(root)
    wrapLooseInlineIn(root)
    return root.innerHTML
  } catch (error) {
    console.error('Error during rich content sanitization:', error)
    return htmlContent
  }
}

// Initial content for the inline editor, sanitized and inlined like
// prepareInlineContent's. Blocks become spans in place, so text between them and
// text of nested blocks is kept once.
export const getInitialInlineContent = (rawContent: string): string => {
  const emptyDocument = '<span data-inline-doc=""></span>'
  if (!rawContent) {
    return emptyDocument
  }
  const content = sanitizeEditorHtml(rawContent)

  try {
    // The spans standing for blocks without a style mean nothing inline
    const inline = stripPlainSpans(convertBlockToInline(stripPlainSpans(content)))
    return inline.includes('data-inline-doc') ? inline : `<span data-inline-doc="">${inline}</span>`
  } catch (error) {
    console.error('Error processing initial content:', error)
    try {
      return `<span data-inline-doc="">${textOnlyHtml(content)}</span>`
    } catch (fallbackError) {
      console.error('Text extraction fallback failed:', fallbackError)
      return emptyDocument
    }
  }
}

// Selects the whole link around the cursor. Only when nothing is selected: a
// selection the user made is kept.
export const expandSelectionToLinkRange = (editor: Editor): boolean => {
  const { selection } = editor.state
  if (!selection.empty || !editor.isActive('link')) {
    return false
  }
  const $pos = selection.$from
  const linkMark = $pos.marks().find((mark: Mark) => mark.type.name === 'link')
  if (!linkMark) {
    return false
  }
  const range = getMarkRange($pos, linkMark.type, linkMark.attrs)
  if (!range || range.from >= range.to) {
    return false
  }
  editor.chain().focus().setTextSelection(range).run()
  return true
}

// Text and background colours. Inside a link they go on the link (see setInlineStyle).
export const handleTextColorChange = (editor: Editor, color: string) => {
  editor.chain().focus().setInlineStyle('color', color || null).run()
}

export const handleBackgroundColorChange = (editor: Editor, backgroundColor: string) => {
  editor.chain().focus().setInlineStyle('background-color', backgroundColor || null).run()
}

// The marks the toolbar describes: those the next typed text gets, or those where
// the selection starts.
const marksAtSelection = (editor: Editor): readonly Mark[] => {
  const { state } = editor
  const { selection } = state
  if (selection.empty) {
    return state.storedMarks ?? selection.$from.marks()
  }
  // The first text of the selection (returning false from nodesBetween only skips
  // a node's children, not its siblings)
  let marks: readonly Mark[] | undefined
  state.doc.nodesBetween(selection.from, selection.to, (node: Node) => {
    if (marks) return false
    if (node.isText) marks = node.marks
    return !marks
  })
  return marks ?? []
}

const blockStyleAtSelection = (editor: Editor): string | null =>
  (editor.state.selection.$from.parent.attrs.style as string | null | undefined) ?? null

export const getEffectiveTextColor = (editor: Editor): string =>
  inlineStyleAt(marksAtSelection(editor), 'color', blockStyleAtSelection(editor))

export const getEffectiveBackgroundColor = (editor: Editor): string =>
  inlineStyleAt(marksAtSelection(editor), 'background-color', blockStyleAtSelection(editor))

// Bold, italic, underline and strike-through, as a tag or as the CSS that draws it
export type Formatting = 'bold' | 'italic' | 'underline' | 'strike'

const FORMATTING_CSS: Record<Formatting, { property: string; draws: (value: string) => boolean }> = {
  bold: { property: 'font-weight', draws: (value) => /^(bold|bolder|[6-9]00)$/.test(value) },
  italic: { property: 'font-style', draws: (value) => value === 'italic' || value === 'oblique' },
  underline: { property: 'text-decoration', draws: (value) => /\bunderline\b/.test(value) },
  strike: { property: 'text-decoration', draws: (value) => /\bline-through\b/.test(value) }
}

const cssValueAtSelection = (editor: Editor, formatting: Formatting): string =>
  inlineStyleAt(marksAtSelection(editor), FORMATTING_CSS[formatting].property, blockStyleAtSelection(editor))
    .trim()
    .toLowerCase()

/**
 * True when the formatting shows on the whole selection, by its tag or by CSS on
 * the text, its link or its paragraph. At a cursor, where the next text goes.
 */
export const isFormattingActive = (editor: Editor, formatting: Formatting): boolean => {
  const { property, draws } = FORMATTING_CSS[formatting]
  const { selection, doc } = editor.state
  if (selection.empty) {
    return editor.isActive(formatting) || draws(cssValueAtSelection(editor, formatting))
  }
  let sawText = false
  let everywhere = true
  doc.nodesBetween(selection.from, selection.to, (node: Node, _pos: number, parent: Node | null) => {
    if (!everywhere || !node.isText) return everywhere
    sawText = true
    const tagged = node.marks.some((mark) => mark.type.name === formatting)
    const css = inlineStyleAt(node.marks, property, parent?.attrs.style as string | null | undefined)
    if (!tagged && !draws(css.trim().toLowerCase())) everywhere = false
    return false
  })
  return sawText && everywhere
}

// What turns a formatting off at a cursor where a paragraph or link draws it
const NEUTRAL_CSS: Record<Formatting, string> = {
  bold: 'normal',
  italic: 'normal',
  underline: 'none',
  strike: 'none'
}

/**
 * Toggles a formatting. Turning it off removes the tag and the CSS that draws it,
 * so text styled `font-weight: bold` stops being bold too.
 */
export const toggleFormatting = (editor: Editor, formatting: Formatting) => {
  if (!isFormattingActive(editor, formatting)) {
    editor.chain().focus().setMark(formatting).run()
    return
  }
  const { property, draws } = FORMATTING_CSS[formatting]
  const value = cssValueAtSelection(editor, formatting)
  const chain = editor.chain().focus().unsetMark(formatting)
  if (editor.state.selection.empty) {
    // Text typed next: a style of its own undoes the paragraph's or link's
    const textStyle = marksAtSelection(editor).find((mark) => mark.type.name === 'textStyle')
    const fromText = getStyleProperty(textStyle?.attrs.style as string | null, property).trim().toLowerCase()
    if (draws(fromText)) chain.setInlineStyle(property, null)
    else if (draws(value)) chain.setInlineStyle(property, NEUTRAL_CSS[formatting])
  } else if (property !== 'text-decoration') {
    // Off everywhere in the selection, whichever runs draw it
    chain.setInlineStyle(property, null)
  } else if (draws(value)) {
    // text-decoration may also hold the other line, which stays
    const other = property === 'text-decoration'
      ? value.replace(formatting === 'underline' ? /\bunderline\b/ : /\bline-through\b/, '').trim()
      : ''
    chain.setInlineStyle(property, other || null)
  }
  chain.run()
}

/** A value the colour picker can show: Liquid or other expressions are not colours. */
export const pickerColor = (value: string): string | undefined =>
  /^#[0-9a-f]{3,8}$/i.test(value) || /^(rgb|hsl)a?\([^{}]*\)$/i.test(value) || /^[a-z]+$/i.test(value)
    ? value
    : undefined

// The link to the web version of the email. Broadcasts and automations fill the variable
// with the page's address when they send; elsewhere it is empty.
export const WEB_VERSION_LINK_HREF = '{{ view_in_browser_url }}'

const WEB_VERSION_LINK_PATTERN = /^\{\{\s*view_in_browser_url\s*\}\}$/

// Reads a saved link target back into the link type and value the link popover edits.
export const parseLinkHref = (href: string): { type: LinkType; value: string } => {
  if (WEB_VERSION_LINK_PATTERN.test(href.trim())) {
    return { type: 'web_version', value: '' }
  }
  if (href.startsWith('mailto:')) {
    return { type: 'email', value: href.slice('mailto:'.length) }
  }
  if (href.startsWith('tel:')) {
    return { type: 'phone', value: href.slice('tel:'.length) }
  }
  if (href.startsWith('#')) {
    return { type: 'anchor', value: href.slice(1) }
  }
  return { type: 'url', value: href }
}

// The link target for a value typed in the link popover.
export const formatLinkHref = (value: string, linkType: string = 'url'): string => {
  const trimmed = value.trim()
  switch (linkType) {
    case 'web_version':
      return WEB_VERSION_LINK_HREF
    case 'email':
      return `mailto:${trimmed}`
    case 'phone':
      return `tel:${trimmed}`
    case 'anchor':
      return `#${trimmed}`
    default:
      return trimmed
  }
}

/**
 * Links the selection to the target typed in the link popover. With nothing
 * selected and the cursor outside any link, it inserts `text` as the link's text:
 * a link needs text to exist at all.
 */
export const createLinkWithStyleMerging = (
  editor: Editor,
  value: string,
  linkType: string = 'url',
  text?: string
) => {
  const href = formatLinkHref(value, linkType)
  const { selection } = editor.state
  const label = (text ?? value).trim()

  if (selection.empty && !editor.isActive('link') && label) {
    // The inserted text keeps the formatting at the cursor; its colours go on the link
    const cursorMarks = editor.state.storedMarks ?? selection.$from.marks()
    let linkStyle: string | null = null
    const marks = cursorMarks.flatMap((mark: Mark) => {
      if (mark.type.name !== 'textStyle') return [mark.toJSON()]
      const style = mark.attrs.style as string | null
      linkStyle = pickStyleProperties(style, LINK_STYLE_PROPERTIES)
      const attrs = { ...mark.attrs, style: omitStyleProperties(style, LINK_STYLE_PROPERTIES) }
      return keepsTextStyle(attrs) ? [{ type: 'textStyle', attrs }] : []
    })
    editor
      .chain()
      .focus()
      .insertContent({
        type: 'text',
        text: label,
        marks: [...marks, { type: 'link', attrs: { href, style: linkStyle } }]
      })
      // Text typed next continues after the link, not inside it
      .command(({ tr }) => {
        tr.setStoredMarks(tr.selection.$from.marks().filter((mark) => mark.type.name !== 'link'))
        return true
      })
      .run()
    return
  }
  editor.chain().focus().setLink({ href }).run()
}
