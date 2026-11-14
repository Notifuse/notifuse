import { describe, it, expect } from 'vitest'
import type { EmailBlock } from '../types'
import { parentIdOf, rebalanceAdded, rebalanceRemoved } from './layoutRebalance'
import { EmailBlockClass } from '../EmailBlockClass'

const col = (id: string, width?: string) => ({
  id,
  type: 'mj-column',
  attributes: width === undefined ? {} : { width },
  children: []
})

// A 400px body whose template gives groups 25% by default
const tree = (sectionChildren: unknown[]) =>
  ({
    id: 'root',
    type: 'mjml',
    children: [
      {
        id: 'head',
        type: 'mj-head',
        children: [
          {
            id: 'attrs',
            type: 'mj-attributes',
            children: [{ id: 'dg', type: 'mj-group', attributes: { width: '25%' }, children: [] }]
          }
        ]
      },
      {
        id: 'body',
        type: 'mj-body',
        attributes: { width: '400px' },
        children: [{ id: 'sec', type: 'mj-section', attributes: {}, children: sectionChildren }]
      }
    ]
  }) as unknown as EmailBlock

const widths = (root: EmailBlock) =>
  (EmailBlockClass.findBlockById(root, 'sec')?.children ?? []).map(
    (child) => (child.attributes as Record<string, unknown>)?.width
  )

describe('rebalanceAdded', () => {
  it("measures a pixel group against the email's body width", () => {
    // 100px of the 400px body: the columns share 75%
    const root = tree([{ id: 'g', type: 'mj-group', attributes: { width: '100px' }, children: [] }, col('a', '100%'), col('new')])
    rebalanceAdded(root, 'sec', 'new')
    expect(widths(root)).toEqual(['100px', '37.5%', '37.5%'])
  })

  it("counts a width-less group at the template's default width", () => {
    const root = tree([{ id: 'g', type: 'mj-group', attributes: {}, children: [] }, col('a', '100%'), col('new')])
    rebalanceAdded(root, 'sec', 'new')
    expect(widths(root)).toEqual([undefined, '37.5%', '37.5%'])
  })

  it('does nothing for a block that is neither a column nor a group', () => {
    const root = tree([col('a', '30%'), { id: 'raw', type: 'mj-raw', attributes: {} }])
    rebalanceAdded(root, 'sec', 'raw')
    expect(widths(root)).toEqual(['30%', undefined])
  })
})

describe('rebalanceRemoved', () => {
  it('scales the columns left after a column went', () => {
    const root = tree([col('a', '20%'), col('b', '30%')])
    rebalanceRemoved(root, 'sec', 'mj-column')
    expect(widths(root)).toEqual(['40%', '60%'])
  })

  it('leaves the columns alone when the groups left still leave them no room', () => {
    const root = tree([
      { id: 'g', type: 'mj-group', attributes: { width: '400px' }, children: [] },
      col('a', '20%'),
      col('b', '30%')
    ])
    rebalanceRemoved(root, 'sec', 'mj-group')
    expect(widths(root)).toEqual(['400px', '20%', '30%'])
  })

  it('does nothing for another type', () => {
    const root = tree([col('a', '20%'), col('b', '30%')])
    rebalanceRemoved(root, 'sec', 'mj-text')
    expect(widths(root)).toEqual(['20%', '30%'])
  })
})

describe('parentIdOf', () => {
  it('finds the block that holds another, or null', () => {
    const root = tree([col('a')])
    expect(parentIdOf(root, 'a')).toBe('sec')
    expect(parentIdOf(root, 'sec')).toBe('body')
    expect(parentIdOf(root, 'root')).toBeNull()
    expect(parentIdOf(root, 'missing')).toBeNull()
  })
})
