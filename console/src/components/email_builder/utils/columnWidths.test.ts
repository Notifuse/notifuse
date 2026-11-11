import { describe, it, expect } from 'vitest'
import type { EmailBlock } from '../types'
import { rebalanceAfterAdd, rebalanceAfterRemove } from './columnWidths'

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
    // A group created before groups were counted, next to a column at 100%
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

  it('counts a group with a pixel width as its equal share', () => {
    const section = container('mj-section', [group('g', '300px'), col('new')])
    rebalanceAfterAdd(section, 'new')
    expect(widths(section)).toEqual(['300px', '50%'])
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

  it("does not touch a group's own columns when the section changes", () => {
    const section = container('mj-section', [group('g'), col('a', '100%')])
    rebalanceAfterRemove(section)
    expect(widths(section.children![0] as EmailBlock)).toEqual(['50%', '50%'])
    expect(widths(section)).toEqual([undefined, '50%'])
  })
})
