import { Parser } from 'htmlparser2'
import type { DisplayCondition, EmailBlock } from '../email_builder/types'

/**
 * Reads an MJML file into a visual editor tree, the way MJML reads it: with htmlparser2, which
 * accepts what HTML accepts (a bare &, an unclosed <p>, an attribute without quotes), and with the
 * content of text-like blocks taken as written, not parsed.
 *
 * Liquid survives the import. Inside a block (a text, an attribute) it is kept as written. Between
 * blocks it is rebuilt from the marks the MJML export writes (pkg/notifuse_mjml/liquid_markers.go):
 * an mj-liquid block, or a block's display condition. Without marks, a passage holding Liquid
 * between blocks becomes one mj-liquid block holding its exact source.
 */

// Blocks whose content is taken as written: the export writes mj-text, mj-button, mj-raw and
// mj-liquid unescaped, and the others are not editor blocks, so they come from MJML written elsewhere.
const RAW_EXACT_TAGS = [
  'mj-text',
  'mj-button',
  'mj-raw',
  'mj-liquid',
  'mj-table',
  'mj-navbar-link',
  'mj-accordion-title',
  'mj-accordion-text'
]
// Blocks whose content is taken as written, then decoded: the export escapes it (escapeContent).
const RAW_DECODED_TAGS = ['mj-title', 'mj-preview', 'mj-style', 'mj-social-element']
const RAW_TAGS = new Set([...RAW_EXACT_TAGS, ...RAW_DECODED_TAGS])

// Blocks whose children are blocks laid out in order, where Liquid can sit between them.
const CONTAINER_TAGS = new Set(['mj-body', 'mj-wrapper', 'mj-section', 'mj-group', 'mj-column'])
// Of those, the ones that accept an mj-liquid child (see ValidChildren in email_builder/types.ts).
const LIQUID_HOLDERS = new Set(['mj-body', 'mj-wrapper', 'mj-section', 'mj-column'])

// Liquid and raw contents are set aside before parsing, as tokens the parser cannot misread. A
// token keeps the newlines of what it stands for, so the lines of parse errors stay right.
const LIQUID_TOKEN = String.fromCharCode(1)
const RAW_TOKEN = String.fromCharCode(2)
const LIQUID_TOKEN_PATTERN = new RegExp(`${LIQUID_TOKEN}(\\d+)\\n*${LIQUID_TOKEN}`, 'g')
const RAW_TOKEN_PATTERN = new RegExp(`${RAW_TOKEN}(\\d+)\\n*${RAW_TOKEN}`, 'g')

// A Liquid comment or raw block (kept whole: its body is not Liquid), or a tag, or an output.
const LIQUID_COMMENT_OR_RAW = /\{%-?\s*(comment|raw)\s*-?%\}[\s\S]*?\{%-?\s*end\1\s*-?%\}/g
const LIQUID_PATTERN =
  /\{%-?\s*(comment|raw)\s*-?%\}[\s\S]*?\{%-?\s*end\1\s*-?%\}|\{%[\s\S]*?%\}|\{\{[\s\S]*?\}\}/g
// The marks of the export, written as Liquid comments.
const MARKER_PATTERN =
  /^\{%-?\s*comment\s*-?%\}\s*(\/?)notifuse:(liquid|condition)\b([\s\S]*?)\{%-?\s*endcomment\s*-?%\}$/

const BLOCK_OPENERS = new Set(['if', 'unless', 'case', 'for', 'tablerow', 'capture'])
const BLOCK_CLOSERS = new Set([
  'endif',
  'endunless',
  'endcase',
  'endfor',
  'endtablerow',
  'endcapture'
])
const ASSIGNING_TAGS = /^(?:assign|capture|increment|decrement)\s+([A-Za-z_][\w-]*)/

interface Marker {
  kind: 'liquid' | 'condition'
  close: boolean
  payload: string
}

interface ParsedElement {
  kind: 'element'
  name: string
  attributes: Record<string, string>
  children: ParsedNode[]
  start: number
  openEnd: number
  end: number
}

interface ParsedText {
  kind: 'text'
  start: number
  end: number
}

type ParsedNode = ParsedElement | ParsedText

// A block as the import builds it, before it is handed over as an EmailBlock.
interface ImportedBlock {
  id: string
  type: string
  attributes: Record<string, unknown>
  content?: string
  children?: EmailBlock[]
  display_condition?: DisplayCondition
}

