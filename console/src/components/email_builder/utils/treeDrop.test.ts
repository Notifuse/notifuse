import { describe, it, expect } from 'vitest'
import type { EmailBlock } from '../types'
import { resolveDropTarget, type DropSide } from './treeDrop'

interface Node {
  id: string
  type: string
  children?: Node[]
}

const n = (id: string, type: string, children?: Node[]): Node => ({ id, type, children })
const text = (id: string) => n(id, 'mj-text')

// The audit's tree: body > S1[C1[T1,BT1,IM1], C2[T2,SO[SE1,SE2]]], W[SW[CW[TW]]], S2[G[G1[TG1],G2[TG2]]],
// S3[C3[D,SP,T3,T4]], R (raw), L (liquid)
const build = (patch: (body: Node[]) => Node[] = (b) => b): EmailBlock => {
  const body: Node[] = [
    n('S1', 'mj-section', [
      n('C1', 'mj-column', [text('T1'), n('BT1', 'mj-button'), n('IM1', 'mj-image')]),
      n('C2', 'mj-column', [
        text('T2'),
        n('SO', 'mj-social', [n('SE1', 'mj-social-element'), n('SE2', 'mj-social-element')])
      ])
    ]),
    n('W', 'mj-wrapper', [n('SW', 'mj-section', [n('CW', 'mj-column', [text('TW')])])]),
    n('S2', 'mj-section', [
      n('G', 'mj-group', [n('G1', 'mj-column', [text('TG1')]), n('G2', 'mj-column', [text('TG2')])])
    ]),
    n('S3', 'mj-section', [n('C3', 'mj-column', [n('D', 'mj-divider'), n('SP', 'mj-spacer'), text('T3'), text('T4')])]),
    n('R', 'mj-raw'),
    n('L', 'mj-liquid')
  ]
  return n('ROOT', 'mjml', [n('HEAD', 'mj-head', []), n('B', 'mj-body', patch(body))]) as unknown as EmailBlock
}

// Every container with children is expanded, as the editor opens the tree
const expandedOf = (tree: EmailBlock, except: string[] = []): string[] => {
  const keys: string[] = []
  const walk = (b: EmailBlock) => {
    if (b.children?.length && !except.includes(b.id)) keys.push(b.id)
    b.children?.forEach(walk)
  }
  walk(tree)
  return keys
}

const resolve = (tree: EmailBlock, drag: string, drop: string, side: DropSide, collapsed: string[] = []) =>
  resolveDropTarget(tree, drag, drop, side, expandedOf(tree, collapsed))

const tree = build()

// Empty containers inserted in the body for the empty-container cases
const withEmpties = build((body) => [
  n('ES', 'mj-section', []),
  n('S5', 'mj-section', [n('EC', 'mj-column', []), n('C5', 'mj-column', [text('T5'), n('ESO', 'mj-social', [])])]),
  n('EW', 'mj-wrapper', []),
  n('S6', 'mj-section', [n('EG', 'mj-group', []), n('C6', 'mj-column', [text('T6')])]),
  ...body
])

