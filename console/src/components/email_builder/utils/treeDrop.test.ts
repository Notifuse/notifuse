import { describe, it, expect } from 'vitest'
import type { EmailBlock, MJMLComponentType } from '../types'
import { depthOf, placesInGap, resolveDrop, type DragSource } from './treeDrop'

interface Node {
  id: string
  type: string
  children?: Node[]
}

const n = (id: string, type: string, children?: Node[]): Node => ({ id, type, children })
const text = (id: string) => n(id, 'mj-text')
const doc = (body: Node[]) =>
  n('ROOT', 'mjml', [n('HEAD', 'mj-head', []), n('B', 'mj-body', body)]) as unknown as EmailBlock

// The audit's tree: body > S1[C1[T1,BT1,IM1], C2[T2,SO[SE1,SE2]]], W[SW[CW[TW]]], S2[G[G1[TG1],G2[TG2]]],
// S3[C3[...c3]], R (raw), L (liquid)
const auditBody = (
  c3: Node[] = [n('D', 'mj-divider'), n('SP', 'mj-spacer'), text('T3'), text('T4')]
) => [
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
  n('S3', 'mj-section', [n('C3', 'mj-column', c3)]),
  n('R', 'mj-raw'),
  n('L', 'mj-liquid')
]

const tree = doc(auditBody())

// Empty containers in front of the audit body: ES 0, S5 1, EW 2, S6 3, then S1 at 4
const withEmpties = doc([
  n('ES', 'mj-section', []),
  n('S5', 'mj-section', [
    n('EC', 'mj-column', []),
    n('C5', 'mj-column', [text('T5'), n('ESO', 'mj-social', [])])
  ]),
  n('EW', 'mj-wrapper', []),
  n('S6', 'mj-section', [n('EG', 'mj-group', []), n('C6', 'mj-column', [text('T6')])]),
  ...auditBody()
])

const gap = (upperId: string, lowerId: string | null) => ({ upperId, lowerId })
const place = (parentId: string, slot: number, depth: number, emptyInside = false) => ({
  parentId,
  slot,
  depth,
  emptyInside
})
const target = (parentId: string, index: number, depth: number) => ({ parentId, index, depth })

const find = (t: EmailBlock, id: string): EmailBlock | undefined =>
  t.id === id ? t : (t.children ?? []).map((c) => find(c, id)).find(Boolean)

// The drag source of a block of the tree, as the tree panel builds it
const drag = (t: EmailBlock, id: string): DragSource => ({
  id,
  type: find(t, id)!.type,
  depth: depthOf(t, id)!
})
const resolve = (t: EmailBlock, id: string, upper: string, lower: string | null, shift = 0) =>
  resolveDrop(t, drag(t, id), gap(upper, lower), shift)

describe('depthOf', () => {
  it('counts levels from the body', () => {
    expect(['B', 'S1', 'C1', 'T1', 'W', 'TW'].map((id) => depthOf(tree, id))).toEqual([
      0, 1, 2, 3, 1, 4
    ])
  })

  it('returns null for an unknown block or one outside the body', () => {
    expect(depthOf(tree, 'nope')).toBeNull()
    expect(depthOf(tree, 'HEAD')).toBeNull()
  })
})

describe('placesInGap', () => {
  it('gives the slot between two siblings', () => {
    expect(placesInGap(tree, gap('T3', 'T4'))).toEqual([place('C3', 3, 3)])
  })

  it('gives the slot after each level whose content ends at the upper row, down to the body', () => {
    expect(placesInGap(tree, gap('T4', 'R'))).toEqual([
      place('C3', 4, 3),
      place('S3', 1, 2),
      place('B', 4, 1)
    ])
  })

  it('gives the first slot of an open container', () => {
    expect(placesInGap(tree, gap('S3', 'C3'))).toEqual([place('S3', 0, 2)])
    expect(placesInGap(tree, gap('B', 'S1'))).toEqual([place('B', 0, 1)])
  })

  it('gives the end of a collapsed container, then the slots after it', () => {
    expect(placesInGap(tree, gap('C3', 'R'))).toEqual([
      place('C3', 4, 3),
      place('S3', 1, 2),
      place('B', 4, 1)
    ])
  })

  it('marks the inside of an empty container', () => {
    expect(placesInGap(withEmpties, gap('EW', 'S6'))).toEqual([
      place('EW', 0, 2, true),
      place('B', 3, 1)
    ])
  })

  it('stops at the body after the last row', () => {
    expect(placesInGap(tree, gap('L', null))).toEqual([place('B', 6, 1)])
  })

  it('counts a hidden first child in the slot', () => {
    const hidden = doc(
      auditBody([n('RH', 'mj-raw'), n('D', 'mj-divider'), n('SP', 'mj-spacer'), text('T4')])
    )
    expect(placesInGap(hidden, gap('C3', 'D'))).toEqual([place('C3', 1, 3)])
    expect(placesInGap(hidden, gap('D', 'SP'))).toEqual([place('C3', 2, 3)])
  })

  it('gives no place for unknown rows', () => {
    expect(placesInGap(tree, gap('nope', 'T4'))).toEqual([])
    expect(placesInGap(tree, gap('T3', 'nope'))).toEqual([])
  })
})