const asEmailBlock = (block: ImportedBlock) => block as unknown as EmailBlock
const liquidBlock = (content: string, id = generateId()) =>
  asEmailBlock({ id, type: 'mj-liquid', attributes: {}, content })

/** The result of an MJML import: the tree, and how many passages became mj-liquid blocks. */
export interface MjmlImport {
  tree: EmailBlock
  liquidBlocksKept: number
}

/** What an MJML file holds that the import cannot read. */
export type MjmlImportErrorReason =
  | 'not_closed'
  | 'closes_nothing'
  | 'liquid_in_tag'
  | 'liquid_between_blocks'
  | 'outside_root'
  | 'mark_not_closed'
  | 'mark_not_opened'
  | 'condition_mark_invalid'

const REASON_TEXT: Record<MjmlImportErrorReason, (subject: string) => string> = {
  not_closed: (tag) => `<${tag}> is not closed`,
  closes_nothing: (tag) => `</${tag}> closes no open tag`,
  liquid_in_tag: () => 'Liquid inside a tag cannot be imported into the visual editor',
  liquid_between_blocks: (tag) =>
    `Liquid between the blocks of <${tag}> cannot be imported into the visual editor`,
  outside_root: () => 'Liquid or markup outside <mjml> cannot be imported',
  mark_not_closed: (kind) => `This notifuse:${kind} mark of a Notifuse export is not closed`,
  mark_not_opened: (kind) => `This notifuse:${kind} mark of a Notifuse export closes no mark`,
  condition_mark_invalid: () =>
    'This notifuse:condition mark of a Notifuse export holds no block or no valid condition'
}

/**
 * MJML the import cannot read, at a line of the file. subject is the tag or the mark it names.
 * The message is English, « Invalid MJML syntax: line:column: … »; the console words it from reason.
 */
export class MjmlImportError extends Error {
  constructor(
    readonly reason: MjmlImportErrorReason,
    readonly line: number,
    readonly column: number,
    readonly subject = ''
  ) {
    super(`Invalid MJML syntax: ${line}:${column}: ${REASON_TEXT[reason](subject)}`)
  }
}

// The characters the import sets Liquid and raw contents aside with: dropped from the file first.
// They are not text an email can show.
const TOKEN_CHARACTERS = new RegExp(`[${String.fromCharCode(1)}${String.fromCharCode(2)}]`, 'g')

let entityDecoder: HTMLTextAreaElement | undefined

/** Decodes the HTML entities of text. A textarea holds raw text: nothing put in it is run. */
function decodeEntities(text: string): string {
  if (!text.includes('&')) return text
  entityDecoder ??= document.createElement('textarea')
  entityDecoder.innerHTML = text
  return entityDecoder.value
}

const camelCase = (name: string) =>
  name.replace(/-([a-zA-Z])/g, (_, letter: string) => letter.toUpperCase())
const generateId = () => Math.random().toString(36).slice(2, 11)
const newlinesOf = (text: string) => text.replace(/[^\n]/g, '')

class MjmlReader {
  private readonly liquid: string[] = []
  private readonly markers = new Map<number, Marker>()
  private readonly raw: string[] = []
  private source = ''
  // The mj-liquid blocks made of Liquid found between blocks, without marks.
  private readonly keptIds = new Set<string>()

  read(mjml: string): EmailBlock {
    const withLiquidAside = this.setLiquidAside(mjml.replace(TOKEN_CHARACTERS, ''))
    this.source = this.setRawContentsAside(
      this.setCommentsAside(this.setLiquidBlocksAside(withLiquidAside))
    )
    const top = this.parse()
    const root = top.find((node): node is ParsedElement => node.kind === 'element')
    if (!root || root.name !== 'mjml') throw new Error('Root element must be <mjml>')
    // Only comments and blank text may sit around the root.
    const outside = top.find(
      (node) =>
        node !== root &&
        (node.kind === 'element' ||
          this.slice(node.start, node.end).replace(RAW_TOKEN_PATTERN, '').trim() !== '')
    )
    if (outside) throw this.syntaxError(outside.start, 'outside_root')
    return this.convert(root).block
  }

