import { describe, it, expect } from 'vitest'
import type { EmailBlock } from '../types'
import { bodyWidthOf, rebalanceAfterAdd, rebalanceAfterRemove, sectionOverflow } from './columnWidths'

interface Node {
  id: string
  type: string
  attributes?: Record<string, unknown>
  children?: Node[]
}

const col = (id: string, width?: string): Node => ({
  id,
  type: 'mj-column',
  attributes: width === undefined ? {} : { width },
  children: []
})
const group = (id: string, width?: string): Node => ({
  id,
  type: 'mj-group',
  attributes: width === undefined ? {} : { width },
  children: [col(`${id}-c1`, '50%'), col(`${id}-c2`, '50%')]
})
const container = (type: string, children: Node[]) =>
  ({ id: 'X', type, attributes: {}, children }) as unknown as EmailBlock
const widths = (block: EmailBlock) =>
  (block.children ?? []).map((child) => (child.attributes as Record<string, unknown>)?.width)

describe('rebalanceAfterAdd', () => {
  it('gives the new column its share and keeps the ratio of the others', () => {
    const section = container('mj-section', [col('a', '30%'), col('b', '70%'), col('new', '100%')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['20%', '46.67%', '33.33%'])
  })

  it('lets the last column in order take the rounding remainder', () => {
    const section = container('mj-section', [col('a', '50%'), col('b', '50%'), col('new')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['33.33%', '33.33%', '33.34%'])
  })

  it('equalises when the other widths are not percentages adding up to 100', () => {
    for (const others of [
      ['200px', '400px'],
      ['auto', '50%'],
      [undefined, '50%'],
      ['40%', '40%']
    ]) {
      const section = container('mj-section', [
        col('a', others[0]),
        col('b', others[1]),
        col('new')
      ])
      rebalanceAfterAdd(section, 'new')
      expect(widths(section)).toEqual(['33.33%', '33.33%', '33.34%'])
    }
  })

  it('leaves raw HTML and Liquid alone and does not count them', () => {
    const section = container('mj-section', [
      col('a', '100%'),
      { id: 'r', type: 'mj-raw', attributes: {} },
      { id: 'l', type: 'mj-liquid', attributes: {} },
      col('new')
    ])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['50%', undefined, undefined, '50%'])
  })

  it('gives a new column its part of what the groups leave, the others keeping their ratio', () => {
    // The width-less group takes a third of the section once the column is added
    const section = container('mj-section', [group('g'), col('a', '50%'), col('new')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual([undefined, '33.33%', '33.34%'])
  })

  it('equalises the columns in what the groups leave when they did not fill it', () => {
    // A group without a width (cleared, or imported) next to a column at 100%
    const section = container('mj-section', [group('g'), col('a', '100%'), col('new')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual([undefined, '33.33%', '33.34%'])
  })

  it('makes room for a new group without a width, keeping the ratio of the columns', () => {
    const section = container('mj-section', [col('a', '30%'), col('b', '70%'), group('new')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['20%', '46.67%', undefined])
  })

  it('makes room for a new group with a percentage width', () => {
    const section = container('mj-section', [group('new', '60%'), col('a', '50%'), col('b', '50%')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['60%', '20%', '20%'])
  })

  it("counts a group with a pixel width as its part of the body's width", () => {
    // 150px of the default 600px body
    const section = container('mj-section', [group('g', '150px'), col('new')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['150px', '75%'])

    // 150px of a 300px body
    const narrow = container('mj-section', [group('g', '150px'), col('new')])
    rebalanceAfterAdd(narrow, 'new', {}, { bodyWidth: 300 })
    expect(widths(narrow)).toEqual(['150px', '50%'])
  })

  it("measures a group's pixel width against the section less its side padding", () => {
    // 600px less 50px each side leaves 500px, of which the group takes 200px
    const section = {
      ...container('mj-section', [group('g', '200px'), col('new')]),
      attributes: { paddingLeft: '50px', paddingRight: '50px' }
    } as EmailBlock
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['200px', '60%'])

    // The padding shorthand of an imported section, and the template's default attributes
    const imported = {
      ...container('mj-section', [group('g', '200px'), col('new')]),
      attributes: { padding: '20px 50px' }
    } as EmailBlock
    rebalanceAfterAdd(imported, 'new')
    expect(widths(imported)).toEqual(['200px', '60%'])

    const byDefault = container('mj-section', [group('g', '200px'), col('new')])
    rebalanceAfterAdd(byDefault, 'new', { 'mj-section': { padding: '0 25px 0 75px' } })
    expect(widths(byDefault)).toEqual(['200px', '60%'])
  })

  it("counts a group at the width the template's default attributes give it", () => {
    const section = container('mj-section', [group('g'), col('new')])
    rebalanceAfterAdd(section, 'new', { 'mj-group': { width: '40%' } })
    expect(widths(section)).toEqual([undefined, '60%'])

    // The group's own width wins
    const own = container('mj-section', [group('g', '30%'), col('a', '60%'), col('b', '40%')])
    rebalanceAfterRemove(own, { 'mj-group': { width: '40%' } })
    expect(widths(own)).toEqual(['30%', '42%', '28%'])
  })

  it('leaves the columns alone when a new group leaves them no room', () => {
    const section = container('mj-section', [group('g', '50%'), group('new', '50%'), col('a', '50%')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['50%', '50%', '50%'])
  })

  it('does not count groups that leave the columns no room', () => {
    const section = container('mj-section', [group('g', '100%'), col('new', '30%')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['100%', '100%'])
  })

  it('rebalances the columns of a group the same way', () => {
    const group = container('mj-group', [col('a', '30%'), col('b', '70%'), col('new')])
    rebalanceAfterAdd(group, 'new')
    expect(widths(group)).toEqual(['20%', '46.67%', '33.33%'])
  })

  it('gives a lone column 100%', () => {
    const section = container('mj-section', [col('new', '30%')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['100%'])
  })

  it('does nothing outside a section or a group', () => {
    const column = container('mj-column', [col('new', '30%')])
    rebalanceAfterAdd(column, 'new')
    expect(widths(column)).toEqual(['30%'])
  })
})

describe('rebalanceAfterRemove', () => {
  it('scales the remaining columns to 100% keeping their ratio', () => {
    const section = container('mj-section', [col('a', '20%'), col('b', '30%')])
    rebalanceAfterRemove(section)
    expect(widths(section)).toEqual(['40%', '60%'])
  })

  it('equalises three columns with pixel widths, the last taking the remainder', () => {
    const section = container('mj-section', [
      col('a', '100px'),
      col('b', '200px'),
      col('c', '300px')
    ])
    rebalanceAfterRemove(section)
    expect(widths(section)).toEqual(['33.33%', '33.33%', '33.34%'])
  })

  it('equalises zero widths instead of writing NaN', () => {
    const section = container('mj-section', [col('a', '0%'), col('b', '0%')])
    rebalanceAfterRemove(section)
    expect(widths(section)).toEqual(['50%', '50%'])
  })

  it('gives the last remaining column 100% and leaves an empty section unchanged', () => {
    const section = container('mj-section', [col('a', '30%')])
    rebalanceAfterRemove(section)
    expect(widths(section)).toEqual(['100%'])

    const empty = container('mj-section', [])
    rebalanceAfterRemove(empty)
    expect(empty.children).toEqual([])
  })

  it('scales the columns to what the groups leave, keeping their ratio', () => {
    const section = container('mj-section', [group('g'), col('a', '20%'), col('b', '30%')])
    rebalanceAfterRemove(section)
    expect(widths(section)).toEqual([undefined, '26.67%', '40%'])
  })

  it('gives the columns the whole section back when the last group goes', () => {
    const section = container('mj-section', [col('a', '25%'), col('b', '25%')])
    rebalanceAfterRemove(section)
    expect(widths(section)).toEqual(['50%', '50%'])
  })

  it('leaves the columns alone when the group that went still leaves them no room', () => {
    // A third group at 50% went: the two left still fill the section
    const section = container('mj-section', [group('g', '50%'), group('g2', '50%'), col('c', '50%')])
    rebalanceAfterRemove(section, {}, { removed: 'mj-group' })
    expect(widths(section)).toEqual(['50%', '50%', '50%'])
  })

  it('gives the columns the section when a column went and the groups leave them no room', () => {
    const section = container('mj-section', [group('g', '100%'), col('a', '20%'), col('b', '30%')])
    rebalanceAfterRemove(section, {}, { removed: 'mj-column' })
    expect(widths(section)).toEqual(['100%', '40%', '60%'])
  })

  it("does not touch a group's own columns when the section changes", () => {
    const section = container('mj-section', [group('g'), col('a', '100%')])
    rebalanceAfterRemove(section)
    expect(widths(section.children![0] as EmailBlock)).toEqual(['50%', '50%'])
    expect(widths(section)).toEqual([undefined, '50%'])
  })
})

describe('sectionOverflow', () => {
  it('is 0 when the columns and groups fit', () => {
    expect(sectionOverflow(container('mj-section', [group('g', '40%'), col('a', '60%')]))).toBe(0)
    expect(sectionOverflow(container('mj-section', [group('g'), col('a')]))).toBe(0)
  })

  it('tells how far they go past the section', () => {
    expect(sectionOverflow(container('mj-section', [group('g', '70%'), col('a', '60%')]))).toBe(30)
    // A width-less column takes its share; a group in pixels its part of the body
    expect(sectionOverflow(container('mj-section', [group('g', '450px'), col('a')]))).toBe(25)
    expect(sectionOverflow(container('mj-section', [group('g', '450px'), col('a')]), {}, { bodyWidth: 900 })).toBe(0)
  })

  it("uses the template's default group width", () => {
    expect(sectionOverflow(container('mj-section', [group('g'), col('a', '50%')]), { 'mj-group': { width: '75%' } })).toBe(
      25
    )
  })

  it('is 0 outside a section', () => {
    expect(sectionOverflow(container('mj-group', [col('a', '80%'), col('b', '80%')]))).toBe(0)
  })
})

describe('bodyWidthOf', () => {
  const tree = (width?: string) =>
    ({
      id: 'root',
      type: 'mjml',
      children: [{ id: 'body', type: 'mj-body', attributes: width === undefined ? {} : { width }, children: [] }]
    }) as unknown as EmailBlock

  it("reads the body's pixel width, else MJML's 600px", () => {
    expect(bodyWidthOf(tree('480px'))).toBe(480)
    expect(bodyWidthOf(tree())).toBe(600)
    expect(bodyWidthOf(tree('80%'))).toBe(600)
  })
})
