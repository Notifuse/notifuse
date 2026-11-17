import type { EmailBlock } from '../types'
import { parseHtmlInert } from '../../../utils/safeHtml'

/** A heading of the email, in document order. blockId is null in compiled HTML. */
export interface OutlineHeading {
  blockId: string | null
  level: number
  text: string
}

/** A heading more than one level below the heading before it. */
export interface HeadingSkip {
  heading: OutlineHeading
  previousLevel: number
}

// Blocks whose HTML holds headings written in the editor or by hand
const HEADING_HOLDERS = new Set(['mj-text', 'mj-raw'])

const headingsOfHtml = (html: string, blockId: string | null): OutlineHeading[] =>
  Array.from(parseHtmlInert(html).querySelectorAll('h1, h2, h3, h4, h5, h6')).map((element) => ({
    blockId,
    level: Number(element.tagName.slice(1)),
    text: (element.textContent ?? '').trim()
  }))

/**
 * The headings of the email's body, in document order, from its Text and Raw HTML
 * blocks. Display conditions are ignored: every heading counts. Headings that an
 * mj-liquid block writes only exist in the compiled HTML (htmlHeadingOutline).
 */
export const headingOutline = (tree: EmailBlock): OutlineHeading[] => readOutline(tree).headings

interface TreeOutline {
  headings: OutlineHeading[]
  // For each block of the body, the level of the last heading before it
  levelBefore: Map<string, number | null>
}

// Every Text block asks for the outline on each render: the tree is read once per
// version (trees are replaced, not mutated, on each edit)
const outlines = new WeakMap<EmailBlock, TreeOutline>()

const readOutline = (tree: EmailBlock): TreeOutline => {
  const cached = outlines.get(tree)
  if (cached) return cached
  const outline: TreeOutline = { headings: [], levelBefore: new Map() }
  const body = tree.children?.find((child) => child.type === 'mj-body')
  const visit = (block: EmailBlock) => {
    outline.levelBefore.set(block.id, outline.headings.at(-1)?.level ?? null)
    const content = (block as { content?: unknown }).content
    if (HEADING_HOLDERS.has(block.type) && typeof content === 'string') {
      outline.headings.push(...headingsOfHtml(content, block.id))
    }
    block.children?.forEach(visit)
  }
  if (body) visit(body)
  outlines.set(tree, outline)
  return outline
}

/** The headings of compiled email HTML, in document order. */
export const htmlHeadingOutline = (html: string): OutlineHeading[] => headingsOfHtml(html, null)

/**
 * The headings that skip a level: more than one level below the heading before
 * them. The first heading never skips, and going back up never does.
 */
export const findSkips = (outline: OutlineHeading[]): HeadingSkip[] =>
  outline.flatMap((heading, index) => {
    const previous = outline[index - 1]
    return previous && heading.level > previous.level + 1 ? [{ heading, previousLevel: previous.level }] : []
  })

/** The level of the last heading in the blocks before `blockId`, null when none. */
export const headingLevelBeforeBlock = (tree: EmailBlock, blockId: string): number | null =>
  readOutline(tree).levelBefore.get(blockId) ?? null