  /** How many passages of tree became mj-liquid blocks. One kept whole in another is not counted. */
  liquidBlocksKept(tree: EmailBlock): number {
    const own = this.keptIds.has(tree.id) ? 1 : 0
    return own + (tree.children ?? []).reduce((sum, child) => sum + this.liquidBlocksKept(child), 0)
  }

  // === Setting aside and restoring ===

  private setLiquidAside(mjml: string): string {
    return mjml.replace(LIQUID_PATTERN, (match) => {
      const index = this.liquid.push(match) - 1
      const marker = MARKER_PATTERN.exec(match)
      if (marker) {
        this.markers.set(index, {
          close: marker[1] === '/',
          kind: marker[2] as Marker['kind'],
          payload: marker[3].trim()
        })
      }
      return `${LIQUID_TOKEN}${index}${newlinesOf(match)}${LIQUID_TOKEN}`
    })
  }

  // The content of a Liquid block the export marked is set aside whole: it is not parsed, so it can
  // hold Liquid inside a tag, or one half of a tag.
  private setLiquidBlocksAside(source: string): string {
    let out = ''
    let from = 0
    let open: RegExpExecArray | undefined
    for (const token of source.matchAll(LIQUID_TOKEN_PATTERN)) {
      const marker = this.markers.get(Number(token[1]))
      if (marker?.kind !== 'liquid') continue
      const at = token.index ?? 0
      if (!marker.close) {
        open ??= token
        continue
      }
      if (!open) continue // a close without an open: reported where the marks are read
      const contentStart = (open.index ?? 0) + open[0].length
      const content = source.slice(contentStart, at)
      const index = this.raw.push(content) - 1
      out +=
        source.slice(from, contentStart) + `${RAW_TOKEN}${index}${newlinesOf(content)}${RAW_TOKEN}`
      from = at
      open = undefined
    }
    if (open) throw this.syntaxError(open.index ?? 0, 'mark_not_closed', 'liquid', source)
    return out + source.slice(from)
  }

  // An HTML comment is set aside whole: what it holds is neither a block nor a closing tag. It is
  // kept where it is written (a text, an Outlook conditional comment of an mj-raw).
  private setCommentsAside(source: string): string {
    return source.replace(/<!--[\s\S]*?-->/g, (comment) => {
      const index = this.raw.push(comment) - 1
      return `${RAW_TOKEN}${index}${newlinesOf(comment)}${RAW_TOKEN}`
    })
  }

  // As MJML does, the content of a text-like block is not parsed: it is set aside whole.
  private setRawContentsAside(source: string): string {
    const opening = new RegExp(
      `<(${[...RAW_TAGS].join('|')})((?:\\s(?:[^>"']|"[^"]*"|'[^']*')*)?)>`,
      'gi'
    )
    let out = ''
    let from = 0
    let match: RegExpExecArray | null
    while ((match = opening.exec(source)) !== null) {
      if (match[2].trimEnd().endsWith('/')) continue
      const contentStart = match.index + match[0].length
      const closing = new RegExp(`</${match[1]}\\s*>`, 'i')
      const close = closing.exec(source.slice(contentStart))
      if (!close) continue
      const content = source.slice(contentStart, contentStart + close.index)
      const index = this.raw.push(content) - 1
      out +=
        source.slice(from, contentStart) + `${RAW_TOKEN}${index}${newlinesOf(content)}${RAW_TOKEN}`
      from = contentStart + close.index
      opening.lastIndex = from
    }
    return out + source.slice(from)
  }

  /**
   * The original text of a slice of the source: raw contents and Liquid put back. « decoded »
   * decodes the entities of the whole, Liquid included, as for the content the export escapes;
   * « attribute » decodes the Liquid only, the parser having decoded the rest of the value.
   */
  private restore(text: string, mode: 'exact' | 'decoded' | 'attribute' = 'exact'): string {
    let withRaw = text
    // A content set aside can hold a comment set aside before it.
    for (let previous = ''; previous !== withRaw;) {
      previous = withRaw
      withRaw = withRaw.replace(RAW_TOKEN_PATTERN, (_, index: string) => this.raw[Number(index)])
    }
    const restored = withRaw.replace(LIQUID_TOKEN_PATTERN, (_, index: string) => {
      const liquid = this.liquid[Number(index)]
      return mode === 'attribute' ? decodeEntities(liquid) : liquid
    })
    return mode === 'decoded' ? decodeEntities(restored) : restored
  }

