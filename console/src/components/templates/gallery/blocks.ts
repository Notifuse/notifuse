import { EmailBlockClass } from '../../email_builder/EmailBlockClass'
import type { EmailBlock, MJMLComponentType } from '../../email_builder/types'

// The gallery's look: one accent, read from the buttons' default in mj-attributes, so a template
// changes it in one place.
export const ACCENT = '#1d4ed8'
const INK = '#1f2430'
const SOFT = '#5b6170'
const LINE = '#e4e6ec'
const PAGE = '#f4f5f8'
const FONT = 'Arial, Helvetica, sans-serif'

// Every block of a model gets an ID from its model and its order, so a model builds the same
// tree each time.
export class BlockIds {
  private count = 0
  constructor(private readonly prefix: string) {}
  next(type: string): string {
    this.count += 1
    return `${this.prefix}-${type.replace('mj-', '')}-${this.count}`
  }
}

type Attributes = Record<string, unknown>

function make(ids: BlockIds, type: MJMLComponentType, attributes: Attributes = {}, content?: string, children?: EmailBlock[]): EmailBlock {
  // Built as a plain block: the union type of EmailBlock ties each type to its own children.
  const block = EmailBlockClass.createBlock(type, ids.next(type), content) as unknown as {
    attributes: Attributes
    children?: EmailBlock[]
  }
  block.attributes = { ...block.attributes, ...attributes }
  if (children) block.children = children
  return block as unknown as EmailBlock
}

export interface ModelBuilder {
  ids: BlockIds
  /** The whole email: head (defaults, preview text) and body. */
  email: (preheader: string, sections: EmailBlock[]) => EmailBlock
  section: (columns: EmailBlock[], attributes?: Attributes) => EmailBlock
  /** Columns that stay side by side on a phone. */
  group: (columns: EmailBlock[]) => EmailBlock
  column: (children: EmailBlock[], attributes?: Attributes) => EmailBlock
  text: (html: string, attributes?: Attributes) => EmailBlock
  button: (label: string, href: string, attributes?: Attributes) => EmailBlock
  image: (src: string, alt: string, attributes?: Attributes) => EmailBlock
  divider: () => EmailBlock
  spacer: (height: string) => EmailBlock
  /** MJML with Liquid, for a loop over a list of the template data. */
  liquid: (mjml: string) => EmailBlock
}

/** The block builders of one model, its IDs prefixed with the model's. */
export function modelBuilder(modelId: string): ModelBuilder {
  const ids = new BlockIds(modelId)

  const head = (preheader: string) =>
    make(ids, 'mj-head', {}, undefined, [
      make(ids, 'mj-attributes', {}, undefined, [
        make(ids, 'mj-text', { color: INK, fontFamily: FONT, fontSize: '15px', lineHeight: '1.6', paddingLeft: '32px', paddingRight: '32px', paddingTop: '8px', paddingBottom: '8px' }),
        make(ids, 'mj-button', { backgroundColor: ACCENT, color: '#ffffff', fontFamily: FONT, fontSize: '15px', fontWeight: 'bold', borderRadius: '4px', innerPadding: '13px 26px', paddingTop: '12px', paddingBottom: '12px' }),
        make(ids, 'mj-section', { backgroundColor: '#ffffff', paddingTop: '12px', paddingBottom: '12px' })
      ]),
      make(ids, 'mj-preview', {}, preheader)
    ])

  return {
    ids,
    email: (preheader, sections) =>
      make(ids, 'mjml', {}, undefined, [
        head(preheader),
        make(ids, 'mj-body', { width: '600px', backgroundColor: PAGE }, undefined, sections)
      ]),
    section: (columns, attributes = {}) => make(ids, 'mj-section', { backgroundColor: '#ffffff', ...attributes }, undefined, columns),
    group: (columns) => make(ids, 'mj-group', {}, undefined, columns),
    column: (children, attributes = {}) => make(ids, 'mj-column', { verticalAlign: 'top', ...attributes }, undefined, children),
    text: (html, attributes = {}) => make(ids, 'mj-text', { color: INK, fontFamily: FONT, fontSize: '15px', lineHeight: '1.6', ...attributes }, html),
    button: (label, href, attributes = {}) =>
      make(ids, 'mj-button', { backgroundColor: ACCENT, color: '#ffffff', fontFamily: FONT, fontSize: '15px', fontWeight: 'bold', borderRadius: '4px', href, ...attributes }, label),
    image: (src, alt, attributes = {}) => make(ids, 'mj-image', { src, alt, paddingLeft: '0px', paddingRight: '0px', paddingTop: '0px', paddingBottom: '0px', ...attributes }),
    divider: () => make(ids, 'mj-divider', { borderColor: LINE, borderWidth: '1px', paddingLeft: '32px', paddingRight: '32px' }),
    spacer: (height) => make(ids, 'mj-spacer', { height }),
    liquid: (mjml) => make(ids, 'mj-liquid', {}, mjml)
  }
}

/** Text styles the models share, as mj-text attributes. */
export const STYLE = {
  h1: { fontSize: '27px', lineHeight: '1.25', fontWeight: 'bold' },
  h2: { fontSize: '20px', lineHeight: '1.3', fontWeight: 'bold' },
  soft: { color: SOFT },
  small: { fontSize: '13px', color: SOFT },
  center: { align: 'center' },
  tag: { fontSize: '11px', letterSpacing: '1px', textTransform: 'uppercase', fontWeight: 'bold', color: ACCENT },
  code: { fontFamily: "'Courier New', Courier, monospace", fontSize: '32px', letterSpacing: '8px', fontWeight: 'bold', align: 'center' },
  footer: { fontSize: '12px', color: SOFT, align: 'center', lineHeight: '1.6' }
} as const

export const COLORS = { ink: INK, soft: SOFT, line: LINE, page: PAGE, accentSoft: '#e8eefc' }