describe('resolveDropTarget', () => {
  it('moves a block down within its parent to the slot the line shows', () => {
    expect(resolve(tree, 'D', 'T3', 1)).toEqual({ parentId: 'C3', index: 2 })
    expect(resolve(tree, 'T1', 'BT1', 1)).toEqual({ parentId: 'C1', index: 1 })
    expect(resolve(tree, 'S1', 'R', 1)).toEqual({ parentId: 'B', index: 4 })
  })

  it('moves a block up', () => {
    expect(resolve(tree, 'T4', 'D', 1)).toEqual({ parentId: 'C3', index: 1 })
    expect(resolve(tree, 'S3', 'B', -1)).toEqual({ parentId: 'B', index: 0 })
  })

  it('returns null when the block would stay where it is', () => {
    expect(resolve(tree, 'SP', 'D', 1)).toBeNull()
    expect(resolve(tree, 'D', 'D', 1)).toBeNull()
    // The first valid candidate decides: "after SW in W" is SW's own place, so no fall-through to the body
    expect(resolve(tree, 'SW', 'SW', 1)).toBeNull()
  })

  it('moves a block to another parent', () => {
    expect(resolve(tree, 'T1', 'T2', 1)).toEqual({ parentId: 'C2', index: 1 })
    expect(resolve(tree, 'T1', 'C2', 0)).toEqual({ parentId: 'C2', index: 0 })
    expect(resolve(tree, 'R', 'C1', 0)).toEqual({ parentId: 'C1', index: 0 })
  })

  it('refuses positions whose parent does not accept the block', () => {
    expect(resolve(tree, 'T1', 'S1', 0)).toBeNull()
    expect(resolve(tree, 'S2', 'C1', 0)).toBeNull()
    expect(resolve(tree, 'T1', 'SE1', 1)).toBeNull()
    expect(resolve(tree, 'T1', 'L', 1)).toBeNull()
    expect(resolve(tree, 'C1', 'L', 1)).toBeNull()
    expect(resolve(tree, 'T1', 'B', -1)).toBeNull()
  })

  it('places a section dropped below nested content after the enclosing section', () => {
    for (const drop of ['TG2', 'G2', 'G', 'S2']) {
      expect(resolve(tree, 'S1', drop, 1)).toEqual({ parentId: 'B', index: 2 })
    }
    expect(resolve(tree, 'T1', 'G2', 1)).toEqual({ parentId: 'G2', index: 1 })
  })

  it('puts a block lifted above the body at the end of the body', () => {
    expect(resolve(tree, 'SW', 'B', 1)).toEqual({ parentId: 'B', index: 6 })
  })

  it('fills an empty container dropped on', () => {
    expect(resolve(withEmpties, 'T1', 'EC', 1)).toEqual({ parentId: 'EC', index: 0 })
    expect(resolve(withEmpties, 'C1', 'ES', 1)).toEqual({ parentId: 'ES', index: 0 })
    expect(resolve(withEmpties, 'SE1', 'ESO', 1)).toEqual({ parentId: 'ESO', index: 0 })
    expect(resolve(withEmpties, 'S1', 'EW', 1)).toEqual({ parentId: 'EW', index: 0 })
    expect(resolve(withEmpties, 'C1', 'EG', 1)).toEqual({ parentId: 'EG', index: 0 })
  })

  it('drops into a collapsed container when its parent refuses the block', () => {
    expect(resolve(tree, 'T1', 'C3', 1, ['C3'])).toEqual({ parentId: 'C3', index: 4 })
    // Raw HTML is accepted by the section: rc-tree's level comes first
    expect(resolve(tree, 'R', 'C3', 1, ['C3'])).toEqual({ parentId: 'S3', index: 1 })
    expect(resolve(tree, 'T1', 'C3', 0, ['C3'])).toEqual({ parentId: 'C3', index: 0 })
  })

  it('goes down the open last children when the level above refuses the block', () => {
    const small = n('ROOT', 'mjml', [
      n('HEAD', 'mj-head', []),
      n('B', 'mj-body', [
        n('S1', 'mj-section', [n('C1', 'mj-column', [text('T1')])]),
        n('S3', 'mj-section', [n('C3', 'mj-column', [n('D', 'mj-divider'), text('T4')])]),
        n('R', 'mj-raw')
      ])
    ]) as unknown as EmailBlock
    expect(resolve(small, 'T1', 'S3', 1)).toEqual({ parentId: 'C3', index: 2 })
    // A closed section hides its column: no gap to show
    expect(resolve(small, 'T1', 'S3', 1, ['S3'])).toBeNull()
  })

  it('never puts a block inside itself', () => {
    // Nesting rules alone refuse these
    expect(resolve(tree, 'C1', 'T1', 1)).toBeNull()
    expect(resolve(tree, 'W', 'SW', 1)).toBeNull()
    // A malformed tree, a wrapper inside a column, where only the descendant check refuses the section
    const malformed = n('ROOT', 'mjml', [
      n('HEAD', 'mj-head', []),
      n('B', 'mj-body', [n('S1', 'mj-section', [n('C1', 'mj-column', [n('WX', 'mj-wrapper', [])])]), n('R', 'mj-raw')])
    ]) as unknown as EmailBlock
    expect(resolve(malformed, 'S1', 'WX', 1)).toBeNull()
  })

  it('returns null for unknown blocks', () => {
    expect(resolve(tree, 'nope', 'T1', 1)).toBeNull()
    expect(resolve(tree, 'T1', 'nope', 1)).toBeNull()
  })
})