  // Tokens keep the newlines of what they stand for: a line of the source is a line of the file.
  private syntaxError(
    index: number,
    reason: MjmlImportErrorReason,
    subject = '',
    source = this.source
  ): MjmlImportError {
    const before = source.slice(0, index)
    return new MjmlImportError(
      reason,
      before.split('\n').length,
      index - before.lastIndexOf('\n'),
      subject
    )
  }

  // === Parsing ===

  private parse(): ParsedNode[] {
    const top: ParsedNode[] = []
    const stack: ParsedElement[] = []
    const explicitCloses = new Set<number>()
    let attributes: Record<string, string> = {}
    let error: MjmlImportError | undefined
    const childrenOf = () => (stack.length ? stack[stack.length - 1].children : top)

    const parser: Parser = new Parser(
      {
        onopentagname: () => {
          attributes = {}
        },
        onattribute: (name, value) => {
          if (name.includes(LIQUID_TOKEN) || name.includes(RAW_TOKEN)) {
            error ??= this.syntaxError(parser.startIndex, 'liquid_in_tag')
          }
          // A repeated attribute keeps its last value, as the MJML export writes it.
          attributes[name] = value
        },
        onopentag: (name) => {
          const element: ParsedElement = {
            kind: 'element',
            name,
            attributes,
            children: [],
            start: parser.startIndex,
            openEnd: parser.endIndex,
            end: parser.endIndex
          }
          childrenOf().push(element)
          stack.push(element)
        },
        onclosetag: (name, isImplied) => {
          const element = stack.pop()
          if (!element) return
          // A self-closing tag is closed "implied" by the token that opened it.
          const selfClosing = isImplied && parser.endIndex === element.openEnd
          if (isImplied && !selfClosing && (name.startsWith('mj-') || name === 'mjml')) {
            error ??= this.syntaxError(element.start, 'not_closed', name)
          }
          if (!isImplied) explicitCloses.add(parser.endIndex)
          element.end = isImplied && !selfClosing ? this.lastEnd(element) : parser.endIndex
        },
        ontext: () => {
          const siblings = childrenOf()
          const last = siblings[siblings.length - 1]
          if (last?.kind === 'text' && last.end + 1 >= parser.startIndex) {
            last.end = parser.endIndex
          } else {
            siblings.push({ kind: 'text', start: parser.startIndex, end: parser.endIndex })
          }
        }
      },
      { recognizeSelfClosing: true, lowerCaseAttributeNames: false, decodeEntities: true }
    )
    parser.end(this.source)
    if (error) throw error

    // htmlparser2 drops a closing tag that closes nothing.
    const closing = /<\/(mj-[\w-]+|mjml)\s*>/gi
    let match: RegExpExecArray | null
    while ((match = closing.exec(this.source)) !== null) {
      if (!explicitCloses.has(match.index + match[0].length - 1)) {
        throw this.syntaxError(match.index, 'closes_nothing', match[1])
      }
    }
    return top
  }

  private lastEnd(element: ParsedElement): number {
    const last = element.children[element.children.length - 1]
    return last ? last.end : element.openEnd
  }

  private slice(start: number, end: number): string {
    return this.source.slice(start, end + 1)
  }

  // === Conversion ===