describe('resolveDrop', () => {
  it('moves a block down within its parent to the slot the line shows', () => {
    expect(resolve(tree, 'D', 'T3', 'T4')).toEqual(target('C3', 2, 3))
  })

  it('keeps the dragged block’s level when the pointer goes straight down', () => {
    // After the wrapper, not at its end
    expect(resolve(tree, 'S1', 'TW', 'S2')).toEqual(target('B', 1, 1))
    expect(resolve(tree, 'S3', 'TW', 'S2')).toEqual(target('B', 2, 1))
    // Raw HTML and Liquid between sections stay in the body
    expect(resolve(tree, 'L', 'T4', 'R')).toEqual(target('B', 4, 1))
  })

  it('goes one level in or out for each indent the pointer moves sideways', () => {
    expect(resolve(tree, 'S1', 'TW', 'S2', 1)).toEqual(target('W', 1, 2))
    expect(resolve(tree, 'S3', 'TW', 'S2', 1)).toEqual(target('W', 1, 2))
    expect(resolve(tree, 'R', 'T4', 'R', 2)).toEqual(target('C3', 4, 3))
    // A column leaves its group over its own last text
    expect(resolve(tree, 'G2', 'TG2', 'S3', -1)).toEqual(target('S2', 1, 2))
  })

  it('fills an empty container unless the pointer moved left', () => {
    expect(resolve(withEmpties, 'S1', 'EW', 'S6')).toEqual(target('EW', 0, 2))
    expect(resolve(withEmpties, 'S1', 'EW', 'S6', 1)).toEqual(target('EW', 0, 2))
    expect(resolve(withEmpties, 'S1', 'EW', 'S6', -1)).toEqual(target('B', 3, 1))
    expect(resolve(withEmpties, 'C1', 'EG', 'C6')).toEqual(target('EG', 0, 3))
    expect(resolve(withEmpties, 'C1', 'EG', 'C6', -1)).toEqual(target('S6', 1, 2))
    expect(resolve(withEmpties, 'R', 'EC', 'C5')).toEqual(target('EC', 0, 3))
  })

  it('takes the only place that accepts the block, wherever the pointer is', () => {
    expect(resolve(withEmpties, 'T1', 'EC', 'C5')).toEqual(target('EC', 0, 3))
    expect(resolve(withEmpties, 'T1', 'EC', 'C5', -2)).toEqual(target('EC', 0, 3))
  })

  it('takes the deeper place on a tie', () => {
    // Pointer depth 3: the group refuses raw HTML, so the places at depths 4 and 2 are equally near
    expect(resolve(tree, 'R', 'TG2', 'S3', 2)).toEqual(target('G2', 1, 4))
  })

  it('refuses places whose parent does not accept the block', () => {
    expect(resolve(tree, 'T2', 'SE1', 'SE2')).toBeNull()
    expect(resolve(tree, 'T1', 'L', null)).toBeNull()
    expect(resolve(tree, 'S1', 'T3', 'T4')).toBeNull()
  })

  it('returns null when the block would stay where it is', () => {
    expect(resolve(tree, 'SP', 'D', 'SP')).toBeNull()
    expect(resolve(tree, 'S3', 'T4', 'R')).toBeNull()
    expect(resolve(tree, 'S2', 'TW', 'S2')).toBeNull()
    expect(resolve(tree, 'R', 'T4', 'R')).toBeNull()
    // The nearest level is the column's own place in its group
    expect(resolve(tree, 'G2', 'TG2', 'S3')).toBeNull()
  })

  it('never puts a block inside itself', () => {
    expect(resolve(tree, 'W', 'TW', 'S2', 1)).toBeNull()
    // A malformed tree, a wrapper inside a column, where only the descendant check refuses the section
    const malformed = doc([
      n('S1', 'mj-section', [n('C1', 'mj-column', [n('WX', 'mj-wrapper', [])])]),
      n('R', 'mj-raw')
    ])
    for (const shift of [0, 2, 3]) expect(resolve(malformed, 'S1', 'WX', 'R', shift)).toBeNull()
  })

  it('places a block that is not in the tree yet from its type and depth', () => {
    const fromLibrary: DragSource = { type: 'mj-raw' as MJMLComponentType, depth: 1 }
    expect(resolveDrop(tree, fromLibrary, gap('T4', 'R'), 0)).toEqual(target('B', 4, 1))
  })

  it('returns null for an unknown block', () => {
    const unknown: DragSource = { id: 'nope', type: 'mj-text' as MJMLComponentType, depth: 3 }
    expect(resolveDrop(tree, unknown, gap('T3', 'T4'), 0)).toBeNull()
    expect(resolve(tree, 'T1', 'nope', 'T4')).toBeNull()
  })
})