  /** Converts element; escalate tells its container to keep it in a Liquid block. */
  private convert(element: ParsedElement): { block: EmailBlock; escalate: boolean } {
    const block: ImportedBlock = { id: generateId(), type: element.name, attributes: {} }
    const attributes: Record<string, unknown> = {}
    for (const [name, value] of Object.entries(element.attributes)) {
      attributes[camelCase(name)] = this.restore(value, 'attribute')
    }
    block.attributes = attributes

    if (RAW_TAGS.has(element.name)) {
      const inner = element.children.map((child) => this.slice(child.start, child.end)).join('')
      const mode = RAW_DECODED_TAGS.includes(element.name) ? 'decoded' : 'exact'
      let content = this.restore(inner, mode).trim()
      if (content) {
        // The text editor wraps its content in paragraphs: a bare text gets one, Liquid aside.
        const markup = content.replace(LIQUID_PATTERN, '').trimStart()
        if (element.name === 'mj-text' && !markup.startsWith('<')) content = `<p>${content}</p>`
        block.content = content
      }
      return { block: asEmailBlock(block), escalate: false }
    }

    if (CONTAINER_TAGS.has(element.name)) {
      const { children, escalate } = this.convertContainer(element)
      if (children.length > 0) block.children = children
      return { block: asEmailBlock(block), escalate }
    }

    const elements = element.children.filter(
      (child): child is ParsedElement => child.kind === 'element'
    )
    const converted = elements.map((child) => this.convert(child))
    if (converted.length > 0) {
      block.children = converted.map((child) => child.block)
    } else {
      const text = element.children.map((child) => this.slice(child.start, child.end)).join('')
      const content = this.restore(text, 'decoded').trim()
      if (content) block.content = content
    }
    // Liquid between the children of a block that cannot hold a Liquid block (mj-social,
    // mj-navbar, mj-hero…) keeps the whole block in a Liquid block of its container.
    const liquid = element.children.find((child) =>
      child.kind === 'text'
        ? converted.length > 0 && this.slice(child.start, child.end).includes(LIQUID_TOKEN)
        : converted[elements.indexOf(child)].escalate
    )
    if (liquid && (element.name === 'mj-head' || element.name === 'mjml')) {
      throw this.syntaxError(liquid.start, 'liquid_between_blocks', element.name)
    }
    return { block: asEmailBlock(block), escalate: !!liquid }
  }

  private convertContainer(element: ParsedElement): { children: EmailBlock[]; escalate: boolean } {
    const items = this.applyMarkers(this.itemsOf(element))
    const children: EmailBlock[] = []
    let escalate = false
    for (let index = 0; index < items.length; index++) {
      const item = items[index]
      // Text between blocks starts a passage when it holds Liquid; plain text is dropped.
      const seed =
        item.kind === 'text'
          ? this.slice(item.start, item.end).includes(LIQUID_TOKEN)
          : item.escalate
      if (!seed) {
        if (item.kind === 'block') children.push(item.block)
        continue
      }
      const end = this.regionEnd(items, index)
      const source = this.restore(this.slice(item.start, items[end].end)).trim()
      const kept = liquidBlock(source)
      children.push(kept)
      this.keptIds.add(kept.id)
      const assigned = assignedNames(source)
      const after = this.restore(this.source.slice(element.end + 1))
      if (!LIQUID_HOLDERS.has(element.name) || [...assigned].some((name) => reads(after, name)))
        escalate = true
      index = end
    }
    // An escalated container ends up whole in a Liquid block of its parent: its own blocks are
    // not counted.
    return { children, escalate }
  }

  /** The last item a passage starting at items[start] must hold, for its Liquid to work. */
  private regionEnd(items: ContentItem[], start: number): number {
    let end = start
    for (;;) {
      const source = this.restore(this.slice(items[start].start, items[end].end))
      if (liquidDepth(this.textLiquid(items.slice(start, end + 1))) > 0 && end < items.length - 1) {
        end++
        continue
      }
      const assigned = [...assignedNames(source)]
      let reader = end
      for (let index = items.length - 1; index > end; index--) {
        const text = this.restore(this.slice(items[index].start, items[index].end))
        if (assigned.some((name) => reads(text, name))) {
          reader = index
          break
        }
      }
      if (reader === end) return end
      end = reader
    }
  }

  private itemsOf(element: ParsedElement): ContainerItem[] {
    const items: ContainerItem[] = []
    for (const child of element.children) {
      if (child.kind === 'element') {
        const { block, escalate } = this.convert(child)
        items.push({ kind: 'block', block, escalate, start: child.start, end: child.end })
        continue
      }
      // A text is split around the marks it holds.
      const text = this.slice(child.start, child.end)
      let from = 0
      for (const token of text.matchAll(LIQUID_TOKEN_PATTERN)) {
        const marker = this.markers.get(Number(token[1]))
        if (!marker) continue
        const at = token.index ?? 0
        if (at > from)
          items.push({ kind: 'text', start: child.start + from, end: child.start + at - 1 })
        items.push({
          kind: 'marker',
          marker,
          start: child.start + at,
          end: child.start + at + token[0].length - 1
        })
        from = at + token[0].length
      }
      if (from < text.length)
        items.push({ kind: 'text', start: child.start + from, end: child.end })
    }
    return items
  }

  private applyMarkers(items: ContainerItem[]): ContentItem[] {
    const out: ContentItem[] = []
    for (let index = 0; index < items.length; index++) {
      const item = items[index]
      if (item.kind !== 'marker') {
        out.push(item)
        continue
      }
      const { marker } = item
      const closeIndex = items.findIndex(
        (other, at) =>
          at > index &&
          other.kind === 'marker' &&
          other.marker.kind === marker.kind &&
          other.marker.close
      )
      if (marker.close || closeIndex < 0) {
        throw this.syntaxError(
          item.start,
          marker.close ? 'mark_not_opened' : 'mark_not_closed',
          marker.kind
        )
      }
      const close = items[closeIndex]
      if (marker.kind === 'liquid') {
        const content = this.restore(this.source.slice(item.end + 1, close.start))
        out.push({
          kind: 'block',
          block: liquidBlock(content),
          escalate: false,
          start: item.start,
          end: close.end
        })
      } else {
        const target = items.slice(index + 1, closeIndex).find((other) => other.kind === 'block')
        let condition: DisplayCondition | undefined
        try {
          condition = JSON.parse(marker.payload) as DisplayCondition
        } catch {
          condition = undefined
        }
        if (!target || target.kind !== 'block' || !condition) {
          throw this.syntaxError(item.start, 'condition_mark_invalid')
        }
        target.block.display_condition = condition
        out.push({ ...target, start: item.start, end: close.end })
      }
      index = closeIndex
    }
    return out
  }

  /** The text of the text items of items: the Liquid written at this level. */
  private textLiquid(items: ContainerItem[]): string {
    return items
      .filter((item) => item.kind === 'text')
      .map((item) => this.restore(this.slice(item.start, item.end)))
      .join('')
  }
}

type ContentItem =
  | { kind: 'block'; block: EmailBlock; escalate: boolean; start: number; end: number }
  | { kind: 'text'; start: number; end: number }
type ContainerItem = ContentItem | { kind: 'marker'; marker: Marker; start: number; end: number }

/** liquid without its Liquid comments and raw blocks, whose bodies are not run. */
const withoutComments = (liquid: string) => liquid.replace(LIQUID_COMMENT_OR_RAW, '')

/** How many Liquid blocks (if, for…) liquid leaves open. */
function liquidDepth(liquid: string): number {
  let depth = 0
  for (const tag of withoutComments(liquid).matchAll(/\{%-?\s*(\w+)/g)) {
    if (BLOCK_OPENERS.has(tag[1])) depth++
    else if (BLOCK_CLOSERS.has(tag[1])) depth = Math.max(0, depth - 1)
  }
  return depth
}

/** The variables the Liquid of source sets (assign, capture…), {% liquid %} lines included. */
function assignedNames(source: string): Set<string> {
  const names = new Set<string>()
  for (const tag of withoutComments(source).matchAll(/\{%-?([\s\S]*?)-?%\}/g)) {
    for (const line of tag[1].split('\n')) {
      const match = ASSIGNING_TAGS.exec(line.trim().replace(/^liquid\s+/, ''))
      if (match) names.add(match[1])
    }
  }
  return names
}

/** Whether the Liquid of text reads the variable name (not item.name, nor name_2). */
function reads(text: string, name: string): boolean {
  const pattern = new RegExp(`(?<![\\w.-])${name.replace(/[-]/g, '\\-')}(?![\\w-])`)
  for (const token of withoutComments(text).matchAll(/\{\{[\s\S]*?\}\}|\{%[\s\S]*?%\}/g)) {
    if (pattern.test(token[0])) return true
  }
  return false
}

/** Reads an MJML file into a visual editor tree; throws on MJML it cannot read. */
export function importMjmlSource(mjml: string): MjmlImport {
  const reader = new MjmlReader()
  try {
    const tree = reader.read(mjml)
    return { tree, liquidBlocksKept: reader.liquidBlocksKept(tree) }
  } catch (error) {
    if (error instanceof MjmlImportError) throw error
    throw new Error(`Failed to convert MJML to JSON: ${error}`)
  }
}

/** Reads an MJML file into a visual editor tree; throws on MJML it cannot read. */
export function convertMjmlToJsonBrowser(mjml: string): EmailBlock {
  return importMjmlSource(mjml).tree
}
